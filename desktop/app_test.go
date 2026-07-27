package main

import (
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
)

func TestToKebabCase_RunStarted(t *testing.T) {
	got := toKebabCase("RUN_STARTED")
	if got != "run-started" {
		t.Errorf("toKebabCase(RUN_STARTED) = %q, want %q", got, "run-started")
	}
}

func TestToKebabCase_handoff_start(t *testing.T) {
	got := toKebabCase("handoff_start")
	if got != "handoff-start" {
		t.Errorf("toKebabCase(handoff_start) = %q, want %q", got, "handoff-start")
	}
}

func TestToKebabCase_empty(t *testing.T) {
	if got := toKebabCase(""); got != "" {
		t.Errorf("toKebabCase('') = %q, want ''", got)
	}
}

func TestMapAguiEvent_RunStarted(t *testing.T) {
	ev := agui.RunStartedEvent{
		BaseEvent: agui.BaseEvent{
			Type: agui.EventRunStarted,
		},
		ThreadID: "th-1",
		RunID:    "run-1",
	}
	name := mapAguiEventToWailsName(ev)
	if name != "agui:agent-start" {
		t.Errorf("mapAguiEventToWailsName(RunStartedEvent) = %q, want %q", name, "agui:agent-start")
	}
}

func TestMapAguiEvent_Custom(t *testing.T) {
	ev := agui.CustomEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventCustom},
		Name:      "a2ui",
		Value:     map[string]any{"kind": "createSurface"},
	}
	name := mapAguiEventToWailsName(ev)
	if name != "agui:a2ui" {
		t.Errorf("mapAguiEventToWailsName(CustomEvent{a2ui}) = %q, want %q", name, "agui:a2ui")
	}
}

func TestMapAguiEvent_HandoffEnd(t *testing.T) {
	ev := agui.CustomEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventCustom},
		Name:      "handoff_end",
		Value:     map[string]any{},
	}
	name := mapAguiEventToWailsName(ev)
	if name != "agui:handoff-end" {
		t.Errorf("mapAguiEventToWailsName(CustomEvent{handoff_end}) = %q, want %q", name, "agui:handoff-end")
	}
}

func TestMapAguiEvent_DefaultFallback(t *testing.T) {
	// 未显式处理的类型应通过 eventTypeOf 回退
	ev := agui.StepStartedEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventStepStarted},
		StepName:  "turn_1",
	}
	name := mapAguiEventToWailsName(ev)
	if name != "agui:step-started" {
		t.Errorf("mapAguiEventToWailsName(StepStartedEvent) = %q, want %q", name, "agui:step-started")
	}
}

func TestEventTypeOf(t *testing.T) {
	ev := agui.RunFinishedEvent{
		BaseEvent: agui.BaseEvent{
			Type:      agui.EventRunFinished,
			Timestamp: float64(time.Now().UnixMilli()),
		},
		ThreadID: "th-1",
		RunID:    "run-1",
	}
	typ := eventTypeOf(ev)
	if typ != agui.EventRunFinished {
		t.Errorf("eventTypeOf(RunFinishedEvent) = %q, want %q", typ, agui.EventRunFinished)
	}
}

func TestGenerateRunID(t *testing.T) {
	id := generateRunID()
	if len(id) < 10 {
		t.Errorf("generateRunID() = %q, too short", id)
	}
	// 格式验证：run-<unix_nano>
	if len(id) < 10 || id[:4] != "run-" {
		t.Errorf("generateRunID() = %q, should start with 'run-'", id)
	}
}

// ── 事件映射集成测试 ──────────────────────────────
//
// 验证 agentcore.Event → agui.Converter.Convert → mapAguiEventToWailsName
// 整条链路的正确性。

