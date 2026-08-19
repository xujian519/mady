package agentadapter

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
)

// recorder is a minimal chat.EventSubscriber capture sink. It records the
// single ChatEvent dispatched to its handler and whether the handler fired
// at all. The "called" flag lets mismatch tests assert the adapter's `!ok`
// early-return guard suppressed dispatch.
type recorder struct {
	got    chat.ChatEvent
	called bool
}

func (r *recorder) handle() func(chat.ChatEvent) {
	return func(ce chat.ChatEvent) {
		r.got = ce
		r.called = true
	}
}

// eventCase describes one happy-path agentcore→ChatEvent conversion.
// coreKind mirrors chatType so the mismatch table can reuse the same rows.
type eventCase struct {
	name     string
	chatType chat.ChatEventType
	coreKind agentcore.EventType
	event    agentcore.Event
	assert   func(t *testing.T, ce chat.ChatEvent)
}

// eventCases returns the full matrix of event mappings handled by the
// adapter's On switch. Both TestEventMapping and TestEventMapping_TypeMismatch
// iterate it, so every mapping is exercised in both the happy and the
// type-guard paths.
func eventCases() []eventCase {
	return []eventCase{
		{
			name:     "AgentStart",
			chatType: chat.ChatEventAgentStart,
			coreKind: agentcore.EventAgentStart,
			event:    agentcore.NewAgentStartEvent("patent-agent", "analyze claim 1"),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.AgentStartChatEvent)
				if !ok {
					t.Fatalf("want AgentStartChatEvent, got %T", ce)
				}
				if got.AgentName != "patent-agent" || got.Input != "analyze claim 1" {
					t.Fatalf("unexpected fields: %+v", got)
				}
				if got.ChatEventKind() != chat.ChatEventAgentStart {
					t.Fatalf("kind = %q, want %q", got.ChatEventKind(), chat.ChatEventAgentStart)
				}
			},
		},
		{
			name:     "AgentEnd",
			chatType: chat.ChatEventAgentEnd,
			coreKind: agentcore.EventAgentEnd,
			event:    agentcore.NewAgentEndEvent("legal-agent", "summary output"),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.AgentEndChatEvent)
				if !ok {
					t.Fatalf("want AgentEndChatEvent, got %T", ce)
				}
				if got.AgentName != "legal-agent" || got.Output != "summary output" {
					t.Fatalf("unexpected fields: %+v", got)
				}
				if got.ChatEventKind() != chat.ChatEventAgentEnd {
					t.Fatalf("kind = %q", got.ChatEventKind())
				}
			},
		},
		{
			name:     "AgentError",
			chatType: chat.ChatEventAgentError,
			coreKind: agentcore.EventAgentError,
			event:    agentcore.NewAgentErrorEvent(errors.New("boom")),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.AgentErrorChatEvent)
				if !ok {
					t.Fatalf("want AgentErrorChatEvent, got %T", ce)
				}
				if got.Err == nil || got.Err.Error() != "boom" {
					t.Fatalf("unexpected err: %+v", got)
				}
			},
		},
		{
			name:     "AgentInterrupt_NilReason",
			chatType: chat.ChatEventAgentInterrupt,
			coreKind: agentcore.EventAgentInterrupt,
			event:    agentcore.NewAgentInterruptEvent("mady", nil),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.AgentInterruptChatEvent)
				if !ok {
					t.Fatalf("want AgentInterruptChatEvent, got %T", ce)
				}
				if got.Reason != "" {
					t.Fatalf("want empty Reason, got %q", got.Reason)
				}
				if got.Data != nil {
					t.Fatalf("want nil Data, got %+v", got.Data)
				}
			},
		},
		{
			name:     "AgentInterrupt_WithReason",
			chatType: chat.ChatEventAgentInterrupt,
			coreKind: agentcore.EventAgentInterrupt,
			event: agentcore.NewAgentInterruptEvent("mady", &agentcore.InterruptReason{
				Reason: "review_gate",
				Data:   map[string]any{"report_id": "r-42", "count": float64(3)},
			}),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.AgentInterruptChatEvent)
				if !ok {
					t.Fatalf("want AgentInterruptChatEvent, got %T", ce)
				}
				if got.Reason != "review_gate" {
					t.Fatalf("Reason = %q", got.Reason)
				}
				if got.Data["report_id"] != "r-42" {
					t.Fatalf("Data = %+v", got.Data)
				}
				if got.Data["count"] != float64(3) {
					t.Fatalf("count not preserved: %+v", got.Data)
				}
			},
		},
		{
			name:     "TurnStart",
			chatType: chat.ChatEventTurnStart,
			coreKind: agentcore.EventTurnStart,
			event:    agentcore.NewTurnStartEvent(7),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.TurnStartChatEvent)
				if !ok {
					t.Fatalf("want TurnStartChatEvent, got %T", ce)
				}
				if got.Turn != 7 {
					t.Fatalf("Turn = %d", got.Turn)
				}
			},
		},
		{
			name:     "TurnEnd",
			chatType: chat.ChatEventTurnEnd,
			coreKind: agentcore.EventTurnEnd,
			event:    agentcore.NewTurnEndEvent(3, agentcore.TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.TurnEndChatEvent)
				if !ok {
					t.Fatalf("want TurnEndChatEvent, got %T", ce)
				}
				if got.Turn != 3 {
					t.Fatalf("Turn = %d", got.Turn)
				}
				if got.Usage.PromptTokens != 10 || got.Usage.CompletionTokens != 20 || got.Usage.TotalTokens != 30 {
					t.Fatalf("Usage = %+v", got.Usage)
				}
			},
		},
		{
			name:     "MessageDelta_Text",
			chatType: chat.ChatEventMessageDelta,
			coreKind: agentcore.EventMessageDelta,
			event:    agentcore.NewMessageDeltaEvent("hello", agentcore.BlockKindText),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.MessageDeltaChatEvent)
				if !ok {
					t.Fatalf("want MessageDeltaChatEvent, got %T", ce)
				}
				if got.Delta != "hello" || got.Kind != "text" {
					t.Fatalf("unexpected: %+v", got)
				}
			},
		},
		{
			name:     "MessageDelta_Thinking",
			chatType: chat.ChatEventMessageDelta,
			coreKind: agentcore.EventMessageDelta,
			event:    agentcore.NewMessageDeltaEvent("pondering", agentcore.BlockKindThinking),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.MessageDeltaChatEvent)
				if !ok {
					t.Fatalf("want MessageDeltaChatEvent, got %T", ce)
				}
				if got.Delta != "pondering" || got.Kind != "thinking" {
					t.Fatalf("unexpected: %+v", got)
				}
			},
		},
		{
			name:     "ToolCallStart",
			chatType: chat.ChatEventToolCallStart,
			coreKind: agentcore.EventToolCallStart,
			event: agentcore.NewToolCallStartEvent(agentcore.ToolCall{
				ID: "call_1", Name: "search_patents", Arguments: `{"q":"半导体"}`,
			}),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.ToolCallStartChatEvent)
				if !ok {
					t.Fatalf("want ToolCallStartChatEvent, got %T", ce)
				}
				tc := got.ToolCall
				if tc.ID != "call_1" || tc.Name != "search_patents" || tc.Arguments != `{"q":"半导体"}` {
					t.Fatalf("ToolCall = %+v", tc)
				}
			},
		},
		{
			name:     "ToolCallEnd_Success",
			chatType: chat.ChatEventToolCallEnd,
			coreKind: agentcore.EventToolCallEnd,
			event:    agentcore.NewToolCallEndEvent("call_1", "search_patents", "12 results", nil, 250*time.Millisecond),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.ToolCallEndChatEvent)
				if !ok {
					t.Fatalf("want ToolCallEndChatEvent, got %T", ce)
				}
				if got.ToolCallID != "call_1" || got.ToolName != "search_patents" {
					t.Fatalf("ids = %+v", got)
				}
				if got.Result != "12 results" {
					t.Fatalf("Result = %q", got.Result)
				}
				if got.Err != nil {
					t.Fatalf("want nil Err, got %v", got.Err)
				}
				if got.Duration != 250*time.Millisecond {
					t.Fatalf("Duration = %v", got.Duration)
				}
			},
		},
		{
			name:     "ToolCallEnd_Error",
			chatType: chat.ChatEventToolCallEnd,
			coreKind: agentcore.EventToolCallEnd,
			event:    agentcore.NewToolCallEndEvent("call_2", "search_patents", "", errors.New("timeout"), 0),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.ToolCallEndChatEvent)
				if !ok {
					t.Fatalf("want ToolCallEndChatEvent, got %T", ce)
				}
				if got.Err == nil || got.Err.Error() != "timeout" {
					t.Fatalf("Err = %v", got.Err)
				}
				if got.Result != "" {
					t.Fatalf("want empty Result, got %q", got.Result)
				}
			},
		},
		{
			name:     "HandoffStart_Visible",
			chatType: chat.ChatEventHandoffStart,
			coreKind: agentcore.EventHandoffStart,
			event:    agentcore.NewHandoffStartEvent("mady", "patent", "delegate", "claim analysis", false),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.HandoffStartChatEvent)
				if !ok {
					t.Fatalf("want HandoffStartChatEvent, got %T", ce)
				}
				if got.SourceAgent != "mady" || got.TargetAgent != "patent" {
					t.Fatalf("agents = %+v", got)
				}
				if got.Mode != "delegate" || got.Context != "claim analysis" {
					t.Fatalf("mode/context = %+v", got)
				}
				if got.Invisible {
					t.Fatal("want Visible (Invisible=false)")
				}
			},
		},
		{
			name:     "HandoffStart_Invisible",
			chatType: chat.ChatEventHandoffStart,
			coreKind: agentcore.EventHandoffStart,
			event:    agentcore.NewHandoffStartEvent("mady", "legal", "transfer", "silent ctx", true),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.HandoffStartChatEvent)
				if !ok {
					t.Fatalf("want HandoffStartChatEvent, got %T", ce)
				}
				if !got.Invisible {
					t.Fatal("want Invisible=true")
				}
				if got.TargetAgent != "legal" {
					t.Fatalf("TargetAgent = %q", got.TargetAgent)
				}
			},
		},
		{
			name:     "HandoffEnd_Success",
			chatType: chat.ChatEventHandoffEnd,
			coreKind: agentcore.EventHandoffEnd,
			event:    agentcore.NewHandoffEndEvent("patent", "analysis done", 2*time.Second, nil, false),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.HandoffEndChatEvent)
				if !ok {
					t.Fatalf("want HandoffEndChatEvent, got %T", ce)
				}
				if got.TargetAgent != "patent" || got.Output != "analysis done" {
					t.Fatalf("fields = %+v", got)
				}
				if got.Duration != 2*time.Second {
					t.Fatalf("Duration = %v", got.Duration)
				}
				if got.Err != nil {
					t.Fatalf("want nil Err, got %v", got.Err)
				}
				if got.Invisible {
					t.Fatal("want Invisible=false")
				}
			},
		},
		{
			name:     "HandoffEnd_Error",
			chatType: chat.ChatEventHandoffEnd,
			coreKind: agentcore.EventHandoffEnd,
			event:    agentcore.NewHandoffEndEvent("legal", "", 0, errors.New("handoff failed"), true),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.HandoffEndChatEvent)
				if !ok {
					t.Fatalf("want HandoffEndChatEvent, got %T", ce)
				}
				if got.Err == nil || got.Err.Error() != "handoff failed" {
					t.Fatalf("Err = %v", got.Err)
				}
				if !got.Invisible {
					t.Fatal("want Invisible=true")
				}
			},
		},
		{
			name:     "CompactionStart",
			chatType: chat.ChatEventCompactionStart,
			coreKind: agentcore.EventCompactionStart,
			event:    agentcore.NewCompactionStartEvent(9000, 8000),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.CompactionStartChatEvent)
				if !ok {
					t.Fatalf("want CompactionStartChatEvent, got %T", ce)
				}
				if got.TokensBefore != 9000 || got.ContextWindow != 8000 {
					t.Fatalf("fields = %+v", got)
				}
			},
		},
		{
			name:     "CompactionEnd",
			chatType: chat.ChatEventCompactionEnd,
			coreKind: agentcore.EventCompactionEnd,
			event:    agentcore.NewCompactionEndEvent(9000, 4000, 5, 150*time.Millisecond),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.CompactionEndChatEvent)
				if !ok {
					t.Fatalf("want CompactionEndChatEvent, got %T", ce)
				}
				if got.TokensBefore != 9000 || got.TokensAfter != 4000 || got.MessagesCut != 5 {
					t.Fatalf("fields = %+v", got)
				}
				if got.Duration != 150*time.Millisecond {
					t.Fatalf("Duration = %v", got.Duration)
				}
			},
		},
		{
			name:     "AutoRetry",
			chatType: chat.ChatEventAutoRetry,
			coreKind: agentcore.EventAutoRetry,
			event:    agentcore.NewAutoRetryEvent(2, 5, 500*time.Millisecond, errors.New("rate limited")),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.AutoRetryChatEvent)
				if !ok {
					t.Fatalf("want AutoRetryChatEvent, got %T", ce)
				}
				if got.Attempt != 2 || got.MaxRetries != 5 {
					t.Fatalf("attempt/max = %+v", got)
				}
				if got.Delay != 500*time.Millisecond {
					t.Fatalf("Delay = %v", got.Delay)
				}
				if got.Err == nil || got.Err.Error() != "rate limited" {
					t.Fatalf("Err = %v", got.Err)
				}
			},
		},
		{
			name:     "ApprovalPrompt_NoData",
			chatType: chat.ChatEventApprovalPrompt,
			coreKind: agentcore.EventApprovalPrompt,
			event:    agentcore.NewApprovalPromptEvent("mady", "approve tool?", nil),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.ApprovalPromptChatEvent)
				if !ok {
					t.Fatalf("want ApprovalPromptChatEvent, got %T", ce)
				}
				if got.Content != "approve tool?" {
					t.Fatalf("Content = %q", got.Content)
				}
				if got.Data != nil {
					t.Fatalf("want nil Data, got %+v", got.Data)
				}
			},
		},
		{
			name:     "ApprovalPrompt_WithData",
			chatType: chat.ChatEventApprovalPrompt,
			coreKind: agentcore.EventApprovalPrompt,
			event: func() agentcore.Event {
				ev := agentcore.NewApprovalPromptEvent("mady", "review gate", nil)
				ev.Data = map[string]any{
					"title":      "证据复核",
					"judgment":   "通过",
					"confidence": 0.85,
					"evidences": []any{
						map[string]any{"id": "e1", "title": "证据1", "role": "核心证据", "summary": "摘要", "status": float64(1)},
					},
					"checklist": []any{
						map[string]any{"label": "项1", "checked": true},
					},
					"risks": []any{"风险A"},
				}
				return ev
			}(),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.ApprovalPromptChatEvent)
				if !ok {
					t.Fatalf("want ApprovalPromptChatEvent, got %T", ce)
				}
				if got.Content != "review gate" {
					t.Fatalf("Content = %q", got.Content)
				}
				if got.Data == nil {
					t.Fatal("want non-nil parsed Data")
				}
				if got.Data.Title != "证据复核" || got.Data.Judgment != "通过" || got.Data.Confidence != 0.85 {
					t.Fatalf("header = %+v", got.Data)
				}
				if len(got.Data.Evidences) != 1 || got.Data.Evidences[0].ID != "e1" ||
					got.Data.Evidences[0].Status != component.EvidenceConfirmed {
					t.Fatalf("Evidences = %+v", got.Data.Evidences)
				}
				if len(got.Data.Checklist) != 1 || !got.Data.Checklist[0].Checked {
					t.Fatalf("Checklist = %+v", got.Data.Checklist)
				}
				if len(got.Data.Risks) != 1 || got.Data.Risks[0] != "风险A" {
					t.Fatalf("Risks = %+v", got.Data.Risks)
				}
			},
		},
		{
			name:     "TaskCreated",
			chatType: chat.ChatEventTaskCreated,
			coreKind: agentcore.EventTaskCreated,
			event: agentcore.NewTaskCreatedEvent(&agentcore.Task{
				ID: "1", Subject: "检索现有技术",
				Status: agentcore.TaskInProgress, Priority: agentcore.TaskPriorityHigh,
			}),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.TaskCreatedChatEvent)
				if !ok {
					t.Fatalf("want TaskCreatedChatEvent, got %T", ce)
				}
				if got.Task == nil {
					t.Fatal("want non-nil Task")
				}
				if got.Task.ID != "1" || got.Task.Subject != "检索现有技术" ||
					got.Task.Status != "in_progress" || got.Task.Priority != "high" {
					t.Fatalf("Task = %+v", got.Task)
				}
			},
		},
		{
			name:     "TaskUpdated",
			chatType: chat.ChatEventTaskUpdated,
			coreKind: agentcore.EventTaskUpdated,
			event: agentcore.NewTaskUpdatedEvent(&agentcore.Task{
				ID: "2", Subject: "撰写权利要求",
				Status: agentcore.TaskCompleted, Priority: agentcore.TaskPriorityNormal,
			}, "in_progress", "completed"),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.TaskUpdatedChatEvent)
				if !ok {
					t.Fatalf("want TaskUpdatedChatEvent, got %T", ce)
				}
				if got.Task == nil {
					t.Fatal("want non-nil Task")
				}
				if got.Task.ID != "2" || got.Task.Status != "completed" || got.Task.Priority != "normal" {
					t.Fatalf("Task = %+v", got.Task)
				}
			},
		},
		{
			name:     "PlanTaskFeedbackAdded",
			chatType: chat.ChatEventPlanTaskFeedbackAdded,
			coreKind: agentcore.EventPlanTaskFeedbackAdded,
			event:    agentcore.NewPlanTaskFeedbackAddedEvent("sess-9", "用户补充的背景技术", "step-3"),
			assert: func(t *testing.T, ce chat.ChatEvent) {
				got, ok := ce.(chat.PlanTaskFeedbackAddedChatEvent)
				if !ok {
					t.Fatalf("want PlanTaskFeedbackAddedChatEvent, got %T", ce)
				}
				if got.SessionID != "sess-9" || got.Text != "用户补充的背景技术" || got.StepID != "step-3" {
					t.Fatalf("unexpected fields: %+v", got)
				}
				if got.ChatEventKind() != chat.ChatEventPlanTaskFeedbackAdded {
					t.Fatalf("kind = %v, want %v", got.ChatEventKind(), chat.ChatEventPlanTaskFeedbackAdded)
				}
			},
		},
	}
}

