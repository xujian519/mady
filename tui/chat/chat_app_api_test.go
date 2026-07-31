package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

// TestNewChatAppWithHost verifies the constructor that takes a host directly.
func TestNewChatAppWithHost(t *testing.T) {
	vt := terminal.NewVirtualTerminal(80, 24)
	host := &testAppHost{vt: vt}
	app := NewChatAppWithHost(ChatAppConfig{Title: "T"}, host)
	if app == nil {
		t.Fatal("NewChatAppWithHost returned nil")
	}
	if app.Host() != host {
		t.Fatalf("Host() = %v, want the injected host", app.Host())
	}
	if app.StatusBar() == nil || app.Editor() == nil || app.Loader() == nil {
		t.Fatal("expected StatusBar/Editor/Loader components to be constructed")
	}
	if app.Keybindings() == nil {
		t.Fatal("expected keybindings manager")
	}
	if app.History() == nil {
		t.Fatal("expected history")
	}
}

// TestChatAppAccessors verifies the component accessor getters.
func TestChatAppAccessors(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})

	if app.Host() == nil {
		t.Error("Host() should be non-nil after SetHost")
	}
	if app.History() == nil {
		t.Error("History() should be non-nil")
	}
	if app.Editor() == nil {
		t.Error("Editor() should be non-nil")
	}
	if app.Loader() == nil {
		t.Error("Loader() should be non-nil")
	}
	if app.Keybindings() == nil {
		t.Error("Keybindings() should be non-nil")
	}
	if app.StatusBar() == nil {
		t.Error("StatusBar() should be non-nil")
	}
	if app.JudgmentView() == nil {
		t.Error("JudgmentView() should be non-nil")
	}
	if app.Footer() == nil {
		t.Error("Footer() should be non-nil")
	}
}

// TestChatAppUpdateStatusBar verifies the status bar title composition.
func TestChatAppUpdateStatusBar(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.UpdateStatusBar("openai", "gpt-4o", "patent")
	rendered := strings.Join(app.StatusBar().Render(80), "\n")
	if !strings.Contains(rendered, "openai/gpt-4o") || !strings.Contains(rendered, "patent") {
		t.Fatalf("status bar should show provider/model/mode, got %q", rendered)
	}
}

// TestChatAppUpdateJudgmentView verifies UpdateJudgmentView applies the
// current FSM state to the judgment view without panicking.
func TestChatAppUpdateJudgmentView(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.UpdateJudgmentView()
	if status := app.JudgmentView().Status(); status != "initializing" {
		t.Errorf("initial JV status = %q, want %q", status, "initializing")
	}

	// After agent start, the view must reflect streaming.
	app.onAgentStart(AgentStartChatEvent{})
	app.UpdateJudgmentView()
	if status := app.JudgmentView().Status(); status != "streaming" {
		t.Errorf("JV status = %q, want %q", status, "streaming")
	}
}

// TestChatAppDone verifies Done() returns the host's completion channel.
func TestChatAppDone(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	select {
	case <-app.Done():
		// testAppHost.Done returns an already-closed channel
	default:
		t.Fatal("Done() channel should be closed by testAppHost")
	}
}

// TestChatAppStartStop verifies lifecycle start/stop round-trips.
func TestChatAppStartStop(t *testing.T) {
	vt := terminal.NewVirtualTerminal(80, 24)
	host := &testAppHost{vt: vt}
	cfg := ChatAppConfig{}
	cfg.Host = host
	app := NewChatApp(cfg)
	app.SetHost(host)

	if host.started {
		t.Fatal("host should not be started yet")
	}
	if err := app.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !host.started {
		t.Fatal("host should be started after Start")
	}
	if len(host.children) != 1 {
		t.Fatalf("expected 1 child added (layout), got %d", len(host.children))
	}
	if err := app.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if host.started {
		t.Fatal("host should be stopped after Stop")
	}
}

// TestChatAppSetHostRebindsComponents verifies SetHost rewires loader and
// invalidate callbacks to the new host.
func TestChatAppSetHostRebindsComponents(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	vt2 := terminal.NewVirtualTerminal(100, 30)
	host2 := &testAppHost{vt: vt2}
	app.SetHost(host2)
	if app.Host() != host2 {
		t.Fatalf("Host() = %v, want host2", app.Host())
	}
	cols, rows := app.TerminalSize()
	if cols != 100 || rows != 30 {
		t.Fatalf("TerminalSize = %d x %d, want 100 x 30", cols, rows)
	}
}

// TestSubmitApprovalCommand verifies the review-gate callback path: close the
// overlay and submit the command through the normal submit flow.
func TestSubmitApprovalCommand(t *testing.T) {
	var captured []string
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnSubmit: func(_ context.Context, in string) { captured = append(captured, in) },
	})

	// Open a review gate first so CloseReviewGate has something to remove.
	app.OpenReviewGate(ReviewGateData{Title: "gate", Judgment: "ok"})
	if len(app.host.(*testAppHost).overlays) != 1 {
		t.Fatal("expected review gate overlay pushed")
	}

	app.submitApprovalCommand("/approve")
	if len(app.host.(*testAppHost).overlays) != 0 {
		t.Fatal("review gate overlay should be removed after submit")
	}
	msgs := app.History().Messages()
	if len(msgs) == 0 || msgs[0].Role != RoleUser || msgs[0].Text != "/approve" {
		t.Fatalf("expected user echo of /approve, got %+v", msgs)
	}
}

