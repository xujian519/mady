#!/usr/bin/env bash
# 全量自动扫描入口
# 用法: ./audit/scripts/scan-all.sh [--phase 1-5] [--module root|tools|tui|desktop|all]
# 默认: 全部 phase + 全部 module
set -euo pipefail

PHASE="${1:-all}"
MODULE="${2:-all}"

echo "# Mady 全量自动扫描 — $(date '+%Y-%m-%d %H:%M')"
echo ""

run_build() {
  echo "## go build"
  go build ./... && echo "  PASS: root" || echo "  FAIL: root"
  (cd tools && go build ./... && echo "  PASS: tools" || echo "  FAIL: tools")
  (cd tui && go build ./... && echo "  PASS: tui" || echo "  FAIL: tui")
  (cd desktop && go build ./... && echo "  PASS: desktop" || echo "  FAIL: desktop")
}

run_vet() {
  echo "## go vet"
  go vet ./... && echo "  PASS: root" || echo "  FAIL: root"
  (cd tools && go vet ./... && echo "  PASS: tools" || echo "  FAIL: tools")
  (cd tui && go vet ./... && echo "  PASS: tui" || echo "  FAIL: tui")
  (cd desktop && go vet ./... && echo "  PASS: desktop" || echo "  FAIL: desktop")
}

run_lint() {
  echo "## golangci-lint"
  echo "--- root ---"
  golangci-lint run --timeout 5m ./... 2>&1 | tail -3
  echo "--- tools ---"
  golangci-lint run --timeout 3m ./tools/... 2>&1 | tail -3
  echo "--- tui ---"
  golangci-lint run --timeout 3m ./tui/... 2>&1 | tail -3
  echo "--- desktop ---"
  golangci-lint run --timeout 3m ./desktop/... 2>&1 | tail -3
}

run_pattern_scans() {
  echo "## 专项模式扫描"

  echo "--- D2: dot import ---"
  grep -rn 'import\s\+\.\s"' --include="*.go" . 2>/dev/null | grep -v '.git/' | grep -v 'graphify-out' || echo "  0 violations"

  echo "--- D3: init() panic ---"
  for f in $(grep -rl 'func init()' --include="*.go" . 2>/dev/null | grep -v '.git/' | grep -v 'graphify-out' | grep -v '_test.go'); do
    if grep -A20 'func init()' "$f" | grep -q 'panic('; then
      echo "  PANIC: $f"
    fi
  done
  echo "  scan complete"

  echo "--- D4: anti-pattern dirs ---"
  find . -type d \( -name "common" -o -name "utils" -o -name "base" -o -name "helpers" \) 2>/dev/null | grep -v '.git/' | grep -v 'node_modules' || echo "  0 violations"

  echo "--- D6: error caps ---"
  grep -rn 'fmt\.Errorf.*\"[A-Z]' --include="*.go" . 2>/dev/null | grep -v '_test.go' | grep -v '.git/' | grep -v '"JSON\|"YAML\|"URL\|"HTTP\|"SQL\|"ID\|"TLS\|"SSE\|"IPC\|"OA\|"OK\|"P0\|"MCP\|"API\|"CSV\|"PDF\|"LLM\|"AI\|"UI\|"DB\|"IO\|"OS\|"IP' | wc -l | xargs -I{} echo "  {} potential violations"

  echo "--- D9: Config without Validate ---"
  config_count=$(grep -rn "type.*Config\s*struct" --include="*.go" . | grep -v '_test.go' | grep -v '.git/' | grep -v 'graphify-out' | wc -l | tr -d ' ')
  validate_count=$(grep -rn "func.*Config.*Validate" --include="*.go" . | grep -v '_test.go' | grep -v '.git/' | grep -v 'graphify-out' | wc -l | tr -d ' ')
  echo "  Config structs: $config_count, with Validate: $validate_count"

  echo "--- oversize files (>500 lines) ---"
  find . -name "*.go" -not -name "*_test.go" -not -path "./.git/*" -not -path "./graphify-out/*" -not -path "./vendor/*" -exec wc -l {} \; 2>/dev/null | sort -rn | awk '$1 > 500 {print}' | wc -l | xargs -I{} echo "  {} files >500 lines"
}

case "$PHASE" in
  1|all)
    run_build
    run_vet
    run_lint
    run_pattern_scans
    ;;
  2|all)
    echo "Phase 2+ 需人工抽样，本脚本仅覆盖全自动扫描部分"
    ;;
esac

echo ""
echo "# 扫描完成"
