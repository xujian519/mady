package chat

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

// TestNewChatAutocompleteApply verifies the OnApply wiring: applying a
// suggestion injects the trigger + suggestion text into the editor.
func TestNewChatAutocompleteApply(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{
		Providers: []core.AutocompleteProvider{&fakeProvider{}},
	})
	app.editor.SetValue("@file:cmd/")
	if !app.ac.Active() {
		t.Fatal("setup: autocomplete should be active")
	}

	// Enter applies the current suggestion (tui.select.confirm binding).
	app.ac.Update(core.KeyMsg{Data: "\r"})
	if got := app.editor.GetValue(); got != "@file:cmd/suggestion" {
		t.Fatalf("applied value = %q, want %q", got, "@file:cmd/suggestion")
	}
	if app.skipRefresh {
		t.Fatal("skipRefresh should be consumed by the next refresh")
	}
}

// TestNewChatAutocompleteDismiss verifies the OnDismiss wiring pops the last
// path segment on @file: values.
func TestNewChatAutocompleteDismiss(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{
		Providers: []core.AutocompleteProvider{&fakeProvider{}},
	})
	app.editor.SetValue("@file:cmd/mady/")
	if !app.ac.Active() {
		t.Fatal("setup: autocomplete should be active")
	}

	// Escape pops the last path segment (the popup re-opens at the parent).
	app.ac.Update(core.KeyMsg{Data: "\x1b"})
	if got := app.editor.GetValue(); got != "@file:cmd/" {
		t.Fatalf("dismissed value = %q, want %q", got, "@file:cmd/")
	}
}

// TestNewChatAutocompleteNilProviders verifies no autocomplete is built when
// no providers are configured.
func TestNewChatAutocompleteNilProviders(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	if app.ac != nil {
		t.Fatal("no providers → no autocomplete")
	}
}

// TestBindChatEditorEventsOnChange verifies editor OnChange wires autocomplete
// refresh only when a provider exists.
func TestBindChatEditorEventsOnChange(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{
		Providers: []core.AutocompleteProvider{&fakeProvider{}},
	})
	// Typing into the editor triggers OnChange → ac.Refresh.
	app.editor.Update(core.KeyMsg{Data: "@"})
	app.editor.Update(core.KeyMsg{Data: "f"})
	// @f does not activate the "@file:" trigger yet (word "@f").
	if app.ac.Active() {
		t.Fatal("autocomplete should not be active for @f")
	}
	app.editor.Update(core.KeyMsg{Data: "i"})
	app.editor.Update(core.KeyMsg{Data: "l"})
	app.editor.Update(core.KeyMsg{Data: "e"})
	app.editor.Update(core.KeyMsg{Data: ":"})
	if !app.ac.Active() {
		t.Fatal("autocomplete should activate on @file:")
	}
}

// TestBindChatEditorEventsSubmit verifies the editor's Enter routes through
// onEditorSubmit (already covered elsewhere; guards the binding).
func TestBindChatEditorEventsSubmit(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.editor.Update(core.KeyMsg{Data: "bind test"})
	app.editor.Update(core.KeyMsg{Data: "\r"})
	msgs := app.History().Messages()
	if len(msgs) != 1 || msgs[0].Role != RoleUser || msgs[0].Text != "bind test" {
		t.Fatalf("editor submit binding broken: %+v", msgs)
	}
}

// TestNewChatHistoryWithConfig verifies theme and renderer application.
func TestNewChatHistoryWithConfig(t *testing.T) {
	th := DefaultChatHistoryTheme()
	th.UserPrefix = "» "
	rr := &DefaultReasoningRenderer{Show: true}

	cfg := ChatAppConfig{Theme: &th, ReasoningRenderer: rr}
	h := newChatHistoryWithConfig(cfg)
	if h.theme.UserPrefix != "» " {
		t.Fatalf("theme not applied: %q", h.theme.UserPrefix)
	}
	if h.reasoningRenderer != rr {
		t.Fatal("reasoning renderer not applied")
	}

	// Default config keeps defaults.
	h2 := newChatHistoryWithConfig(ChatAppConfig{})
	if h2.theme.UserPrefix != DefaultChatHistoryTheme().UserPrefix {
		t.Fatalf("default theme not applied: %q", h2.theme.UserPrefix)
	}
	if _, ok := h2.reasoningRenderer.(HiddenReasoningRenderer); !ok {
		t.Fatalf("default renderer = %T, want HiddenReasoningRenderer", h2.reasoningRenderer)
	}
}

// TestNewChatEditorConfig verifies editor config knobs.
func TestNewChatEditorConfig(t *testing.T) {
	km := terminal.NewKeybindingsManager(terminal.DefaultKeybindings())
	editor := newChatEditor(ChatAppConfig{EditorMinRows: 2, EditorMaxRows: 5, EditorPrompt: ">> "}, km)
	if editor.GetValue() != "" {
		t.Fatal("fresh editor should be empty")
	}
}

// TestNewChatHeaderNil verifies no header without a title.
func TestNewChatHeaderNil(t *testing.T) {
	if h := newChatHeader(ChatAppConfig{}); h != nil {
		t.Fatal("no title → no header")
	}
	if h := newChatHeader(ChatAppConfig{Title: "T"}); h == nil {
		t.Fatal("title → header expected")
	}
}

// TestApplyChatDefaults verifies default editor config values.
func TestApplyChatDefaults(t *testing.T) {
	cfg := applyChatDefaults(ChatAppConfig{})
	if cfg.EditorMinRows != 1 || cfg.EditorMaxRows != 8 || cfg.EditorPrompt != "❯ " {
		t.Fatalf("defaults = %+v", cfg)
	}
	// Existing values are preserved.
	cfg2 := applyChatDefaults(ChatAppConfig{EditorMinRows: 3, EditorMaxRows: 9, EditorPrompt: "x"})
	if cfg2.EditorMinRows != 3 || cfg2.EditorMaxRows != 9 || cfg2.EditorPrompt != "x" {
		t.Fatalf("custom values overwritten: %+v", cfg2)
	}
}

// TestEditorFrameRender verifies the border-wrapped editor frame.
func TestEditorFrameRender(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	f := &editorFrame{editor: app.editor}
	out := f.Render(80)
	if len(out) < 3 {
		t.Fatalf("frame should have top+bottom borders, got %d lines", len(out))
	}
	first := core.StripAnsi(out[0])
	last := core.StripAnsi(out[len(out)-1])
	if !strings.HasPrefix(first, "╭") || !strings.HasSuffix(first, "╮") {
		t.Fatalf("top border should be ╭…╮, got %q", first)
	}
	if !strings.HasPrefix(last, "╰") || !strings.HasSuffix(last, "╯") {
		t.Fatalf("bottom border should be ╰…╯, got %q", last)
	}
	if !strings.Contains(last, "↑↓") {
		t.Fatalf("bottom border should contain the compact hint (wide frame), got %q", last)
	}
}
