# Mady Go 规范审阅报告

> 审阅范围：全仓库 670+ Go 源文件
> 对应规范：`docs/GO-DEVELOPMENT-STANDARDS.md`
> 审阅方法：自动化工具（go vet/golangci-lint）+ 深度代码审查 agent + 人工审计
> 审阅日期：2026-07-15

---

## 基线状态

| 检查项 | 结果 |
|--------|------|
| `go build ./...` | ✅ 通过 |
| `go vet ./...` | ✅ 通过（0 issues） |
| `golangci-lint run ./...` | ✅ 通过（0 issues） |
| `go test -race ./...` | ✅ 通过（基线） |
| gofmt 格式化 | ✅ golangci-lint 已启用 gofmt 检查 |

**结论**：自动化工具层（go vet + golangci-lint + gofmt）已无问题，基础代码质量和格式化良好。

---

## 按优先级汇总

### 🔴 P0 — 必须修复

| # | 类别 | 文件 | 问题 | 来源 |
|---|------|------|------|------|
| P0-1 | test | `protocol/jsonrpc/` | 零测试覆盖，整个包无任何测试 | Agent3 |
| P0-2 | test | `workflows/` | 零测试覆盖，工作流编排逻辑无测试 | Agent3 |
| P0-3 | test | `domains/reasoning/collector/` | 7个源文件，零测试覆盖 | Agent3 |
| P0-4 | test | `integration/*_test.go` | 使用 `package integration`（内部包）而非 `package integration_test`（外部包），违反黑盒测试边界 | Agent3 |
| P0-5 | concurrency | `server/disclosure.go:109` | 任务执行 goroutine 缺少 panic recovery，panic 时 `doneCh` 永远不关闭导致调用方永久阻塞 | Agent2 |
| P0-6 | concurrency | `tools/browser_session.go:210` | 清理 ticker goroutine 无法停止（无 stopCh/context），Manager 被丢弃时 goroutine 泄漏 | Agent2 |
| P0-7 | error | `tools/browser_lightpanda.go:303` | `fmt.Errorf` 使用 `%v` 包装错误，应使用 `%w` 以支持 `errors.Is/As` | Agent2 |
| P0-8 | error | 19 个文件 | `json.Marshal(xxx)` 错误被 `_` 丢弃 | Agent2 |
| P0-9 | error | `server/server.go:795`, `agui/handler.go:220` | `json.NewEncoder(w).Encode(v)` 错误被丢弃，HTTP 响应写入不可见 | Agent2 |
| P0-10 | error | `tools/execute_code_ptc.go:160` | `conn.Write(b)` 错误被丢弃 | Agent2 |
| P0-11 | error | `agentcore/errors.go` | `NewRetryableError`, `NewFatalError`, `NewHandoffError`, `NewGuardrailError` 零使用率，配套 `IsXxx` 函数成为死代码 | Agent2 |
| P0-12 | style | `tools/browser.go:17` | 可变全局指针 `defaultBrowserManager`，非线程安全惰性初始化 | Agent1 |
| P0-13 | style | `tools/browser_advanced.go:19` | 全局可变 `sync.Map` `globalBrowserManagers`，违反禁止全局状态规范 | Agent1 |
| P0-14 | style | `tools/browser_supervisor.go:503` | 全局 `globalSupervisorRegistry`，所有浏览器管理器共用一个可变注册表 | Agent1 |

### 🟡 P1 — 建议在本轮修复

| # | 类别 | 文件 | 问题 |
|---|------|------|------|
| P1-1 | test | `cmd/mady/` | 主入口零测试（测试文件 0 个 vs 1 个源文件） |
| P1-2 | test | `guardrails/guardian/` | 测试覆盖率低（6 源文件 vs 1 测试文件 = 17%） |
| P1-3 | test | `mcp/` | 测试覆盖率低（12 源文件 vs 3 测试文件 = 25%） |
| P1-4 | test | `acp/` | 测试覆盖率低（5 源文件 vs 1 测试文件 = 20%） |
| P1-5 | test | 16 个文件共 30 处 | 测试中使用 `time.Sleep`，导致脆弱测试 | Agent3 |
| P1-6 | error | `tools/browser_advanced.go:46` | 非 main 包中使用 `os.Exit(0)` | Agent2 |
| P1-7 | concurrency | `mcp/discovery.go:622-676` | 6 个异步刷新 goroutine 无 panic recovery | Agent2 |
| P1-8 | concurrency | `mcp/tools_refresh.go:120` | 刷新循环 goroutine 无 panic recovery | Agent2 |
| P1-9 | concurrency | `tui/theme/watch.go:19` | 主题文件监听 goroutine 无 panic recovery | Agent2 |
| P1-10 | concurrency | `a2a/server.go:226` | 清理 goroutine 无 panic recovery | Agent2 |
| P1-11 | concurrency | `acp/server.go:645` | Agent.Run goroutine 无 panic recovery | Agent2 |
| P1-12 | concurrency | `tools/browser_session.go:210` | 清理 goroutine 无 panic recovery | Agent2 |
| P1-13 | context | ~25 处（agentcore/domain/mcp/知识库） | 使用 `context.Background()` 替代传播的父 context | Agent2 |

### 🟢 P2 — 长期改进

