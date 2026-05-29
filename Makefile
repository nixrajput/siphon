SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

lint: ## Run golangci-lint
	golangci-lint run

test: ## Run unit tests
	go test ./...

test-verbose: ## Run unit tests verbosely
	go test -v ./...

test-integration: ## Run integration tests (build tag: integration)
	go test -tags=integration ./...

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
