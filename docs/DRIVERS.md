# Adding a database driver

This guide walks through adding a new database engine to siphon (e.g. MySQL in
Phase E). It reflects the real interfaces and conventions in the codebase — the
signatures below compile-match `internal/driver/driver.go`, so you can copy from
them.

## Table of contents

- [Overview](#overview)
- [The contract](#the-contract)
- [Capabilities](#capabilities)
- [Registration](#registration)
- [Integration tests](#integration-tests)
- [Error mapping](#error-mapping)

## Overview

Drivers live in `internal/driver/<name>/` — one package per engine (the Postgres
driver is `internal/driver/postgres/`). A driver is a compile-time Go package
that shells out to the engine's native tools for data movement (e.g.
`pg_dump`/`pg_restore`) and may use a client library for fast schema reads.

siphon is strictly layered; imports flow **downward only**, enforced at lint
time by `golangci-lint`'s `depguard`:

```text
  internal/cli   internal/tui     presentation (Cobra · Bubble Tea)
        └──────┬───────┘
        internal/app              application verbs (backup, restore, sync, …)
               │
   internal/driver/<engine>       database adapters — your code lives here
               │
  config · secrets · profile · dumps · jobs · errs   domain + support packages
```

**Drivers are stateless.** The `Driver` value carries no connection state;
state lives in the `Conn` returned by `Connect`, scoped per-verb. `Conn` is also
the unit of cancellation — the `ctx` passed to each verb must propagate to any
spawned subprocess so a cancel tears it down cleanly.

## The contract

Implement two interfaces from `internal/driver/driver.go`:

```go
// Driver is the protocol-level abstraction for a database engine.
type Driver interface {
	Name() string
	Capabilities() Capabilities
	Connect(ctx context.Context, p Profile) (Conn, error)
}

// Conn is an open connection plus the verbs that operate on it.
type Conn interface {
	Inspect(ctx context.Context) (*Schema, error)
	Backup(ctx context.Context, opt BackupOpts, w io.Writer) error
	Restore(ctx context.Context, opt RestoreOpts, r io.Reader) error
	Verify(ctx context.Context, r io.Reader) (*VerifyReport, error)
	Close() error
}
```

What each method does:

- **`Name()`** — the lowercase engine name (`"postgres"`), matching a profile's
  `Driver` field. Used by the registry and in user-facing messages.
- **`Capabilities()`** — declares what the engine supports (see below).
- **`Connect(ctx, p)`** — opens a connection from a `Profile` and returns a
  `Conn`. Probe the connection here (the Postgres driver pings with a bounded
  retry) and map failures to `errs.ErrConnectionFailed`.
- **`Inspect(ctx)`** — returns a `*Schema` (tables with row estimates and sizes).
- **`Backup(ctx, opt, w)`** — writes a dump to `w` (an `io.Writer`).
- **`Restore(ctx, opt, r)`** — reads a dump from `r` (an `io.Reader`).
- **`Verify(ctx, r)`** — reads a dump from `r` and returns a `*VerifyReport`. At
  minimum, compute a checksum (the Postgres driver hashes the stream with SHA-256
  → `Checksum: "sha256:…"`).
- **`Close()`** — releases the connection.

`Connect` receives a `Profile`. Secrets are already resolved before this struct
reaches `Connect` — never wire a `SecretRef` here:

```go
type Profile struct {
	Name     string
	Driver   string
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}
```

The options and result structs (`BackupOpts`, `RestoreOpts`, `Schema`,
`VerifyReport`) are also defined in `internal/driver/driver.go` — read that file
for their fields.

## Capabilities

`Capabilities` is a struct of 13 boolean flags. Each gates a UI affordance or
feature path:

```go
type Capabilities struct {
	Incremental             bool
	NativeStream            bool
	PerTable                bool
	SchemaOnly              bool
	DataOnly                bool
	Parallel                bool
	Compression             bool
	BinaryFormat            bool
	CrossEngineSource       bool
	CrossEngineTarget       bool
	CDC                     bool
	NativeBackpressure      bool
	CrossVersionIncremental bool
}
```

**Declare these honestly.** A verb pre-flight can gate an affordance through
`app.RequireCapability(deps, profileName, cap)` (in `internal/app/capgate.go`):
if the resolved driver doesn't support `cap`, it returns an
`errs.ErrDriverUnsupported` error with an actionable hint up front, instead of
crashing partway through. Over-declaring a capability you don't actually support
turns that clean rejection into a runtime failure.

> **Status:** the gate helper and the `Capabilities` flags exist today, but the
> verbs are only gated where a feature is actually wired. Native streaming
> (`NativeStream`) and parallel backup (`Parallel`) are deferred to Phase F, so
> `sync --stream` / `backup --jobs` are not yet gated — the gate will be wired in
> alongside those features. Declare your flags honestly now so the gating is
> correct the moment a verb starts honoring them.

When you add support for a capability, flip its flag — that single change lights
up the corresponding UI path.

## Registration

A driver registers itself with the process-wide registry from its package
`init()`, mirroring the `database/sql.Register` convention:

```go
func init() { driver.Register(&Driver{}) }
```

`driver.Register(d)` panics if called twice for the same `Name()` — a duplicate
almost always signals a copy-paste bug worth surfacing loudly at startup.
`driver.Get(name)` resolves a profile's `Driver` string to the registered driver
(returning `errs.ErrDriverUnsupported` if none matches). Both live in
`internal/driver/registry.go`.

`init()` only runs if the package is imported. The side-effect import that pulls
your driver in lives in **`internal/app/drivers.go`**:

```go
import _ "github.com/nixrajput/siphon/internal/driver/postgres" // register the postgres driver
```

Add a matching blank import line there for your driver.

> **Why `internal/app/drivers.go` and not `internal/cli/root.go`?** The depguard
> rule `cli-may-not-import-domain` forbids `internal/cli` from importing
> `internal/driver/**` — even a blank `_` import would fail lint. Presentation
> layers (CLI, TUI) reach drivers only through the application layer's
> `DefaultDrivers()`, which is backed by the registry. So the import that wires a
> driver into the build belongs in the app layer.

## Integration tests

Drivers get four contract tests for free from the shared harness at
`internal/driver/_testing/` (package `drivertesting`). Ship an
`integration_test.go` with a `//go:build integration` tag that calls
`drivertesting.RunDriverSuite`:

```go
//go:build integration

package mysql

func TestSuite_MySQL(t *testing.T) {
	prof, cleanup, opener := startMySQL(t) // your own setup, e.g. testcontainers
	drivertesting.RunDriverSuite(t, func() driver.Driver { return Driver{} },
		drivertesting.Fixtures{
			Profile:   prof,
			Cleanup:   cleanup,
			SQLOpener: opener,
			Seed:          func(ctx context.Context, db *sql.DB) error { /* … */ },
			VerifyRestore: func(ctx context.Context, db *sql.DB) error { /* … */ },
		})
}
```

`RunDriverSuite(t, ctor, fx)` runs four subtests against a real database:

- **`Connect_And_Inspect`** — `Connect` succeeds and `Inspect` returns a schema.
- **`BackupRestore_Roundtrip`** — `Seed` → `Backup` → `Restore{Clean:true}` →
  `VerifyRestore`.
- **`Cancel_PropagatesToSubprocess`** — cancelling the `ctx` mid-backup returns a
  non-nil error promptly (no subprocess leak).
- **`BadCredentials_ReturnsErrConnectionFailed`** — `Connect` with a wrong
  password returns an error matching `errors.Is(err, errs.ErrConnectionFailed)`.

The `Fixtures` struct (in `internal/driver/_testing/fixtures.go`) you populate:

- **`Profile`** — points at a freshly-started, empty test database.
- **`Seed(ctx, db)`** — runs SQL to populate a known fixture; called before Backup.
- **`VerifyRestore(ctx, db)`** — asserts the database state matches what `Seed`
  produced; called after the Backup/Restore round-trip.
- **`Cleanup()`** — tears down the test database; called via `t.Cleanup`.
- **`SQLOpener()`** — returns a `*sql.DB` on the same database, used by `Seed`
  and `VerifyRestore` (they can't go through `driver.Conn`, which doesn't expose
  raw SQL).

The worked example is `internal/driver/postgres/integration_test.go` — copy its
shape. The suite runs behind the `integration` build tag (`make test-integration`).

## Error mapping

Drivers (and verbs) return the structured error type from `internal/errs/errs.go`:

```go
type Error struct {
	Op    string // the verb name, e.g. "backup", "restore"
	Code  Code   // exit-code taxonomy bucket
	Cause error  // underlying cause; matched by errors.Is / errors.As
	Hint  string // user-actionable remediation, rendered in CLI/TUI display
}
```

`Code` is one of the exit-code buckets: `CodeOK` (0), `CodeUser` (1),
`CodeSystem` (2), `CodeIntegrity` (3), `CodePartial` (4), `CodeCancelled` (130),
`CodeTerminated` (143).

Two requirements the harness and UI depend on:

- **Bad credentials must surface as `errs.ErrConnectionFailed`.** The
  `BadCredentials_ReturnsErrConnectionFailed` subtest asserts
  `errors.Is(err, errs.ErrConnectionFailed)`. The Postgres driver does this with
  a small `wrapConnErr` helper:

  ```go
  func wrapConnErr(err error) error {
  	return &errs.Error{
  		Op:    "postgres.connect",
  		Code:  errs.CodeSystem,
  		Cause: errs.ErrConnectionFailed,
  		Hint:  err.Error(),
  	}
  }
  ```

  Because `*errs.Error` unwraps to `Cause`, `errors.Is(err, errs.ErrConnectionFailed)`
  matches.

- **`Verify` computes a checksum.** At minimum, hash the dump stream (the
  Postgres driver uses SHA-256 → `VerifyReport.Checksum = "sha256:…"`).

Set `Code` to the bucket that matches the failure (`CodeUser` for bad input,
`CodeSystem` for infrastructure, `CodeIntegrity` for checksum/corruption, and so
on) so the CLI exits with the right POSIX code.
