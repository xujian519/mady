# Mady 执行器全量质量审阅报告

> 审阅日期：2026-07-23
> 审阅范围：`agentcore/` 包核心执行引擎及子模块（排除已有事件系统审阅报告覆盖的文件）
> 已有参考：`docs/review/event-system-review-2026-07-23.md`（覆盖 event.go / event_types.go / pubsub.go / event_test.go / stream_events.go）

## 总体评估

| 维度 | 评分 | 说明 |
|------|------|------|
| 架构设计 | ⭐⭐⭐⭐⭐ | 分层清晰、职责明确、LifecycleHook + Extension + Middleware 三路扩展机制成熟 |
| 代码质量 | ⭐⭐⭐⭐ | 注释丰富、命名规范、少量一致性问题 |
| 并发安全 | ⭐⭐⭐⭐⭐ | `configMu` 锁粒度合理、Pool 实现正确、race detector 全通过 |
| 错误处理 | ⭐⭐⭐⭐ | panic 恢复覆盖完善、少量错误传播路径需打磨 |
| 测试覆盖 | ⭐⭐⭐⭐⭐ | 55 个测试文件覆盖几乎所有非平凡源文件，race 全通过 |
| 生产就绪度 | ⭐⭐⭐⭐ | 构建/测试全通过，存在 1 个 off-by-one 风险和若干可优化点 |

## 与已有审阅报告的交叉引用

本报告假设 `docs/review/event-system-review-2026-07-23.md` 中识别的 14 个事件系统问题已独立跟踪，不再重复。

## 问题清单

### 🟠 严重

#### 1. MaxTurns off-by-one：实际允许的轮次数比配置多 1

**文件**: `agentcore/agent_run_phase.go:13`
**严重级别**: 🟠 严重

```go
if turn-loopStartTurn > a.config.MaxTurns {
```

使用 `>` 而非 `>=` 比较。当 `turn-loopStartTurn == MaxTurns` 时，条件不成立，允许执行额外一轮。例如 `MaxTurns=20` 时，实际可执行 21 轮（turn 0 到 turn 20）。

**复现场景**: 用户配置 MaxTurns=3，期望最多执行 3 轮工具调用循环，但实际执行了 4 轮才被阻止。可能导致额外的 LLM 调用费用。

**修复建议**: 改为 `>=`：
```go
if turn-loopStartTurn >= a.config.MaxTurns {
```

**注意**: 若现有行为是设计上期望的（例如第一轮视为"思考轮"不计入），请补充文档说明。

---

#### 2. `ExecuteAll` 串行模式下 tool call 循环不检查 context cancellation

**文件**: `agentcore/executor.go:309-321`
**严重级别**: 🟠 严重

```go
func (e *Executor) executeSerial(ctx context.Context, calls []ToolCall, state *AgentState, cb *ExecuteCallbacks) []ToolResult {
	results := make([]ToolResult, len(calls))
	for i, tc := range calls {
		if cb != nil && cb.OnStart != nil {
			cb.OnStart(tc)
		}
		results[i] = e.Execute(ctx, tc, state)
		if cb != nil && cb.OnEnd != nil {
			cb.OnEnd(results[i])
		}
	}
	return results
}
```

串行模式下，循环不检查 `ctx.Done()`。如果用户取消请求，已在执行的工具调用会通过 `Execute` 内部的 context 传播取消（若工具函数监听 ctx），但**尚未启动**的工具调用仍会继续执行，直到循环结束。

`executeParallel` 通过 `pool.Acquire(ctx)` 实现了上下文感知，但串行模式缺乏同等保护。

**复现场景**: 用户取消请求后，5 个串行工具调用仍有 3 个未执行——但它们全部被执行。对于高延迟工具（如浏览器自动化的 MCP 调用），取消响应变慢。

**修复建议**: 在循环开始时添加 context 检查：
```go
for i, tc := range calls {
	select {
	case <-ctx.Done():
		// Fill remaining results with cancellation
		for j := i; j < len(calls); j++ {
			results[j] = ToolResult{ToolCallID: calls[j].ID, ToolName: calls[j].Name, Result: "工具执行被中断"}
		}
		return results
	default:
	}
	// ... existing logic ...
}
```

---

