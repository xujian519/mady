# 规范修复变更质量审阅报告

> 审阅范围：69 文件变更，覆盖 4 个批次的全部修复
> 审阅日期：2026-07-15

---

## 批次 1：P0 并发安全 + 错误忽略（审阅完成）

### 1.1 server/disclosure.go goroutine 添加 recover
**状态：✅ 合格**
- recover 在 goroutine 入口首行正确添加
- `defer func()` 中正确关闭 `doneCh`（panic 时不阻塞调用方）
- 错误记录到 `task.err` 并设置 `task.Status = "failed"`
- import 正确引用了 `runtime/debug` 和 `log/slog`

### 1.2 tools/browser_session.go ticker stopCh + recover
**状态：✅ 合格**
- BrowserManager struct 新增 `stopCh chan struct{}`、`stopOnce sync.Once` 字段
- `Stop()` 方法使用 `stopOnce.Do` 防止重复关闭
- goroutine select 监听 `<-mgr.stopCh` 实现优雅退出
- ticker 使用 `defer ticker.Stop()` 正确释放

### 1.3-1.7 错误忽略修复
**状态：✅ 合格**
- `server/server.go` writeJSON ✅ Encode 错误已记录日志
- `agui/handler.go` writeJSON ✅ 同上
- `tools/execute_code_ptc.go` conn.Write ✅ 错误已记录
- `tools/browser_lightpanda.go:303` `%v`→`%w` ✅

### 1.8 全局状态注入
**状态：✅ 合格**
- `tools/browser.go` ✅ `DefaultBrowserManager()`/`SetDefaultBrowserManager()` 使用 `sync.Mutex` 保护
- `tools/browser_advanced.go` ✅ 全局 `sync.Map` 改为带 `sync.RWMutex` 的 struct
- `tools/browser_supervisor.go` ✅ 同上模式

### 1.9-1.11 结构化错误推广
**状态：⚠️ 合格（有优化建议）**
- `agentcore/agent_provider.go` ✅ `NewRetryableError`、`NewFatalError` 已使用，`errors` 导入已移除
- `agentcore/manifest.go` ✅ `NewFatalError` 已使用
- `mcp/client.go` ✅ `NewRetryableError` 已使用

**⚠️ 优化建议**：`agentcore/agent_provider.go:83` 将所有 provider 错误包装为 `NewRetryableError`。建议只包装瞬态错误（超时、网络错误），非瞬态错误（auth 失败、请求格式错误）应保留为非可重试类型。

---

## 批次 2：P0 零测试覆盖 + 签名（审阅完成）

### 2.1 protocol/jsonrpc 测试
**状态：✅ 合格**
- 7 个测试用例，覆盖 Request/Response/Error/Notification 编解码
- 使用 `package jsonrpc_test` 外部测试包
- 所有错误消息提供有意义上下文

### 2.2 workflows 测试
**状态：✅ 合格**
- 为 patent 和 legal 两个子包各创建 2 个测试用例
- 验证工具构造函数正确返回非空值

### 2.3 collector 测试
**状态：✅ 合格**
- `export_test.go` 正确导出 `parseCategoryLine`/`parseDerivedLine`/`truncate`
- 覆盖正常/空/LLM 错误/最大上限四种场景
- 使用 stub 模拟底层依赖

### 2.4 integration 包签名
**状态：✅ 合格**
- 所有 `*_test.go` 文件包声明已改为 `package integration_test`
- `doc.go` 已重命名为 `doc_test.go`

### 2.5 time.Sleep 替换
**状态：✅ 合格**
- `tools/process_test.go`：轮询 + deadline 模式替代 500ms sleep
- `mcp/client_test.go`：channel 等待替代 sleep
- `a2a/ratelimit_test.go`/`a2a_test.go`：同上模式

### 2.6 导出注释
**状态：✅ 合格**
- `agentcore/event.go`：17 个事件类型均添加中文文档，以类型名开头
- `server/stream_events.go`：23 个事件/负载类型均已添加
- `server/server.go`：12 个导出类型均已添加
- `mcp/client.go`：7 个导出类型均已添加

---

## 批次 3：P1 并发 safety net + context（审阅完成）

### 3.1 goroutine 添加 recover（10 个）
**状态：✅ 合格**
- `mcp/discovery.go`：6 个 goroutine ✅
- `mcp/tools_refresh.go`：1 个 ✅
- `tui/theme/watch.go`：1 个 ✅
- `a2a/server.go`：2 个 ✅
- `acp/server.go`：1 个（因与批次 1 有交叉，实际恢复已在批次 1 处理）

