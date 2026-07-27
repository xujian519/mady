# A2UI / AGUI 审阅发现 — 维度 7-8（错误处理 + 并发安全）

## Dimension 7: Error Handling（错误处理）

### P0(复用): Content-Type 不一致
- **文件**: `agui/handler.go:67,103-105`
- **同 P0-1**。这条既是正确性 bug 也是错误处理问题——错误路径返回 JSON 但 headers 已设置为 SSE。

### P0(复用): agent.Run 错误不发送 RUN_ERROR
- **文件**: `agui/handler.go:183-189`
- **同 P0-2**。错误被静默吞没，前端无法感知执行失败。

### P2-12: convertToolCallEnd err 覆盖 result
- **文件**: `agui/converter.go:420-422`
- **问题**: `err != nil` 时直接 `result = err.Error()`，覆盖原 `result` 值。
- **影响**: 若 tool call 既返回结果又有错误，结果丢失。
- **建议**: 区分 PartialSuccess/Error 语义，或放弃 result 时仍保留原值用于日志。

### P2-13: writeSSE marshal 失败发送空 data
- **文件**: `agui/handler.go:225-228`
- **问题**: Marshaling 失败时发送 `data: {}`。消费端收到空对象 `{}` 无法与合法空对象事件区分。
- **建议**: 发送可识别的错误占位符（如 `data: {"$error":"marshal failed"}`），或直接关闭连接。

### P3-10: writeJSON marshal 失败无 fallback
- **文件**: `agui/handler.go:235-241`
- **问题**: `w.WriteHeader(status)` 在调用 `Encode` 之前已执行。若 Encode 失败（极少见），客户端仅收到空 body + 状态码。服务端仅 `slog.Warn`。
- **建议**: 可接受（HTTP Header 已发送无法回退），slog 日志已足够。

### P3-11: ApplyUpdate 错误未包装 sentinel
- **文件**: `a2ui/surface.go:123-125`
- **问题**: `ApplyUpdate` 返回的原始错误（如 "array index out of range"）直接透传给 `SurfaceStore.Apply` 的调用方。
- **影响**: 调用方难以用 `errors.Is` 分类处理。
- **建议**: 使用 `fmt.Errorf("%w: %v", ErrInvalidPath, err)` 包装。

### P3-12: Sentinel error 模式一致（已验证 ✅）
- **文件**: `a2ui/message.go:203-206`
- **评估**: `ErrMultipleBodies` 和 `ErrNoBody` 使用 `errors.New` 定义；调用方通过 `fmt.Errorf("...: %w", sentinel, detail)` 包装。`errors.Is` 可正确检测。模式一致。

---

## Dimension 8: Concurrency（并发安全）

### ✅ 已验证: defer unregister 模式正确
- **文件**: `agui/handler.go:124-138,140-149`
- **评估**: 两个回调（OnAll + On(EventTurnEnd)）均有 `defer unregister()` 和 `defer unregisterSnap()`，防止 agent 池化时回调泄漏到后续请求。这是非常正确的并发模式，包含中文注释解释原因（line 124-127）。

### ✅ 已验证: config 读写锁正确
- **文件**: `agui/handler.go:25-35`
- **评估**: `UpdateConfig`(写) 用 `Lock()`+`defer Unlock()`，`snapshotConfig`(读) 用 `RLock()`+`defer RUnlock()`。获取快照后操作本地副本，持有锁时间极短。

### P2-14: SSE 写入 mutex 必要性评估
- **文件**: `agui/handler.go:128-137`
- **评估**: `sync.Mutex` 保护 `http.ResponseWriter` 不被两个并发的 event callback（OnAll + On(EventTurnEnd)）同时写入。这是必需的保护，因为 `http.Flusher` 非并发安全。
- **建议**: 锁持有时间短（写入 JSON + Flush），且 Agent 事件频率受人类交互节奏控制，非瓶颈。**标记为观察点，无需优化**。

### P2-15: Converter 原子操作 check-then-act 模式
- **文件**: `agui/converter.go:17-19, 116-127, 129-139, 330-362, 365-396`
- **问题**: `activeMsgID`, `activeThinkingID` 使用 `atomic.Value` 存储，但访问模式是 `Load` → check → `Store`——非原子操作。在多 goroutine 并发调用 `Convert` 时存在竞态。
- **影响**: 但当前设计下，`Convert` 由单个 `OnAll` 回调在单 goroutine 中串行调用（因为 agent 事件是串行的），所以实际不会发生并发。原子操作是防御性编程。
- **建议**: 文档化 Converter 的并发模型："Converter 设计为在单个 goroutine 中使用；原子操作用于防御性保护，但在跨 goroutine 使用时需要外部同步"。

### P2-16: SurfaceStore 声明非并发安全
- **文件**: `a2ui/surface.go:47-49`
- **评估**: `SurfaceStore` 明确文档化为非并发安全。"guard it externally if shared across goroutines"。这是合理的设计选择。
- **验证**: 调用方（`a2ui/binding_a2a.go`, `a2ui/a2ui_test.go`）在单 goroutine 上下文中使用，无并发风险。

### P3-13: Encoder/Decoder 包装标准库无额外锁
- **文件**: `a2ui/stream.go:10-62`
- **评估**: `json.Encoder` 和 `json.Decoder` 非并发安全，`stream.Encoder`/`Decoder` 也未添加锁。调用方需自行保护。
- **建议**: 文档化此限制（目前无显式说明）。
