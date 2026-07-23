package agentcore

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/xujian519/mady/agentcore/iface"
)

// =============================================================================
// iface.AgentRunner 适配器
// =============================================================================

type agentRunnerAdapter struct {
	inner *Agent
}

// NewAgentRunner 将 *Agent 包装为 iface.AgentRunner。
func NewAgentRunner(a *Agent) iface.AgentRunner {
	if a == nil {
		return nil
	}
	return &agentRunnerAdapter{inner: a}
}

func (a *agentRunnerAdapter) Run(ctx context.Context, input string) (string, error) {
	return a.inner.Run(ctx, input)
}

func (a *agentRunnerAdapter) Continue(ctx context.Context) (string, error) {
	return a.inner.Continue(ctx)
}

func (a *agentRunnerAdapter) Resume(ctx context.Context, interruptData map[string]any) (string, error) {
	if len(interruptData) > 0 {
		slog.Warn("iface_adapter: Resume interruptData is not forwarded to agentcore Agent")
	}
	return a.inner.Resume(ctx)
}

func (a *agentRunnerAdapter) Close() {
	a.inner.Close()
}

func (a *agentRunnerAdapter) State() iface.AgentState {
	s := a.inner.State()
	st := s.Status()
	status := iface.StatusIdle
	switch st {
	case StatusRunning:
		status = iface.StatusRunning
	case StatusFinished:
		status = iface.StatusFinished
	case StatusError:
		status = iface.StatusError
	case StatusInterrupted:
		status = iface.StatusInterrupted
	}
	return iface.AgentState{
		Status:    status,
		TurnCount: s.Turn(),
	}
}

// =============================================================================
// iface.EventBus 适配器
// =============================================================================

type eventBusAdapter struct {
	inner *EventBus
}

// NewIFaceEventBus 将 *EventBus 包装为 iface.EventBus。
func NewIFaceEventBus(eb *EventBus) iface.EventBus {
	if eb == nil {
		return nil
	}
	return &eventBusAdapter{inner: eb}
}

func (w *eventBusAdapter) On(eventType iface.EventType, handler iface.EventHandler) func() {
	return w.inner.On(EventType(eventType), func(e Event) {
		handler(wrapIFaceEvent(e))
	})
}

func (w *eventBusAdapter) OnAll(handler iface.EventHandler) func() {
	return w.inner.OnAll(func(e Event) {
		handler(wrapIFaceEvent(e))
	})
}

func (w *eventBusAdapter) Emit(event iface.Event) {
	if p := event.Payload(); p != nil {
		// 如果有 payload，用 payloadEvent 包装以保留原始事件体
		pe := &payloadEvent{
			baseEvent: baseEvent{Kind: EventType(event.Type()), At: time.Now()},
			payload:   p,
		}
		w.inner.Emit(pe)
		return
	}
	w.inner.Emit(baseEvent{Kind: EventType(event.Type()), At: time.Now()})
}

func (w *eventBusAdapter) EmitMustDeliver(ctx context.Context, event iface.Event) {
	if p := event.Payload(); p != nil {
		pe := &payloadEvent{
			baseEvent: baseEvent{Kind: EventType(event.Type()), At: time.Now()},
			payload:   p,
		}
		w.inner.EmitMustDeliver(ctx, pe)
		return
	}
	w.inner.EmitMustDeliver(ctx, baseEvent{Kind: EventType(event.Type()), At: time.Now()})
}

func (w *eventBusAdapter) Close() {
	w.inner.Close()
}

// wrapIFaceEvent 将 agentcore.Event 包装为 iface.Event。
func wrapIFaceEvent(e Event) iface.Event {
	return &ifaceWrappedEvent{inner: e}
}

type ifaceWrappedEvent struct {
	inner Event
}

func (w *ifaceWrappedEvent) Type() iface.EventType {
	return iface.EventType(w.inner.EventKind())
}

func (w *ifaceWrappedEvent) Payload() any {
	if pe, ok := w.inner.(*payloadEvent); ok {
		return pe.payload
	}
	return w.inner
}

// payloadEvent 是 agentcore.Event 的扩展，携带通过 iface 适配器传入的 payload。
type payloadEvent struct {
	baseEvent
	payload any
}

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

func (a *ifaceLifecycleHookAdapter) BeforeMessagePersist(ctx context.Context, arc *AgentRunContext, msg *Message) error {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	return a.inner.BeforeMessagePersist(ctx, ifaceARC)
}

func (a *ifaceLifecycleHookAdapter) AfterMessagePersist(ctx context.Context, arc *AgentRunContext, msg Message) {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	a.inner.AfterMessagePersist(ctx, ifaceARC)
}

func (a *ifaceLifecycleHookAdapter) BeforeCompactionPersist(ctx context.Context, arc *AgentRunContext, msgs []Message) ([]Message, error) {
	ifaceARC := &iface.AgentRunContext{Input: arc.Input, TurnCount: arc.Turn}
	err := a.inner.BeforeCompactionPersist(ctx, ifaceARC)
	return msgs, err
}

func (a *ifaceLifecycleHookAdapter) AfterCompactionPersist(ctx context.Context, arc *AgentRunContext, msgs []Message) {
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
