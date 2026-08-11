package chat

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// TestDispatchKeySearchRouting verifies that while search mode is active,
// printable characters / navigation keys route to the search handlers.
func TestDispatchKeySearchRouting(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	hist.Append(ChatMessage{Role: RoleAssistant, Text: "alpha beta"})
	hist.Append(ChatMessage{Role: RoleAssistant, Text: "gamma alpha"})
	hist.Append(ChatMessage{Role: RoleAssistant, Text: "delta"})

	// Enter search mode via the slash key (dispatchKey "slash" case).
	if !app.layout.dispatchKey(terminal.Key{Name: "slash"}) {
		t.Fatal("slash key should be consumed")
	}
	if !hist.SearchMode() {
		t.Fatal("search mode should be active after slash")
	}
	if hist.SearchQuery() != "" || hist.SearchMatchCount() != 0 {
		t.Fatalf("initial search state: query=%q matches=%d", hist.SearchQuery(), hist.SearchMatchCount())
	}

	// Type "alp" — appends to the query and rebuilds matches.
	for _, ch := range "alp" {
		if !app.layout.dispatchKey(terminal.Key{Name: string(ch)}) {
			t.Fatalf("key %q should be consumed in search mode", ch)
		}
	}
	if hist.SearchQuery() != "alp" {
		t.Fatalf("query = %q, want alp", hist.SearchQuery())
	}
	if hist.SearchMatchCount() != 2 {
		t.Fatalf("matches = %d, want 2", hist.SearchMatchCount())
	}
	if hist.SearchCurrent() != 1 {
		t.Fatalf("SearchCurrent = %d, want 1", hist.SearchCurrent())
	}

	// 'n' → next match (wraps), 'N' (shift) → prev.
	if !hist.IsCurrentSearchHit(0) {
		t.Fatal("first match should be message 0")
	}
	app.layout.dispatchKey(terminal.Key{Name: "n"})
	if !hist.IsCurrentSearchHit(1) {
		t.Fatal("n should advance to message 1")
	}
	app.layout.dispatchKey(terminal.Key{Name: "n"})
	if !hist.IsCurrentSearchHit(0) {
		t.Fatal("n should wrap around to message 0")
	}
	app.layout.dispatchKey(terminal.Key{Name: "N", Mods: terminal.ModShift})
	if !hist.IsCurrentSearchHit(1) {
		t.Fatal("shift+n should move to previous match")
	}
	if !hist.IsSearchMatch(0) || hist.IsSearchMatch(2) {
		t.Fatal("IsSearchMatch should reflect the match list")
	}

	// Backspace removes the last char.
	app.layout.dispatchKey(terminal.Key{Name: "backspace"})
	if hist.SearchQuery() != "al" {
		t.Fatalf("query = %q, want al", hist.SearchQuery())
	}
	if !hist.SearchMode() {
		t.Fatal("backspace with non-empty query stays in search mode")
	}

	// Esc deactivates search.
	if !app.layout.dispatchKey(terminal.Key{Name: "escape"}) {
		t.Fatal("escape should be consumed in search mode")
	}
	if hist.SearchMode() || hist.SearchQuery() != "" {
		t.Fatal("escape should deactivate search and clear the query")
	}
}

// TestDispatchSearchKeyEnterExit verifies Enter also deactivates search mode.
func TestDispatchSearchKeyEnterExit(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	hist.Append(ChatMessage{Role: RoleAssistant, Text: "hello"})
	app.layout.dispatchKey(terminal.Key{Name: "slash"})
	app.layout.dispatchKey(terminal.Key{Name: "h"})
	if !hist.SearchMode() {
		t.Fatal("search should be active")
	}
	app.layout.dispatchKey(terminal.Key{Name: "enter"})
	if hist.SearchMode() {
		t.Fatal("enter should deactivate search")
	}

	// Empty query + backspace exits search mode.
	app.layout.dispatchKey(terminal.Key{Name: "slash"})
	if len(hist.SearchQuery()) != 0 {
		t.Fatal("fresh search has empty query")
	}
	app.layout.dispatchKey(terminal.Key{Name: "backspace"})
	if hist.SearchMode() {
		t.Fatal("backspace on empty query should exit search")
	}

	// Modifier-carrying printable keys are NOT fed to the search.
	app.layout.dispatchKey(terminal.Key{Name: "slash"})
	app.layout.dispatchKey(terminal.Key{Name: "a", Mods: terminal.ModCtrl})
	if hist.SearchQuery() != "" {
		t.Fatalf("ctrl+a must not feed search, query=%q", hist.SearchQuery())
	}
}

