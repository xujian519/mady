# 已知限制（Known Limitations）

> 代码库中明确知道但尚未修复的功能缺口。每个条目记录影响范围、原因和跟踪链接。

## 桌面端 A2UI Action 入站处理未实现

**文件**：`server/desktop.go` — `SendAction()`

**描述**：`SendAction` 可以将客户端的 A2UI action 投递到 agent 事件总线，但 agent 侧
尚未注册 `EventA2UI` 的入站处理器来消费此事件。投递的事件仅到达事件总线，不会被
agent 执行循环处理。

**影响**：桌面模式下通过 A2UI 发送的 action（如下拉选择、按钮点击等交互事件）
不会影响 Agent 行为。

**原因**：A2UI 协议集成分两阶段——第一阶段完成事件输出（Agent→UI），第二阶段
实现事件输入（UI→Agent）。当前完成第一阶段，第二阶段规划中。

**状态**：待规划。当 A2UI 入站处理器实现时，需在 `agentcore/hooks.go` 中注册
`EventA2UI` 的 `LifecycleHook`，解析 `ClientAction` 并注入 agent 上下文。
