# Mady Makefile
GO ?= go
GOFLAGS ?=
GOPATH ?= $(shell $(GO) env GOPATH)
BINDIR ?= ./build
COMMIT_HASH ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -ldflags "-s -w -X main.commitHash=$(COMMIT_HASH) -X main.buildTime=$(BUILD_TIME)"

.PHONY: all build test test-race test-short coverage vet lint fmt clean \
        install-hooks install-lint \
        build-cli-chat build-wiki-import \
        run-cli-chat run-server run-tui-demo run-a2a-server run-a2a-client \
        help

# Default target
all: vet build test

# --- Build ---
build:
	$(GO) build $(GOFLAGS) ./...

build-release:
	$(GO) build $(GOFLAGS) $(LDFLAGS) ./...

build-cli-chat:
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINDIR)/cli-chat ./example/cli-chat/

build-wiki-import:
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINDIR)/wiki-import ./example/wiki-import/

# --- Test ---
test:
	$(GO) test $(GOFLAGS) -count=1 ./...

test-race:
	$(GO) test $(GOFLAGS) -race -count=1 ./...

test-short:
	$(GO) test $(GOFLAGS) -short -count=1 ./...

test-verbose:
	$(GO) test $(GOFLAGS) -v -count=1 ./...

# --- Coverage ---
coverage:
	$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo ""
	@$(GO) tool cover -func=coverage.out | tail -1

coverage-check:
	$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out | tail -1

# --- Lint ---
vet:
	$(GO) vet $(GOFLAGS) ./...

lint: vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
		echo "---"; \
		cd tools && golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run: make install-lint"; \
	fi

# --- Format ---
fmt:
	$(GO) fmt ./...
	cd tools && $(GO) fmt ./...

# --- Clean ---
clean:
	rm -rf $(BINDIR) coverage.out coverage.html

# --- Tools Installation ---
install-lint:
	@echo "Installing golangci-lint..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin v1.64.8

install-hooks:
	@if command -v pre-commit >/dev/null 2>&1; then \
		pre-commit install; \
		pre-commit install --hook-type commit-msg; \
		echo "Pre-commit hooks installed."; \
	else \
		echo "pre-commit not installed. Install it first: pip install pre-commit"; \
	fi

# --- Run Examples ---
run-cli-chat:
	$(GO) run ./example/cli-chat/

run-server:
	$(GO) run ./example/a2a-server/

run-a2a-client:
	$(GO) run ./example/a2a-client/

run-tui-demo:
	$(GO) run ./example/tui-demo/

# --- Help ---
help:
	@echo "Mady Makefile"
	@echo "============="
	@echo ""
	@echo "Build:"
	@echo "  build              Build all packages"
	@echo "  build-release      Build with version info (commit hash + build time)"
	@echo "  build-cli-chat     Build cli-chat binary"
	@echo "  build-wiki-import  Build wiki-import binary"
	@echo ""
	@echo "Test:"
	@echo "  test               Run all tests"
	@echo "  test-race          Run tests with race detector"
	@echo "  test-short         Run tests in short mode"
	@echo "  test-verbose       Run tests with verbose output"
	@echo ""
	@echo "Quality:"
	@echo "  vet                Run go vet"
	@echo "  lint               Run golangci-lint (if installed)"
	@echo "  fmt                Format all source files"
	@echo "  coverage           Generate coverage report (coverage.html)"
	@echo ""
	@echo "Run:"
	@echo "  run-cli-chat       Run CLI chat application"
	@echo "  run-server         Run A2A server example"
	@echo "  run-a2a-client     Run A2A client example"
	@echo "  run-tui-demo       Run TUI demo"
	@echo ""
	@echo "Setup:"
	@echo "  install-lint       Install golangci-lint"
	@echo "  install-hooks      Install pre-commit git hooks"
	@echo ""
	@echo "Other:"
	@echo "  clean              Remove build artifacts"
	@echo "  all                Run vet + build + test (default)"
