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
| Typed schema introspection (`SchemaInspector`, with primary keys) | ✅ Works (Postgres + MySQL/MariaDB) |
| `sync --cross-engine` snapshot (existing data) | ✅ Works (integration-tested in CI: Postgres → MySQL) |
| Cross-engine CDC (continuous change replication) | ✅ Works — see [docs/CDC.md](CDC.md) |

`sync --cross-engine` is capability-gated on `CrossEngineSource` (source) and
`CrossEngineTarget` (target). All three drivers now declare both, backed by
`driver.SchemaInspector` (typed column introspection from `information_schema` /
`pg_catalog`, including primary keys) and `driver.CanonicalTransfer` (the
engine-side emit/consume). `runCrossEngineSync` inspects the source schema,
streams its rows as canonical JSONL through a bounded `jobs.Stream`, and the
target re-creates the tables (with primary keys) and inserts the rows —
translating types via `MapToNative`.

```bash
siphon sync --from pg-prod --to mysql-staging --cross-engine
```

Scope: this is **data + table structure** (columns, types, primary keys) — not
behavior. Indexes (beyond the primary key), foreign keys, triggers, views, and
functions are **not** translated. The live Postgres → MySQL round-trip is
validated by an integration test in CI (`internal/app/cross_engine_integration_test.go`);
it is compile-checked locally where Docker is unavailable.

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

The v1 cross-engine scope is **data + table structure**:

- Triggers, views, and stored functions are **skipped**.
- Primary keys **are** recreated (the consume path emits `CREATE TABLE … PRIMARY KEY (…)`).
- Secondary index recreation is **not implemented yet**: the consume path issues
  `CREATE TABLE IF NOT EXISTS` (column definitions + primary key) plus row INSERTs, so
  only data and table structure transfer. Indexes and any constraints beyond the
  inline column defs are a follow-up.
- Foreign keys are **deferred**.
- Engines covered by the matrix: `postgres`, `mysql`, `mariadb`. An unknown
  engine or an unmappable canonical type is an error from `MapToNative`.
