package chat

import (
	"strings"
	"testing"
)

// TestChatAppAgentEndTruncationNotice 是 S1 回归防护：收尾事件携带
// FinishReason="length"（max_tokens 截断）时，聊天历史须追加系统提示，
// 告知用户输出可能不完整。
func TestChatAppAgentEndTruncationNotice(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})

	app.onAgentStart(AgentStartChatEvent{})
	app.onMessageDelta(MessageDeltaChatEvent{Delta: "partial answer"})
	app.onAgentEnd(AgentEndChatEvent{Output: "partial answer", FinishReason: "length"})

	msgs := app.History().Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 msgs (answer + notice), got %d", len(msgs))
	}
	notice := msgs[1]
	if notice.Role != RoleSystem {
		t.Fatalf("notice role = %v, want RoleSystem", notice.Role)
	}
	if !strings.Contains(notice.Text, "内容可能不完整") {
		t.Fatalf("notice text = %q", notice.Text)
	}
}

// TestChatAppAgentEndAbnormalNotice 覆盖流异常终止（FinishReason="error"）的提示。
func TestChatAppAgentEndAbnormalNotice(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})

	app.onAgentStart(AgentStartChatEvent{})
	app.onMessageDelta(MessageDeltaChatEvent{Delta: "partial"})
	app.onAgentEnd(AgentEndChatEvent{Output: "partial", FinishReason: "error"})

	msgs := app.History().Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 msgs (answer + notice), got %d", len(msgs))
	}
	if msgs[1].Role != RoleSystem || !strings.Contains(msgs[1].Text, "输出可能不完整") {
		t.Fatalf("unexpected notice: %+v", msgs[1])
	}
}

// TestChatAppAgentEndNormalNoNotice 确认正常结束（无 FinishReason 标记）不追加提示。
func TestChatAppAgentEndNormalNoNotice(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})

	app.onAgentStart(AgentStartChatEvent{})
	app.onMessageDelta(MessageDeltaChatEvent{Delta: "full answer"})
	app.onAgentEnd(AgentEndChatEvent{Output: "full answer", FinishReason: "stop"})

	msgs := app.History().Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 msg, got %d", len(msgs))
	}
	if msgs[0].Text != "full answer" {
		t.Fatalf("text = %q", msgs[0].Text)
	}
}
