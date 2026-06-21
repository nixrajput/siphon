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

The driver-level capture machinery also exists:

- **Postgres** (`internal/driver/postgres/incremental.go`) creates a temporary
  physical replication slot and records the start/end LSN around a base backup,
  so a later incremental can resume from the correct WAL position.
- **MySQL/MariaDB** (`internal/driver/_mysqlcommon/incremental.go`) captures the
  binlog file + position via `SHOW BINARY LOG STATUS` (MySQL 8.4+) or
  `SHOW MASTER STATUS` (older MySQL / MariaDB).

## The CLI surface

```bash
# Restore a dump, walking its base→incremental chain automatically.
siphon restore <dump-id> --profile <target>

# Stop applying the chain after a specific dump (point-in-chain restore).
siphon restore <dump-id> --profile <target> --up-to <intermediate-id>

# Request an incremental backup (NOT yet wired — see Status).
siphon backup <profile> --incremental --base <base-dump-id>
```

## Status

| Capability | Status |
| --- | --- |
| Dump envelope (type/base/parent, WAL & binlog fields) | ✅ Works |
| Chain resolution (`ResolveChain`) | ✅ Works |
| Chain-walking restore (base → incrementals, in order) | ✅ Works |
| `restore --up-to <id>` (stop chain early) | ✅ Works |
| Driver-level capture machinery (Postgres WAL slot/LSN, MySQL/MariaDB binlog pos) | ✅ Exists |
| `backup --incremental --base <id>` end-to-end | ⚠️ Not yet wired (follow-up) |

The restore-side chain machinery and the envelope are in place and tested. The
`--incremental` backup path is a documented follow-up: the driver machinery and
the chain restore are both ready, but the wiring that ties them together — read
the base envelope for the parent WAL/binlog position, invoke the driver's
incremental capture method, and write an `incremental`-type catalog entry — is
not yet connected. Until then `siphon backup --incremental` returns a clear
error: *"incremental backup is not yet wired end-to-end (Phase F follow-up); the
driver-level machinery exists"* (`internal/cli/backup.go`).

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

Requesting an incremental backup (returns the not-wired error today):

```bash
siphon backup prod --incremental --base base-0
# Error: incremental backup is not yet wired end-to-end (Phase F follow-up);
#        the driver-level machinery exists
```

## Limitations and runtime gates

These apply to the incremental **backup** path once it is wired:

- **Postgres** anchors WAL retention with a temporary physical replication slot.
  Orphan-slot cleanup is **not yet built** — an orphaned slot pins WAL on the
  server and must be dropped manually until automatic cleanup ships.
- **MySQL/MariaDB** require `binlog_format=ROW` for usable incrementals.
- **Cross-version incrementals are unsupported** (`CrossVersionIncremental:
  false`): a chain must be captured and restored against the same engine major
  version.
