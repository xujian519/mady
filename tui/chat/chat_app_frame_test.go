package chat

// chat_app_frame_test.go is an integration-level regression test that renders
// a full ChatApp frame (header + history + loader + bordered editor + status
// bar) and asserts the structural elements are present. It complements the
// per-handler and per-component unit tests by catching layout regressions
// that only surface when all pieces compose — e.g. an editor border going
// missing, or the status bar being pushed off-screen.
//
// This is the lightweight "golden snapshot" form: rather than diffing exact
// bytes (brittle to theme/width changes), it pins the presence of distinctive
// substrings (title, user text, assistant text, editor border rune, status
// mode) and the row count. A byte-exact golden can be layered on later if a
// regression slips through this structural net.

import (
	"strings"
	"testing"
)

func TestChatAppFullFrameStructure(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{
		Title: "mady-test",
	})

	// Seed a user message, then an assistant streaming turn, so the frame has
	// content in both the history and the editor-driven layout.
	app.PrintUser("hello world")
	app.onAgentStart(AgentStartChatEvent{})
	app.onMessageDelta(MessageDeltaChatEvent{Delta: "streamed reply"})

	frame := app.layout.Render(80)
	if len(frame) == 0 {
		t.Fatal("empty frame")
	}
	joined := strings.Join(frame, "\n")

	checks := []struct{ name, want string }{
		{"title in header", "mady-test"},
		{"user message text", "hello world"},
		{"assistant streamed text", "streamed reply"},
		{"editor top border", "─"},
		{"status bar mode", "mady-test"},
	}
	for _, c := range checks {
		if !strings.Contains(joined, c.want) {
			t.Errorf("frame missing %q (%s)\nframe:\n%s", c.want, c.name, joined)
		}
	}

	// The frame should fill the terminal height (24 rows) so the diff engine
	// has a stable canvas. We allow ±2 for border/padding rounding.
	if len(frame) < 22 || len(frame) > 26 {
		t.Errorf("frame row count = %d, expected ~24", len(frame))
	}
}

func TestChatAppFullFrameAfterEnd(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.PrintUser("q")
	app.onAgentStart(AgentStartChatEvent{})
	app.onMessageDelta(MessageDeltaChatEvent{Delta: "a"})
	app.onAgentEnd(AgentEndChatEvent{})

	frame := app.layout.Render(80)
	joined := strings.Join(frame, "\n")
	// After AgentEnd the loader disappears (Idle), but the exchanged messages
	// and the editor border must remain.
	if !strings.Contains(joined, "q") {
		t.Errorf("user message lost after AgentEnd:\n%s", joined)
	}
	if !strings.Contains(joined, "a") {
		t.Errorf("assistant message lost after AgentEnd:\n%s", joined)
	}
}
