#!/usr/bin/env bash
# pre-commit hook: 对根模块和 tools/ 子模块分别运行 golangci-lint。
#
# 设计要点：
# 1. go.work 多模块结构下，根目录的 `golangci-lint run ./...` 不会覆盖 tools/ 子模块，
#    必须分别执行（与 Makefile lint target、CI matrix 行为对齐）。
# 2. 跨机器兼容：优先使用 $(go env GOPATH)/bin/golangci-lint（make install-lint 的安装位置），
#    缺失则回退 PATH 中的 golangci-lint。
# 3. 未安装时只警告不阻断提交（避免阻塞新贡献者），并提示 `make install-lint`。
# 4. 根模块失败立即退出（短路），与 CI matrix 行为一致。
set -euo pipefail

# 解析 golangci-lint 路径
GOPATH_BIN="$(go env GOPATH)/bin/golangci-lint"
if [ -x "$GOPATH_BIN" ]; then
    LINT="$GOPATH_BIN"
elif command -v golangci-lint >/dev/null 2>&1; then
    LINT="$(command -v golangci-lint)"
else
    echo "[golangci-lint] 未安装，跳过 lint 检查（建议运行：make install-lint）" >&2
    exit 0
fi

# 根模块（须在仓库根目录调用，pre-commit 默认 cwd 即为仓库根）
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

"$LINT" run ./...
echo "---"
cd tools && "$LINT" run ./...
