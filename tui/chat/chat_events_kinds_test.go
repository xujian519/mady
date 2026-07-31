package chat

import (
	"testing"
)

// TestChatEventKindMethods verifies every event type reports its kind.
func TestChatEventKindMethods(t *testing.T) {
	cases := []struct {
		name string
		ev   ChatEvent
		want ChatEventType
	}{
		{"agent_start", AgentStartChatEvent{}, ChatEventAgentStart},
		{"agent_end", AgentEndChatEvent{}, ChatEventAgentEnd},
		{"agent_interrupt", AgentInterruptChatEvent{}, ChatEventAgentInterrupt},
		{"approval_prompt", ApprovalPromptChatEvent{}, ChatEventApprovalPrompt},
		{"agent_error", AgentErrorChatEvent{}, ChatEventAgentError},
		{"turn_start", TurnStartChatEvent{}, ChatEventTurnStart},
		{"turn_end", TurnEndChatEvent{}, ChatEventTurnEnd},
		{"message_delta", MessageDeltaChatEvent{}, ChatEventMessageDelta},
		{"tool_start", ToolCallStartChatEvent{}, ChatEventToolCallStart},
		{"tool_end", ToolCallEndChatEvent{}, ChatEventToolCallEnd},
		{"handoff_start", HandoffStartChatEvent{}, ChatEventHandoffStart},
		{"handoff_end", HandoffEndChatEvent{}, ChatEventHandoffEnd},
		{"compaction_start", CompactionStartChatEvent{}, ChatEventCompactionStart},
		{"compaction_end", CompactionEndChatEvent{}, ChatEventCompactionEnd},
		{"auto_retry", AutoRetryChatEvent{}, ChatEventAutoRetry},
		{"task_created", TaskCreatedChatEvent{}, ChatEventTaskCreated},
		{"task_updated", TaskUpdatedChatEvent{}, ChatEventTaskUpdated},
		{"skill_loaded", SkillLoadedChatEvent{}, ChatEventSkillLoaded},
		{"skills_reloaded", SkillsReloadedChatEvent{}, ChatEventSkillsReloaded},
		{"a2ui", A2UIChatEvent{}, ChatEventA2UI},
		{"plantask_status", PlanTaskStatusChangedChatEvent{}, ChatEventPlanTaskStatusChanged},
		{"plantask_feedback", PlanTaskFeedbackAddedChatEvent{}, ChatEventPlanTaskFeedbackAdded},
		{"plantask_interrupt", PlanTaskInterruptedChatEvent{}, ChatEventPlanTaskInterrupted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ev.ChatEventKind(); got != tc.want {
				t.Fatalf("ChatEventKind() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestChatEventTypeString verifies the display names, including unknown values.
func TestChatEventTypeString(t *testing.T) {
	if got := ChatEventAgentStart.String(); got != "agent_start" {
		t.Fatalf("String() = %q, want agent_start", got)
	}
	if got := ChatEventType(999).String(); got != "unknown" {
		t.Fatalf("unknown String() = %q, want unknown", got)
	}
	if got := ChatEventPlanTaskInterrupted.String(); got != "plantask_interrupted" {
		t.Fatalf("String() = %q, want plantask_interrupted", got)
	}
	// GoString delegates to String.
	if got := ChatEventAgentEnd.GoString(); got != "agent_end" {
		t.Fatalf("GoString() = %q, want agent_end", got)
	}
}

// TestOverlayHandleAccessors verifies the overlayHandle setters/getters.
func TestOverlayHandleAccessors(t *testing.T) {
	h := &overlayHandle{content: nil, focus: true, dimBackground: false, widthPct: 50, heightPct: 40}
	h.SetOverlayFocus(false)
	if h.OverlayWantsFocus() {
		t.Fatal("focus should be false after SetOverlayFocus(false)")
	}
	h.SetOverlayDimBackground(true)
	if !h.OverlayDimBackground() {
		t.Fatal("dim should be true after SetOverlayDimBackground(true)")
	}
	if h.OverlayAnchor() != 0 || h.OverlayPercentX() != 0 || h.OverlayPercentY() != 0 {
		t.Fatalf("anchor/percent defaults wrong: %+v", h)
	}
	if h.OverlayWidthPct() != 50 || h.OverlayHeightPct() != 40 {
		t.Fatalf("size = %d x %d", h.OverlayWidthPct(), h.OverlayHeightPct())
	}
	if h.OverlayContent() != nil {
		t.Fatal("content should be nil")
	}
	if h.OverlayCategory() != 0 {
		t.Fatalf("default category = %d, want 0", h.OverlayCategory())
	}
}

// TestSelectionBgFallback verifies the selection background fallback chain.
func TestSelectionBgFallback(t *testing.T) {
	bg := selectionBgFallback()
	if bg == "" {
		t.Fatal("fallback must return a non-empty SGR sequence")
	}
	if len(bg) < 2 || bg[0] != '\x1b' {
		t.Fatalf("expected ESC-prefixed sequence, got %q", bg)
	}
}
