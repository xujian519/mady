package chat

// state_test.go pins the ChatApp FSM with a table-driven test of every
// transition. Adding a new state or event means adding a row here, which
// forces the change to be deliberate rather than incidental.

import (
	"testing"
)

func TestTransitionFSM(t *testing.T) {
	cases := []struct {
		name  string
		from  AppState
		event eventKind
		want  AppState
	}{
		// Initializing
		{"initializing + agentReady → idle", StateInitializing, evtAgentReady, StateIdle},
		{"initializing + agentStart → streaming", StateInitializing, evtAgentStart, StateStreaming},
		{"initializing + agentError → failed", StateInitializing, evtAgentError, StateFailed},
		{"initializing + approvalRequest → awaiting", StateInitializing, evtApprovalRequest, StateAwaitingConfirm},
		{"initializing + delta → initializing (steady)", StateInitializing, evtMessageDelta, StateInitializing},

		// Idle
		{"idle + agentStart → streaming", StateIdle, evtAgentStart, StateStreaming},
		{"idle + delta → idle (no stream started)", StateIdle, evtMessageDelta, StateIdle},
		{"idle + approval request → awaiting", StateIdle, evtApprovalRequest, StateAwaitingConfirm},

		// Streaming
		{"streaming + delta → streaming", StateStreaming, evtMessageDelta, StateStreaming},
		{"streaming + toolStart → tool-running", StateStreaming, evtToolStart, StateToolRunning},
		{"streaming + compactionStart → compacting", StateStreaming, evtCompactionStart, StateCompacting},
		{"streaming + approvalRequest → awaiting", StateStreaming, evtApprovalRequest, StateAwaitingConfirm},
		{"streaming + agentEnd → idle", StateStreaming, evtAgentEnd, StateIdle},
		{"streaming + agentError → idle", StateStreaming, evtAgentError, StateIdle},
		{"streaming + turnEnd → streaming (boundary)", StateStreaming, evtTurnEnd, StateStreaming},
		{"streaming + handoffStart → streaming", StateStreaming, evtHandoffStart, StateStreaming},

		// ToolRunning
		{"tool + toolEnd → streaming", StateToolRunning, evtToolEnd, StateStreaming},
		{"tool + compactionStart → compacting", StateToolRunning, evtCompactionStart, StateCompacting},
		{"tool + approvalRequest → awaiting", StateToolRunning, evtApprovalRequest, StateAwaitingConfirm},
		{"tool + agentError → idle", StateToolRunning, evtAgentError, StateIdle},

		// Compacting
		{"compacting + compactionEnd → streaming", StateCompacting, evtCompactionEnd, StateStreaming},
		{"compacting + agentEnd → idle", StateCompacting, evtAgentEnd, StateIdle},
		{"compacting + delta → compacting (steady)", StateCompacting, evtMessageDelta, StateCompacting},
		{"compacting + approvalRequest → awaiting", StateCompacting, evtApprovalRequest, StateAwaitingConfirm},

		// AwaitingConfirm
		{"awaiting + approvalDecision → streaming", StateAwaitingConfirm, evtApprovalDecision, StateStreaming},
		{"awaiting + agentError → idle", StateAwaitingConfirm, evtAgentError, StateIdle},
		{"awaiting + delta → awaiting (steady)", StateAwaitingConfirm, evtMessageDelta, StateAwaitingConfirm},

		// Failed
		{"failed + agentReady → idle", StateFailed, evtAgentReady, StateIdle},
		{"failed + agentStart → streaming (retry)", StateFailed, evtAgentStart, StateStreaming},
		{"failed + approvalRequest → awaiting", StateFailed, evtApprovalRequest, StateAwaitingConfirm},
		{"failed + delta → failed (steady)", StateFailed, evtMessageDelta, StateFailed},
	}
	for _, c := range cases {
		got := Transition(c.from, c.event)
		if got != c.want {
			t.Errorf("%s: Transition(%s, evt=%d) = %s, want %s",
				c.name, c.from, c.event, got, c.want)
		}
	}
}

func TestEventKindForMapsChatEvents(t *testing.T) {
	cases := []struct {
		evt  ChatEvent
		want eventKind
	}{
		{AgentStartChatEvent{}, evtAgentStart},
		{MessageDeltaChatEvent{}, evtMessageDelta},
		{ToolCallStartChatEvent{}, evtToolStart},
		{ToolCallEndChatEvent{}, evtToolEnd},
		{TurnStartChatEvent{}, evtTurnStart},
		{TurnEndChatEvent{}, evtTurnEnd},
		{AgentEndChatEvent{}, evtAgentEnd},
		{AgentErrorChatEvent{Err: nil}, evtAgentError},
		{CompactionStartChatEvent{}, evtCompactionStart},
		{CompactionEndChatEvent{}, evtCompactionEnd},
		{HandoffStartChatEvent{}, evtHandoffStart},
		{HandoffEndChatEvent{}, evtHandoffEnd},
		{AutoRetryChatEvent{}, evtAutoRetry},
	}
	for _, c := range cases {
		if got := EventKindFor(c.evt); got != c.want {
			t.Errorf("EventKindFor(%T) = %d, want %d", c.evt, got, c.want)
		}
	}
}

// TestEventKindForUnknownIsNoOp confirms the default mapping (evtUnknown) is a
// genuine no-op in every state — it must never flip Idle→Streaming the way a
// careless evtAgentStart default would.
func TestEventKindForUnknownIsNoOp(t *testing.T) {
	for _, s := range []AppState{StateInitializing, StateIdle, StateStreaming, StateToolRunning, StateCompacting, StateAwaitingConfirm, StateFailed} {
		if got := Transition(s, evtUnknown); got != s {
			t.Errorf("evtUnknown should be a no-op in %s, got %s", s, got)
		}
	}
	// And a truly unknown ChatEvent maps to evtUnknown (not evtAgentStart).
	ue := unknownTestEvent{}
	if got := EventKindFor(ue); got != evtUnknown {
		t.Errorf("EventKindFor(unknown) = %d, want evtUnknown", got)
	}
}

type unknownTestEvent struct{}

func (unknownTestEvent) ChatEventKind() ChatEventType { return "unknown-test-event" }

func TestAppStateString(t *testing.T) {
	want := map[AppState]string{
		StateInitializing:    "initializing",
		StateIdle:            "idle",
		StateStreaming:       "streaming",
		StateToolRunning:     "tool-running",
		StateAwaitingConfirm: "awaiting-confirm",
		StateCompacting:      "compacting",
		StateFailed:          "failed",
	}
	for s, w := range want {
		if s.String() != w {
			t.Errorf("AppState(%d).String() = %q, want %q", s, s.String(), w)
		}
	}
}