// TestOpenReviewGateFromData verifies the payload → overlay mapping and that
// the pass/back/block callbacks submit the correct commands through the
// ReviewGate component keys (p / b / f).
func TestOpenReviewGateFromData(t *testing.T) {
	var captured []string
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnSubmit: func(_ context.Context, in string) { captured = append(captured, in) },
	})

	app.openReviewGateFromData(&ReviewGatePayload{
		Title:      "复核",
		Judgment:   "结论1",
		Confidence: 0.8,
		Risks:      []string{"r1"},
	})
	host := app.host.(*testAppHost)
	if len(host.overlays) != 1 {
		t.Fatalf("expected 1 overlay, got %d", len(host.overlays))
	}

	// Trigger OnPass via the 'p' key.
	rg := host.overlays[0].OverlayContent().(core.Updatable)
	rg.Update(core.KeyMsg{Data: "p"})
	if len(captured) != 1 || captured[0] != "/approve" {
		t.Fatalf("OnPass should submit /approve, got %v", captured)
	}
	if len(host.overlays) != 0 {
		t.Fatal("overlay should be removed after pass")
	}

	// Open again; this time hit 'b' (back) → supplement-evidence command.
	app.openReviewGateFromData(&ReviewGatePayload{Judgment: "x"})
	if len(host.overlays) != 1 {
		t.Fatal("expected overlay re-opened")
	}
	host.overlays[0].OverlayContent().(core.Updatable).Update(core.KeyMsg{Data: "b"})
	if len(captured) != 2 || !strings.Contains(captured[1], "请补充证据") {
		t.Fatalf("OnBack should submit supplement command, got %v", captured)
	}

	// Open again; hit 'f' (block) → reject command.
	app.openReviewGateFromData(&ReviewGatePayload{Judgment: "y"})
	host.overlays[0].OverlayContent().(core.Updatable).Update(core.KeyMsg{Data: "f"})
	if len(captured) != 3 || !strings.HasPrefix(captured[2], "/reject") {
		t.Fatalf("OnBlock should submit /reject, got %v", captured)
	}
}

// TestOpenReviewGateFromDataNilPayload is a no-op guard.
func TestOpenReviewGateFromDataNilPayload(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.openReviewGateFromData(nil)
	if len(app.host.(*testAppHost).overlays) != 0 {
		t.Fatal("nil payload must not open an overlay")
	}
	app.openReviewGateFromData(&ReviewGatePayload{}) // empty judgment
	if len(app.host.(*testAppHost).overlays) != 0 {
		t.Fatal("empty judgment payload must not open an overlay")
	}
}

// TestTerminalSizeNoHost falls back to 80x24 when no host is attached.
func TestTerminalSizeNoHost(t *testing.T) {
	cfg := ChatAppConfig{}
	app := NewChatApp(cfg) // no host
	cols, rows := app.TerminalSize()
	if cols != 80 || rows != 24 {
		t.Fatalf("TerminalSize = %d x %d, want 80 x 24 fallback", cols, rows)
	}
}

// TestChatAppConfirmFlow verifies the inline y/n confirmation lifecycle:
// pending → yes, pending → no, pending → timeout.
func TestChatAppConfirmFlow(t *testing.T) {
	t.Run("yes", func(t *testing.T) {
		yes, no := false, false
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.StartConfirm(InlineConfirm{Prompt: "del?", OnYes: func() { yes = true }, OnNo: func() { no = true }})

		if app.ConfirmPending() == nil || app.ConfirmPending().Prompt != "del?" {
			t.Fatalf("ConfirmPending = %+v", app.ConfirmPending())
		}
		if app.State() != StateConfirmPending {
			t.Fatalf("state = %s, want confirm-pending", app.State())
		}
		app.ConfirmYes()
		if !yes || no {
			t.Fatal("OnYes should fire, OnNo should not")
		}
		if app.ConfirmPending() != nil {
			t.Fatal("confirm should be cleared after yes")
		}
		if app.State() != StateIdle {
			t.Fatalf("state = %s, want idle", app.State())
		}
	})

	t.Run("no", func(t *testing.T) {
		yes, no := false, false
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.StartConfirm(InlineConfirm{Prompt: "del?", OnYes: func() { yes = true }, OnNo: func() { no = true }})
		app.ConfirmNo()
		if !no || yes {
			t.Fatal("OnNo should fire, OnYes should not")
		}
		if app.ConfirmPending() != nil || app.State() != StateIdle {
			t.Fatal("confirm should be cleared after no")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		noDone := make(chan struct{})
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.StartConfirm(InlineConfirm{
			Prompt:  "del?",
			OnNo:    func() { close(noDone) },
			Timeout: 20 * time.Millisecond,
		})
		select {
		case <-noDone:
		case <-time.After(2 * time.Second):
			t.Fatal("confirm timeout should invoke OnNo")
		}
		if app.ConfirmPending() != nil {
			t.Fatal("confirm should be cleared after timeout")
		}
		if app.State() != StateIdle {
			t.Fatalf("state = %s, want idle", app.State())
		}
	})

	t.Run("no callbacks", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.StartConfirm(InlineConfirm{Prompt: "p"})
		app.ConfirmYes()
		app.StartConfirm(InlineConfirm{Prompt: "p"})
		app.ConfirmNo()
		if app.State() != StateIdle {
			t.Fatal("state should be idle with nil callbacks")
		}
	})

	t.Run("confirm timeout direct call", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.StartConfirm(InlineConfirm{Prompt: "p", OnNo: func() {}})
		app.confirmTimeout()
		if app.ConfirmPending() != nil {
			t.Fatal("confirmTimeout should clear pending confirm")
		}
	})
}