// TestEventMapping walks every agentcore→ChatEvent conversion the adapter
// knows about, asserts the resulting ChatEvent type, kind, and key fields.
func TestEventMapping(t *testing.T) {
	for _, tc := range eventCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			agent := newAgentWithBus()
			sub := &mockSubscriber{}
			BindAgent(sub, agent)

			rec := &recorder{}
			sub.sub.On(tc.chatType, rec.handle())

			emitAndWait(t, agent, tc.event)

			if !rec.called {
				t.Fatalf("adapter did not emit %s for %s", tc.chatType, tc.coreKind)
			}
			if rec.got.ChatEventKind() != tc.chatType {
				t.Fatalf("ChatEventKind = %q, want %q", rec.got.ChatEventKind(), tc.chatType)
			}
			tc.assert(t, rec.got)
		})
	}
}

// TestEventMapping_TypeMismatch drives each registered handler with an event
// whose EventKind matches but whose concrete type does not. The adapter must
// silently drop it (the `!ok` early return) and never invoke the chat handler.
func TestEventMapping_TypeMismatch(t *testing.T) {
	for _, tc := range eventCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			agent := newAgentWithBus()
			sub := &mockSubscriber{}
			BindAgent(sub, agent)

			rec := &recorder{}
			sub.sub.On(tc.chatType, rec.handle())

			// A stub event with the matching kind but the wrong concrete type.
			emitAndWait(t, agent, testGateEvent{kind: tc.coreKind, at: time.Now()})

			if rec.called {
				t.Fatalf("handler fired for mismatched type (%s): got %+v", tc.coreKind, rec.got)
			}
		})
	}
}

