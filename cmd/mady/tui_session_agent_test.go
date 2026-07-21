package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

// localAppHost implements chat.AppHost for testing. It uses a VirtualTerminal
// to provide TerminalSize and accepts calls without side effects.
type localAppHost struct {
	vt       *terminal.VirtualTerminal
	children []core.Component
	overlays []chat.OverlayRef
	started  bool
}

func (h *localAppHost) Start() error              { h.started = true; return nil }
func (h *localAppHost) Stop() error               { h.started = false; return nil }
func (h *localAppHost) Done() <-chan struct{}     { ch := make(chan struct{}); close(ch); return ch }
func (h *localAppHost) AddChild(c core.Component) { h.children = append(h.children, c) }
func (h *localAppHost) Focus(c core.Component)    {}
func (h *localAppHost) RequestRender()            {}
func (h *localAppHost) PushOverlay(ov chat.OverlayRef) {
	h.overlays = append(h.overlays, ov)
}
func (h *localAppHost) RemoveOverlay(ov chat.OverlayRef) bool {
	for i, o := range h.overlays {
		if o == ov {
			h.overlays = append(h.overlays[:i], h.overlays[i+1:]...)
			return true
		}
	}
	return false
}
func (h *localAppHost) TerminalSize() (cols, rows int64) { return h.vt.Size() }
func (h *localAppHost) EnableMouse(mode string)          {}
func (h *localAppHost) DisableMouse()                    {}

// testAppForSession creates a minimal ChatApp suitable for testing tuiSession
// interactions. It uses a VirtualTerminal so Start() succeeds.
func testAppForSession(t *testing.T) *chat.ChatApp {
	t.Helper()
	vt := terminal.NewVirtualTerminal(80, 24)
	host := &localAppHost{vt: vt}
	app := chat.NewChatApp(chat.ChatAppConfig{Host: host})
	app.SetHost(host)
	if err := app.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { app.Stop() })
	return app
}

// TestErrorSeverityLabels verifies that severityLabel returns appropriate
// prefixes for each severity level.
func TestErrorSeverityLabels(t *testing.T) {
	tests := []struct {
		se   ErrorSeverity
		want string
	}{
		{RunFailure, "❌"},
		{PostProcessFailure, "⚠"},
		{Degradation, "⚠"},
	}
	for _, tt := range tests {
		got := severityLabel(tt.se)
		if got != tt.want {
			t.Errorf("severityLabel(%d) = %q, want %q", tt.se, got, tt.want)
		}
	}
}

// TestShowUserErrorAppendsToHistory verifies that showUserError adds a system
// message (via PrintSystem) to the chat history with the correct severity
// prefix for each error level. The format strings match those used in the
// actual submitInput and resumeIfInterrupted code paths.
func TestShowUserErrorAppendsToHistory(t *testing.T) {
	app := testAppForSession(t)
	s := &tuiSession{app: app}

	// Match the format from submitInput's agent.Run failure path.
	showUserError(s, RunFailure, "Agent 执行失败: %v", errors.New("connection refused"))
	// Match the format from SaveState failure path.
	showUserError(s, Degradation, "会话保存失败（不影响本次输出）: %v", errors.New("permission denied"))

	msgs := app.History().Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	// First message: RunFailure with ❌ prefix + context.
	if msgs[0].Role != chat.RoleSystem {
		t.Errorf("msg[0] role = %v, want System", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Text, "❌") {
		t.Errorf("msg[0] text = %q, want ❌ prefix", msgs[0].Text)
	}
	// Second message: Degradation with ⚠ prefix + context.
	if msgs[1].Role != chat.RoleSystem {
		t.Errorf("msg[1] role = %v, want System", msgs[1].Role)
	}
	if !strings.Contains(msgs[1].Text, "⚠") {
		t.Errorf("msg[1] text = %q, want ⚠ prefix", msgs[1].Text)
	}
}

// TestRebuildAgentPanicRecover verifies that rebuildAgent does not cause a
// test-level panic when called repeatedly, even when config is incomplete.
// The defer recover inside rebuildAgent catches any panic from agentcore.New,
// logs it, and shows a user-visible error — the test should not crash.
func TestRebuildAgentPanicRecover(t *testing.T) {
	app := testAppForSession(t)
	s := &tuiSession{
		app:   app,
		ctx:   context.Background(),
		store: mustNewTransientStore(),
	}

	// repeatedAgentRebuild calls rebuildAgent in a loop, expecting the defer
	// recover to catch any panics and log them rather than crashing the test.
	repeatedAgentRebuild := func(count int) {
		for i := 0; i < count; i++ {
			s.rebuildAgent()
		}
	}

	// The test itself must not panic. rebuildAgent's internal defer recover
	// handles panics from agentcore.New and logs them.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("rebuildAgent panicked and escaped recover: %v", r)
		}
	}()
	repeatedAgentRebuild(3)
	// No assertion on agent state — with incomplete config, agent will be nil.
	// The purpose of this test is to verify the panic recovery, not the result.
}

// mustNewTransientStore creates an in-memory settings store.  Panics on error.
func mustNewTransientStore() *SettingsStore {
	s, err := NewSettingsStore("")
	if err != nil {
		panic(err)
	}
	return s
}
