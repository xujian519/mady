# Mady 事件系统审阅报告

> 审阅日期：2026-07-23
> 审阅范围：`agentcore/event_types.go`、`agentcore/event.go`、`agentcore/pubsub.go`、`agentcore/event_test.go`、`agentcore/agent_run.go`（终端事件）、`agentcore/agent.go`（EB 集成）、`agentcore/iface_adapter.go`、`server/stream_events.go`、`knowledge/eval_consumer.go`

## 总体评估

| 维度 | 评分 | 说明 |
|------|------|------|
| 架构设计 | ⭐⭐⭐⭐⭐ | 三层分离清晰，职责边界明确，泛型 Broker 复用度高 |
| 代码质量 | ⭐⭐⭐⭐ | 注释完善，命名规范，少量一致性问题 |
| 并发安全 | ⭐⭐⭐⭐ | 单 goroutine 分发 + RWMutex 保护，race detector 全通过。TOCTOU 窗口已有文档标注 |
| 错误处理 | ⭐⭐⭐⭐ | safeCall panic 恢复、关闭安全防护到位；日志级别需微调 |
| 测试覆盖 | ⭐⭐⭐ | 核心路径覆盖充分，但 JSON 序列化、handler 注销、MustDeliver 超时等分支缺失 |
| 生产就绪度 | ⭐⭐⭐⭐ | 构建/测试全通过，吞吐量 ~4M events/s。SSE 消费方有两类事件缺失映射 |

---

## 问题清单

### 🟠 严重

#### 1. A2UIEvent 和 ApprovalPromptEvent 在 SSE 适配层缺失映射

**文件**: `server/stream_events.go:311-488` (`agentEventPayload` 函数)

`agentEventPayload` 的 type switch 未处理 `A2UIEvent` 和 `ApprovalPromptEvent`，它们会落入 `default: return e` 分支。这意味着这两个事件在 SSE 流中不会转换为结构化的 `StreamPayload`，而是以原始 Go 对象形式暴露——前端可能无法正确解析。

**修复建议**:
```go
case agentcore.A2UIEvent:
    return A2UIStreamPayload{Envelope: ev.Envelope}
case *agentcore.A2UIEvent:
    return A2UIStreamPayload{Envelope: ev.Envelope}
case agentcore.ApprovalPromptEvent:
    return ApprovalPromptStreamPayload{...}
case *agentcore.ApprovalPromptEvent:
    return ApprovalPromptStreamPayload{...}
```

同时需在 `server/stream_events.go` 中新增 `A2UIStreamPayload` 和 `ApprovalPromptStreamPayload` 结构体。

---

#### 2. 终端事件发射路径不一致

**文件**: `agentcore/agent_run.go:165-179`

终端事件统一发射的 switch 只处理 `StatusFinished` 和 `StatusInterrupted`。当 `StatusError` 时，`AgentErrorEvent` 在 `failLoop()` 中提前发射，不经过该 switch。虽然功能正确，但造成了**三种终止状态有三种不同的发射路径**：
- `StatusFinished` → switch 中 emit `AgentEndEvent`
- `StatusInterrupted` → switch 中 emit `AgentInterruptEvent`
- `StatusError` → `failLoop()` 中 emit `AgentErrorEvent`，不经过 switch

**修复建议**: 将 `failLoop` 中的 emit 移除，改为在 switch 中增加 `case StatusError` 分支发射 `AgentErrorEvent`。保持所有终端事件发射路径一致：
```go
switch a.state.Status() {
case StatusFinished:
    a.emit(&AgentEndEvent{...})
case StatusInterrupted:
    a.emit(&AgentInterruptEvent{...})
case StatusError:
    // Error event already populated via failLoop/other paths
    // Optionally emit a terminal wrapper here
}
```

---

### 🟡 建议

#### 3. SkillsReloadedEvent 返回值类型不一致

**文件**: `agentcore/event_types.go:131-150`

`NewSkillsReloadedEvent` 返回 `SkillsReloadedEvent`（值类型），而所有其他事件构造函数（`NewAgentStartEvent`、`NewA2UIEvent` 等）返回指针类型。这导致：
- `stream_events.go` 中需要同时处理值类型和指针类型的 case 分支
- 事件传递时发生值拷贝（SkillsReloadedEvent 含多个切片字段）

**修复建议**: 改为返回 `*SkillsReloadedEvent` 或统一文档说明此例外的理由。

---

#### 4. EmitMustDeliver 在生产代码中未被使用

**文件**: 全局

`EmitMustDeliver` 仅出现在 `iface_adapter.go` 和测试代码中。所有 agentcore 内部的事件发射（`agent_run.go`、`agent_run_phase.go`）全部使用 `Emit`（非阻塞丢包）。文档中提到的"终端事件应使用 EmitMustDeliver"的建议未被实际遵循。

**影响**: `AgentEndEvent`、`AgentErrorEvent`、`AgentInterruptEvent` 等关键终端事件在高负载时可能被静默丢弃。

**修复建议**: 审计所有终端事件发射点，将 `emit()` 调用替换为 `emitMustDeliver()`。或者，如果当前设计认为丢包可接受，则更新文档说明。

---

#### 5. JSON 序列化/反序列化 0% 测试覆盖率

**文件**: `agentcore/event_types.go:321-484`

4 个事件的 `MarshalJSON`/`UnmarshalJSON` 完全没有测试。跨进程/网络传输时如果序列化格式发生变化，不会在 CI 中被捕获。

**修复建议**: 至少为 `AgentErrorEvent` 和 `AutoRetryEvent` 添加序列化往返测试（marshal → unmarshal → 验证字段一致性）。