// TestBindAgent_UnknownChatType verifies that subscribing to a ChatEventType
// the adapter does not handle is a safe no-op (no panic, no registration).
func TestBindAgent_UnknownChatType(t *testing.T) {
	agent := newAgentWithBus()
	sub := &mockSubscriber{}
	BindAgent(sub, agent)

	rec := &recorder{}
	sub.sub.On(chat.ChatEventType(99), rec.handle())

	// Emitting a real agentcore event must not reach the bogus handler.
	emitAndWait(t, agent, agentcore.NewTurnStartEvent(1))
	if rec.called {
		t.Fatal("handler for unknown chat type should never fire")
	}
}

// TestParseReviewGateData exercises every branch of the review-gate data
// parser: nil/empty/missing-judgment early returns, full payload decode, and
// per-field type-assertion fallbacks (non-map evidences, non-bool checklist,
// non-string risks, non-float confidence/status).
func TestParseReviewGateData(t *testing.T) {
	cases := []struct {
		name string
		data map[string]any
		want *chat.ReviewGatePayload
	}{
		{
			name: "nil data returns nil",
			data: nil,
			want: nil,
		},
		{
			name: "empty map returns nil",
			data: map[string]any{},
			want: nil,
		},
		{
			name: "no judgment key returns nil",
			data: map[string]any{"title": "t", "confidence": 0.5},
			want: nil,
		},
		{
			name: "empty judgment returns nil",
			data: map[string]any{"judgment": ""},
			want: nil,
		},
		{
			name: "judgment only yields minimal payload",
			data: map[string]any{"judgment": "pass"},
			want: &chat.ReviewGatePayload{Judgment: "pass"},
		},
		{
			name: "judgment with non-float confidence defaults to 0",
			data: map[string]any{"judgment": "pass", "confidence": 1},
			want: &chat.ReviewGatePayload{Judgment: "pass", Confidence: 0},
		},
		{
			name: "full payload with evidences checklist and risks",
			data: map[string]any{
				"title":      "证据复核",
				"judgment":   "通过",
				"confidence": 0.9,
				"evidences": []any{
					map[string]any{
						"id": "e1", "title": "证据1", "role": "核心证据",
						"summary": "摘要", "status": float64(2), // EvidenceDisputed
					},
					map[string]any{"id": "e2", "status": float64(0)}, // EvidencePending, minimal
				},
				"checklist": []any{
					map[string]any{"label": "项1", "checked": true},
					map[string]any{"label": "项2", "checked": false},
				},
				"risks": []any{"风险A", "风险B"},
			},
			want: &chat.ReviewGatePayload{
				Title:      "证据复核",
				Judgment:   "通过",
				Confidence: 0.9,
				Evidences: []component.ReviewEvidence{
					{ID: "e1", Title: "证据1", Role: "核心证据", Summary: "摘要", Status: component.EvidenceDisputed},
					{ID: "e2", Status: component.EvidencePending},
				},
				Checklist: []component.ReviewCheckItem{
					{Label: "项1", Checked: true},
					{Label: "项2", Checked: false},
				},
				Risks: []string{"风险A", "风险B"},
			},
		},
		{
			name: "non-map evidence entries are skipped",
			data: map[string]any{
				"judgment":  "pass",
				"evidences": []any{"not-a-map", 42, nil},
			},
			want: &chat.ReviewGatePayload{Judgment: "pass"},
		},
		{
			name: "non-map checklist entries are skipped",
			data: map[string]any{
				"judgment":  "pass",
				"checklist": []any{"oops", 7},
			},
			want: &chat.ReviewGatePayload{Judgment: "pass"},
		},
		{
			name: "non-string risk entries are skipped",
			data: map[string]any{
				"judgment": "pass",
				"risks":    []any{42, true, "valid-risk"},
			},
			want: &chat.ReviewGatePayload{Judgment: "pass", Risks: []string{"valid-risk"}},
		},
		{
			name: "evidence status as non-float defaults to EvidencePending",
			data: map[string]any{
				"judgment":  "pass",
				"evidences": []any{map[string]any{"id": "e1", "status": 2}}, // int, not float64
			},
			want: &chat.ReviewGatePayload{
				Judgment:  "pass",
				Evidences: []component.ReviewEvidence{{ID: "e1", Status: component.EvidencePending}},
			},
		},
		{
			name: "checklist checked as non-bool defaults to false",
			data: map[string]any{
				"judgment":  "pass",
				"checklist": []any{map[string]any{"label": "项1", "checked": "yes"}},
			},
			want: &chat.ReviewGatePayload{
				Judgment:  "pass",
				Checklist: []component.ReviewCheckItem{{Label: "项1", Checked: false}},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := parseReviewGateData(tc.data)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseReviewGateData mismatch\n got: %+v\nwant: %+v", got, tc.want)
			}
		})
	}
}

