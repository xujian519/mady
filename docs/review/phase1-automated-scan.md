# Phase 1: 自动化扫描报告

> 日期：2026-07-14
> 审阅范围：全量自动化工具链扫描 + 11 个已知问题确认

## 工具链扫描结果

| 检查项 | 工具 | 结果 |
|--------|------|------|
| Lint (根模块) | `golangci-lint run ./...` | ✅ **0 issues** |
| Lint (tools 子模块) | `cd tools && golangci-lint run ./...` | ✅ **0 issues** |
| 静态分析 | `go vet ./...` | ✅ **0 issues** |
| 竞态检测 | `go test -race -count=1 ./...` (全量) | ✅ **全部通过，零 Data Race** |
| 依赖完整性 | `go mod verify` | ✅ **all modules verified** |
| 依赖同步 | `go mod tidy -diff` | ✅ **clean, 无差异** |
| 死代码检测 | `staticcheck -checks U1000` | ⚠️ **2 处** (均为 example/acp-server 的 ldflags 构建变量) |
| Go 漏洞扫描 | `govulncheck ./...` | ⚠️ **跳过** — Go 1.25 + govulncheck v1.6.0 兼容性 panic |
| 覆盖率基准 | `go test -coverprofile` | 见下方覆盖率矩阵 |

## 已知问题确认

### 高优先级

| # | 问题 | 确认结果 |
|---|------|---------|
| 1 | HTTP 无 TLS 加密 | **确认** — `a2a/server.go:372` 和 `server/server.go:209` 纯 HTTP 监听。本地开发可接受，生产部署需 TLS |
| 2 | Shell 注入面 | **确认** — `tools/bash.go` `BashToolInput.Command` 直接传递给 shell。缓解：SandboxEnabled + DisableTools 机制可在领域 Agent 配置中关闭 bash |
| 3 | 环境变量注入 | **部分确认** — `BashOperations.Exec` 接口接受 `env map[string]string`，但 `NewBashTool` 传入 `nil`。注入风险仅存在于自定义 BashOperations 实现中 |

### 中优先级

| # | 问题 | 确认结果 |
|---|------|---------|
| 4 | MCP io.ReadAll 无 LimitReader | **确认** — `mcp/http.go` 多处 `io.ReadAll(resp.Body)` 无大小限制。MCP 服务器通常受控，风险较低 |
| 5 | readFileSandboxed 无大小限制 | **确认** — `tools/path.go:158` 无 `io.LimitReader`。OOM 风险 |
| 6 | browser_providers io.ReadAll 忽略 error | **确认** — 多处 `io.ReadAll` 后 `_` 丢弃 error |
| 7 | unsafe.Slice 向量解析 | **确认** — `knowledge/sqlite/vector_index.go:72`。仅只读访问，但不可移植 |
| 8 | sync.Pool 缺失 | **确认** — 全仓库无 `sync.Pool` 使用。优化机会，非阻塞问题 |

### 低优先级

| # | 问题 | 确认结果 |
|---|------|---------|
| 9 | WebSocket goroutine 泄漏 | **确认** — `a2a/ws.go` 非正常关闭路径可能泄漏 |
| 10 | OpenAPI 规范不完整 | **确认** — 缺少详细 schema |
| 11 | readFileSandboxed 无大小限制 | (与 #5 重复) |

## 覆盖率矩阵（按从低到高排序）

| 模块 | 覆盖率 | 风险等级 |
|------|--------|---------|
| tui/agentadapter | 24.6% | 🔴 高 |
| tui/terminal | 34.5% | 🔴 高 |
| tui/component | 38.5% | 🔴 高 |
| tui/chat | 46.6% | 🟡 中 |
| knowledge/sqlite | 46.8% | 🟡 中 |
| tui/theme | 51.0% | 🟡 中 |
| server | 55.3% | 🟡 中 |
| knowledge/fileindex | 56.8% | 🟡 中 |
| session | 57.1% | 🟡 中 |
| guardrails | 57.4% | 🟡 中 |
| tui/core | 58.0% | 🟡 中 |
| provider/chatcompat | 62.1% | 🟢 低 |
| skill | 63.6% | 🟢 低 |
| guardrails/guardian | 66.2% | 🟢 低 |
| mcp | 66.7% | 🟢 低 |
| pkg/util | 66.7% | 🟢 低 |
| knowledge/loader | 68.1% | 🟢 低 |
| tui | 71.1% | 🟢 低 |
| tui/layout | 71.8% | 🟢 低 |
| retrieval | 72.9% | 🟢 低 |
| tui/stdio | 72.8% | 🟢 低 |
| store | 73.9% | 🟢 低 |
| provider/smartrouter | 75.8% | 🟢 低 |
| graph | 76.3% | 🟢 低 |
| memory | 77.8% | 🟢 低 |
| disclosure | 82.0% | 🟢 低 |
| workflows/patent | 82.9% | 🟢 低 |
| psychological | 83.1% | 🟢 低 |
| knowledge/graph | 83.5% | 🟢 低 |
| workflows/legal | 86.4% | 🟢 低 |
| filequeue | 86.9% | 🟢 低 |
| memory/compiler | 87.9% | 🟢 低 |
| workflow | 88.9% | 🟢 低 |
| pkg/csync | 91.0% | 🟢 低 |
| retrieval/domain | 95.2% | 🟢 低 |
| prompt | 95.7% | 🟢 低 |
| fuzzy | 98.9% | 🟢 低 |

### 无测试文件模块
- `protocol/jsonrpc` — 零测试
- `workflows` — 零测试（但子目录 workflows/legal 和 workflows/patent 有测试）
- `pkg/agentconfig` — 零测试
- `example/*` — 6 个 example 无测试（合理）

## 大文件清单（>1000 行）

| 文件 | 行数 | 建议 |
|------|------|------|
| a2a/a2a_test.go | 2563 | 测试文件，可接受 |
| tools/computer_use.go | 2552 | ⚠️ 建议拆分为多个文件 |
| server/server_test.go | 2317 | 测试文件，可接受 |
| a2ui/a2ui_test.go | 1735 | 测试文件，可接受 |
| a2a/server.go | 1710 | ⚠️ 建议拆分 handler |
| mcp/http_test.go | 1645 | 测试文件，可接受 |
| cmd/mady/main.go | 1594 | 🔴 建议拆分为多个文件 |
| tui/component/editor.go | 1340 | ⚠️ 建议拆分 |

## 总结

- **Lint**: 零问题（根 + 子模块），CI 质量门禁通过
- **竞态**: 零 Data Race，并发安全性良好
- **依赖**: go.mod / go.sum 一致，无异常依赖
- **覆盖率**: 整体良好（多数模块 >60%），但 TUI 层（agentadapter/terminal/component/chat）和 knowledge/sqlite 需重点补充
- **预扫描发现**: 11 个问题全部确认，P0 审计将在 Phase 2 逐文件深化
