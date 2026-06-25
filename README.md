<div align="center">

# siphon

**Sync any database, anywhere.**

A single binary that turns the painful, error-prone sprawl of `pg_dump` → `pg_restore` shell scripts into a guided, observable workflow — with named profiles, integrity-checked dumps, and server-to-server streaming, across multiple database engines.

[![CI](https://github.com/nixrajput/siphon/actions/workflows/ci.yml/badge.svg)](https://github.com/nixrajput/siphon/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/nixrajput/siphon.svg)](https://pkg.go.dev/github.com/nixrajput/siphon)
[![Go Version](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Status](<https://img.shields.io/badge/status-pre--1.0%20(active)-orange>)](#project-status)

</div>

---

> [!WARNING]
> **Pre-1.0 — active development.** Postgres, MySQL, and MariaDB backup/restore/sync/verify/inspect work end-to-end today (Phases B + E), and bare `siphon` opens an interactive multi-panel dashboard (Phase C). The driver layer is hardened with a shared cross-driver test harness, capability gating, and connection retry (Phase D). Advanced transfer is complete (Phase F): incremental backup/restore, cross-engine sync, and CDC continuous replication all work end-to-end. Ops features (Phase G) are underway — the dump catalog can now live in an S3 / S3-compatible bucket (`storage:` config block); secret backends, retention, and telemetry are still on the [roadmap](#roadmap). APIs, flags, and the on-disk dump format may change before 1.0. Track progress via the milestone tags (`phase-a` … `phase-f`).

## Table of contents

- [siphon](#siphon)
  - [Table of contents](#table-of-contents)
  - [Why siphon](#why-siphon)
  - [Project status](#project-status)
  - [Requirements](#requirements)
  - [Install](#install)
  - [Quick start](#quick-start)
  - [Commands](#commands)
  - [Configuration](#configuration)
  - [Architecture](#architecture)
  - [Development](#development)
  - [Roadmap](#roadmap)
  - [Contributing](#contributing)
  - [License](#license)

## Why siphon

- **One CLI, many databases.** Postgres, MySQL, and MariaDB all work today (MySQL and MariaDB share a common `_mysqlcommon` backend). The driver interface is engine-agnostic, so SQLite, MongoDB, SQL Server, and ClickHouse can follow.
- **Native, not reimplemented.** siphon shells out to `pg_dump`/`pg_restore`, `mysqldump`/`mysql`, and `mariadb-dump`/`mariadb` for the actual data movement — you inherit 20+ years of correctness from the official tools, wrapped in a consistent UX.
- **Integrity by default.** Every dump is checksummed (SHA-256) and recorded in a sidecar metadata file. `siphon verify` re-hashes the dump and flags corruption or tampering — and fails with a distinct exit code so CI can catch it.
- **Built for scripts and humans.** A Cobra command tree with predictable flags and POSIX exit codes for automation; an interactive Bubble Tea dashboard when you invoke `siphon` bare.
- **Named profiles + secret refs.** Store connection details once; reference secrets as `env:VAR`, `keychain://<name>` (OS credential store), or `awssm://<id>#<key>` (AWS Secrets Manager) — resolved at runtime so plaintext passwords never live in your config.
- **Streaming sync.** `siphon sync src dst` pipes a backup straight into a restore with no intermediate file on disk.

## Project status

| Phase                             | What it delivers                                                                                                                                                                                                                                                                                                                                                                                                                        | Status      |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------- |
| **A** — Skeleton                  | Go module, Cobra CLI, TUI placeholder, `Driver` interface + registry, `errs`/`config`/`secrets`/`profile` packages, golangci-lint + depguard, cross-platform CI                                                                                                                                                                                                                                                                         | ✅ Complete |
| **B** — Postgres walking skeleton | `backup`, `restore`, `sync`, `verify`, `inspect`, `dumps`, `config`, `profile` working end-to-end against PostgreSQL                                                                                                                                                                                                                                                                                                                    | ✅ Complete |
| **C** — TUI dashboard             | Multi-panel Bubble Tea dashboard (profiles · dumps · jobs) with live job progress, backup/restore modal forms, and snapshot tests                                                                                                                                                                                                                                                                                                       | ✅ Complete |
| **D** — Driver hardening          | Shared cross-driver test harness (`RunDriverSuite`), capability-gating helper (`RequireCapability`), Postgres connection-probe retry, and a `docs/DRIVERS.md` contributor guide                                                                                                                                                                                                                                                         | ✅ Complete |
| **E** — MySQL + MariaDB           | Both drivers via a shared `_mysqlcommon` package (shared `Conn`, `mysqldump`/`mariadb-dump` backup, client-pipe restore), exercised by the Phase D `RunDriverSuite` harness                                                                                                                                                                                                                                                             | ✅ Complete |
| **F** — Advanced transfer         | All four advanced-transfer modes work end-to-end: bounded-buffer streaming sync; **incremental** backup/restore (`backup --incremental --base <id>` captures a bounded change set via Postgres logical decoding / MySQL-MariaDB binlog, `restore` replays the base→incremental chain, Postgres orphan-slot sweep); **cross-engine** sync (`sync --cross-engine` — typed `SchemaInspector` introspection → canonical type-mapping, e.g. Postgres → MySQL); and **CDC** (`siphon cdc` / `sync --continuous` — unbounded change streaming with snapshot→stream handoff, resumable, same- and cross-engine). Live DB paths are integration-tested in CI — see [docs/INCREMENTAL.md](docs/INCREMENTAL.md), [docs/CROSS_ENGINE.md](docs/CROSS_ENGINE.md), [docs/CDC.md](docs/CDC.md) | ✅ Complete |
| **G** — Ops features              | **Cloud storage** (✅): the dump catalog can live in an S3 / S3-compatible bucket via a pluggable `storage.Store` backend (local + S3; GCS/Azure are a fast-follow), `storage:` config block, SHA-256 integrity end-to-end — see [docs/STORAGE.md](docs/STORAGE.md). **Retention** (✅): chain-aware pruning (`siphon dumps prune`) with keep-last-N / max-age / GFS rules, per-profile `retention:` config; see [docs/RETENTION.md](docs/RETENTION.md). **Ops suite** (✅): an append-only **audit log** of destructive ops (`audit:` config), **2FA/group gating** (a profile group can require a typed confirmation and/or TOTP before destructive ops), opt-in aggregate **telemetry** (`telemetry:` config), **`siphon schedule`** (manages recurring backups in your crontab), and **`siphon tunnel`** (SSH local-forward to a DB via a bastion). **Multi-backend secrets** (✅): `keychain://` (OS credential store) and `awssm://` (AWS Secrets Manager) join `env:` as secret-ref schemes. See [docs/OPS.md](docs/OPS.md).                                                                                                                                                                                                                                                              | ✅ Complete |
| **H** — Distribution              | GoReleaser, Homebrew tap, Scoop bucket, install script, docs site                                                                                                                                                                                                                                                                                                                                                                       | ⏳ Planned  |

## Requirements

- **[Go](https://go.dev/dl/) 1.26 or newer** — to build from source (the only install method until Phase H).
- **Database client tools** — siphon shells out to the native dump/restore tools; it does not embed a client. You only need the tools for the engines you actually use:
  - **PostgreSQL** profiles need `pg_dump`, `pg_restore`, `psql`.
  - **MySQL** profiles need `mysqldump`, `mysql`.
  - **MariaDB** profiles need `mariadb-dump`, `mariadb` (the renamed binaries shipped by MariaDB 10.5+; older installs that only ship `mysqldump`/`mysql` are not yet supported).

  | Platform      | PostgreSQL                                                                                                      | MySQL / MariaDB                                      |
  | ------------- | --------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
  | macOS         | `brew install postgresql@16`                                                                                    | `brew install mysql-client` / `brew install mariadb` |
  | Debian/Ubuntu | `sudo apt install postgresql-client`                                                                            | `sudo apt install mysql-client mariadb-client`       |
  | Fedora/RHEL   | `sudo dnf install postgresql`                                                                                   | `sudo dnf install mysql mariadb`                     |
  | Windows       | Install the [EDB PostgreSQL](https://www.postgresql.org/download/windows/) package and add its `bin/` to `PATH` | Install the MySQL / MariaDB client and add to `PATH` |

- **Docker** _(optional)_ — only needed to run the integration test suite (`make test-integration`).

## Install

> Pre-1.0: no Homebrew tap or prebuilt binaries yet. Build from source.

```bash
git clone https://github.com/nixrajput/siphon.git
cd siphon
make build
./bin/siphon --version
```

This produces `./bin/siphon`. Move it onto your `PATH` (e.g. `sudo install -m 0755 bin/siphon /usr/local/bin/siphon`) to call it as `siphon`.

## Quick start

```bash
# 1. Register a connection profile (secrets via env: refs, never plaintext in config)
export PROD_DB_PASS='…'
siphon profile add prod \
    --driver postgres \
    --host db.example.com \
    --user app_user \
    --password 'env:PROD_DB_PASS' \
    --database app_prod \
    --sslmode require

siphon profile list                 # show saved profiles

# 2. Inspect the schema (tables, row estimates, on-disk sizes)
siphon inspect prod

# 3. Back it up — written to ~/.local/share/siphon/dumps/ with a checksummed sidecar
siphon backup prod
siphon dumps list                   # newest first

# 4. Verify integrity (re-hashes the dump against the recorded checksum)
siphon verify <dump-id>

# 5. Restore into another profile
siphon restore --profile staging --dump <dump-id> --clean

# 6. Stream prod → staging directly, no intermediate file
siphon sync prod staging
```

Exit codes follow a POSIX-friendly taxonomy (`0` ok, `1` user error, `2` system error, `3` integrity failure, `130` cancelled) so `siphon backup prod && upload` and CI pipelines behave correctly.

## Commands

| Command                              | Description                                                  |
| ------------------------------------ | ------------------------------------------------------------ |
| `siphon backup [profile]`            | Dump a database to a checksummed file in the catalog         |
| `siphon restore [dump-id]`           | Load a dump into a database (`--clean` to drop-and-recreate) |
| `siphon sync [from] [to]`            | Backup + restore in one streamed pass                        |
| `siphon verify <dump-id>`            | Re-hash a dump and check it against its recorded checksum    |
| `siphon inspect <profile>`           | Show tables, row estimates, and sizes for a profile          |
| `siphon dumps list\|inspect\|prune`  | List, inspect, or prune saved dumps                          |
| `siphon profile add\|list\|show\|rm` | Manage named connection profiles                             |
| `siphon config path\|edit`           | Show or edit the config file                                 |
| `siphon schedule add\|list\|remove`  | Manage cron-scheduled recurring backups                      |
| `siphon tunnel <profile>`            | Open an SSH tunnel to a DB via its bastion                   |
| `siphon` _(bare)_                    | Launch the interactive multi-panel dashboard                 |

Run `siphon <command> --help` for full flags.

## Configuration

siphon reads a YAML config from an [XDG](https://specifications.freedesktop.org/basedir-spec/latest/)-compliant path. Find it with `siphon config path`:

- **Linux:** `$XDG_CONFIG_HOME/siphon/config.yaml` → `~/.config/siphon/config.yaml`
- **macOS:** `~/.config/siphon/config.yaml`
- **Windows:** `%APPDATA%\siphon\config.yaml`

Override the location with `SIPHON_CONFIG_HOME`. A profile entry looks like:

```yaml
version: 1
defaults:
  dump_dir: ~/.local/share/siphon/dumps # where backups + sidecars are stored
  jobs: 4
profiles:
  prod:
    driver: postgres
    host: db.example.com
    port: 5432
    user: app_user
    password: env:PROD_DB_PASS # resolved from $PROD_DB_PASS at runtime
    database: app_prod
    sslmode: require
```

Secret references are resolved at runtime, so the config file is safe to commit. Supported schemes: `env:VAR` (environment variable), `keychain://<account>` or `keychain://<service>/<account>` (OS keychain — macOS Keychain / Windows Credential Manager / Linux Secret Service), and `awssm://<secret-id>` or `awssm://<secret-id>#<json-key>` (AWS Secrets Manager; enable with `secrets.awssm: true`). A value matching no scheme is treated as a literal. See [docs/OPS.md](docs/OPS.md#secret-backends).

By default the dump catalog lives on local disk at `defaults.dump_dir`. To store dumps in an S3 / S3-compatible bucket instead, add a `storage:` block:

```yaml
storage:
  type: s3                # "local" (default) | "s3"
  bucket: my-siphon-dumps # required for s3
  prefix: prod            # optional key prefix
  region: us-east-1
  endpoint: ""            # optional: custom endpoint for MinIO / R2
```

Credentials are resolved from the standard AWS chain (env vars, `~/.aws`, instance role) — never stored in the config file. See [docs/STORAGE.md](docs/STORAGE.md) for details.

A `retention:` block (default + optional per-profile override) drives `siphon dumps prune`, which deletes old backups as whole chains so an incremental is never orphaned:

```yaml
defaults:
  retention:
    keep_last: 7
    max_age: 720h            # 30 days
    gfs: { daily: 7, weekly: 4, monthly: 6 }
```

`siphon dumps prune` is dry-run by default; pass `--apply` to delete. Flags override the configured policy per run. See [docs/RETENTION.md](docs/RETENTION.md) for details.

## Architecture

siphon is a strictly layered Go application; imports flow **downward only**, enforced at lint time by `golangci-lint`'s `depguard` (an upward import fails CI):

```
  cmd/siphon                      entry point (one-line main)
        │
  internal/cli   internal/tui     presentation (Cobra · Bubble Tea) — siblings
        └──────┬───────┘
        internal/app              application verbs (backup, restore, sync, …)
               │
   internal/driver/<engine>       database adapters (postgres; mysql/mariadb in v1.0)
               │
  config · secrets · profile · dumps · jobs · errs   domain + support packages
```

- **Drivers** are compile-time Go packages that shell out to native tools for data movement and use a client library (pgx) for fast schema reads.
- **Dumps** live on disk as `<id>.dump` with a `<id>.meta.json` sidecar (profile, driver, size, SHA-256 checksum, timestamp).
- **Jobs** run long operations on a goroutine and stream progress `Event`s that the CLI heartbeat (and, later, the TUI) renders.

## Development

```bash
make help              # list all targets
make build             # produce ./bin/siphon
make run               # go run ./cmd/siphon (launches the TUI on bare invoke)
make test              # unit tests
make test-integration  # integration tests against a real Postgres (needs Docker)
make lint              # golangci-lint, incl. depguard layer enforcement
make tidy              # go mod tidy
make clean             # remove bin/ and dist/
```

Tests run race-clean (`go test -race ./...`). The integration suite is gated behind the `integration` build tag and spins up `postgres:16-alpine` via testcontainers.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full contributor guide and [SECURITY.md](SECURITY.md) for reporting vulnerabilities.

## Roadmap

v1.0 targets a single mega-release covering four pillars:

1. **Foundation** — the CLI, TUI, profiles, config, and dump catalog _(Phases A–C)_.
2. **Drivers** — Postgres, MySQL, and MariaDB _(Phases D–E)_.
3. **Advanced transfer** — incremental backups, native server-to-server streaming, cross-engine sync, and CDC _(Phase F)_.
4. **Ops** — cloud storage (S3/GCS/Azure), multi-backend secrets, profile groups, team mode, audit log, retention policies, and opt-in telemetry _(Phase G)_.

Distribution (Homebrew, Scoop, install script, signed release binaries) lands in Phase H alongside the 1.0 tag.

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md), keep changes within the layered architecture (depguard will tell you if you stray), and make sure `make test` and `make lint` pass before opening a PR.

Adding a new database engine? See [docs/DRIVERS.md](docs/DRIVERS.md) for the driver contributor guide.

Concept docs: [docs/INCREMENTAL.md](docs/INCREMENTAL.md) (incremental backup + restore), [docs/CROSS_ENGINE.md](docs/CROSS_ENGINE.md) (cross-engine sync + the type-map matrix), and [docs/CDC.md](docs/CDC.md) (continuous CDC sync, same- and cross-engine). All three work end-to-end; live DB behavior is integration-tested in CI.

## License

[MIT](LICENSE) © [Nikhil Rajput](https://github.com/nixrajput)
