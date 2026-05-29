# Contributing to siphon

Thanks for your interest. This project is in active early development; the design spec at `docs/superpowers/specs/2026-05-28-siphon-v1-design.md` is the source of truth.

## Development setup

```bash
make tidy
make test
make lint
make build
```

## Layer rules (enforced by `golangci-lint`'s `depguard`)

- `cmd/` may only depend on `internal/cli`.
- `internal/cli` and `internal/tui` are siblings; neither may import the other (except the root command, which launches the TUI on bare invocation).
- `internal/cli` and `internal/tui` may only depend on `internal/app` (and shared `internal/errs`, `internal/config`).
- `internal/app` may depend on `internal/driver`, `internal/config`, `internal/secrets`, `internal/dumps`, `internal/jobs`, `internal/errs`.
- `internal/driver` packages may not import anything from `cli`, `tui`, or `app`.

Upward imports fail CI.

## Tests

- Unit tests: colocated `*_test.go`, run via `make test`.
- Integration tests: behind `//go:build integration`, run via `make test-integration`.
- TUI snapshot tests: under `internal/tui/testdata/` (Phase C onward).

## Commits

Follow Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`, `ci:`, `test:`, `refactor:`, `build:`).
