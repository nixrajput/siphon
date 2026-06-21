# Change data capture (CDC)

CDC ("continuous sync" / follow mode) keeps a target database in step with a
source by streaming changes as they happen, rather than re-running a full
backup/restore. In siphon this is **scaffolding today** — the resume-state
plumbing and capability gating are in place, but continuous streaming is a
future deliverable and does not run.

## Table of contents

- [What exists](#what-exists)
- [The CLI surface](#the-cli-surface)
- [Status](#status)
- [Resume state](#resume-state)
- [Limitations](#limitations)

## What exists

`internal/app/cdc.go` holds:

- **`CDCState`** — the persisted resume record (`job_id`, source/target
  profiles, `last_applied_lsn` for Postgres, `last_binlog_file` +
  `last_binlog_pos` for MySQL/MariaDB, `updated_at`).
- **`saveCDCState` / `loadCDCState`** — JSON state files under a per-user
  directory resolved from `SIPHON_STATE_HOME`, then `XDG_STATE_HOME`, then
  `$HOME/.local/state/siphon/cdc`.
- **`RunCDC`** — a capability-gated entry point. It resolves the source and
  target profiles, then requires both drivers to advertise `CapCDC`. Its body is
  a **polling scaffold**: a 10-second ticker that persists state each tick and
  resumes from a prior run's state on restart. It is **not** real change
  streaming.

`CDCState` save/load round-trip, the state-dir resolution, and the
no-capability rejection are unit-tested (`internal/app/cdc_test.go`).

## The CLI surface

There is **no `siphon cdc` subcommand wired today.** `RunCDC` exists as an
application function but is not registered as a Cobra command
(`internal/cli/root.go` wires `backup`, `restore`, `sync`, `verify`, `inspect`,
`profile`, `dumps`, `config`, `schedule`, `tunnel` — no `cdc`).

`sync --continuous` exposes the flag but does not run CDC; it returns a clear
error pointing at the (not-yet-wired) `siphon cdc` follow-up:

```bash
siphon sync --from pg-prod --to pg-replica --continuous
# Error: continuous CDC sync is not wired here; use `siphon cdc` (Phase F Task 10)
```

## Status

| Capability | Status |
| --- | --- |
| `CDCState` persistence (save/load) | ✅ Works (unit-tested) |
| State-dir resolution (`SIPHON_STATE_HOME` / `XDG_STATE_HOME`) | ✅ Works (unit-tested) |
| Resume from prior state on restart | ✅ Works (in the scaffold loop) |
| Capability gating on `CapCDC` | ✅ Works |
| `siphon cdc` CLI subcommand | ❌ Not wired (no Cobra command) |
| Continuous change streaming | ⚠️ Scaffold only (polling tick; not real CDC) |

CDC does **not run today**. No driver declares `CapCDC` true, so `RunCDC` is
rejected with `ErrDriverUnsupported`. Even if a driver enabled it, the loop is a
polling scaffold, not logical-replication streaming.

## Resume state

When CDC streaming is built, `RunCDC` resumes from the last persisted position.
State files live at:

```text
$SIPHON_STATE_HOME/cdc/<job-id>.state        # if SIPHON_STATE_HOME is set
$XDG_STATE_HOME/siphon/cdc/<job-id>.state     # else if XDG_STATE_HOME is set
$HOME/.local/state/siphon/cdc/<job-id>.state  # default
```

Each file is JSON: source/target profiles plus the last applied position (LSN
for Postgres, binlog file + offset for MySQL/MariaDB).

## Limitations

- **Not enabled on any driver** — `CapCDC` is `false` everywhere, so CDC is
  rejected up front.
- **No real streaming** — the scaffold polls on a ticker. Real logical-
  replication streaming (`pglogrepl` for Postgres, binlog tailing for
  MySQL/MariaDB) is deferred to a future revision.
- **No CLI entry point** — `RunCDC` is internal application scaffolding, not a
  user-facing command yet.
