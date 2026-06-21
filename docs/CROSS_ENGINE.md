# Cross-engine sync

Cross-engine sync moves data between *different* database engines (e.g. Postgres
→ MySQL) by pivoting through a **canonical schema**: the source's native column
types are normalized into an engine-independent vocabulary, then mapped back to
the target engine's dialect. Rows travel as JSONL with per-engine identifier
quoting and bind-parameter placeholders.

## Table of contents

- [How it works](#how-it-works)
- [The CLI surface](#the-cli-surface)
- [Status](#status)
- [Type-mapping matrix](#type-mapping-matrix)
- [Scope and limitations](#scope-and-limitations)

## How it works

The canonical-schema machinery lives in `internal/app/canonical*.go`:

- `CanonicalType` / `CanonicalColumn` / `CanonicalSchema` are the
  engine-independent pivot (`canonical.go`).
- `MapToNative(engine, col)` renders a canonical column as a native column type
  for `postgres`, `mysql`, or `mariadb`. `VARCHAR`/`NUMERIC` carry their
  precision (and scale, for `NUMERIC`) when known; otherwise the type is emitted
  bare and the engine applies its default.
- Identifiers are quoted per engine (`"…"` for Postgres, `` `…` `` for
  MySQL/MariaDB) and escaped, since table/column names cannot be passed as bind
  parameters. Placeholders are `$n` (Postgres) or `?` (MySQL/MariaDB).
- The emit/consume halves (`canonical_emit.go`, `canonical_consume.go`) stream a
  schema + rows as JSONL.

This machinery is built and unit-tested (`canonical_test.go`).

## The CLI surface

```bash
# Cross-engine sync from a Postgres source profile into a MySQL target profile.
siphon sync --from pg-prod --to mysql-staging --cross-engine
```

## Status

| Capability | Status |
| --- | --- |
| Canonical type vocabulary + `MapToNative` matrix | ✅ Works (unit-tested) |
| JSONL emit/consume with per-engine quoting + placeholders | ✅ Works (unit-tested) |
| `sync --cross-engine` end-to-end | ⚠️ Gated off (follow-up) |

The type-mapping and JSONL-transfer machinery exists and is tested, but
`sync --cross-engine` is **capability-gated** on `CrossEngineSource`
(source driver) and `CrossEngineTarget` (target driver). **No driver declares
either capability today**, so the request is rejected with
`ErrDriverUnsupported` (`internal/app/sync.go`, `runCrossEngineSync`).

The reason is deliberate: cross-engine translation needs a *typed*
`CanonicalSchema` (column types), and `driver.Inspect` does not carry column
types yet. Rather than fabricate a schema, the path stays gated until typed
schema introspection lands and a driver flips its cross-engine capabilities to
true. At that point `runCrossEngineSync` gains the real
emit → translate → consume pipeline.

```bash
siphon sync --from pg-prod --to mysql-staging --cross-engine
# Error: cross-engine sync requires typed schema introspection, not yet available
```

## Type-mapping matrix

`MapToNative` maps each canonical type to a native type. These mappings are real
and usable even though the CLI path is gated:

| Canonical type | Postgres | MySQL | MariaDB |
| --- | --- | --- | --- |
| `int` | `integer` | `INT` | `INT` |
| `bigint` | `bigint` | `BIGINT` | `BIGINT` |
| `text` | `text` | `TEXT` | `TEXT` |
| `varchar` | `varchar` | `VARCHAR` | `VARCHAR` |
| `boolean` | `boolean` | `TINYINT(1)` | `TINYINT(1)` |
| `numeric` | `numeric` | `DECIMAL` | `DECIMAL` |
| `uuid` | `uuid` | `CHAR(36)` | `CHAR(36)` |
| `timestamptz` | `timestamptz` | `TIMESTAMP` | `TIMESTAMP` |
| `json` | `jsonb` | `JSON` | `JSON` |

`varchar` with a known length renders as e.g. `VARCHAR(255)`; `numeric` with
known precision/scale renders as e.g. `DECIMAL(10,2)`.

## Scope and limitations

The v1 cross-engine scope (once the path is wired) is **data only**:

- Triggers, views, and stored functions are **skipped**.
- Index recreation is **not implemented yet**: the consume path issues
  `CREATE TABLE IF NOT EXISTS` (column definitions only) plus row INSERTs, so
  only data and table structure transfer. Indexes and any constraints beyond the
  inline column defs are a follow-up.
- Foreign keys are **deferred**.
- Engines covered by the matrix: `postgres`, `mysql`, `mariadb`. An unknown
  engine or an unmappable canonical type is an error from `MapToNative`.