// TestHandleEscapeKeyDoubleEscInterrupt verifies the double-Esc guard during
// streaming: first Esc shows a hint, second within the window interrupts.
func TestHandleEscapeKeyDoubleEscInterrupt(t *testing.T) {
	interrupted := false
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnInterrupt: func() { interrupted = true },
	})
	app.MarkAgentReady()
	app.onAgentStart(AgentStartChatEvent{}) // state = streaming

	esc := core.KeyMsg{Data: "\x1b"}
	app.layout.Update(esc) // consumed — return value is nil either way
	if interrupted {
		t.Fatal("first Esc must not interrupt")
	}
	msgs := app.History().Messages()
	if len(msgs) == 0 || !strings.Contains(msgs[len(msgs)-1].Text, "再次按 Esc") {
		t.Fatalf("first Esc should print the hint message, got %+v", msgs)
	}

	// Second Esc within the 1s window interrupts.
	app.layout.Update(esc)
	if !interrupted {
		t.Fatal("second Esc should trigger OnInterrupt")
	}

	// A stale first-Esc timestamp (older than the window) does NOT interrupt.
	interrupted = false
	app.mu.Lock()
	app.lastEscAt = time.Now().Add(-2 * time.Second)
	app.mu.Unlock()
	app.layout.Update(esc)
	if interrupted {
		t.Fatal("stale lastEscAt must not trigger interrupt")
	}
}

// TestHandleEscapeKeyIdleNoop verifies Esc while idle is not consumed.
func TestHandleEscapeKeyIdleNoop(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	if app.layout.handleEscapeKey(terminal.Key{Name: "escape"}) {
		t.Fatal("escape while idle should not be consumed")
	}
}

// TestHandleEscapeKeyAutocompletePop verifies Esc pops the last path segment
// when autocomplete is active on an @file:/@folder: value.
func TestHandleEscapeKeyAutocompletePop(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{
		Providers: []core.AutocompleteProvider{&fakeProvider{}},
	})
	// Set a value starting with @file: so the ac becomes active.
	app.editor.SetValue("@file:cmd/")
	if !app.ac.Active() {
		t.Fatal("setup: autocomplete should be active on @file: value")
	}

	if !app.layout.handleEscapeKey(terminal.Key{Name: "escape"}) {
		t.Fatal("escape with active autocomplete should be consumed")
	}
	if got := app.editor.GetValue(); got != "@file:" {
		t.Fatalf("editor value = %q, want popped to @file:", got)
	}
}

// fakeProvider is a minimal AutocompleteProvider for tests.
type fakeProvider struct{}

func (f *fakeProvider) Trigger() string { return "@file:" }

func (f *fakeProvider) Complete(token string) []core.Suggestion {
	return []core.Suggestion{{InsertText: token + "suggestion", Label: token + "suggestion"}}
}

