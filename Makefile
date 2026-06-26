SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

lint: ## Run golangci-lint
	golangci-lint run

# NOTE: `go test ./...` skips directories whose names start with `_` (Go tool
# convention), so underscore-prefixed packages with real tests (e.g.
# internal/driver/_mysqlcommon) are silently excluded. Name them explicitly by
# import path so their unit tests run. Use the import path (not a `_*` shell
# glob) so this is identical to what CI runs and works regardless of shell.
UNDERSCORE_PKGS := github.com/nixrajput/siphon/internal/driver/_mysqlcommon

test: ## Run unit tests
	go test ./... $(UNDERSCORE_PKGS)

test-verbose: ## Run unit tests verbosely
	go test -v ./... $(UNDERSCORE_PKGS)

test-integration: ## Run integration tests (build tag: integration)
	go test -tags=integration ./... $(UNDERSCORE_PKGS)

build: ## Build the siphon binary into ./bin
	@mkdir -p bin
	go build -ldflags "-s -w -X github.com/nixrajput/siphon/internal/cli.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o bin/siphon ./cmd/siphon

run: ## Run siphon from source (bare invocation -> TUI)
	go run ./cmd/siphon

install: ## Install siphon to GOBIN
	go install ./cmd/siphon

tidy: ## Run go mod tidy
	go mod tidy

clean: ## Remove build artifacts
	rm -rf bin/ dist/ coverage.out coverage.html

hooks: ## Enable the committed git hooks (one-time, per clone)
	git config core.hooksPath .githooks
	@echo "git hooks enabled (core.hooksPath=.githooks)"

web-lint: ## Lint the web/ app (ESLint)
	cd web && npm run lint

web-format: ## Format the web/ app (Prettier)
	cd web && npm run format

.PHONY: help lint test test-verbose test-integration build run install tidy clean hooks web-lint web-format