#### 3. 并行模式 goroutine 泄漏风险：`pool.Acquire` 失败后 `wg.Done()` 可能不执行

**文件**: `agentcore/executor.go:333-354`
**严重级别**: 🟠 严重

当前代码在 goroutine 入口处 `defer wg.Done()`。`pool.Acquire` 在错误检查分支中 `return` 后，`defer pool.Release()` **不会执行**（因为它定义在 `if err` 分支之后），但 `defer wg.Done()` **会执行**。所以当前实现中 `wg.Wait()` 正常完成。

**但问题在于**：如果 goroutine 内部在 `defer pool.Release()` 之前 panic（例如 `pool.Acquire` 内部的 `select` 竞态导致写 closed channel 以外的 panic），则 `pool.Release()` 不会执行，但 `wg.Done()` 会执行（defer 栈执行）。WaitGroup 不泄漏，但 pool slot 泄漏。

实际上当前实现在 `Acquire` 成功后 `defer pool.Release()` 才注册，panic 路径不会跳过已注册的 defer。**确认当前实现安全**。但代码结构脆弱——任何后续修改在 `Acquire` 和 `defer pool.Release()` 之间添加可能 panic 的代码都会导致 slot 泄漏。

**修复建议**: 将 `defer pool.Release()` 移到 goroutine 顶部，用 flag 保护：
```go
go func(idx int, call ToolCall) {
	defer wg.Done()
	var acquired bool
	defer func() {
		if acquired {
			pool.Release()
		}
	}()
	...
	if err := pool.Acquire(ctx); err != nil {
		results[idx] = ToolResult{...}
		return
	}
	acquired = true
	...
}(i, tc)
```

**优先级**: 🟡 建议（当前无泄漏，但结构脆弱）

---

#### 4. `InvokeTool` 绕过配置校验和生命周期钩子的 BeforeAgentRun

**文件**: `agentcore/agent.go:349-356`
**严重级别**: 🟠 严重

```go
func (a *Agent) InvokeTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	tc := ToolCall{Name: name, Arguments: string(args)}
	result := a.executor.Execute(ctx, tc, a.state)
	...
}
```

`InvokeTool` 不检查 `configErr`，不检查 Agent 当前状态（`StatusRunning`/`StatusError`），不触发 `BeforeAgentRun`/`AfterAgentRun` 生命周期。如果 Agent 配置无效，它能执行工具；如果一个已失败的 Agent 被调用 `InvokeTool`，它仍然能执行。

**复现场景**:
1. `New(Config)` 返回配置无效的 Agent（configErr 被设置但 New 不 panic）
2. 调用者未调用 `Run()`，直接调用 `InvokeTool()`
3. 工具被执行——可能违反配置意图

**修复建议**: 添加前置检查：
```go
func (a *Agent) InvokeTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	if a.configErr != nil {
		return "", fmt.Errorf("agent not configured properly: %w", a.configErr)
	}
	...
}
```

---

### 🟡 建议

#### 5. `EvalHook` 的 `scoreFaithfulness` 在无上下文时返回乐观默认值 0.8

**文件**: `knowledge/eval.go:148-152`
**严重级别**: 🟡 建议

```go
func scoreFaithfulness(answer string, req *agentcore.ProviderRequest) float64 {
	...
	contextText := extractContextText(req.Messages)
	if contextText == "" {
		return 0.8 // 无上下文时默认较高（没有可违反的材料）
	}
	...
}
```

在无检索上下文的普通对话场景中，忠实度被评为 0.8。这会使 `EvalConsumer`（`knowledge/eval_consumer.go:64`）中 `Faithfulness < AlertThreshold`（0.6）的警告永远不会触发。用户可能因此错过模型幻觉。

**修复建议**: 区分"无上下文"和"有上下文但信誉良好"：
- 返回 0（表示无法评估）而非 0.8
- 或在 EvalConsumer 的阈值检查中跳过 Faithfulness=0 的结果

#### 6. `toolCallSignature` 使用未排序的工具名拼接，重复检测可能漏判

**文件**: `agentcore/agent_run_tool.go:14-20`
**严重级别**: 🟡 建议

```go
func toolCallSignature(calls []ToolCall) string {
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Name
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}
```

函数签名已排序，是正确的。参数顺序不同但工具集相同的调用会被正确识别为重复。

