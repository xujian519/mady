#!/bin/bash
# verify-workers.sh — 验证 Worker 注册完整性
# 检查所有在默认 Catalog 中定义的 Worker 是否能在 Registry 中找到。
set -euo pipefail

echo "=== Worker 注册完整性检查 ==="

# 使用 go run 运行一个临时的验证程序
cd "$(git rev-parse --show-toplevel 2>/dev/null || echo "${0%/*}/..")"

tmpfile=$(mktemp)
trap 'rm -f "$tmpfile"' EXIT

cat > "$tmpfile" <<'GOEOF'
package main

import (
	"fmt"
	"os"

	"github.com/xujian519/mady/agentcore/worker"
)

func main() {
	catalog := worker.NewCatalog()
	for _, d := range worker.DefaultWorkers() {
		if err := catalog.Register(d); err != nil {
			fmt.Fprintf(os.Stderr, "❌ 注册失败: %v\n", err)
			os.Exit(1)
		}
	}

	// 验证 Catalog 完整性
	issues := catalog.Verify()
	for _, iss := range issues {
		fmt.Fprintf(os.Stderr, "⚠️  验证问题: %s\n", iss)
	}
	if len(issues) > 0 {
		os.Exit(1)
	}

	// 验证 Registry 完整性
	registry := worker.NewRegistry()
	skipped := worker.RegisterDefaultWorkers(registry, catalog)

	fmt.Printf("✅ 全部 %d 个 Worker 注册验证通过\n", registry.Count())
	for _, d := range registry.List() {
		fmt.Printf("  - [%s] %s\n", string(d.Tier), d.Name)
	}
	if len(skipped) > 0 {
		fmt.Printf("\n⚠️  惰性注册（跳过）: %v\n", skipped)
	}
}
GOEOF

go run "$tmpfile"
echo "=== 验证完成 ==="
