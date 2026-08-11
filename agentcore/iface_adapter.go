package agentcore

import (
	"context"
	"errors"

	"github.com/xujian519/mady/agentcore/iface"
)

// =============================================================================
// iface.LifecycleHook → agentcore.LifecycleHook 适配器
// =============================================================================

type ifaceLifecycleHookAdapter struct {
	BaseLifecycleHook
	inner iface.LifecycleHook
}

// NewIFaceLifecycleHook 将 iface.LifecycleHook 包装为 agentcore.LifecycleHook。
// guardrails 等外部模块导出 iface.LifecycleHook 后，通过此适配器注入 agentcore hook 链。
func NewIFaceLifecycleHook(h iface.LifecycleHook) LifecycleHook {
	if h == nil {
		return nil
	}
	return &ifaceLifecycleHookAdapter{inner: h}
}

func (a *ifaceLifecycleHookAdapter) BeforeAgentRun(ctx context.Context, arc *AgentRunContext) error {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	return a.inner.BeforeAgentRun(ctx, ifaceARC)
}

func (a *ifaceLifecycleHookAdapter) AfterAgentRun(ctx context.Context, arc *AgentRunContext, output string, err error) {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	a.inner.AfterAgentRun(ctx, ifaceARC, output, err)
}

func (a *ifaceLifecycleHookAdapter) BeforeTurn(ctx context.Context, arc *AgentRunContext) error {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	return a.inner.BeforeTurn(ctx, ifaceARC)
}

func (a *ifaceLifecycleHookAdapter) AfterTurn(ctx context.Context, arc *AgentRunContext, info TurnInfo) {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	// ToolCount 置 0：agentcore.TurnInfo 不提供本回合工具调用计数，len(arc.Messages) 是消息总数而非工具数。
	ifaceInfo := iface.TurnInfo{HadToolCalls: info.HadToolCalls, ToolCount: 0}
	a.inner.AfterTurn(ctx, ifaceARC, ifaceInfo)
}

func (a *ifaceLifecycleHookAdapter) BeforeModelCall(ctx context.Context, arc *AgentRunContext, mcc *ModelCallContext) error {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	ifaceMCC := &iface.ModelCallContext{}
	if mcc != nil && mcc.Request != nil {
		ifaceMCC.Model = mcc.Request.Model
		ifaceMCC.Messages = len(mcc.Request.Messages)
	}
	return a.inner.BeforeModelCall(ctx, ifaceARC, ifaceMCC)
}

func (a *ifaceLifecycleHookAdapter) AfterModelCall(ctx context.Context, arc *AgentRunContext, mcc *ModelCallContext) {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	ifaceMCC := &iface.ModelCallContext{}
	if mcc != nil && mcc.Request != nil {
		ifaceMCC.Model = mcc.Request.Model
		ifaceMCC.Messages = len(mcc.Request.Messages)
	}
	if mcc != nil && mcc.Response != nil {
		ifaceMCC.Content = mcc.Response.Content
		ifaceMCC.SuppressPersist = mcc.Response.SuppressPersist
		ifaceMCC.HasToolCalls = len(mcc.Response.ToolCalls) > 0
	}
	a.inner.AfterModelCall(ctx, ifaceARC, ifaceMCC)
	// Write-back: 将 iface 层修改同步回 agentcore 层
	if mcc != nil && mcc.Response != nil {
		if ifaceMCC.SuppressPersist {
			mcc.Response.SuppressPersist = true
		}
		if ifaceMCC.Content != mcc.Response.Content {
			mcc.Response.Content = ifaceMCC.Content
		}
		if ifaceMCC.Blocked {
			err := NewNodeError("内容安全检查未通过", nil, "guardrail", "blocked")
			mcc.Err = errors.Join(mcc.Err, err)
		}
	}
}

func (a *ifaceLifecycleHookAdapter) BeforeToolExecution(ctx context.Context, arc *AgentRunContext, tec *ToolExecutionContext) error {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	ifaceTEC := &iface.ToolExecutionContext{
		ToolCalls: len(tec.ToolCalls),
		ToolNames: toolNames(tec.ToolCalls),
	}
	return a.inner.BeforeToolExecution(ctx, ifaceARC, ifaceTEC)
}

func (a *ifaceLifecycleHookAdapter) AfterToolExecution(ctx context.Context, arc *AgentRunContext, tec *ToolExecutionContext) {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	ifaceTEC := &iface.ToolExecutionContext{
		ToolCalls: len(tec.ToolCalls),
		ToolNames: toolNames(tec.ToolCalls),
	}
	a.inner.AfterToolExecution(ctx, ifaceARC, ifaceTEC)
}

func (a *ifaceLifecycleHookAdapter) BeforeMessagePersist(ctx context.Context, arc *AgentRunContext, _ *Message) error {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	return a.inner.BeforeMessagePersist(ctx, ifaceARC)
}

func (a *ifaceLifecycleHookAdapter) AfterMessagePersist(ctx context.Context, arc *AgentRunContext, _ Message) {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	a.inner.AfterMessagePersist(ctx, ifaceARC)
}

func (a *ifaceLifecycleHookAdapter) BeforeCompactionPersist(ctx context.Context, arc *AgentRunContext, msgs []Message) ([]Message, error) {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	err := a.inner.BeforeCompactionPersist(ctx, ifaceARC)
	return msgs, err
}

func (a *ifaceLifecycleHookAdapter) AfterCompactionPersist(ctx context.Context, arc *AgentRunContext, _ []Message) {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	a.inner.AfterCompactionPersist(ctx, ifaceARC)
}

// toolNames 从 ToolCall 切片提取名称列表。
func toolNames(calls []ToolCall) []string {
	names := make([]string, len(calls))
	for i, tc := range calls {
		names[i] = tc.Name
	}
	return names
}
