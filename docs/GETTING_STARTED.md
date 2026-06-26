# Getting started

This walks from install to your first backup, restore, and sync. It assumes you
have the native client tools for your engine on `PATH` (siphon shells out to
them — `pg_dump`/`pg_restore`, `mysqldump`/`mysql`, or `mariadb-dump`/`mariadb`).

## Table of contents

- [Install](#install)
- [Register a profile](#register-a-profile)
- [Back up](#back-up)
- [Verify](#verify)
- [Restore](#restore)
- [Sync](#sync)
- [Exit codes](#exit-codes)
- [Where to go next](#where-to-go-next)

## Install

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/nixrajput/siphon/main/scripts/install.sh | sh

brew install nixrajput/siphon/siphon   # Homebrew
scoop install siphon                   # Scoop (Windows)
```

Confirm it's on your `PATH`:

```bash
siphon --version
```

## Register a profile

A profile is a named connection. Store the password as a **secret reference**
(`env:`, `keychain://`, `awssm://`) so the config file never holds plaintext.

```bash
export PROD_DB_PASS='…'
siphon profile add prod \
  --driver postgres \
  --host db.example.com \
  --user app_user \
  --password 'env:PROD_DB_PASS' \
  --database app_prod \
  --sslmode require

siphon profile list
```

Inspect the schema to confirm the connection works:

```bash
siphon inspect prod      # tables, row estimates, on-disk sizes
```

## Back up

```bash
siphon backup prod       # writes a checksummed dump to the catalog
siphon dumps list        # newest first; note the dump id
```

Each dump is a single file prefixed with a metadata envelope and recorded with a
SHA-256 checksum in a sidecar.

## Verify

```bash
siphon verify <dump-id>  # re-hashes the dump against its recorded checksum
```

A mismatch exits with the integrity code (see below), so CI can catch corruption
or tampering.

## Restore

```bash
siphon restore --profile staging --dump <dump-id> --clean
```

`--clean` drops and recreates objects before loading. For an incremental dump,
`restore` resolves and replays the whole base→incremental chain in order.

## Sync

Back up the source and restore into the target in one streamed pass — no
intermediate file on disk:

```bash
siphon sync prod staging
```

A backup failure propagates to the restore side, so a truncated dump is never
committed as if it were clean.

## Exit codes

siphon uses a POSIX-friendly taxonomy so scripts and CI behave correctly:

| Code | Meaning |
| --- | --- |
| `0` | success |
| `1` | user error (bad input, missing profile, failed confirmation) |
| `2` | system error (I/O, network, the underlying tool failed) |
| `3` | integrity failure (a checksum did not match) |
| `130` | cancelled (Ctrl-C) |

So `siphon backup prod && upload-somewhere` only uploads on a clean backup.

## Where to go next

- [Configuration reference](CONFIGURATION.md) — the full config-file schema.
- [Incremental backup](INCREMENTAL.md), [cross-engine sync](CROSS_ENGINE.md),
  and [CDC](CDC.md) — the advanced transfer modes.
- [Storage backends](STORAGE.md) and [retention](RETENTION.md) — where dumps
  live and how they're pruned.
- [Operational features](OPS.md) — audit log, 2FA, telemetry, schedule, tunnel.
