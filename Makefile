# Mady Makefile
GO ?= go
GOFLAGS ?=
GOPATH ?= $(shell $(GO) env GOPATH)
BINDIR ?= ./build
# install target 的安装前缀。默认 ~/.local（匹配 ~/.local/bin 常见 PATH 布局）。
# 覆盖示例：make install PREFIX=/usr/local
PREFIX ?= $(HOME)/.local
COMMIT_HASH ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -ldflags "-s -w -X main.commitHash=$(COMMIT_HASH) -X main.buildTime=$(BUILD_TIME)"
GOLANGCI_LINT_VERSION ?= v2.12.2

.PHONY: all build test test-race test-short test-integration test-verbose test-disclosure-smoke test-approval-audit test-dry-run-gate coverage vet lint fmt clean \
        install install-hooks install-lint \
        build-cli-chat build-wiki-import build-acp-server build-mady \
        run-cli-chat run-server run-tui-demo run-a2a-server run-a2a-client run-mady run-acp-server \
        eval eval-race \
        help

# Default target
# 覆盖根模块 + tools 子模块（go.work 多模块结构）。
# 注：单独 `go build/test/vet ./...` 在根目录执行时不会覆盖 tools/ 子模块，
# 这里通过显式两段调用来保证一致性（CI 的 matrix 也覆盖了相同路径）。
all: vet build test

# "提交前真实标准"——比 all 更完整：包含 lint（golangci-lint）与 test-race（竞态检测）。
# 提交前请运行 make verify 而非 make all，以确保门禁完整闭合。
verify: lint check-arch build test-race

# TOOLS_BUILD_DIR 用于在 tools 子模块执行命令时切换工作目录。
# 所有 `cd tools && go ...` 调用都使用 `$(GO)` 与 `$(GOFLAGS)`，与根模块保持一致。

# --- Build ---
build:
	$(GO) build $(GOFLAGS) ./...
	cd tools && $(GO) build $(GOFLAGS) ./...

build-release:
	$(GO) build $(GOFLAGS) $(LDFLAGS) ./...
	cd tools && $(GO) build $(GOFLAGS) $(LDFLAGS) ./...

build-cli-chat:
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINDIR)/cli-chat ./example/cli-chat/

build-wiki-import:
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINDIR)/wiki-import ./example/wiki-import/

build-acp-server:
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINDIR)/mady-acp ./example/acp-server/

build-mady:
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINDIR)/mady ./cmd/mady/

# --- Test ---
# 所有 test target 都同时跑根模块和 tools 子模块，避免本地与 CI 不一致。
# 失败时使用 `&&` 短路：根模块失败则不再跑 tools（与 matrix CI 行为一致）。
test:
	$(GO) test $(GOFLAGS) -count=1 ./...
	cd tools && $(GO) test $(GOFLAGS) -count=1 ./...

test-race:
	$(GO) test $(GOFLAGS) -race -count=1 ./...
	cd tools && $(GO) test $(GOFLAGS) -race -count=1 ./...

test-short:
	$(GO) test $(GOFLAGS) -short -count=1 ./...
	cd tools && $(GO) test $(GOFLAGS) -short -count=1 ./...

test-integration:
	$(GO) test $(GOFLAGS) -tags integration -count=1 ./integration/...

test-verbose:
	$(GO) test $(GOFLAGS) -v -count=1 ./...
	cd tools && $(GO) test $(GOFLAGS) -v -count=1 ./...

# disclosure smoke 验证最小 happy path：analyze -> awaiting_review -> review -> export
test-disclosure-smoke:
	$(GO) test $(GOFLAGS) -count=1 -run TestDisclosureHappyPathSmoke ./server

# approval audit 验证 TUI / Server / ACP 三条人工决策留痕入口的记录语义一致
test-approval-audit:
	$(GO) test $(GOFLAGS) -count=1 ./domains ./server ./acp ./cmd/mady

# disclosure 内部 dry-run gate：主路径 happy path + 留痕一致性
test-dry-run-gate: test-disclosure-smoke test-approval-audit

# --- Eval Suite (CI Gate) ---
# eval runs the benchmark test suite under evaluate/benchmark.
# Run this before merging Prompt/Rule/Skill changes.
eval:
	$(GO) test $(GOFLAGS) -v ./evaluate/benchmark/...

eval-race:
	$(GO) test $(GOFLAGS) -race -v ./evaluate/benchmark/...