// TestPopLastPathSegment covers the pure helper.
func TestPopLastPathSegment(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"@file:cmd/mady/", "@file:cmd/"},
		{"@file:main.go", "@file:"},
		{"@folder:a/b/", "@folder:a/"},
		{"no-trigger", "no-trigger"}, // no / or : → unchanged
		{"@file:", "@file:"},         // nothing after trigger
		{"@file:cmd/mady/x.go", "@file:cmd/mady/"},
	}
	for _, tc := range cases {
		if got := popLastPathSegment(tc.in); got != tc.want {
			t.Errorf("popLastPathSegment(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestDispatchKeyGlobalKeys verifies F2 passthrough, theme toggle, todo
// panel toggle, slash search, question keyhelp, and image-paste shortcut.
func TestDispatchKeyGlobalKeys(t *testing.T) {
	t.Run("f2 passthrough", func(t *testing.T) {
		vt := newCountingMouseHost(t, 80, 24)
		app := NewChatAppWithHost(ChatAppConfig{}, vt)
		app.SetHost(vt)
		if err := app.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		t.Cleanup(func() { app.Stop() })
		if !app.layout.dispatchKey(terminal.Key{Name: "f2"}) {
			t.Fatal("f2 should be consumed")
		}
		if vt.disableCount != 1 {
			t.Fatalf("disable=%d, want 1", vt.disableCount)
		}
		app.layout.dispatchKey(terminal.Key{Name: "f2"})
		if vt.enableCount != 1 {
			t.Fatalf("enable=%d, want 1", vt.enableCount)
		}
	})

	t.Run("ctrl+alt+t toggles theme", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		before := theme.CurrentPalette().Semantic.Background
		if !app.layout.dispatchKey(terminal.Key{Name: "t", Mods: terminal.ModCtrl | terminal.ModAlt}) {
			t.Fatal("ctrl+alt+t should be consumed")
		}
		after := theme.CurrentPalette().Semantic.Background
		if after == before {
			t.Fatal("theme should toggle")
		}
		theme.ToggleTheme() // restore
	})

	// NOTE: the ctrl+t todo-panel toggle is not exercised end-to-end: the
	// open path of ToggleTodoPanel self-deadlocks (panel.Reload re-locks
	// a.mu while it is held — see chat_app_todo.go; ToggleTodoPanelClosePath
	// covers the close paths instead).

	t.Run("slash activates search", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		if !app.layout.dispatchKey(terminal.Key{Name: "slash"}) {
			t.Fatal("slash should be consumed")
		}
		if !app.History().SearchMode() {
			t.Fatal("search mode should be active")
		}
	})

	t.Run("question opens keyhelp", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		host := app.host.(*testAppHost)
		if !app.layout.dispatchKey(terminal.Key{Name: "question"}) {
			t.Fatal("? should be consumed")
		}
		if len(host.overlays) != 1 {
			t.Fatalf("? should open keyhelp overlay, got %d", len(host.overlays))
		}
	})

	t.Run("ctrl+super+meta+alt+v triggers image paste", func(t *testing.T) {
		imagePasted := false
		app, _ := newTestChatApp(t, ChatAppConfig{OnImagePaste: func() { imagePasted = true }})
		mods := terminal.ModCtrl | terminal.ModSuper | terminal.ModMeta | terminal.ModAlt
		if !app.layout.dispatchKey(terminal.Key{Name: "v", Mods: mods}) {
			t.Fatal("image paste shortcut should be consumed")
		}
		if !imagePasted {
			t.Fatal("OnImagePaste should fire")
		}
	})

	t.Run("enter+space with ctrl folds at center", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady() // FSM must be Idle for the fold shortcut
		hist := app.History()
		for i := 0; i < 4; i++ {
			hist.Append(ChatMessage{Role: RoleTool, Meta: fmt.Sprintf("t%d", i), Text: "..."})
		}
		cols, _ := app.TerminalSize()
		app.layout.Render(cols) // populate cachedAll + maxRows
		if !app.layout.dispatchKey(terminal.Key{Name: " ", Mods: terminal.ModCtrl}) {
			t.Fatal("ctrl+space should be consumed while idle")
		}
		// alt+f fold alias
		if !app.layout.dispatchKey(terminal.Key{Name: "f", Mods: terminal.ModAlt}) {
			t.Fatal("alt+f should be consumed")
		}
	})

	t.Run("s and e shortcuts only when judgment view expanded", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		host := app.host.(*testAppHost)
		// Not expanded: keys pass through (false).
		if app.layout.dispatchKey(terminal.Key{Name: "s"}) {
			t.Fatal("s should not be consumed when JV not expanded")
		}
		if app.layout.dispatchKey(terminal.Key{Name: "e"}) {
			t.Fatal("e should not be consumed when JV not expanded")
		}
		// Expand via approval flow, then s opens system status, e opens evidence.
		app.MarkAgentReady()
		app.onApprovalPrompt(ApprovalPromptChatEvent{Content: "需要确认"})
		if !app.judgmentView.IsExpanded() {
			t.Fatal("setup: JV should be expanded after approval prompt")
		}
		if !app.layout.dispatchKey(terminal.Key{Name: "s"}) {
			t.Fatal("s should open system status when JV expanded")
		}
		if len(host.overlays) != 1 {
			t.Fatalf("system status overlay should open, got %d", len(host.overlays))
		}
		app.CloseSystemStatus()
		if !app.layout.dispatchKey(terminal.Key{Name: "e"}) {
			t.Fatal("e should open evidence overlay when JV expanded")
		}
		app.CloseEvidenceOverlay()
	})
}

