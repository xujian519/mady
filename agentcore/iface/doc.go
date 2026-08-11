// Package iface provides the core interface contracts for the Mady agent runtime.
//
// These interfaces define the boundaries between agentcore and its consumers
// (guardrails, server, etc.). External modules should depend on these interfaces
// rather than on agentcore concrete types.
//
// # 当前提供的契约
//
//   - LifecycleHook: 生命周期拦截点（窄视图）
//
// # 历史说明
//
// 本包曾提供 iface.Event / EventBus（2026-07 引入，server 侧经
// agentcore.NewIFaceEventBus 适配）。由于 server 本就直接依赖 agentcore，
// 适配链（Event → payloadEvent → iface.Event 往返转换）价值有限，
// 已于 2026-08-11 移除，server 直连 agentcore.EventBus。
//
// # 接口收缩策略（Narrow View Strategy）
//
// iface.LifecycleHook 是 agentcore.LifecycleHook 的降采样收缩视图。
// 这是有意为之的设计决策，基于以下三条原则：
//
//  1. 依赖倒置（Dependency Inversion）
//     外部模块（guardrails、psychological 等）只应依赖 iface，
//     不直接依赖 agentcore 的内部类型（ProviderRequest、Message 等）。
//
//  2. 安全边界（Security Boundary）
//     不将 *Message、[]Message、*ProviderRequest 等可写引用传播到 iface 层，
//     防止外部模块意外或恶意修改 agentcore 内部状态。
//
//  3. 最小信息原则（Least Information）
//     消息级的拦截由 agentcore 层的粒度 Observer 接口（MessagePersistObserver
//     等）承担，iface 层只需要阶段级事件通知。
//
// # 已知限制
//
// The LifecycleHook interface exceeds the ≤5 guideline (10 methods) due to
// the inherent complexity of lifecycle hooks. This is an accepted trade-off.
//
// The iface abstraction intentionally simplifies context types:
//   - BeforeMessagePersist / AfterMessagePersist: 不携带 Message 参数。
//   - BeforeCompactionPersist / AfterCompactionPersist: 不携带 []Message 参数，
//     且 BeforeCompactionPersist 仅返回 error（agentcore 版本返回 ([]Message, error)）。
//   - ToolExecutionContext: 仅暴露工具调用计数和名称，不暴露 arguments 和 results。
//   - TurnInfo.ToolCount: 始终为 0（适配器无法从 agentcore.TurnInfo 获取此数据）。
//   - AgentRunContext: 不携带 Messages 切片和 Agent 实例引用。
//
// Extensions that need full-fidelity context (e.g. evidence, which requires
// tool arguments and results) should depend on agentcore directly rather than
// going through iface.
//
// # 历史说明
//
// 本包曾定义 iface.Extension / ToolProvider / ContextProvider / Store /
// ChatProvider / AgentContext 接口（2026-07-22 创建），作为扩展系统的
// 窄视图抽象。由于几乎所有扩展都需要完整上下文（工具参数、Message 引用等），
// 这些接口从未被实现，已于 2026-07-26 移除。扩展系统直接使用 agentcore.* 接口。
package iface