---

#### 6. EvalConsumer 使用硬编码事件类型字符串

**文件**: `knowledge/eval_consumer.go:49`、`knowledge/eval.go:124`

`agentcore.EventType("eval_result")` 是硬编码的字符串字面量，而非在 `event_types.go` 中声明的常量。

**修复建议**: 如果 `eval_result` 事件属于公共协议，应添加到 `agentcore/event_types.go` 中。如果仅 knowledge 包内部使用，建议在 `knowledge/eval.go` 中声明包级常量替代硬编码。

---

#### 7. PublishMustDeliver 超时日志级别偏高

**文件**: `agentcore/pubsub.go:233`

超时时使用 `slog.Error` 级别。在订阅者消费慢或高负载场景下，每秒可产生大量 Error 日志。

**修复建议**: 降级为 `slog.Warn`，因为这是预期的背压行为而非系统错误。

---

#### 8. errorType/reconstructError 仅支持 2 种标准错误

**文件**: `agentcore/event_types.go:460-484`

只识别 `context.Canceled` 和 `context.DeadlineExceeded`。其他所有错误在反序列化时退化为 `errors.New(msg)`，丢失类型信息。

**修复建议**: 优先考虑是否必要。当前事件系统主要用于进程内（不经过网络传输），JSON 序列化仅在 SSE 输出时使用。如果未来有跨进程场景，可引入错误类型注册表。

---

### 🔵 优化建议

#### 9. Event 接口可扩展性

`Event` 接口只有 `EventKind()` 和 `EventTime()` 两个方法。随着消费者增多（如分布式追踪），可能需要 `EventID()`（唯一 ID）或 `EventAgentName()`（来源 Agent）。建议预留扩展点。

#### 10. 缺少 panic 计数器

`safeCall` 只打印堆栈到 stderr，没有可编程的监控指标。建议增加 `PanicCount() uint64` 方法。

#### 11. Drain 在 Close 竞态下等待超时

`Drain()` (event.go:237-263) 在 Close 竞态（TOCTOU）时会等待完整的 `drainTimeout`（默认 5s）才返回，而不是立即检测到 broker 已关闭。

**修复建议**: 在 `ack` select 中增加对 `eb.done` 的监听：
```go
select {
case <-ack:
case <-eb.done:
case <-timer.C:
}
```

#### 12. stream_events.go 值/指针双份 case 重复

`agentEventPayload` 对每个事件类型都写了两个 case（值接收者和指针接收者），代码高度重复。可考虑使用辅助函数或接口统一。

#### 13. Broker Subscribe 关闭后返回值文档不明确

当 broker 已 shutdown 时，`Subscribe` 返回立即关闭的通道。调用方需要正确处理从已关闭通道接收的零值。建议在文档中明确此行为。

#### 14. 缺少背压反馈机制

`DropCount` 增长时，生产者完全不知情。可考虑增加 `OnDrop` 回调或通过 EventBus 发射 `EventBusSaturated` 事件通知上游限速。

---

## 测试改进计划

### 覆盖率对比

| 函数 | 当前覆盖率 | 目标 | 优先级 |
|------|-----------|------|--------|
| `offID` / `offAllID` | 0% | 90%+ | 🔴 高 |
| JSON `MarshalJSON`/`UnmarshalJSON` (×4 事件) | 0% | 90%+ | 🔴 高 |
| `SetDrainTimeout` | 0% | 80%+ | 🟡 中 |
| `errorType` / `reconstructError` | 0% | 80%+ | 🟡 中 |
| `DropCount` / `MustDeliverDropCount` | 0% | 80%+ | 🟡 中 |
| `SubscriberCount` | 0% | 80%+ | 🔵 低 |
| `EmitMustDeliver` (closed path) | 66.7% | 100% | 🟡 中 |
| `PublishMustDeliver` (超时路径) | 57.1% | 80%+ | 🟡 中 |

### 建议新增测试

1. **TestEventBus_HandlerDeregistration**: 验证 On/OnAll 返回的取消函数确实阻止后续事件到达
2. **TestEventBus_JSONRoundtrip**: 对每个含 error 的事件测试 marshal → unmarshal 往返
3. **TestEventBus_MustDeliverTimeout**: 验证订阅者满时 MustDeliver 超时行为
4. **TestEventBus_DrainTimeout**: 验证 SetDrainTimeout 配置生效
5. **TestEventBus_DropCount**: 验证缓冲区满时 DropCount 正确递增
6. **TestBroker_SubscribeAfterShutdown**: 验证 shutdown 后 subscribe 返回已关闭通道
7. **TestTerminalEventConsistency**: 验证每个终止状态（Finished/Error/Interrupted）都发射了唯一终端事件

---

## 性能基准

```
BenchmarkEventBusThroughput-14    3,989,474 ops/s    288.9 ns/op
```

- Apple M4 Pro，14 线程
- 单 handler，Emit 非阻塞模式
- 约 400 万 events/s 的吞吐量，对绝大多数场景绰绰有余

---

## 结论

Mady 事件系统整体架构设计优秀，三层分离清晰，并发安全性经过良好的考量。生产就绪度高，所有测试在 race detector 下全通过。

需要立即关注的问题：
1. **A2UIEvent/ApprovalPromptEvent 在 SSE 流中缺失映射** — 直接影响前端功能
2. **EmitMustDeliver 在生产代码中未被使用** — 终端事件可能在高负载时丢失
3. **JSON 序列化零测试覆盖** — 跨进程场景下存在格式漂移风险

建议在下一个迭代中优先修复上述 3 个问题，并按优先级补充测试用例。
