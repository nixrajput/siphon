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