func TestAgentEventToWailsName(t *testing.T) {
	converter := agui.NewConverter("th-test", "run-test")

	tests := []struct {
		name  string
		event agentcore.Event
		want  string // 期望的 Wails 事件名
	}{
		{
			name:  "AgentStartEvent → agui:agent-start",
			event: &agentcore.AgentStartEvent{},
			want:  "agui:agent-start",
		},
		{
			name:  "MessageDeltaEvent → agui:message-delta",
			event: &agentcore.MessageDeltaEvent{Delta: "hello"},
			want:  "agui:message-delta",
		},
		{
			name:  "ToolCallStartEvent → agui:tool-call-start",
			event: &agentcore.ToolCallStartEvent{ToolCall: agentcore.ToolCall{ID: "tc-1", Name: "test"}},
			want:  "agui:tool-call-start",
		},
		{
			name:  "HandoffStartEvent → agui:handoff-start",
			event: &agentcore.HandoffStartEvent{TargetAgent: "patent"},
			want:  "agui:handoff-start",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aguiEvents := converter.Convert(tt.event)
			if len(aguiEvents) == 0 {
				t.Fatal("expected at least one AGUI event")
			}
			// 遍历所有 AGUI 事件，检查任一匹配
			for _, ev := range aguiEvents {
				got := mapAguiEventToWailsName(ev)
				if got == tt.want {
					return // 匹配成功
				}
			}
			// 未匹配：列出所有实际事件名
			var names []string
			for _, ev := range aguiEvents {
				names = append(names, mapAguiEventToWailsName(ev))
			}
			t.Errorf("no event matched %q; got %v", tt.want, names)
		})
	}
}

func TestEventMappingWithConverter_ThinkingThenText(t *testing.T) {
	converter := agui.NewConverter("th-1", "run-1")

	// Thinking delta → agui:thinking-delta
	thinkingEvents := converter.Convert(&agentcore.MessageDeltaEvent{
		Delta: "thinking step 1",
		Kind:  agentcore.BlockKindThinking,
	})
	found := false
	for _, ev := range thinkingEvents {
		if mapAguiEventToWailsName(ev) == "agui:thinking-delta" {
			found = true
			break
		}
	}
	if !found {
		t.Error("thinking delta event did not produce agui:thinking-delta")
	}

	// Text delta after thinking → agui:message-delta
	converter2 := agui.NewConverter("th-1", "run-1")
	textEvents := converter2.Convert(&agentcore.MessageDeltaEvent{
		Delta: "answer text",
		Kind:  agentcore.BlockKindText,
	})
	found = false
	for _, ev := range textEvents {
		if mapAguiEventToWailsName(ev) == "agui:message-delta" {
			found = true
			break
		}
	}
	if !found {
		t.Error("text delta event did not produce agui:message-delta")
	}
}

func TestEventMapping_HandoffInvisible(t *testing.T) {
	// Invisible Handoff 事件在前端必须静默过滤（由 client.ts 决策）
	// 此处验证映射产生正确的 Wails 事件名，前端 bridge 据此决定不渲染。
	converter := agui.NewConverter("th-1", "run-1")

	events := converter.Convert(&agentcore.HandoffStartEvent{
		TargetAgent: "patent",
	})
	if len(events) == 0 {
		t.Fatal("expected at least one event for HandoffStartEvent")
	}
	name := mapAguiEventToWailsName(events[0])
	if name != "agui:handoff-start" {
		t.Errorf("HandoffStart → Wails event = %q, want %q", name, "agui:handoff-start")
	}
}

func TestEventMapping_CustomEventA2UI(t *testing.T) {
	converter := agui.NewConverter("th-1", "run-1")

	// 模拟发送 A2UI 事件的 CustomEvent
	customAgui := agui.CustomEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventCustom},
		Name:      "a2ui",
		Value:     map[string]any{"kind": "createSurface"},
	}
	name := mapAguiEventToWailsName(customAgui)
	if name != "agui:a2ui" {
		t.Errorf("CustomEvent{a2ui} → %q, want %q", name, "agui:a2ui")
	}

	// 检查 converter 是否产生了 TextMessageEnd
	endEvents := converter.Convert(&agentcore.AgentEndEvent{Output: "done"})
	hasEnd := false
	for _, ev := range endEvents {
		n := mapAguiEventToWailsName(ev)
		if n == "agui:run-finished" || n == "agui:run-finished-with-success" {
			hasEnd = true
		}
	}
	if !hasEnd {
		t.Error("AgentEndEvent did not produce a run-finished event")
	}
}
