# Operational features

Phase G's ops suite adds five operational capabilities: an audit log, 2FA /
group gating, opt-in telemetry, scheduled backups, and an SSH tunnel helper.
The first three share one interception point in the app layer (`guardedOp`),
which wraps the destructive verbs (backup, restore, sync, prune) exactly once.

## Table of contents

- [Audit log](#audit-log)
- [2FA & group gating](#2fa--group-gating)
- [Telemetry](#telemetry)
- [Scheduled backups](#scheduled-backups)
- [SSH tunnel](#ssh-tunnel)

## Audit log

An append-only JSONL trail of destructive operations — who ran what, against
which profile, when, and the outcome. Off by default.

```yaml
audit:
  enabled: true
  path: ~/.local/state/siphon/audit.log   # optional; this is the default
```

Each line records `time`, `op`, `profile`, `target`, `actor` (OS user),
`outcome` (`ok`/`error`), `error`, and `duration_ms`. Audit writes are
best-effort: a failure to write the log never fails the operation it records.

## 2FA & group gating

A profile belongs to a **group**; a group can require a second deliberate step
before any destructive op on its profiles:

```yaml
groups:
  critical:
    confirm_destructive: true          # operator must retype the profile name
    require_2fa: true                  # operator must enter a current TOTP code
    totp_secret: env:SIPHON_PROD_TOTP  # base32 RFC-6238 secret (a secret-ref)

profiles:
  prod:
    driver: postgres
    group: critical
    # ...
```

`confirm_destructive` prompts the operator to retype the profile name;
`require_2fa` prompts for a 6-digit TOTP verified (with ±1 step skew) against the
group's `totp_secret` — the same code your authenticator app shows. The check
runs **before** the operation, so a failed confirmation aborts before any
destructive work. `require_2fa` with no resolvable secret fails closed. The TOTP
secret is a secret-ref, so the plaintext never lives in config.

This is offline by design — siphon is a local CLI, so "2FA" means a TOTP
(standard, no network) rather than a push notification.

## Telemetry

Opt-in aggregate operational metrics: per-op counts and error tallies, flushed
as JSON. Off by default.

```yaml
telemetry:
  enabled: true
  path: ~/.local/state/siphon/telemetry.json   # optional; this is the default
```

Telemetry records **only** the operation name and outcome — never profile names,
hosts, dump IDs, the actor, or any data. It is composed onto the audit seam, so
enabling it adds no new interception in the verbs.

## Scheduled backups

`siphon schedule` manages recurring backups by maintaining a delimited,
siphon-owned block in your **crontab** — siphon does not run a scheduler daemon;
your system's cron invokes `siphon backup <profile>` on the schedule.

```bash
siphon schedule add prod --cron "0 2 * * *"   # nightly at 02:00
siphon schedule list
siphon schedule remove prod
```

Entries outside the siphon-managed block are preserved. Re-adding a profile
updates its schedule in place; removing the last entry drops the managed block.
Requires the `crontab` command.

> **Gating caveat:** a scheduled job runs `siphon backup <profile>` non-interactively
> under cron. If the profile's group sets `confirm_destructive` or `require_2fa`,
> that backup will block waiting for input it can never receive and the cron job
> will fail. Don't schedule backups for gated profiles (a non-interactive bypass
> for trusted automation is a future enhancement).

## SSH tunnel

`siphon tunnel <profile>` opens an SSH local-forward to a profile's database
through a configured bastion, using your **system ssh client** (your ssh config,
keys, and agent all apply). It runs in the foreground and holds the tunnel open
until you press Ctrl-C.

```yaml
profiles:
  prod:
    driver: postgres
    host: db.internal
    port: 5432
    tunnel:
      bastion: jump@bastion.example.com
      local_port: 15432         # optional; defaults to the DB port
```

```bash
siphon tunnel prod
# tunnel open: localhost:15432 → db.internal:5432 via jump@bastion.example.com (Ctrl-C to close)
```

Run it in one terminal and point a client (or another siphon command) at the
printed local address in another. siphon delegates to `ssh -L` rather than
reimplementing SSH or holding a connection in a daemon.
