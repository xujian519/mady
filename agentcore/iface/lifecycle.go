package iface

import "context"

// =============================================================================
// 生命周期钩子的上下文类型
// =============================================================================

// AgentRunContext 在 Agent 运行开始时创建，贯穿整个运行周期。
type AgentRunContext struct {
	Input     string
	TurnCount int64
}

// ModelCallContext 在每次 LLM 调用时创建。
type ModelCallContext struct {
	Model           string
	Messages        int
	Content         string
	SuppressPersist bool
	HasToolCalls    bool
	Blocked         bool
}

// ToolExecutionContext 在工具调用执行时创建。
type ToolExecutionContext struct {
	ToolCalls int
	ToolNames []string
}

// TurnInfo 包含单轮执行的摘要信息。
type TurnInfo struct {
	HadToolCalls bool
	ToolCount    int
}

// =============================================================================
// 生命周期钩子接口
// =============================================================================

// LifecycleHook 提供 agent 执行各阶段的拦截点。
//
// 注意：此接口是 agentcore.LifecycleHook 的降采样收缩视图（narrow view），
// 并非完全同步。为了不向 iface 层暴露 agentcore 内部类型
// （*Message、[]Message、*ProviderRequest 等），以下方法在签名上做了有意收缩
// （适配层 ifaceLifecycleHookAdapter 已在 iface_adapter.go 中处理）：
//
//	BeforeMessagePersist:     agentcore(msg *Message) → iface(无 msg)
//	AfterMessagePersist:      agentcore(msg Message)  → iface(无 msg)
//	BeforeCompactionPersist:  agentcore(msgs []Message, 返回 []Message) → iface(无 msgs, 仅 error)
//	AfterCompactionPersist:   agentcore(msgs []Message) → iface(无 msgs)
//
// 新增方法时请同步更新 iface_adapter.go 中的 ifaceLifecycleHookAdapter。
type LifecycleHook interface {
	BeforeAgentRun(ctx context.Context, arc *AgentRunContext) error
	AfterAgentRun(ctx context.Context, arc *AgentRunContext, output string, err error)
	BeforeTurn(ctx context.Context, arc *AgentRunContext) error
	AfterTurn(ctx context.Context, arc *AgentRunContext, info TurnInfo)
	BeforeModelCall(ctx context.Context, arc *AgentRunContext, mcc *ModelCallContext) error
	AfterModelCall(ctx context.Context, arc *AgentRunContext, mcc *ModelCallContext)
	BeforeToolExecution(ctx context.Context, arc *AgentRunContext, tec *ToolExecutionContext) error
	AfterToolExecution(ctx context.Context, arc *AgentRunContext, tec *ToolExecutionContext)
	BeforeMessagePersist(ctx context.Context, arc *AgentRunContext) error
	AfterMessagePersist(ctx context.Context, arc *AgentRunContext)
	BeforeCompactionPersist(ctx context.Context, arc *AgentRunContext) error
	AfterCompactionPersist(ctx context.Context, arc *AgentRunContext)
}

// BaseLifecycleHook provides no-op default implementations for all LifecycleHook methods.
type BaseLifecycleHook struct{}

// BeforeAgentRun is called before the agent starts a run. Default: no-op.
func (BaseLifecycleHook) BeforeAgentRun(_ context.Context, _ *AgentRunContext) error { return nil }

// AfterAgentRun is called after the agent finishes a run. Default: no-op.
func (BaseLifecycleHook) AfterAgentRun(_ context.Context, _ *AgentRunContext, _ string, _ error) {}

// BeforeTurn is called before each agent turn. Default: no-op.
func (BaseLifecycleHook) BeforeTurn(_ context.Context, _ *AgentRunContext) error { return nil }

// AfterTurn is called after each agent turn. Default: no-op.
func (BaseLifecycleHook) AfterTurn(_ context.Context, _ *AgentRunContext, _ TurnInfo) {}

// BeforeModelCall is called before a model invocation. Default: no-op.
func (BaseLifecycleHook) BeforeModelCall(_ context.Context, _ *AgentRunContext, _ *ModelCallContext) error {
	return nil
}

// AfterModelCall is called after a model invocation completes. Default: no-op.
func (BaseLifecycleHook) AfterModelCall(_ context.Context, _ *AgentRunContext, _ *ModelCallContext) {}

// BeforeToolExecution is called before a tool is executed. Default: no-op.
func (BaseLifecycleHook) BeforeToolExecution(_ context.Context, _ *AgentRunContext, _ *ToolExecutionContext) error {
	return nil
}

// AfterToolExecution is called after a tool execution completes. Default: no-op.
func (BaseLifecycleHook) AfterToolExecution(_ context.Context, _ *AgentRunContext, _ *ToolExecutionContext) {
}

// BeforeMessagePersist is called before messages are persisted. Default: no-op.
func (BaseLifecycleHook) BeforeMessagePersist(_ context.Context, _ *AgentRunContext) error {
	return nil
}

// AfterMessagePersist is called after messages are persisted. Default: no-op.
func (BaseLifecycleHook) AfterMessagePersist(_ context.Context, _ *AgentRunContext) {}

// BeforeCompactionPersist is called before compaction data is persisted. Default: no-op.
func (BaseLifecycleHook) BeforeCompactionPersist(_ context.Context, _ *AgentRunContext) error {
	return nil
}

// AfterCompactionPersist is called after compaction data is persisted. Default: no-op.
func (BaseLifecycleHook) AfterCompactionPersist(_ context.Context, _ *AgentRunContext) {}