| # | 类别 | 文件 | 问题 |
|---|------|------|------|
| P2-1 | style | `tools/computer_use.go`(2553行) 等 13 个文件 | 文件超过 500 行，需标记拆分为 TODO |
| P2-2 | style | 14 处 | `time.After` 使用，select 中会导致 goroutine 泄漏（部分已在 WP7 修复） |
| P2-3 | style | ~4 处 | `interface{}` 未迁移到 `any`（注释/JSON 场景） |
| P2-4 | docs | 22 个模块 | 缺少 doc.go 包级文档（含 `agentcore/`、`mcp/`、`a2a/` 等核心模块） |
| P2-5 | test | 低表格驱动测试采用率 | 仅 28% 测试文件使用表格驱动模式 |
| P2-6 | arch | `domains/router.go:294` | `RouterConfigWithRegistry` 白名单 `["mady-router"]` 与其他函数 `["mady-router","chat-agent"]` 不一致（低风险） |
| P2-7 | arch | `tools/bash.go:304` | 10 分钟延迟清理 temp file 的 goroutine 使用 `time.Sleep`，重启时泄漏 |
| P2-8 | arch | ~15 个接口方法 | 缺少 `context.Context` 参数（`tools/vision.go`、`tools/git.go`、`tools/patch.go` 等） |

---

## 各模块健康度评估

| 模块 | 测试覆盖 | 错误处理 | 文档 | 并发安全 | 整体 |
|------|---------|---------|------|---------|------|
| `agentcore/` | 🟢 71% | 🟢 良好 | ⚠️ 无 doc.go | 🟢 良好 | 🟢 |
| `tools/` | ⚠️ 39% | ⚠️ 缺失结构化错误 | 🟢 有 doc.go | ⚠️ 多个 goroutine 缺 recover | ⚠️ |
| `domains/` | 🟢 良好 | 🟢 良好 | 🟢 有 doc.go | 🟢 良好 | 🟢 |
| `mcp/` | ⚠️ 25% | ⚠️ 裸 fmt.Errorf | ⚠️ 无 doc.go | ⚠️ 7 个 goroutine 缺 recover | 🔴 |
| `a2a/` | ⚠️ 50% | ⚠️ 裸 fmt.Errorf | ⚠️ 无 doc.go | ⚠️ 1 个 goroutine 缺 recover | ⚠️ |
| `acp/` | ⚠️ 20% | ⚠️ 裸 fmt.Errorf | ⚠️ 无 doc.go | ⚠️ 1 个 goroutine 缺 panic recover | ⚠️ |
| `server/` | ⚠️ 25% | ⚠️ 裸 fmt.Errorf | 🟢 有 doc.go | 🔴 `disclosure.go` 缺 recover | 🔴 |
| `guardrails/` | ⚠️ 17%(guardian) | ⚠️ 裸 fmt.Errorf | 🟢 有 doc.go | 🟢 良好 | ⚠️ |
| `knowledge/` | ⚠️ 30-50% | ⚠️ 部分裸错误 | 🟢 有 doc.go | 🟢 良好 | ⚠️ |
| `tui/` | ⚠️ 20-37% | Info | 🔴 无 doc.go | 🟢 良好 | ⚠️ |
| `graph/` | 🟢 良好 | 🟢 使用 NodeError | ⚠️ 无 doc.go | 🟢 有 recover | 🟢 |
| `cmd/mady/` | 🔴 零测试 | — | ⚠️ 无 doc.go | ⚠️ | 🔴 |
| `workflows/` | 🔴 零测试 | ⚠️ 裸 fmt.Errorf | 🟢 有 doc.go | ⚠️ | 🔴 |
| `protocol/jsonrpc/` | 🔴 零测试 | — | ⚠️ 无 doc.go | 🟢 | 🔴 |

---

## 推荐修复批次

### 批次 1：P0 并发与错误修复（高优先级）
- `server/disclosure.go:109` — 添加 panic recovery
- `tools/browser_session.go:210` — 添加 stopCh 和 recover
- `tools/browser_lightpanda.go:303` — `%v` → `%w`
- 19 处 `json.Marshal` 错误检查添加
- `server/server.go:795` + `agui/handler.go:220` Encode 错误修复
- `tools/execute_code_ptc.go:160` conn.Write 错误检查

### 批次 2：P0 测试覆盖（高优先级）
- 为 `protocol/jsonrpc/` 编写基础测试
- 为 `workflows/` 编写基础测试
- 为 `domains/reasoning/collector/` 编写基础测试
- 修复 `integration/` 包签名为 `package integration_test`
- 替换 16 个文件中的 30 处 `time.Sleep`

### 批次 3：P1 并发 safety net 补充
- `mcp/discovery.go` 6 个 goroutine 添加 recover
- `mcp/tools_refresh.go` 添加 recover
- `tui/theme/watch.go` 添加 recover
- `a2a/server.go:226` 添加 recover
- `acp/server.go:645` 添加 recover
- `tools/browser_advanced.go:46` 移除 `os.Exit(0)`

### 批次 4：P1-P2 长期改进
- 结构化错误使用推广（tools/mcp/knowledge/disclosure/workflows/guardrails/domains/rules）
- `doc.go` 补充（22 个模块）
- `time.After` → `time.NewTimer` 替换
- context.Background() → 传播父 context
- 接口方法添加 context 参数
- `interface{}` → `any` 迁移
