package agui

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// 本文件是协议面契约护栏（M67-1）。契约清单见
// docs/code-health/protocol-contract-baseline.md §3.3。
//
// 这些断言刻意写成「逐字比对」：AGUI 的事件名、字段名、ID 格式是与前端和
// 第三方客户端的线协议契约，任何改动都属破坏性变更，必须走 M70 定义的
// 变更流程（改 fixture + 改文档 + 人工 review），不能在重构卡里顺手改。

// TestContractAGUIEventTypeRegistry 锁定 23 个事件类型字符串。
// SSE 的 event: 行直接取这些值，改名即断流。
func TestContractAGUIEventTypeRegistry(t *testing.T) {
	want := []struct {
		constant EventType
		wire     string
	}{
		{EventRunStarted, "RUN_STARTED"},
		{EventRunFinished, "RUN_FINISHED"},
		{EventRunError, "RUN_ERROR"},
		{EventStepStarted, "STEP_STARTED"},
		{EventStepFinished, "STEP_FINISHED"},
		{EventTextMessageStart, "TEXT_MESSAGE_START"},
		{EventTextMessageContent, "TEXT_MESSAGE_CONTENT"},
		{EventTextMessageEnd, "TEXT_MESSAGE_END"},
		{EventThinkingStart, "THINKING_START"},
		{EventThinkingTextMessageStart, "THINKING_TEXT_MESSAGE_START"},
		{EventThinkingTextMessageContent, "THINKING_TEXT_MESSAGE_CONTENT"},
		{EventThinkingTextMessageEnd, "THINKING_TEXT_MESSAGE_END"},
		{EventThinkingEnd, "THINKING_END"},
		{EventToolCallStart, "TOOL_CALL_START"},
		{EventToolCallArgs, "TOOL_CALL_ARGS"},
		{EventToolCallEnd, "TOOL_CALL_END"},
		{EventToolCallResult, "TOOL_CALL_RESULT"},
		{EventStateSnapshot, "STATE_SNAPSHOT"},
		{EventStateDelta, "STATE_DELTA"},
		{EventMessagesSnapshot, "MESSAGES_SNAPSHOT"},
		{EventCustom, "CUSTOM"},
		{EventRaw, "RAW"},
		{EventContextUsage, "CONTEXT_USAGE"},
	}

	seen := make(map[string]bool, len(want))
	for _, c := range want {
		if string(c.constant) != c.wire {
			t.Errorf("事件类型漂移：常量值为 %q，契约要求 %q", c.constant, c.wire)
		}
		if seen[c.wire] {
			t.Errorf("事件类型重复：%q", c.wire)
		}
		seen[c.wire] = true
	}
	if len(want) != 23 {
		t.Fatalf("事件类型数量应为 23，当前 fixture 为 %d", len(want))
	}
}

