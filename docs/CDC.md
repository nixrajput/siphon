# Change data capture (CDC)

CDC ("continuous sync" / follow mode) keeps a target database in step with a
source by streaming changes as they happen, rather than re-running a full
backup/restore. siphon streams the source's logical change stream and applies
each change to the target continuously, until interrupted. It works
**same-engine** (Postgres→Postgres, MySQL→MySQL) and **cross-engine**
(Postgres→MySQL) alike, because changes are carried as engine-neutral
`CanonicalChange`s and replayed natively on the target.

## Table of contents

- [How it works](#how-it-works)
- [The CLI surface](#the-cli-surface)
- [Status](#status)
- [Resume state](#resume-state)
- [Limitations](#limitations)

## How it works

`internal/app/cdc.go` holds:

- **`CDCState`** — the persisted resume record (`job_id`, source/target
  profiles, `last_applied_lsn` for Postgres, `last_binlog_file` +
  `last_binlog_pos` for MySQL/MariaDB, `updated_at`).
- **`saveCDCState` / `loadCDCState`** — JSON state files under a per-user
  directory resolved from `SIPHON_STATE_HOME`, then `XDG_STATE_HOME`, then
  `$HOME/.local/state/siphon/cdc`.
- **`RunCDC`** — the capability-gated entry point. Both drivers must advertise
  `CapCDC`. It connects source and target, then:
  1. **First run (no prior state):** captures the source's current stream
     position (`BasePositioner.CurrentPosition`), takes an initial schema+data
     snapshot via the canonical transfer pipe (`InspectSchema` →
     `EmitCanonical` → `ConsumeCanonical`), persists the start position, then
     streams changes committed after it.
  2. **Restart (prior state exists):** resumes from the saved position, no
     snapshot.
  3. **Stream loop:** `ChangeStreamer.StreamChanges` (unbounded) emits each
     `CanonicalChange`; `RunCDC` applies it via `CanonicalTransfer.ApplyChange`
     on the target and persists the streamer's delivered position on exit.

The `job_id` is a stable hash of `(from, to)`, so re-running the same continuous
sync resumes from the same state file.

`CDCState` save/load round-trip, the state-dir resolution, and the
no-capability rejection are unit-tested (`internal/app/cdc_test.go`). End-to-end
streaming (same-engine, cross-engine, and resume) is covered by integration
tests (`internal/app/cdc_integration_test.go`, `-tags integration`).

## The CLI surface

Two equivalent entry points:

```bash
# Dedicated command
siphon cdc <from> <to>
siphon cdc --from pg-prod --to pg-replica

# sync follow mode (equivalent)
siphon sync --from pg-prod --to pg-replica --continuous
```

Both stream continuously until interrupted. Press Ctrl-C to stop cleanly —
ctx cancellation is the normal termination signal; the final position is
persisted on exit so a later run resumes without a gap.

## Status

| Capability | Status |
| --- | --- |
| `CDCState` persistence (save/load) | ✅ Works (unit-tested) |
| State-dir resolution (`SIPHON_STATE_HOME` / `XDG_STATE_HOME`) | ✅ Works (unit-tested) |
| Resume from prior state on restart | ✅ Works |
| Capability gating on `CapCDC` | ✅ Works (true on postgres, mysql, mariadb) |
| `siphon cdc` CLI subcommand | ✅ Wired |
| `sync --continuous` follow mode | ✅ Routes to `RunCDC` |
| Initial snapshot → stream handoff | ✅ Works |
| Continuous change streaming (same + cross-engine) | ✅ Works |

## Resume state

`RunCDC` resumes from the last persisted position. State files live at:

```text
$SIPHON_STATE_HOME/cdc/<job-id>.state        # if SIPHON_STATE_HOME is set
$XDG_STATE_HOME/siphon/cdc/<job-id>.state     # else if XDG_STATE_HOME is set
$HOME/.local/state/siphon/cdc/<job-id>.state  # default
```

Each file is JSON: source/target profiles plus the last applied position (LSN
for Postgres, binlog file + offset for MySQL/MariaDB).

**Checkpoint/resume granularity is "since the streamer's last delivered
position."** RunCDC persists the position the change streamer reports when the
stream stops — a position tied to what was actually delivered, never ahead of
it. There is deliberately **no** ahead-of-stream periodic checkpoint: writing
the source's *current* WAL/binlog end mid-stream would, after a crash, resume
past changes that were streamed but never applied — silent data loss. After a
crash it resumes from the last persisted position and re-applies the tail. This
is safe because delivery is **at-least-once** and `ApplyChange` is
**idempotent**: INSERT is idempotent (upsert), and UPDATE/DELETE target by
primary key. Re-applying the tail is therefore a no-op — no gaps, no duplicates.

## Limitations

- **Postgres source requires `wal_level=logical`** and sufficient
  `max_replication_slots` / `max_wal_senders`; MySQL/MariaDB source requires
  row-based binlogging. These are the same prerequisites as incremental backup.
- **At-least-once, not exactly-once** — see resume granularity above. Correct
  because apply is idempotent; a target with non-idempotent side effects
  (triggers, etc.) is out of scope.
- **Snapshot consistency window** — the start position is captured before the
  snapshot, so changes committed during the snapshot are re-streamed and
  re-applied idempotently rather than lost.
