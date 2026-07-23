package iface

import "context"

// =============================================================================
// 消息类型
// =============================================================================

// Message 是消息的最小契约。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// =============================================================================
// 消费侧最小上下文接口
// =============================================================================

// AgentContext 是 Agent 运行时的最小上下文接口。
type AgentContext interface {
	Input() string
	Messages() []Message
}

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
// 注意：此接口与 agentcore.LifecycleHook 保持同步。
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

// BaseLifecycleHook 提供所有方法的 no-op 默认实现。
type BaseLifecycleHook struct{}

func (BaseLifecycleHook) BeforeAgentRun(_ context.Context, _ *AgentRunContext) error             { return nil }
func (BaseLifecycleHook) AfterAgentRun(_ context.Context, _ *AgentRunContext, _ string, _ error) {}
func (BaseLifecycleHook) BeforeTurn(_ context.Context, _ *AgentRunContext) error                 { return nil }
func (BaseLifecycleHook) AfterTurn(_ context.Context, _ *AgentRunContext, _ TurnInfo)            {}
func (BaseLifecycleHook) BeforeModelCall(_ context.Context, _ *AgentRunContext, _ *ModelCallContext) error {
	return nil
}
func (BaseLifecycleHook) AfterModelCall(_ context.Context, _ *AgentRunContext, _ *ModelCallContext) {}
func (BaseLifecycleHook) BeforeToolExecution(_ context.Context, _ *AgentRunContext, _ *ToolExecutionContext) error {
	return nil
}
func (BaseLifecycleHook) AfterToolExecution(_ context.Context, _ *AgentRunContext, _ *ToolExecutionContext) {
}
func (BaseLifecycleHook) BeforeMessagePersist(_ context.Context, _ *AgentRunContext) error {
	return nil
}
func (BaseLifecycleHook) AfterMessagePersist(_ context.Context, _ *AgentRunContext) {}
func (BaseLifecycleHook) BeforeCompactionPersist(_ context.Context, _ *AgentRunContext) error {
	return nil
}
func (BaseLifecycleHook) AfterCompactionPersist(_ context.Context, _ *AgentRunContext) {}
