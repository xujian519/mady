package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// TestHandlePastePlaceholder verifies oversized pastes are stored and replaced
// with a placeholder message.
func TestHandlePastePlaceholder(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	big := strings.Repeat("line\n", 50)

	msg := app.handlePastePlaceholder(big)
	paste, ok := msg.(core.PasteMsg)
	if !ok {
		t.Fatalf("handlePastePlaceholder should return core.PasteMsg, got %T", msg)
	}
	if !strings.Contains(paste.Text, "[Pasted text #0 +50 lines]") {
		t.Fatalf("placeholder = %q", paste.Text)
	}

	// A system notice about the paste is appended.
	msgs := app.History().Messages()
	if len(msgs) != 1 || !strings.Contains(msgs[0].Text, "已粘贴大文本 #0") {
		t.Fatalf("expected paste notice, got %+v", msgs)
	}
}

// TestExpandPastePlaceholders verifies placeholder expansion and cleanup.
func TestExpandPastePlaceholders(t *testing.T) {
	t.Run("expands stored text", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.handlePastePlaceholder("FULL TEXT HERE")
		out := app.expandPastePlaceholders("before [Pasted text #0 +0 lines] after")
		if out != "before FULL TEXT HERE after" {
			t.Fatalf("expansion = %q", out)
		}
		app.mu.Lock()
		remaining := len(app.model.pastedTexts)
		app.mu.Unlock()
		if remaining != 0 {
			t.Fatalf("expanded entries should be cleaned up, %d remain", remaining)
		}
	})

	t.Run("unknown id left as-is", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.mu.Lock()
		app.model.pastedTexts = map[int]string{}
		app.mu.Unlock()

		// Unknown reference: the marker stays literal and scanning stops
		// (regression guard for the former infinite loop).
		out := app.expandPastePlaceholders("[Pasted text #42 +3 lines]")
		if out != "[Pasted text #42 +3 lines]" {
			t.Fatalf("unknown-id expansion = %q, want marker left as-is", out)
		}
	})

	t.Run("known marker after unknown marker still expands", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.mu.Lock()
		app.model.pastedTexts = map[int]string{7: "real text"}
		app.mu.Unlock()

		// The unknown marker stops the scan; the known marker before it
		// must still have been expanded on the earlier iteration.
		out := app.expandPastePlaceholders("[Pasted text #7 +1 lines] then [Pasted text #42 +3 lines]")
		if out != "real text then [Pasted text #42 +3 lines]" {
			t.Fatalf("mixed expansion = %q", out)
		}
	})

	t.Run("malformed markers untouched", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		cases := []string{
			"no marker",
			"[Pasted text",
			"[Pasted text #",
			"[Pasted text #x +1 lines]", // non-numeric id
		}
		for _, in := range cases {
			if out := app.expandPastePlaceholders(in); out != in {
				t.Fatalf("expand(%q) = %q, want unchanged", in, out)
			}
		}
	})

	t.Run("multiple placeholders", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.handlePastePlaceholder("AAA")
		app.handlePastePlaceholder("BBB")
		out := app.expandPastePlaceholders("[Pasted text #0 +0 lines]|[Pasted text #1 +0 lines]")
		if out != "AAA|BBB" {
			t.Fatalf("multi expansion = %q", out)
		}
	})
}

// TestFlushQueuedInput verifies FIFO flush with whitespace filtering and
// empty-queue safety.
func TestFlushQueuedInput(t *testing.T) {
	t.Run("flushes FIFO", func(t *testing.T) {
		var submitted []string
		app, _ := newTestChatApp(t, ChatAppConfig{
			OnSubmit: func(_ context.Context, in string) { submitted = append(submitted, in) },
		})
		app.mu.Lock()
		app.model.queuedInput = []string{"one", "  ", "two", "\t", "three"}
		app.mu.Unlock()

		app.flushQueuedInput()
		if len(submitted) != 3 || submitted[0] != "one" || submitted[1] != "two" || submitted[2] != "three" {
			t.Fatalf("flushed = %v, want [one two three]", submitted)
		}
		if app.QueuedInputCount() != 0 {
			t.Fatal("queue should be empty after flush")
		}
		// Whitespace entries produce no user echo.
		msgs := app.History().Messages()
		if len(msgs) != 3 {
			t.Fatalf("expected 3 user echoes, got %d", len(msgs))
		}
	})

	t.Run("empty queue no-op", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.flushQueuedInput()
		if n := len(app.History().Messages()); n != 0 {
			t.Fatalf("empty queue must not append, got %d", n)
		}
	})

	t.Run("nil OnSubmit still echoes", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.mu.Lock()
		app.model.queuedInput = []string{"x"}
		app.mu.Unlock()
		app.flushQueuedInput()
		msgs := app.History().Messages()
		if len(msgs) != 1 || msgs[0].Role != RoleUser {
			t.Fatalf("expected user echo, got %+v", msgs)
		}
	})
}

// TestBusyIdlePlaceholderRestore verifies the editor placeholder round-trips
// through Busy/Idle (placeholder text is observed via the editor's render).
func TestBusyIdlePlaceholderRestore(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	placeholderOf := func() string {
		out := strings.Join(app.editor.Render(40), "\n")
		if strings.Contains(out, "输入消息…") {
			return "default"
		}
		if strings.Contains(out, "Ctrl+C to interrupt") {
			return "busy"
		}
		return "other"
	}
	if got := placeholderOf(); got != "default" {
		t.Fatalf("initial placeholder = %q", got)
	}
	app.Busy("working")
	if got := placeholderOf(); got != "busy" {
		t.Fatalf("busy placeholder = %q", got)
	}
	app.Idle()
	if got := placeholderOf(); got != "default" {
		t.Fatalf("idle placeholder = %q, want restored default", got)
	}
	app.Busy("") // empty message must not clear the loader message
	if !app.loader.IsRunning() {
		t.Fatal("Busy with empty message should still start the loader")
	}
}

// TestQueuedInputIndicatorInLayout verifies the queued-count indicator line
// appears in the layout when inputs are queued.
func TestQueuedInputIndicatorInLayout(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady()
	app.onAgentStart(AgentStartChatEvent{})
	app.onEditorSubmit("queued one")
	app.onEditorSubmit("queued two")
	if app.QueuedInputCount() != 2 {
		t.Fatalf("QueuedInputCount = %d, want 2", app.QueuedInputCount())
	}
	cols, _ := app.TerminalSize()
	out := strings.Join(app.layout.Render(cols), "\n")
	if !strings.Contains(out, "待发送：2 条消息") {
		t.Fatalf("layout should show queued indicator, got %q", out)
	}
}
