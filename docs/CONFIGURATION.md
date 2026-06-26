# Configuration reference

siphon reads one YAML config file. Find or edit it with `siphon config path` /
`siphon config edit`. Location (XDG-compliant; override with `SIPHON_CONFIG_HOME`):

- **Linux:** `$XDG_CONFIG_HOME/siphon/config.yaml` → `~/.config/siphon/config.yaml`
- **macOS:** `~/.config/siphon/config.yaml`
- **Windows:** `%APPDATA%\siphon\config.yaml`

The file is safe to commit **as long as every secret is a reference**, not a
literal (see [Secret references](#secret-references)).

## Table of contents

- [Top-level shape](#top-level-shape)
- [defaults](#defaults)
- [profiles](#profiles)
- [Secret references](#secret-references)
- [storage](#storage)
- [retention](#retention)
- [audit](#audit)
- [telemetry](#telemetry)
- [secrets](#secrets)
- [groups](#groups)

## Top-level shape

```yaml
version: 1
defaults: { … }      # cross-profile defaults
storage: { … }       # where dumps live (local | s3)
audit: { … }         # destructive-op audit log
telemetry: { … }     # opt-in aggregate metrics
secrets: { … }       # optional secret backends
profiles: { … }      # named connections
groups: { … }        # profile groups (gating policy)
```

Every block except `profiles` is optional; omitted blocks use safe defaults.

## defaults

```yaml
defaults:
  dump_dir: ~/.local/share/siphon/dumps  # local catalog path (when storage is local)
  jobs: 4                                # parallel workers where supported
  compression: 1                         # dump compression level
  retention: { … }                       # default retention policy (see below)
```

## profiles

A named connection. The map key is the profile name.

```yaml
profiles:
  prod:
    driver: postgres        # postgres | mysql | mariadb
    host: db.example.com
    port: 5432
    user: app_user
    password: env:PROD_DB_PASS   # a secret reference (see below)
    database: app_prod
    sslmode: require
    group: critical              # optional; ties to a groups: entry
    retention: { … }             # optional per-profile override (replaces defaults)
    tunnel:                      # optional SSH bastion
      bastion: jump@bastion.example.com
      local_port: 15432          # defaults to the DB port
```

## Secret references

Any secret field (today: `password`, and a group's `totp_secret`) is resolved at
runtime by scheme:

| Scheme | Example | Source |
| --- | --- | --- |
| `env` | `env:PROD_DB_PASS` | environment variable |
| `keychain` | `keychain://prod-db` · `keychain://svc/acct` | OS credential store |
| `awssm` | `awssm://prod/db#password` | AWS Secrets Manager (a `#key` selects a JSON field) |
| *(none)* | `hunter2` | literal value — **don't commit this** |

`keychain://` is always available; `awssm://` must be enabled under
[`secrets`](#secrets). See [docs/OPS.md](OPS.md#secret-backends) for detail.

## storage

Where the dump catalog physically lives. Omitted = local at `defaults.dump_dir`.

```yaml
storage:
  type: s3                # "local" (default) | "s3"
  bucket: my-siphon-dumps # required for s3
  prefix: prod            # optional key prefix within the bucket
  region: us-east-1
  endpoint: ""            # optional: custom endpoint for MinIO / R2
```

S3 credentials come from the standard AWS chain, never from config. Full detail:
[docs/STORAGE.md](STORAGE.md).

## retention

Drives `siphon dumps prune`. A profile's block **replaces** the defaults block
wholesale. An empty/omitted policy keeps everything.

```yaml
defaults:
  retention:
    keep_last: 7          # keep the N newest chains
    max_age: 720h         # keep chains younger than this (Go duration)
    gfs: { daily: 7, weekly: 4, monthly: 6 }
```

A chain is kept if it satisfies **any** active rule. Full detail:
[docs/RETENTION.md](RETENTION.md).

## audit

Append-only JSONL log of destructive operations. Off by default.

```yaml
audit:
  enabled: true
  path: ~/.local/state/siphon/audit.log   # optional; this is the default
```

## telemetry

Opt-in aggregate per-op counts and error tallies (op name + outcome only — never
identifying data). Off by default.

```yaml
telemetry:
  enabled: true
  path: ~/.local/state/siphon/telemetry.json   # optional; this is the default
```

## secrets

Enables optional secret backends. `keychain://` works with no config; AWS
Secrets Manager is gated here because constructing it loads AWS config.

```yaml
secrets:
  awssm: true             # enable the awssm:// backend
  awssm_region: us-east-1 # optional; defaults to the AWS chain's region
```

## groups

A group applies a gating policy to its member profiles before destructive ops.

```yaml
groups:
  critical:
    confirm_destructive: true       # operator must retype the profile name
    require_2fa: true               # operator must enter a current TOTP code
    totp_secret: env:SIPHON_PROD_TOTP   # base32 RFC-6238 secret (a secret-ref)
    color: red                      # TUI accent
```

`require_2fa` with no resolvable `totp_secret` fails closed. Full detail:
[docs/OPS.md](OPS.md#2fa--group-gating).