// TestContractAGUIEventJSONFields 锁定各事件的上线字段名与字段顺序。
// 字段顺序虽非 JSON 语义要求，但固定顺序能让 golden 比对稳定、diff 可读。
func TestContractAGUIEventJSONFields(t *testing.T) {
	base := func(typ EventType) BaseEvent {
		return BaseEvent{Type: typ, Timestamp: 1700000000000}
	}
	usage := ContextUsageEvent{
		BaseEvent:     base(EventContextUsage),
		ContextWindow: 128000,
		UsagePercent:  2.34375,
	}
	usage.TokenUsage.PromptTokens = 1000
	usage.TokenUsage.CompletionTokens = 2000
	usage.TokenUsage.TotalTokens = 3000

	cases := []struct {
		name string
		ev   any
		want string
	}{
		{"RunStarted", RunStartedEvent{base(EventRunStarted), "th", "run", ""},
			`{"type":"RUN_STARTED","timestamp":1700000000000,"threadId":"th","runId":"run"}`},
		{"RunFinished", RunFinishedEvent{BaseEvent: base(EventRunFinished), ThreadID: "th", RunID: "run", Result: "ok", Outcome: &RunFinishedOutcome{Type: "success"}, FinishReason: "stop"},
			`{"type":"RUN_FINISHED","timestamp":1700000000000,"threadId":"th","runId":"run","result":"ok","outcome":{"type":"success"},"finishReason":"stop"}`},
		{"RunError", RunErrorEvent{BaseEvent: base(EventRunError), ThreadID: "th", RunID: "run", Message: "boom"},
			`{"type":"RUN_ERROR","timestamp":1700000000000,"threadId":"th","runId":"run","message":"boom"}`},
		{"StepStarted", StepStartedEvent{base(EventStepStarted), "turn_1"},
			`{"type":"STEP_STARTED","timestamp":1700000000000,"stepName":"turn_1"}`},
		{"StepFinished", StepFinishedEvent{base(EventStepFinished), "turn_1"},
			`{"type":"STEP_FINISHED","timestamp":1700000000000,"stepName":"turn_1"}`},
		{"TextStart", TextMessageStartEvent{base(EventTextMessageStart), "msg_1", "assistant"},
			`{"type":"TEXT_MESSAGE_START","timestamp":1700000000000,"messageId":"msg_1","role":"assistant"}`},
		{"TextContent", TextMessageContentEvent{base(EventTextMessageContent), "msg_1", "hi"},
			`{"type":"TEXT_MESSAGE_CONTENT","timestamp":1700000000000,"messageId":"msg_1","delta":"hi"}`},
		{"TextEnd", TextMessageEndEvent{base(EventTextMessageEnd), "msg_1"},
			`{"type":"TEXT_MESSAGE_END","timestamp":1700000000000,"messageId":"msg_1"}`},
		{"ThinkingStart", ThinkingStartEvent{base(EventThinkingStart), "thinking_1", ""},
			`{"type":"THINKING_START","timestamp":1700000000000,"thinkingId":"thinking_1"}`},
		{"ThinkingTextStart", ThinkingTextMessageStartEvent{base(EventThinkingTextMessageStart), "thinking_1", "msg_1"},
			`{"type":"THINKING_TEXT_MESSAGE_START","timestamp":1700000000000,"thinkingId":"thinking_1","messageId":"msg_1"}`},
		{"ThinkingTextContent", ThinkingTextMessageContentEvent{base(EventThinkingTextMessageContent), "thinking_1", "msg_1", "hmm"},
			`{"type":"THINKING_TEXT_MESSAGE_CONTENT","timestamp":1700000000000,"thinkingId":"thinking_1","messageId":"msg_1","delta":"hmm"}`},
		{"ThinkingTextEnd", ThinkingTextMessageEndEvent{base(EventThinkingTextMessageEnd), "thinking_1", "msg_1"},
			`{"type":"THINKING_TEXT_MESSAGE_END","timestamp":1700000000000,"thinkingId":"thinking_1","messageId":"msg_1"}`},
		{"ThinkingEnd", ThinkingEndEvent{base(EventThinkingEnd), "thinking_1"},
			`{"type":"THINKING_END","timestamp":1700000000000,"thinkingId":"thinking_1"}`},
		{"ToolCallStart", ToolCallStartEvent{base(EventToolCallStart), "call_1", "read", ""},
			`{"type":"TOOL_CALL_START","timestamp":1700000000000,"toolCallId":"call_1","toolCallName":"read"}`},
		{"ToolCallArgs", ToolCallArgsEvent{base(EventToolCallArgs), "call_1", `{"path":"a"}`},
			`{"type":"TOOL_CALL_ARGS","timestamp":1700000000000,"toolCallId":"call_1","delta":"{\"path\":\"a\"}"}`},
		{"ToolCallEnd", ToolCallEndEvent{base(EventToolCallEnd), "call_1"},
			`{"type":"TOOL_CALL_END","timestamp":1700000000000,"toolCallId":"call_1"}`},
		{"ToolCallResult", ToolCallResultEvent{base(EventToolCallResult), "msg_2", "call_1", "content", ""},
			`{"type":"TOOL_CALL_RESULT","timestamp":1700000000000,"messageId":"msg_2","toolCallId":"call_1","content":"content"}`},
		{"StateSnapshot", StateSnapshotEvent{base(EventStateSnapshot), map[string]any{"k": "v"}},
			`{"type":"STATE_SNAPSHOT","timestamp":1700000000000,"snapshot":{"k":"v"}}`},
		{"StateDelta", StateDeltaEvent{base(EventStateDelta), []jsonPatchOp{{Op: "replace", Path: "/k", Value: "v"}}},
			`{"type":"STATE_DELTA","timestamp":1700000000000,"delta":[{"op":"replace","path":"/k","value":"v"}]}`},
		{"MessagesSnapshot", MessagesSnapshotEvent{base(EventMessagesSnapshot), []Message{{ID: "m1", Role: MessageRoleUser, Content: "hi"}}},
			`{"type":"MESSAGES_SNAPSHOT","timestamp":1700000000000,"messages":[{"id":"m1","role":"user","content":"hi"}]}`},
		{"Custom", CustomEvent{base(EventCustom), "a2ui", map[string]any{"surfaceId": "s1"}},
			`{"type":"CUSTOM","timestamp":1700000000000,"name":"a2ui","value":{"surfaceId":"s1"}}`},
		{"ContextUsage", usage,
			`{"type":"CONTEXT_USAGE","timestamp":1700000000000,"tokenUsage":{"promptTokens":1000,"completionTokens":2000,"totalTokens":3000},"contextWindow":128000,"usagePercent":2.34375}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := json.Marshal(c.ev)
			if err != nil {
				t.Fatalf("marshal 失败：%v", err)
			}
			if string(got) != c.want {
				t.Errorf("JSON 契约漂移\n期望：%s\n实际：%s", c.want, got)
			}
		})
	}
}

// TestContractAGUIMessageIDSequence 锁定转换器生成的 ID 格式与递增规则。
// 客户端依赖 msg_/thinking_ 前缀做流式消息拼接，格式变更会打散气泡。
func TestContractAGUIMessageIDSequence(t *testing.T) {
	c := NewConverter("th", "run")
	seq := []agentcore.Event{
		makeEvent[agentcore.AgentStartEvent](agentcore.EventAgentStart, nil),
		makeEvent[agentcore.MessageDeltaEvent](agentcore.EventMessageDelta, map[string]any{"delta": "hi", "kind": "text"}),
		makeEvent[agentcore.MessageDeltaEvent](agentcore.EventMessageDelta, map[string]any{"delta": "there", "kind": "text"}),
		makeEvent[agentcore.MessageDeltaEvent](agentcore.EventMessageDelta, map[string]any{"delta": "hmm", "kind": "thinking"}),
	}

	type idPair struct {
		kind      string
		messageID string
		thinkingI string
	}
	var got []idPair
	for _, ev := range seq {
		for _, out := range c.Convert(ev) {
			switch v := out.(type) {
			case RunStartedEvent:
				got = append(got, idPair{kind: "run_started"})
			case TextMessageStartEvent:
				got = append(got, idPair{kind: "text_start", messageID: v.MessageID})
			case TextMessageContentEvent:
				got = append(got, idPair{kind: "text_content", messageID: v.MessageID})
			case ThinkingStartEvent:
				got = append(got, idPair{kind: "thinking_start", thinkingI: v.ThinkingID})
			case ThinkingTextMessageStartEvent:
				got = append(got, idPair{kind: "thinking_text_start", messageID: v.MessageID, thinkingI: v.ThinkingID})
			case ThinkingTextMessageContentEvent:
				got = append(got, idPair{kind: "thinking_text_content", messageID: v.MessageID, thinkingI: v.ThinkingID})
			}
		}
	}

	want := []idPair{
		{kind: "run_started"},
		{kind: "text_start", messageID: "msg_1"},
		{kind: "text_content", messageID: "msg_1"},
		{kind: "text_content", messageID: "msg_1"},
		{kind: "thinking_start", thinkingI: "thinking_1"},
		{kind: "thinking_text_start", messageID: "msg_2", thinkingI: "thinking_1"},
		{kind: "thinking_text_content", messageID: "msg_2", thinkingI: "thinking_1"},
	}
	if len(got) != len(want) {
		t.Fatalf("事件序列长度漂移：期望 %d，实际 %d（%+v）", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 个事件漂移：期望 %+v，实际 %+v", i+1, want[i], got[i])
		}
	}
}

// TestContractAGUISSEFrame 锁定 SSE 帧格式：event 行 + 单行 data + 空行。
func TestContractAGUISSEFrame(t *testing.T) {
	rec := httptest.NewRecorder()
	writeSSE(rec, rec, string(EventRunStarted), RunStartedEvent{
		BaseEvent: BaseEvent{Type: EventRunStarted, Timestamp: 1700000000000},
		ThreadID:  "th",
		RunID:     "run",
	})

	want := "event: RUN_STARTED\n" +
		"data: {\"type\":\"RUN_STARTED\",\"timestamp\":1700000000000,\"threadId\":\"th\",\"runId\":\"run\"}\n\n"
	if rec.Body.String() != want {
		t.Errorf("SSE 帧格式漂移\n期望：%q\n实际：%q", want, rec.Body.String())
	}
}
