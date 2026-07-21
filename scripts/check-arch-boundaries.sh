#!/bin/bash
# 架构边界快速检查（用于 pre-commit 钩子，零依赖）
# 与 .go-arch-lint.yml 保持同步，提供同样的约束但无需安装 go-arch-lint。
# 以 go-arch-lint 为权威源，本脚本作为快速预检补充。
set -euo pipefail

ERRORS=0
PASS=0

check_no_import() {
    local pkg="$1" forbidden="$2" label="${3:-$pkg → $forbidden}"
    local imports
    imports=$(go list -f '{{join .Imports "\n"}}' "$pkg" 2>/dev/null || true)
    if echo "$imports" | grep -qF "$forbidden"; then
        echo "FAIL: $label  —  $pkg 导入了 $forbidden"
        ERRORS=$((ERRORS + 1))
    else
        echo "PASS: $label"
        PASS=$((PASS + 1))
    fi
}

echo "=== 架构边界检查 ==="
echo ""

# agentcore 层
check_no_import ./agentcore "github.com/xujian519/mady/domains" "agentcore → domains"
check_no_import ./agentcore "github.com/xujian519/mady/server" "agentcore → server"

# 基础设施层
check_no_import ./graph "github.com/xujian519/mady/agentcore" "graph → agentcore"
check_no_import ./knowledge "github.com/xujian519/mady/server" "knowledge → server"
check_no_import ./knowledge "github.com/xujian519/mady/tui" "knowledge → tui"
check_no_import ./memory "github.com/xujian519/mady/server" "memory → server"
check_no_import ./retrieval "github.com/xujian519/mady/server" "retrieval → server"
check_no_import ./retrieval "github.com/xujian519/mady/domains" "retrieval → domains"

# TUI 层
check_no_import ./tui/chat "github.com/xujian519/mady/agentcore" "tui/chat → agentcore"

# server 层
check_no_import ./server "github.com/xujian519/mady/tui" "server → tui"
check_no_import ./server "github.com/xujian519/mady/tools" "server → tools"

# provider 层
check_no_import ./provider "github.com/xujian519/mady/domains" "provider → domains"

# disclosure 层
check_no_import ./disclosure "github.com/xujian519/mady/tui" "disclosure → tui"

# tools 子模块
check_no_import ./tools "github.com/xujian519/mady/domains" "tools → domains"

echo ""
echo "=== 结果: $PASS 通过, $ERRORS 失败 ==="

if [ "$ERRORS" -gt 0 ]; then
    echo "错误: 发现 $ERRORS 个架构边界违规"
    exit 1
fi