// TestAgentTaskToInfo covers the nil guard and field projection of the
// agentcore.Task → chat.TaskInfo converter.
func TestAgentTaskToInfo(t *testing.T) {
	cases := []struct {
		name string
		task *agentcore.Task
		want *chat.TaskInfo
	}{
		{
			name: "nil task returns nil",
			task: nil,
			want: nil,
		},
		{
			name: "fully populated task",
			task: &agentcore.Task{
				ID: "1", Subject: "检索现有技术",
				Status: agentcore.TaskInProgress, Priority: agentcore.TaskPriorityUrgent,
			},
			want: &chat.TaskInfo{
				ID: "1", Subject: "检索现有技术",
				Status: "in_progress", Priority: "urgent",
			},
		},
		{
			name: "empty status and priority project to empty strings",
			task: &agentcore.Task{ID: "2", Subject: "待定"},
			want: &chat.TaskInfo{ID: "2", Subject: "待定", Status: "", Priority: ""},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := agentTaskToInfo(tc.task)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("agentTaskToInfo mismatch\n got: %+v\nwant: %+v", got, tc.want)
			}
		})
	}
}

// TestConvertUsage_Table complements the existing convertUsage tests with a
// table covering populated, zero, and asymmetric values.
func TestConvertUsage_Table(t *testing.T) {
	cases := []struct {
		name string
		in   agentcore.TokenUsage
		want chat.TokenUsage
	}{
		{"zero", agentcore.TokenUsage{}, chat.TokenUsage{}},
		{"populated", agentcore.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			chat.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}},
		{"prompt only", agentcore.TokenUsage{PromptTokens: 7}, chat.TokenUsage{PromptTokens: 7}},
		{"negative values preserved", agentcore.TokenUsage{PromptTokens: -1, TotalTokens: -5},
			chat.TokenUsage{PromptTokens: -1, TotalTokens: -5}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := convertUsage(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("convertUsage mismatch\n got: %+v\nwant: %+v", got, tc.want)
			}
		})
	}
}