# --- Knowledge Benchmarks ---
# bench-knowledge runs the full knowledge system benchmark suite with
# vector search enabled. Results are saved to bench-knowledge.txt.
bench-knowledge:
	OMLX_API_KEY=$${OMLX_API_KEY:?error: OMLX_API_KEY not set} KNOWLEDGE_RERANK=on \
	$(GO) test -bench=. -benchmem -count=1 ./knowledge/... 2>&1 | tee bench-knowledge.txt

# --- Coverage ---
# coverage 仅生成根模块覆盖率（与 CI 的 codecov 上传路径对齐）。
# 如需 tools 覆盖率，单独执行 `cd tools && go test -coverprofile=tools.coverage.out ./...`
coverage:
	$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo ""
	@$(GO) tool cover -func=coverage.out | tail -1

coverage-check:
	$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out | tail -1

# --- Architecture Boundary Check ---
.PHONY: check-arch
check-arch:
	@echo "=== Running architecture boundary checks ==="
	@if command -v go-arch-lint >/dev/null 2>&1; then \
		go-arch-lint check; \
		echo "✓ go-arch-lint passed"; \
	else \
		echo "go-arch-lint not installed, falling back to script check..."; \
		scripts/check-arch-boundaries.sh; \
	fi

# --- Lint ---
vet:
	$(GO) vet $(GOFLAGS) ./...
	cd tools && $(GO) vet $(GOFLAGS) ./...

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

# install 把 mady 二进制安装到 $(PREFIX)/bin，使其在任意目录可用。
# manifest 已通过 go:embed 内置，无需额外拷贝资源文件。
# 默认安装到 ~/.local/bin（通常已在 PATH）；如需系统级安装用 PREFIX=/usr/local。
install: build-mady
	@mkdir -p $(PREFIX)/bin
	cp $(BINDIR)/mady $(PREFIX)/bin/mady
	@echo "已安装 mady 到 $(PREFIX)/bin/mady"
	@echo "请确认该目录在 PATH 上（echo $$PATH | grep -q $(PREFIX)/bin && echo OK || echo '提示: $(PREFIX)/bin 不在 PATH 上')"

install-lint:
	@echo "Installing golangci-lint..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin $(GOLANGCI_LINT_VERSION)

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

run-mady:
	$(GO) run ./cmd/mady/ tui

run-acp-server:
	$(GO) run ./example/acp-server/

# --- Help ---
help:
	@echo "Mady Makefile"
	@echo "============="
	@echo ""
	@echo "Note: all build/test/vet targets cover BOTH the root module"
	@echo "      and the ./tools sub-module (go.work multi-module workspace)."
	@echo ""
	@echo "Build:"
	@echo "  build              Build all packages"
	@echo "  build-release      Build with version info (commit hash + build time)"
	@echo "  build-cli-chat     Build cli-chat binary"
	@echo "  build-wiki-import  Build wiki-import binary"
	@echo "  build-acp-server   Build ACP server binary"
	@echo "  build-mady         Build unified mady binary (tui/acp)"
	@echo ""
	@echo "Test:"
	@echo "  test               Run all tests"
	@echo "  test-race          Run tests with race detector"
	@echo "  test-short         Run tests in short mode"
	@echo "  test-integration   Run integration e2e tests (build tag: integration)"
	@echo "  test-verbose       Run tests with verbose output"
	@echo "  test-disclosure-smoke  Run the disclosure happy-path smoke test"
	@echo "  test-approval-audit   Run approval-record consistency tests"
	@echo "  test-dry-run-gate    Run the disclosure internal dry-run gate"
	@echo ""
	@echo "Eval:"
	@echo "  eval               Run Golden Benchmark CI gate (metric chain + case integrity)"
	@echo "  eval-race          Same with race detector"
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
	@echo "  run-mady           Run unified mady (tui mode)"
	@echo "  run-acp-server     Run ACP server example"
	@echo ""
	@echo "Setup:"
	@echo "  install            Install mady to $(PREFIX)/bin (use PREFIX=/usr/local for system-wide)"
	@echo "  install-lint       Install golangci-lint"
	@echo "  install-hooks      Install pre-commit git hooks"
	@echo ""
	@echo "Other:"
	@echo "  clean              Remove build artifacts"
	@echo "  all                Run vet + build + test (default)"
	@echo "  verify             Run lint + build + test-race (pre-commit standard)"
