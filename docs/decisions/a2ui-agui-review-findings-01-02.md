# A2UI / AGUI 审阅发现 — 维度 1-2（正确性 + 安全性）

日期: 2026-07-27
审阅范围: a2ui/ + agui/ + 交叉引用

---

## Dimension 1: Correctness（正确性）

### P0-1: LoadAgent 失败时 Content-Type 不一致
- **文件**: `agui/handler.go:67,103-105`
- **问题**: Line 67 设置 `Content-Type: text/event-stream`，但 LoadAgent 失败时 (line 103-105) 调用 `writeJSON` 再次设置 `Content-Type: application/json`。Go 的 `Header.Set` 会覆盖前值，导致错误响应以 `application/json` 返回，但 SSE 客户端期望 `text/event-stream`。
- **影响**: SSE 客户端收到 JSON 响应会解析失败，且 HTTP 状态码 500 与已设置的 SSE header 冲突。
- **建议修复**: 将 LoadAgent 调用提前到设置 SSE headers 之前，或将错误作为 SSE RUN_ERROR 事件发送。

### P0-2: agent.Run 错误时不发 RUN_ERROR
- **文件**: `agui/handler.go:183-189`
- **问题**: `runErr != nil` 时仅发送 closeAll + RUN_FINISHED，无 RUN_ERROR 事件。消费端无法区分 Agent 执行成功与失败。
- **证据**: line 183-189:
  ```go
  if runErr != nil {
      for _, agEv := range converter.closeAll(time.Now()) { ... }
      writeSSE(w, flusher, string(EventRunFinished), converter.RunFinished(time.Now()))
  }
  ```
  完全缺少 RUN_ERROR 事件发射。
- **影响**: SSE 客户端以为执行成功完成，但实际上发生了 runtime 错误。
- **建议修复**: 添加 `converter.RunError(time.Now(), runErr)` 事件，在 RUN_FINISHED 之前发送。

### P1-1: Thinking delta 未发送 THINKING_TEXT_MESSAGE_END
- **文件**: `agui/converter.go:389-395`
- **问题**: 后续 thinking delta 仅发送 `ThinkingTextMessageContentEvent`，每次都生成新 `msgID`，但前一条 message 永不发送 `ThinkingTextMessageEndEvent`。`CloseThinking()` 也仅发送 `ThinkingEndEvent`，不关闭活跃的 thinking text message。`EventThinkingTextMessageEnd` 类型虽在 `types.go:17` 定义，但整个代码库中从未被发射。
- **影响**: 前端可能累积僵尸思考消息（只有 START + CONTENT，没有 END）。
- **建议修复**: `convertThinkingDelta` 在首次 delta 之后，发送新 CONTENT 之前应先关闭前一条 thinking text message。

### P2-1: arrayIndex 空字符串处理
- **文件**: `a2ui/datamodel.go:168-177`
- **问题**: `arrayIndex` 对 `tok == ""` 调用 `strconv.Atoi("")` 返回 `(0, error)`，错误消息不友好。
- **影响**: 极小，仅影响错误消息可读性。

### P2-2: Convert default 分支暴露内部状态
- **文件**: `agui/converter.go:318-326`
- **问题**: 未知事件类型通过 `CustomEvent{Value: e}` 将完整事件体发送到前端。新加入 agentcore 的事件类型若未在 Convert 注册，会静默暴露。
- **影响**: 潜在的信息泄露风险。

### P2-3: ValidateEnvelope 在 catalog=nil 时跳过类型检查
- **文件**: `a2ui/validate.go:41`
- **问题**: 当 `cat==nil` 时，更新信封和 surface 树的组件类型检查被完全跳过。无 catalog 的调用方可能错过无效组件类型校验。
- **影响**: 运行时若没有 catalog 实例，类型错误被静默容忍。

### P3-1: detectCycles 验证 - 无明显问题
- **文件**: `a2ui/validate.go:116-153`
- **评估**: DFS 三色标记法正确实现。每个组件在 DFS 过程中只设 gray 一次，循环仅在 gray→gray 时报告一次，无重复报告。**无需修复**。

---

## Dimension 2: Security（安全性）

### P1-2: Capabilities GET 端点暴露 SystemPrompt
- **文件**: `agui/converter.go:557` + `agui/handler.go:48-53`
- **问题**: `CapabilitiesFromConfig` 将 `cfg.SystemPrompt` 映射为 `Identity.Description`。任意 GET `/agui/capabilities` 请求者都能获取完整的 System Prompt（常含详细指令、工具定义、敏感配置）。
- **影响**: 严重的信息泄露。
- **建议修复**: (a) 只暴露截断/脱敏版本 (b) 添加配置开关 (c) 为空或不暴露该字段。

### P1-3: callConfigFromInput 是 stub
- **文件**: `agui/handler.go:192-197`
- **问题**: 函数返回空 `CallConfig{}`，`input.Tools` 和 `input.State` 被完全忽略。
- **影响**: 客户端 POST `/agui/` 时传入的工具定义和状态不会被传递给 Agent。功能层缺失。
- **建议修复**: 实现完整的 CallConfig 构建逻辑，或文档化当前限制。

### P1-4: envelopeToMap 错误静默忽略
- **文件**: `a2ui/binding_agentcore.go:16`
- **问题**: `m, _ := envelopeToMap(env)` 忽略 error 返回值。虽然注释称 "always succeeds"，但在极端情况（OOM、类型错误）下可能失败。
- **建议修复**: 至少添加 `slog.Warn` 记录，或返回 error。

### P1-5: "非 A2UI" 与 "格式错误" 无法区分
- **文件**: `a2ui/binding_a2a.go:43-47`
- **问题**: `ParseEnvelope` 失败时返回 `ok=false, err=nil`，与非 A2UI part 返回值完全相同。调用方无法区分 "这不是 A2UI" 和 "这是 A2UI 但格式错误"。
- **影响**: 格式错误的 A2UI 信封被静默跳过，数据丢失。
- **建议修复**: 返回不同的错误信号，或在包级文档注明当前行为。

### P2-4: LoadAgent 错误暴露内部细节
- **文件**: `agui/handler.go:104`
- **问题**: `writeJSON(w, 500, map[string]string{"error": err.Error()})` 将原始 error 消息暴露给客户端，可能含内部路径/配置信息。
- **影响**: 低，但在生产环境中需控制。

### P3-2: ThreadID 无验证直接用作 store key
- **文件**: `agui/handler.go:72-74`
- **问题**: 客户端提供的 `ThreadID` 直接用于 `cfg.Store.Has(ctx, threadID)`。未做格式/长度校验。
- **影响**: 取决于 store 实现，可能被用于访问非预期状态。