// TestChatAppConfirmViaLayoutKeys verifies y/n/Esc key routing while a
// confirmation is pending (dispatchConfirmKey through layout.Update).
func TestChatAppConfirmViaLayoutKeys(t *testing.T) {
	t.Run("y resolves yes", func(t *testing.T) {
		yes, no := false, false
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.StartConfirm(InlineConfirm{Prompt: "p", OnYes: func() { yes = true }, OnNo: func() { no = true }})
		app.layout.Update(core.KeyMsg{Data: "y"})
		if !yes || no || app.State() != StateIdle {
			t.Fatalf("y should confirm: yes=%v no=%v state=%s", yes, no, app.State())
		}
	})

	t.Run("n resolves no", func(t *testing.T) {
		yes, no := false, false
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.StartConfirm(InlineConfirm{Prompt: "p", OnYes: func() { yes = true }, OnNo: func() { no = true }})
		app.layout.Update(core.KeyMsg{Data: "n"})
		if !no || yes || app.State() != StateIdle {
			t.Fatalf("n should cancel: yes=%v no=%v state=%s", yes, no, app.State())
		}
	})

	t.Run("escape resolves no", func(t *testing.T) {
		no := false
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.StartConfirm(InlineConfirm{Prompt: "p", OnNo: func() { no = true }})
		app.layout.Update(core.KeyMsg{Data: "\x1b"})
		if !no || app.State() != StateIdle {
			t.Fatalf("esc should cancel: no=%v state=%s", no, app.State())
		}
	})

	t.Run("other keys ignored while pending", func(t *testing.T) {
		no := false
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.StartConfirm(InlineConfirm{Prompt: "p", OnNo: func() { no = true }})
		app.layout.Update(core.KeyMsg{Data: "x"})
		if no || app.State() != StateConfirmPending {
			t.Fatalf("unrelated key must not resolve: no=%v state=%s", no, app.State())
		}
	})
}

// TestToggleKeyHelp verifies open/close of the keybindings help overlay,
// including the SetOnClose callback path and re-open after close.
func TestToggleKeyHelp(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	host := app.host.(*testAppHost)

	ov := app.ToggleKeyHelp()
	if ov == nil {
		t.Fatal("ToggleKeyHelp should return an overlay on open")
	}
	if len(host.overlays) != 1 {
		t.Fatalf("expected 1 overlay, got %d", len(host.overlays))
	}
	if !ov.OverlayWantsFocus() || !ov.OverlayDimBackground() {
		t.Error("help overlay should request focus and dim background")
	}
	if ov.OverlayWidthPct() != 70 || ov.OverlayHeightPct() != 70 {
		t.Errorf("help overlay size = %d%% x %d%%, want 70 x 70", ov.OverlayWidthPct(), ov.OverlayHeightPct())
	}

	// Second call closes it.
	if got := app.ToggleKeyHelp(); got != nil {
		t.Fatal("second ToggleKeyHelp should close and return nil")
	}
	if len(host.overlays) != 0 {
		t.Fatalf("expected overlay removed, got %d", len(host.overlays))
	}

	// Re-open, then close via CloseKeyHelp (the overlay's onClose path).
	app.ToggleKeyHelp()
	app.CloseKeyHelp()
	if len(host.overlays) != 0 {
		t.Fatalf("CloseKeyHelp should remove overlay, got %d", len(host.overlays))
	}

	// CloseKeyHelp with no open overlay is a no-op.
	app.CloseKeyHelp()
}

// TestToggleKeyHelpOnCloseCallback verifies the overlay's internal close
// callback (Esc inside the help) routes back to CloseKeyHelp.
func TestToggleKeyHelpOnCloseCallback(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	host := app.host.(*testAppHost)
	ov := app.ToggleKeyHelp()
	help := ov.OverlayContent().(core.Updatable)
	help.Update(core.KeyMsg{Data: "\x1b"}) // Esc closes via SetOnClose
	if len(host.overlays) != 0 {
		t.Fatalf("Esc in help should close overlay, got %d", len(host.overlays))
	}
}