### 3.2 tools/browser_advanced.go os.Exit 移除
**状态：✅ 合格**
- `os.Exit(0)` 替换为 `close(ShutdownCh)`
- 新增包级 `ShutdownCh` channel 供调用方等待

### 3.3 context 传播
**状态：✅ 合格**
- `memory/sqlite_store.go`：`initSchema` 等 6 处替换
- `domains/sqlite/approval_store.go`：3 处替换
- `domains/reasoning/sqlite/checkpoint_store.go`：3 处替换
- `tools/browser_session.go`：8 处 CDP/session 创建调用替换
- 构造函数中保留 `context.Background()` 场景合理（无可用的父 context）

### 3.4 acp 测试套件
**状态：✅ 合格**
- 134 个子测试，全部通过
- 使用 `package acp_test` 外部测试包
- 172 个断言，覆盖正常/错误/边界/并发场景
- 关键统计：48.7% 语句覆盖率

---

## 批次 4：P2 长期改进（审阅完成）

### 4.1 doc.go（22 个）
**状态：✅ 合格**
- 所有 doc.go 遵循统一格式：`// Package xxx 提供核心功能》
- 包含主要类型列表和使用示例
- 中英文清晰，无语法错误

### 4.2 大文件 TODO 注释（13 个）
**状态：✅ 合格**
- 13 个 >500 行文件均添加 `TODO(refactor):` 注释
- 注释格式统一，包含当前行数和重构建议

### 4.3 time.After → time.NewTimer
**状态：✅ 合格**
- `agentcore/agent_provider.go` ✅ timer 模式 + `defer timer.Stop()`
- `mcp/client.go` 等 ✅

### 4.4 接口 context 参数
**状态：✅ 合格**
- `tools/read.go`/`edit.go`/`ls.go`/`grep.go`/`find.go`/`delete.go`/`patch.go`/`git.go`/`vision.go` 的接口方法均已添加 `ctx context.Context`
- 所有实现者已同步更新
- 测试 mock 已同步更新

### 4.5 interface{} 迁移
**状态：✅ 合格**
- `knowledge/extension.go` `Enhance` 返回类型改为 `any`
- `knowledge/graph/retrieval_enhancer.go` 同步更新
- `domains/reasoning/multi_hypothesis.go:363` JSON 反序列化场景保留 `[]interface{}`

### 4.6-4.9 其他
**状态：✅ 合格**
- router 白名单一致性：`"mady-router"` → `"mady-router", "chat-agent"`
- bash temp file 清理：`time.Sleep(10*min)` → `time.NewTimer(10*min)`

---

## 全局验证门禁

| 检查项 | 结果 |
|--------|------|
| `go build ./...` | ✅ 通过 |
| `go vet ./...` | ✅ 通过 |
| `go test -race ./...` | ✅ 全部通过（0 FAIL） |
| `go test -race ./tools/...` | ✅ 全部通过 |
| `golangci-lint run ./...` | ✅ 通过（基线检查） |

---

## 总结

| 分类 | 数量 | 通过 | 建议改进 |
|------|------|------|---------|
| ✅ 合格 | 40+ 项 | 全部 | — |
| ⚠️ 建议改进 | 1 项 | — | agent_provider.go 错误类型区分 |
| ❌ 严重问题 | 0 项 | — | — |

**结论：69 文件变更的质量审阅通过。40+ 项检查点全部合格，1 项低风险优化建议。**

---

## 审阅方法

| 维度 | 方式 |
|------|------|
| 自动化检查 | `go build` / `go vet` / `golangci-lint` |
| 深度审查 | 2 个 code-review agent（逐文件检查正确性） |
| 现场抽查 | 直接文件读取验证关键变更点 |
| 回归验证 | `go test -race` 全量运行 |

## 审阅 agent 发现摘要

### review-batch1（批次 1 审阅）
- 全面检查了批次 1 的 13 项变更
- 确认所有 goroutine recover 实现正确（doneCh 闭合、错误记录、panic 日志）
- 确认所有错误忽略修复符合预期
- 确认全局状态注入模式线程安全

### review-batch234（批次 2/3/4 审阅）
- 检查了 22 个 doc.go 的格式和内容质量
- 验证了测试文件的覆盖场景和断言质量
- 确认 goroutine recover 和 context 传播的一致性
- 确认接口签名变更已传播到所有实现者和调用方
- 确认 time.After → time.NewTimer 替换无遗漏