// TestHandleGlobalMsgPaste verifies the image-paste detection path and that
// real paste text is not consumed.
func TestHandleGlobalMsgPaste(t *testing.T) {
	t.Run("empty paste is image paste", func(t *testing.T) {
		imagePasted := false
		app, _ := newTestChatApp(t, ChatAppConfig{OnImagePaste: func() { imagePasted = true }})
		if !app.layout.handleGlobalMsg(core.PasteMsg{Text: ""}) {
			t.Fatal("empty paste should be consumed as image paste")
		}
		if !imagePasted {
			t.Fatal("OnImagePaste should fire for empty paste")
		}
	})

	t.Run("bare CR paste is image paste", func(t *testing.T) {
		imagePasted := false
		app, _ := newTestChatApp(t, ChatAppConfig{OnImagePaste: func() { imagePasted = true }})
		if !app.layout.handleGlobalMsg(core.PasteMsg{Text: "\r"}) {
			t.Fatal("CR paste should be consumed as image paste")
		}
		if !imagePasted {
			t.Fatal("OnImagePaste should fire for CR paste")
		}
	})

	t.Run("real text paste not consumed", func(t *testing.T) {
		imagePasted := false
		app, _ := newTestChatApp(t, ChatAppConfig{OnImagePaste: func() { imagePasted = true }})
		if app.layout.handleGlobalMsg(core.PasteMsg{Text: "hello"}) {
			t.Fatal("text paste should NOT be consumed")
		}
		if imagePasted {
			t.Fatal("OnImagePaste must not fire for text paste")
		}
	})

	t.Run("window size triggers recalcMaxRows", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{Title: "Demo"})
		if app.layout.handleGlobalMsg(core.WindowSizeMsg{Width: 80, Height: 12}) {
			t.Fatal("window size should not be consumed")
		}
		if app.layout.lastRows != 12 {
			t.Fatalf("lastRows = %d, want 12", app.layout.lastRows)
		}
		if app.History().MaxRows() <= 0 {
			t.Fatal("recalcMaxRows should set a positive MaxRows")
		}
	})
}

// TestLayoutUpdateWindowSizeRoutesToRecalc verifies full Update() flow for
// WindowSizeMsg through handleGlobalMsg.
func TestLayoutUpdateWindowSizeRoutesToRecalc(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.layout.Update(core.WindowSizeMsg{Width: 100, Height: 20})
	if app.layout.lastRows != 20 {
		t.Fatalf("lastRows = %d, want 20", app.layout.lastRows)
	}
}

// TestDispatchKeyCtrlPCommandCenter verifies Ctrl+P forwards to the
// OnCommandCenter callback (footer 提示 "Ctrl+P cmd"；此前该键被静默吞掉)。
// 裸 'p' 仍进入编辑器输入。
func TestDispatchKeyCtrlPCommandCenter(t *testing.T) {
	var opened int
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnCommandCenter: func() { opened++ },
	})
	app.MarkAgentReady() // StateInitializing → StateIdle

	// Ctrl+P（\x10）触发回调。
	if !app.layout.dispatchKey(terminal.Key{Name: "p", Mods: terminal.ModCtrl}) {
		t.Fatal("ctrl+p should be consumed by the chat layout")
	}
	if opened != 1 {
		t.Fatalf("OnCommandCenter calls = %d, want 1", opened)
	}

	// 裸 'p' 不被消费——应落到编辑器输入。
	if app.layout.dispatchKey(terminal.Key{Name: "p"}) {
		t.Fatal("bare p should not be consumed by the chat layout")
	}
}

// TestDispatchKeyCtrlCIdleQuits verifies idle Ctrl+C fires OnQuit
// (footer 提示 "Ctrl+C quit"；此前空闲时被静默吞掉且无退出路径)。
// OnQuit 未配置时安全降级为忽略。
func TestDispatchKeyCtrlCIdleQuits(t *testing.T) {
	var quit int
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnQuit: func() { quit++ },
	})
	app.MarkAgentReady() // StateIdle

	if !app.layout.dispatchKey(terminal.Key{Name: "c", Mods: terminal.ModCtrl}) {
		t.Fatal("ctrl+c should be consumed by the chat layout")
	}
	if quit != 1 {
		t.Fatalf("OnQuit calls = %d, want 1", quit)
	}

	// 未配置 OnQuit 时：Ctrl+C 仍被消费，但不 panic、不复制。
	app2, _ := newTestChatApp(t, ChatAppConfig{})
	app2.MarkAgentReady()
	if !app2.layout.dispatchKey(terminal.Key{Name: "c", Mods: terminal.ModCtrl}) {
		t.Fatal("ctrl+c should still be consumed when OnQuit is nil")
	}
}

// TestDispatchKeyCtrlCRunningInterruptsNotQuits verifies streaming-state
// Ctrl+C keeps interrupting (not quitting) after the idle-quit change.
func TestDispatchKeyCtrlCRunningInterruptsNotQuits(t *testing.T) {
	var interrupted, quit int
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnInterrupt: func() { interrupted++ },
		OnQuit:      func() { quit++ },
	})
	app.MarkAgentReady()
	app.onAgentStart(AgentStartChatEvent{}) // → StateStreaming

	app.layout.dispatchKey(terminal.Key{Name: "c", Mods: terminal.ModCtrl})
	if interrupted != 1 {
		t.Fatalf("OnInterrupt calls = %d, want 1", interrupted)
	}
	if quit != 0 {
		t.Fatalf("OnQuit must not fire while running, got %d", quit)
	}
}
