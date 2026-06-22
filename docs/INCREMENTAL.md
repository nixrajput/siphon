# Incremental backups

Incremental backups capture only what changed since an earlier dump, so a chain
of small incrementals can stand in for repeated full dumps. siphon records the
relationship between dumps in the **dump envelope** (a 4 KB JSON header
prepended to every dump) and reconstructs the full picture at restore time by
walking the base → incremental chain.

## Table of contents

- [How it works](#how-it-works)
- [The CLI surface](#the-cli-surface)
- [Status](#status)
- [Examples](#examples)
- [Limitations and runtime gates](#limitations-and-runtime-gates)

## How it works

Every dump carries an envelope (`internal/dumps/envelope.go`) with a `type`
(`base` or `incremental`), a `base_id`/`parent_id`, and engine-specific resume
coordinates: `wal_start`/`wal_end` for Postgres, `binlog_file` +
`binlog_start`/`binlog_end` for MySQL/MariaDB.

`Catalog.ResolveChain` (`internal/dumps/chain.go`) walks `parent_id` backwards
from a target dump to its base, detecting cycles and broken chains rather than
looping or silently truncating. `Restore` then applies the resolved chain in
order, base first (`internal/app/restore.go`). A plain, non-incremental dump
resolves to a single-element chain, so the same restore path serves both.

An incremental backup is a **bounded change capture**: starting from the base
dump's recorded end position, siphon streams the row changes that committed since
then up to a fixed end position captured at backup time, and serializes each as a
JSONL `CanonicalChange` (insert/update/delete with primary key + post-image). The
incremental dump body is therefore engine-neutral change records, not raw
WAL/binlog bytes. At restore time those changes are **replayed** via
`ApplyChange` rather than fed to the native restore tool — base links restore
natively, incremental links replay change records.

The driver-level capture (`driver.IncrementalBackuper`):

- **Postgres** (`internal/driver/postgres/incremental_change.go`) captures the
  current `pg_current_wal_lsn()` as the end bound, then drives the same pgoutput
  logical-decoding loop as CDC with that LSN as a stop target — it returns
  cleanly at the first message boundary past the bound, so every change committed
  at or before it is captured and none after.
- **MySQL/MariaDB** (`internal/driver/_mysqlcommon/incremental.go`) captures the
  current binlog file + offset via `SHOW BINARY LOG STATUS` (MySQL 8.4+) or
  `SHOW MASTER STATUS` (older MySQL / MariaDB) as the end bound, then decodes the
  fork's binlog tool output up to that offset.

## The CLI surface

```bash
# Restore a dump, walking its base→incremental chain automatically.
siphon restore <dump-id> --profile <target>

# Stop applying the chain after a specific dump (point-in-chain restore).
siphon restore <dump-id> --profile <target> --up-to <intermediate-id>

# Take an incremental backup capturing changes since a base dump.
siphon backup <profile> --incremental --base <base-dump-id>
```

`--incremental` requires `--base <dump-id>`; `--base` without `--incremental` (or
`--incremental` without `--base`) is rejected with a clear error.

## Status

| Capability | Status |
| --- | --- |
| Dump envelope (type/base/parent, WAL & binlog fields) | ✅ Works |
| Chain resolution (`ResolveChain`) | ✅ Works |
| Chain-walking restore (base → incrementals, in order) | ✅ Works |
| `restore --up-to <id>` (stop chain early) | ✅ Works |
| `backup --incremental --base <id>` (bounded change capture) | ✅ Works |
| Incremental restore (change replay via `ApplyChange`) | ✅ Works |
| Postgres orphan replication-slot sweep | ✅ Works |

The full incremental path is wired end-to-end: `backup --incremental` reads the
base envelope's end position, captures the bounded change set via the driver's
`IncrementalBackuper`, and writes an `incremental`-type catalog entry whose
envelope carries this capture's end position (so the next incremental resumes
exactly here). Restore replays each incremental link's changes via `ApplyChange`.
The live-server behavior is exercised in CI (integration-tagged tests against a
`wal_level=logical` Postgres); it is compile-checked but not run locally.

## Examples

Chain-walking restore (works today). If `inc-2` was built on `inc-1` on
`base-0`, restoring `inc-2` applies all three in order:

```bash
siphon restore inc-2 --profile prod-replica
```

Point-in-chain restore with `--up-to` (works today) — apply only up to and
including `inc-1`, skipping `inc-2`:

```bash
siphon restore inc-2 --profile prod-replica --up-to inc-1
```

A typo'd `--up-to` is rejected (the dump isn't in the chain) rather than
silently restoring more than asked.

Taking an incremental backup against a base dump:

```bash
siphon backup prod --incremental --base base-0
# Captures changes committed since base-0's end position and writes a new
# incremental dump linked to base-0. Restoring it later replays base-0 then the
# captured changes.
```

## Limitations and runtime gates

These apply to the incremental **backup** path:

- **Postgres** uses a persistent logical replication slot (`siphon_logical`) as
  the change-stream resume anchor. Per-base physical slots are swept
  automatically: before each incremental capture, `SweepOrphanSlots` drops any
  inactive `siphon_*` physical slot (an inactive siphon slot is by definition
  orphaned — a completed backup drops its own, an active one is in use). The
  persistent logical slot is excluded from the sweep so the resume position
  survives between runs.
- **MySQL/MariaDB** require `binlog_format=ROW` for usable incrementals.
- **Cross-version incrementals are unsupported** (`CrossVersionIncremental:
  false`): a chain must be captured and restored against the same engine major
  version.
