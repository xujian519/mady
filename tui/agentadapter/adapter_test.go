package agentadapter

import (
	"errors"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tui/chat"
)

func TestConvertUsage(t *testing.T) {
	u := agentcore.TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}
	cu := convertUsage(u)
	if cu.PromptTokens != 100 || cu.CompletionTokens != 50 || cu.TotalTokens != 150 {
		t.Fatalf("convertUsage = %+v", cu)
	}
}

func TestConvertUsage_Zero(t *testing.T) {
	cu := convertUsage(agentcore.TokenUsage{})
	if cu.PromptTokens != 0 || cu.CompletionTokens != 0 || cu.TotalTokens != 0 {
		t.Fatalf("expected zero, got %+v", cu)
	}
}

func newAgentWithBus() *agentcore.Agent {
	a := &agentcore.Agent{}
	a.SetEventBus(agentcore.NewEventBus())
	return a
}

type testGateEvent struct {
	kind agentcore.EventType
	at   time.Time
}

func (e testGateEvent) EventKind() agentcore.EventType { return e.kind }
func (e testGateEvent) EventTime() time.Time           { return e.at }

type mockSubscriber struct {
	sub chat.EventSubscriber
}

func (s *mockSubscriber) Subscribe(sub chat.EventSubscriber) {
	s.sub = sub
}

func emitAndWait(t *testing.T, agent *agentcore.Agent, e agentcore.Event) {
	t.Helper()
	done := make(chan struct{})
	gateType := agentcore.EventType("test.gate")
	remove := agent.On(gateType, func(agentcore.Event) { close(done) })
	defer remove()
	agent.EmitEvent(e)
	agent.EmitEvent(testGateEvent{kind: gateType, at: time.Now()})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event bus to drain")
	}
}

func TestBindAgent(t *testing.T) {
	agent := newAgentWithBus()
	sub := &mockSubscriber{}
	BindAgent(sub, agent)
	if sub.sub == nil {
		t.Fatal("BindAgent did not register subscriber")
	}
}

func TestAdapterAgentStartMapping(t *testing.T) {
	agent := newAgentWithBus()
	sub := &mockSubscriber{}
	BindAgent(sub, agent)

	var got chat.AgentStartChatEvent
	sub.sub.On(chat.ChatEventAgentStart, func(ce chat.ChatEvent) {
		got = ce.(chat.AgentStartChatEvent)
	})

	emitAndWait(t, agent, agentcore.NewAgentStartEvent("patent-agent", "analyze claim 1"))
	if got.AgentName != "patent-agent" || got.Input != "analyze claim 1" {
		t.Fatalf("unexpected AgentStart event: %+v", got)
	}
}

func TestAdapterTurnEndMapping(t *testing.T) {
	agent := newAgentWithBus()
	sub := &mockSubscriber{}
	BindAgent(sub, agent)

	var got chat.TurnEndChatEvent
	sub.sub.On(chat.ChatEventTurnEnd, func(ce chat.ChatEvent) {
		got = ce.(chat.TurnEndChatEvent)
	})

	usage := agentcore.TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}
	emitAndWait(t, agent, agentcore.NewTurnEndEvent(7, usage))
	if got.Turn != 7 {
		t.Fatalf("want turn 7, got %d", got.Turn)
	}
	if got.Usage.TotalTokens != 30 {
		t.Fatalf("want total tokens 30, got %d", got.Usage.TotalTokens)
	}
}

func TestAdapterMessageDeltaMapping(t *testing.T) {
	agent := newAgentWithBus()
	sub := &mockSubscriber{}
	BindAgent(sub, agent)

	var got chat.MessageDeltaChatEvent
	sub.sub.On(chat.ChatEventMessageDelta, func(ce chat.ChatEvent) {
		got = ce.(chat.MessageDeltaChatEvent)
	})

	emitAndWait(t, agent, agentcore.NewMessageDeltaEvent("hello", agentcore.BlockKindText))
	if got.Delta != "hello" || got.Kind != string(agentcore.BlockKindText) {
		t.Fatalf("unexpected MessageDelta event: %+v", got)
	}
}

func TestAdapterAgentErrorMapping(t *testing.T) {
	agent := newAgentWithBus()
	sub := &mockSubscriber{}
	BindAgent(sub, agent)

	var got chat.AgentErrorChatEvent
	sub.sub.On(chat.ChatEventAgentError, func(ce chat.ChatEvent) {
		got = ce.(chat.AgentErrorChatEvent)
	})

	emitAndWait(t, agent, agentcore.NewAgentErrorEvent(errors.New("boom")))
	if got.Err == nil || got.Err.Error() != "boom" {
		t.Fatalf("unexpected AgentError event: %+v", got)
	}
}

// --- PlanTask HCL 事件映射 ---

// TestAdapterPlanTaskStatusMapping 验证 plantask 状态迁移事件映射到 Chat 层。
func TestAdapterPlanTaskStatusMapping(t *testing.T) {
	agent := newAgentWithBus()
	sub := &mockSubscriber{}
	BindAgent(sub, agent)

	var got chat.PlanTaskStatusChangedChatEvent
	sub.sub.On(chat.ChatEventPlanTaskStatusChanged, func(ce chat.ChatEvent) {
		got = ce.(chat.PlanTaskStatusChangedChatEvent)
	})

	emitAndWait(t, agent, agentcore.NewPlanTaskStatusChangedEvent("s1", "case1", "planning", "awaiting_approval"))
	if got.SessionID != "s1" || got.ToStatus != "awaiting_approval" {
		t.Fatalf("unexpected status event: %+v", got)
	}
}

// TestAdapterPlanTaskInterruptMapping 验证 plantask 中断事件映射到 Chat 层。
func TestAdapterPlanTaskInterruptMapping(t *testing.T) {
	agent := newAgentWithBus()
	sub := &mockSubscriber{}
	BindAgent(sub, agent)

	var got chat.PlanTaskInterruptedChatEvent
	sub.sub.On(chat.ChatEventPlanTaskInterrupted, func(ce chat.ChatEvent) {
		got = ce.(chat.PlanTaskInterruptedChatEvent)
	})

	emitAndWait(t, agent, agentcore.NewPlanTaskInterruptedEvent("s1", "step_2", "用户请求暂停"))
	if got.SessionID != "s1" || got.Reason != "用户请求暂停" {
		t.Fatalf("unexpected interrupt event: %+v", got)
	}
}
