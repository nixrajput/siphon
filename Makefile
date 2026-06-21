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
# internal/driver/_mysqlcommon) are silently excluded. Name them explicitly via
# the `_*` glob so their unit tests actually run. The glob auto-picks up any
# future _*-prefixed package under internal/driver/.
test: ## Run unit tests
	go test ./... ./internal/driver/_*/

test-verbose: ## Run unit tests verbosely
	go test -v ./... ./internal/driver/_*/

test-integration: ## Run integration tests (build tag: integration)
	go test -tags=integration ./... ./internal/driver/_*/

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

.PHONY: help lint test test-verbose test-integration build run install tidy clean
