# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Phase A skeleton: Go module, Cobra CLI with placeholder subcommands, Bubble Tea TUI placeholder, Driver interface + registry, errs package, config stub, Makefile, golangci-lint with depguard, CI workflow.
- Phase B Postgres walking skeleton: `backup`, `restore`, `sync`, `verify`, `inspect`, `dumps`, and `profile` working end-to-end against PostgreSQL (shelling out to `pg_dump`/`pg_restore`, pgx for inspect), SHA-256 dump checksums with sidecar metadata, named profiles with `env:` secret refs, and POSIX exit-code taxonomy.
- Phase C interactive TUI dashboard: multi-panel Bubble Tea UI (profiles · dumps · jobs) with live job-progress subscription, backup/restore modal forms, an error overlay, and golden snapshot tests.
- Phase D driver-layer hardening:
  - Shared cross-driver test harness `RunDriverSuite` under `internal/driver/_testing/`, giving every driver four contract tests for free (connect/inspect, backup→restore round-trip, cancel propagation, bad-credentials sentinel). The Postgres integration test now runs on it.
  - Capability gating helper: `app.RequireCapability` resolves a profile to its driver and rejects unsupported affordances with a `CodeUser` error. The helper and the `Capabilities` flags are in place; verbs are wired to it only where the gated feature is implemented. Streaming (`NativeStream`) and parallel backup (`Parallel`) are deferred to Phase F, so `sync --stream` / `backup --jobs` are not yet gated — the wiring lands with those features.
  - Connection-probe retry on Postgres `Connect` (3 attempts, exponential backoff) per spec §4.3; the generic `Retry` helper moved to `internal/jobs` to keep the driver layer free of an app-layer import.
  - `docs/DRIVERS.md` contributor guide documenting the driver contract, registration, the test harness, capability flags, and error mapping.
- Phase E MySQL + MariaDB drivers:
  - Shared `internal/driver/_mysqlcommon` package: DSN builder, `mysqldump`/`mariadb-dump` arg builder, fork detection, and a shared `Conn` implementing the full `driver.Conn` contract (Inspect via `information_schema`, Backup by shelling to the dump tool, Restore by piping SQL into the client, sha256 Verify, bounded connect-probe retry). Backup/Restore pass the password via `MYSQL_PWD` and stream through stdout/stdin.
  - `mysql` and `mariadb` drivers as thin wrappers that inject the fork-specific binary names and declare capabilities honestly (`Parallel: false` — the dump tools are single-threaded). Registered via side-effect import in `internal/app/drivers.go`.
  - Integration suites for both engines run on the Phase D `RunDriverSuite` harness via testcontainers (`mysql:8.0`, `mariadb:11`).
  - CI installs the Postgres/MySQL/MariaDB client tools and runs the full integration suite on the Ubuntu runner (where Docker is available); unit tests continue to run on Linux/macOS/Windows.
- Phase F advanced-transfer machinery (some CLI paths gated pending follow-up wiring — see the `docs/INCREMENTAL.md`, `docs/CROSS_ENGINE.md`, `docs/CDC.md` Status sections):
  - **Working today:** every dump is prefixed with a 4 KB `SIPH` JSON envelope (type, base/parent IDs, WAL/binlog positions, checksum); `restore` resolves the base→incremental chain and applies it in order, with `--up-to <id>` to stop early (cycle + broken-chain detection). Sync now streams through a bounded `jobs.Stream` (default 64×1 MB) instead of `io.Pipe`, exposing a `FillPercent` backpressure metric and propagating backup failures to the restore side via `CloseErr` (no truncated dump committed as clean).
  - **Incremental backup wired end-to-end (`backup --incremental --base <id>`):** captures a **bounded** change set since the base dump's recorded end position, serialized as engine-neutral JSONL `CanonicalChange` records. Postgres bounds the pgoutput logical-decoding stream by `pg_current_wal_lsn()` captured at backup time; MySQL/MariaDB bound the binlog-tool decode by the current binlog file+offset. The incremental dump's envelope carries this capture's end position so the next incremental resumes exactly there. `restore` replays incremental links via `ApplyChange` (base links still restore natively). Postgres adds an orphan replication-slot sweep (`SweepOrphanSlots`) run before each capture — drops inactive `siphon_*` physical slots while preserving the persistent logical resume slot. `Incremental` capability is now `true` for all three drivers. Live-server behavior is integration-tested in CI (`wal_level=logical`); compile-checked locally.
  - **Machinery in place, CLI gated (follow-up):** cross-engine type-mapping (canonical schema + JSONL emit/consume with per-engine identifier quoting and placeholders) — `sync --cross-engine` is capability-gated off until typed schema introspection exists; CDC continuous mode (state-file persistence + resume + capability gating) ships as a polling scaffold, not real logical-replication streaming.