**但是**重复检测仅在 `turn-loopStartTurn >= 2`（至少第 3 轮）时启动（`agent_run.go:310`）。如果模型在前两轮就卡在重复循环中，不会被检测到。

**影响**: 低。2 轮延迟后检测通常足够。

#### 7. `guardTruncation` 在工具调用 ID 为空时生成空关联

**文件**: `agentcore/agent_run_phase.go:134-151`
**严重级别**: 🟡 建议

当 `FinishReason == "length"` 且工具调用参数无效时，`guardTruncation` 为每个工具调用持久化错误消息。如果 `tc.ID` 为空（某些 Provider 实现可能不生成 ID），持久化的消息缺少关联，可能不影响功能但丢失可追溯性。

**修复建议**: 添加日志警告当 `tc.ID` 为空时。

#### 8. `Config()` 方法的值拷贝共享底层切片

**文件**: `agentcore/agent.go:262-266`
**严重级别**: 🟡 建议

```go
func (a *Agent) Config() Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return a.config
}
```

Config 包含 `Tools []*Tool`、`Handoffs []HandoffConfig`、`Extensions []Extension` 等切片字段。值拷贝时，返回的 Config 与 Agent 的 Config 共享这些切片的底层数组。调用方若修改返回的 Config 的切片，会在无锁保护下影响 Agent 的内部状态。

**修复建议**: 对关键切片执行浅拷贝保护：
```go
func (a *Agent) Config() Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	cfg := a.config
	cfg.Tools = append([]*Tool(nil), cfg.Tools...)
	// ... 其他切片字段同理
	return cfg
}
```

#### 9. `EvalHook` 的 `countContextSnippets` 依赖非标准分隔符

**文件**: `knowledge/eval.go:232-245`
**严重级别**: 🟡 建议

```go
func countContextSnippets(req *agentcore.ProviderRequest) int {
	...
	count := strings.Count(contextText, "---")
	if count > 0 {
		return count / 2 // 每段有两个 --- 标记（开头和结尾）
	}
	return 1
}
```

对上下文片段数量的估算依赖于 `---` 分隔符，而 `extractContextText` 同时搜索 `---`、`参考`、`检索`。如果上下文使用其他格式，计数不准确。

**修复建议**: 如果这是个已知的格式约定，请文档化；否则考虑从上下文构建侧传递结构化片段计数。

#### 10. `EvalConsumer` 硬编码事件类型字符串（与已有事件系统报告交叉引用）

**文件**: `knowledge/eval.go:119`, `knowledge/eval_consumer.go:48`
**严重级别**: 🟡 建议

`EventTypeEvalResult` 在 `knowledge/eval.go` 中定义为包级常量，但没有在 `agentcore/event_types.go` 中注册为公共事件类型。`EvalConsumer.OnEvent` 通过字符串匹配 `EventTypeEvalResult`。

**修复建议**: 若 `eval_result` 属于公共协议，应添加到 `agentcore/event_types.go`。

---

### 🔵 优化建议

#### 11. `Run()` 中 `eventBus.Drain()` 的调用时机

**文件**: `agentcore/agent_run.go:26`
**严重级别**: 🔵 优化

```go
defer a.eventBus.Drain()
```

`Drain()` 在函数返回时执行。但 `Run()` 返回后，调用方可能立即读取已发射的事件。`Drain()` 确保事件被处理，但不对事件投递作严格保证。

**影响**: 低。配合 `EmitMustDeliver` 使用时，终端事件应已被投递。

#### 12. `RateLimitHook.BeforeModelCall` 中切片增长未预分配

**文件**: `agentcore/lifecycle.go:530-549`
**严重级别**: 🔵 优化

```go
r.turnTimestamps = append(r.turnTimestamps, now)
```

每次 BeforeModelCall 时 `append` 到 `turnTimestamps` 切片。此切片在 `BeforeAgentRun` 中重置为 nil 但从未被修剪。在长期运行的 Agent 中，此切片可能增长到数千个条目，但每次只检查最近 1 分钟的时间戳。

**修复建议**: 定期修剪超过 1 分钟的时间戳，或使用环形缓冲区。

#### 13. `Agent.Run()` 中 SystemPrompt 持久化的空值检查

**文件**: `agentcore/agent_run.go:36-41`
**严重级别**: 🔵 优化

