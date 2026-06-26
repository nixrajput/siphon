<div align="center">

# siphon

**Sync any database, anywhere.**

A single binary that turns the painful, error-prone sprawl of `pg_dump` → `pg_restore` shell scripts into a guided, observable workflow — with named profiles, integrity-checked dumps, and server-to-server streaming, across multiple database engines.

[![CI](https://github.com/nixrajput/siphon/actions/workflows/ci.yml/badge.svg)](https://github.com/nixrajput/siphon/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/nixrajput/siphon.svg)](https://pkg.go.dev/github.com/nixrajput/siphon)
[![Go Version](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/status-1.0-brightgreen)](#features)

</div>

---

## Table of contents

- [siphon](#siphon)
  - [Table of contents](#table-of-contents)
  - [Why siphon](#why-siphon)
  - [Features](#features)
  - [Requirements](#requirements)
  - [Install](#install)
  - [Quick start](#quick-start)
  - [Commands](#commands)
  - [Configuration](#configuration)
  - [Architecture](#architecture)
  - [Development](#development)
  - [Roadmap](#roadmap)
  - [Contributing](#contributing)
  - [Support](#support)
  - [License](#license)

## Why siphon

- **One CLI, many databases.** Postgres, MySQL, and MariaDB all work today (MySQL and MariaDB share a common `_mysqlcommon` backend). The driver interface is engine-agnostic, so SQLite, MongoDB, SQL Server, and ClickHouse can follow.
- **Native, not reimplemented.** siphon shells out to `pg_dump`/`pg_restore`, `mysqldump`/`mysql`, and `mariadb-dump`/`mariadb` for the actual data movement — you inherit 20+ years of correctness from the official tools, wrapped in a consistent UX.
- **Integrity by default.** Every dump is checksummed (SHA-256) and recorded in a sidecar metadata file. `siphon verify` re-hashes the dump and flags corruption or tampering — and fails with a distinct exit code so CI can catch it.
- **Built for scripts and humans.** A Cobra command tree with predictable flags and POSIX exit codes for automation; an interactive Bubble Tea dashboard when you invoke `siphon` bare.
- **Named profiles + secret refs.** Store connection details once; with a secret ref like `env:VAR`, `keychain://<account>` (OS credential store), or `awssm://<id>#<key>` (AWS Secrets Manager), the password is resolved at runtime instead of being stored in config. (A value with no scheme is a literal, so don't commit a plaintext password.)
- **Streaming sync.** `siphon sync src dst` pipes a backup straight into a restore with no intermediate file on disk.

## Features

- **Multi-engine, native tooling.** PostgreSQL, MySQL, and MariaDB, each driven by the official client tools (`pg_dump`/`pg_restore`, `mysqldump`/`mysql`, `mariadb-dump`/`mariadb`). The `Driver` interface is engine-agnostic, so more engines can follow.
- **Core workflow.** `backup`, `restore`, `sync`, `verify`, `inspect`, and a `dumps` catalog — driven by named `profile`s and a single config file.
- **Streaming sync.** `siphon sync src dst` pipes a backup straight into a restore through a bounded buffer — no intermediate file, with backpressure and failures propagated end to end.
- **Incremental backup.** `backup --incremental --base <id>` captures a bounded change set since a base via Postgres logical decoding or MySQL/MariaDB binlog; `restore` replays the base→incremental chain in order — see [docs/INCREMENTAL.md](docs/INCREMENTAL.md).
- **Cross-engine sync.** `sync --cross-engine` introspects the source schema into a canonical model and maps types across engines (e.g. Postgres → MySQL) — see [docs/CROSS_ENGINE.md](docs/CROSS_ENGINE.md).
- **CDC (continuous replication).** `siphon cdc` / `sync --continuous` tails the source's change stream and applies it to the target, with a snapshot→stream handoff, resumable state, same- and cross-engine — see [docs/CDC.md](docs/CDC.md).
- **Integrity by default.** Every dump is SHA-256 checksummed in a sidecar; `siphon verify` re-hashes and fails with a distinct exit code so CI catches corruption or tampering.
- **Cloud storage.** Keep the dump catalog locally or in an S3 / S3-compatible bucket via a pluggable `storage.Store` backend (`storage:` config) — see [docs/STORAGE.md](docs/STORAGE.md).
- **Retention.** Chain-aware pruning (`siphon dumps prune`) with keep-last-N / max-age / GFS rules and per-profile `retention:` config — see [docs/RETENTION.md](docs/RETENTION.md).
- **Operational controls.** An append-only audit log of destructive ops, 2FA/group gating (typed confirmation and/or TOTP), opt-in aggregate telemetry, `siphon schedule` (recurring backups via crontab), and `siphon tunnel` (SSH local-forward via a bastion) — see [docs/OPS.md](docs/OPS.md).
- **Secret references.** Resolve passwords at runtime via `env:`, `keychain://` (OS credential store), or `awssm://` (AWS Secrets Manager) instead of storing them in config.
- **Scripts and humans.** A predictable Cobra command tree with POSIX exit codes for automation, plus an interactive Bubble Tea dashboard (profiles · dumps · jobs) when you invoke `siphon` bare.
- **Easy to install, signed.** Cross-platform binaries published via a tag-triggered release with SHA-256 checksums and cosign-keyless signatures, a checksum-verifying `curl | sh` install script, and docs at [siphon.nixrajput.com](https://siphon.nixrajput.com).

## Requirements

- **[Go](https://go.dev/dl/) 1.26 or newer** — only needed to build from source; prebuilt binaries are available via the install script, Homebrew, and Scoop (see [Install](#install)).
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

> The install script is live as of `v1.0.0`. The Homebrew tap and Scoop bucket activate once their tap repos + tokens are provisioned; until then, use the install script or [build from source](#from-source).

**Linux / macOS** — the install script downloads the right release binary and verifies its SHA-256 before installing:

```bash
curl -fsSL https://raw.githubusercontent.com/nixrajput/siphon/main/scripts/install.sh | sh
```

Override the target with `SIPHON_INSTALL_DIR=…` or pin a version with `SIPHON_VERSION=v1.0.0`. The docs site lives at [siphon.nixrajput.com](https://siphon.nixrajput.com).

**Homebrew:**

```bash
brew install nixrajput/siphon/siphon
```

**Scoop (Windows):**

```powershell
scoop bucket add siphon https://github.com/nixrajput/scoop-siphon
scoop install siphon
```

**Prebuilt binaries** for every OS/arch are attached to each [release](https://github.com/nixrajput/siphon/releases), with a `checksums.txt` and a cosign keyless signature. Verify provenance — pin the issuer **and** the signer identity, or a forged Fulcio cert would still pass:

```bash
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --certificate-identity "https://github.com/nixrajput/siphon/.github/workflows/release.yml@refs/tags/v1.0.0" \
  checksums.txt
```

Substitute the tag you downloaded for `v1.0.0`. Then check a binary's archive against the verified `checksums.txt`.

**From source:**

```bash
git clone https://github.com/nixrajput/siphon.git
cd siphon && make build
./bin/siphon --version
```

This produces `./bin/siphon`; move it onto your `PATH` (e.g. `sudo install -m 0755 bin/siphon /usr/local/bin/siphon`).

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

Secret references are resolved at runtime, which keeps the password out of the config file when you use one. Supported schemes: `env:VAR` (environment variable), `keychain://<account>` or `keychain://<service>/<account>` (OS keychain — macOS Keychain / Windows Credential Manager / Linux Secret Service), and `awssm://<secret-id>` or `awssm://<secret-id>#<json-key>` (AWS Secrets Manager; enable with `secrets.awssm: true`). A value matching no scheme is treated as a literal — so a plaintext password lives in the file and should not be committed. See [docs/OPS.md](docs/OPS.md#secret-backends).

By default the dump catalog lives on local disk at `defaults.dump_dir`. To store dumps in an S3 / S3-compatible bucket instead, add a `storage:` block:

```yaml
storage:
  type: s3 # "local" (default) | "s3"
  bucket: my-siphon-dumps # required for s3
  prefix: prod # optional key prefix
  region: us-east-1
  endpoint: "" # optional: custom endpoint for MinIO / R2
```

Credentials are resolved from the standard AWS chain (env vars, `~/.aws`, instance role) — never stored in the config file. See [docs/STORAGE.md](docs/STORAGE.md) for details.

A `retention:` block (default + optional per-profile override) drives `siphon dumps prune`, which deletes old backups as whole chains so an incremental is never orphaned:

```yaml
defaults:
  retention:
    keep_last: 7
    max_age: 720h # 30 days
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

1.0 ships the full feature set above. Beyond it:

- **More storage backends** — Google Cloud Storage and Azure Blob, on the existing pluggable `storage.Store` seam.
- **More engines** — the engine-agnostic `Driver` interface leaves room for SQLite, MongoDB, SQL Server, and ClickHouse.

Ideas and requests are welcome — [open an issue](https://github.com/nixrajput/siphon/issues).

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md), keep changes within the layered architecture (depguard will tell you if you stray), and make sure `make test` and `make lint` pass before opening a PR.

Adding a new database engine? See [docs/DRIVERS.md](docs/DRIVERS.md) for the driver contributor guide.

Concept docs: [docs/INCREMENTAL.md](docs/INCREMENTAL.md) (incremental backup + restore), [docs/CROSS_ENGINE.md](docs/CROSS_ENGINE.md) (cross-engine sync + the type-map matrix), and [docs/CDC.md](docs/CDC.md) (continuous CDC sync, same- and cross-engine). All three work end-to-end; live DB behavior is integration-tested in CI.

## Support

siphon is free and open source. If it saves you time, you can support its continued development — every bit helps and is genuinely appreciated. ❤️

<div align="center">

[![Sponsor on GitHub](https://img.shields.io/badge/Sponsor_on_GitHub-%23EA4AAA.svg?style=for-the-badge&logo=githubsponsors&logoColor=white)](https://github.com/sponsors/nixrajput)
[![Ko-fi](https://img.shields.io/badge/Support_on_Ko--fi-FF5E5B?style=for-the-badge&logo=ko-fi&logoColor=white)](https://ko-fi.com/nixrajput)
[![Buy Me A Coffee](https://img.shields.io/badge/Buy_Me_A_Coffee-FFDD00?style=for-the-badge&logo=buy-me-a-coffee&logoColor=black)](https://www.buymeacoffee.com/nixrajput)
[![Open Collective](https://img.shields.io/badge/Open_Collective-7FADF2?style=for-the-badge&logo=opencollective&logoColor=white)](https://opencollective.com/nixrajput)

</div>

## License

[MIT](LICENSE) © [Nikhil Rajput](https://github.com/nixrajput)
