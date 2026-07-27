# A2UI / AGUI 审阅发现 — 维度 5-6（测试质量 + 集成正确性）

## Dimension 5: Test Quality（测试质量）

### 总览
- **A2UI**: 100.0% 语句覆盖率 ✅ — 107 测试函数，边缘用例覆盖完整
- **AGUI**: 77.7% 语句覆盖率 ❌ — 多个关键函数覆盖率不足

### P1-7: AGUI 测试覆盖率严重不足
- **综合评估**: AGUI 模块覆盖率仅 77.7%，远低于项目标准。
- **覆盖率盲区列表**:

| 函数 | 覆盖率 | 风险 |
|------|--------|------|
| `convertHandoffEnd` | **0.0%** | 完全未测试 |
| `convertCompactionEnd` | **0.0%** | 完全未测试 |
| `GetThreadConfig` | **0.0%** | 完全未测试 |
| `Convert` | **63.3%** | 核心函数大量分支未覆盖 |
| `convertRole` | **66.7%** | default 分支未测试 |
| `callConfigFromInput` | **66.7%** | 非 nil 输入路径未测试 |
| `threadCfgProviderFromConfig` | **42.9%** | 大部分逻辑未测试 |
| `writeSSE` | **50.0%** | marshal 失败路径未测试 |
| `writeJSON` | **75.0%** | encode 失败路径未测试 |
| `extractEventType` | **75.0%** | fallback 路径未测试 |
| `CapabilitiesFromConfig` | **93.3%** | 少量分支遗漏 |
| `handleRun` | **75.3%** | 错误路径未覆盖 |

- **建议**: 为所有覆盖率 < 80% 的函数添加专项测试，重点覆盖 error 路径和边界条件。

### P2-8: makeEvent helper 间接构造测试数据
- **文件**: `agui/converter_test.go:11-23`
- **问题**: `makeEvent[T]` 通过 JSON marshal → unmarshal 构造测试事件，引入间接依赖。若 JSON 序列化有 bug，测试可能通过但逻辑有问题。
- **例子**: `TestConvertAgentStartEvent` 无法区分"Convert 工作正确"和"JSON round-trip 同时出错"。对比 A2UI 测试直接构造结构体的模式更可靠。

### P1(复用): TypeScript 客户端无测试
- **文件**: `agui/client/src/*.ts`
- **问题**: `SSEStreamConsumer.read()`、`parseSSEBody()`、`deserializeEvent()` 等核心逻辑无任何 `.test.ts` 文件覆盖。
- **建议**: 使用 vitest 或 jest 添加 SSE 解析、事件分发、错误处理的单元测试。

### P2-9: 缺少 agentcore 集成的 e2e 测试
- **文件**: `agui/handler_e2e_test.go`
- **问题**: E2E 测试使用 mock provider，不触发真实 agentcore 执行路径（turn management, tool calls, compact, handoff）。
- **建议**: 添加集成测试覆盖真实 agent 执行路径验证 AGUI 事件序列。

### P2-10: A2UI 测试断言模式
- **文件**: `a2ui/a2ui_test.go`
- **问题**: 多数测试使用 `t.Fatalf` 而非 `t.Errorf`，首次断言失败后终止，隐藏后续断言中的失败。
- **影响**: 调试效率低，一个测试用例只能发现一个问题。
- **建议**: 在非 fatal 场景（如字段值断言）使用 `t.Errorf`。

### P3-8: A2UI 缺少边界测试
- 超大组件列表（1000+ 组件）的 JSON 序列化
- 深层嵌套 JSON Pointer（10+ 层级）
- 并发 Apply 到 SurfaceStore 的测试（虽然声明非并发安全）

---

## Dimension 6: Integration Correctness（集成正确性）

### P1(复用): envelopeToMap 错误静默忽略
- **文件**: `a2ui/binding_agentcore.go:16`
- **同 P1-4**。集成层面：调用方 `ToAgentCoreEvent` 返回值不含 error，若 `envelopeToMap` 失败，事件静默丢失。

### P1(复用): DataPartToEnvelope 无法区分错误类型
- **文件**: `a2ui/binding_a2a.go:43-47`
- **同 P1-5**。调用方 `MessageEnvelopes` 无法区分"A2UI 格式错误"和"非 A2UI part"，数据静默丢失。

### ✅ (来自计划 6.3): 双 A2UI 处理路径 — 非问题
- **文件**: `server/stream_events.go:419` + `agui/converter.go:311`
- **分析**: 这是两个独立的 SSE 消费者，服务于不同端点：
  - `server/chat.go` → 服务器自身 SSE 流（用于主聊天 UI）
  - `agui/handler.go` → AGUI 协议 SSE 端点（用于 AG-UI 前端）
- Agent 事件总线分发到所有注册监听器，二者互不冲突。
- **降级**: P1 → **✅ 设计如此，无需修复**

### P2-11: integration/ 目录缺少 A2UI/AGUI 集成测试
- **问题**: 7 个集成测试文件（`chain_e2e_test.go`, `doomloop_e2e_test.go`, `guardrails_e2e_test.go` 等）均不涉及 A2UI/AGUI。
- **建议**: 添加 A2UI builder → binding_agui → converter → SSE 的端到端集成测试，
  以及 A2UI envelope → A2A binding → envelope 的往返集成测试。

### P3-9: `/agui/{path}` 通配符未在 handler 内使用
- **文件**: `server/server.go:291`, `agui/handler.go:37-46`
- **问题**: `mux.Handle("/agui/{path}", ...)` 注册后，Handler 不读取 `{path}` 值。所有 `/agui/` 前缀的请求都到同一 handler 按 method 分发。
- **影响**: 无功能影响。但若将来需要 `/agui/events` 和 `/agui/capabilities` 分开处理，需重建路由。
- **建议**: 当前行为可接受（GET=capabilities, POST=run），通配符为未来扩展预留。