```go
if sp := a.systemPrompt(); sp != "" && !a.state.HasSystemPrompt() {
	...
}
```

SystemPrompt 为空时跳过持久化。但如果 SystemPrompt 在后续通过 Extension.SystemPromptProvider 添加（在 `Run()` 之前通过 `Register()`），此时 `state.HasSystemPrompt()` 为 false 但 `systemPrompt()` 返回的是旧值。

实际流程中 `Register()` 在 `New()` 中被调用，修改了 `config.SystemPrompt`，然后 `Run()` 读取 `systemPrompt()`，所以值是正确的。没有竞态问题。✅

#### 14. `CloseablePool.Acquire` 中 slot double-check 块的竞态窗口

**文件**: `agentcore/concurrency/pool.go:117-126`
**严重级别**: 🔵 优化

```go
select {
case p.sem <- struct{}{}:
	// Acquired a slot. Double-check if pool was closed during our wait.
	select {
	case <-p.closed:
		<-p.sem // release the slot
		return ErrPoolClosed
	default:
		return nil
	}
```

当 pool 被 close 时，已获得 slot 的 goroutine 会释放 slot 并返回 ErrPoolClosed。但在 `<-p.sem` 释放 slot 和 return 之间，close 通知可能已被另一个 goroutine 消费。这不会导致安全问题——slot 被正确释放——只是一个微妙的竞态窗口，可能让一个刚被 close 的 pool 的 slot 短暂可用。

**影响**: 极低。不导致泄漏或数据损坏。

## 测试改进计划

### 覆盖率缺口

| 函数/路径 | 当前状态 | 建议 |
|-----------|----------|------|
| `executor.go:executeSerial` 的 context 取消路径 | 未覆盖 | 增加 ctx canceled 时串行执行提前终止的测试 |
| `executor.go:executeParallel` 的 pool.Acquire 错误路径 | 未覆盖 | 增加 pool full + ctx canceled 场景测试 |
| `agent_run.go:runLoop` 的 followUp 分支 | 未覆盖 | 增加 followUp 消息处理的完整集成测试 |
| `lifecycle.go:RateLimitHook` 长时间运行后的切片修剪 | 未覆盖 | 增加 >1000 次调用的历史修剪测试 |
| `knowledge/eval.go:scoreFaithfulness` 空上下文路径 | 未覆盖 | 增加无上下文输入测试 |

### 建议新增测试

1. **TestSerialExecutionContextCancel**: 验证串行模式下 context 取消后未执行工具被跳过
2. **TestParallelPoolAcquireError**: 验证并发池获取失败时结果正确且无泄漏
3. **TestRunLoopFollowUpMessages**: 验证 followUp 消息被正确处理
4. **TestInvokeToolConfigError**: 验证无效配置的 Agent 调用 InvokeTool 返回错误
5. **TestEvalScoringEmptyContext**: 验证无上下文时 scoreFaithfulness 返回 0
6. **TestRateLimitHookMemoryBound**: 验证 RateLimitHook 在大量调用后不无限增长

## 验证结果

| 检查项 | 结果 |
|--------|------|
| `go build ./...` | ✅ 通过 |
| `go vet ./agentcore/...` | ✅ 通过（无警告） |
| `go test -race ./agentcore/...` | ✅ 全部通过（8 个子包） |
| `go test -race ./knowledge/...` | ✅ 全部通过（6 个子包） |
| 已有事件系统报告交叉引用 | ✅ 已标注 |

## 结论

Mady 执行器整体架构设计优秀，代码质量高，并发安全性经过良好的考量。55 个测试文件覆盖了几乎所有非平凡功能，race detector 全通过。

**需要优先关注的问题：**

1. **🔴 MaxTurns off-by-one**（`agent_run_phase.go:13`）— `>` 应为 `>=`，实际允许的轮次数比配置多 1
2. **🔴 串行执行不支持 context 取消**（`executor.go:309`）— 用户取消请求后未执行工具仍会执行
3. **🔴 InvokeTool 绕过配置校验**（`agent.go:349`）— 无效配置的 Agent 仍可执行工具
4. **🟡 并发池 slot 保护可加强**（`executor.go:333`）— 防止未来代码修改引入 leak

其余发现以优化建议为主，不影响当前功能正确性。
