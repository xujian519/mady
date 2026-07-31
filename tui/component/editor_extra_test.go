package component

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

func TestEditorSetters(t *testing.T) {
	e := NewEditor(nil)
	e.SetPrompt("A> ", "B> ")
	if e.promptFirst != "A> " || e.promptCont != "B> " {
		t.Fatalf("unexpected prompts %q/%q", e.promptFirst, e.promptCont)
	}
	e.SetPromptFn(func(s string) string { return s })
	e.SetTextFn(func(s string) string { return s })
	e.SetSelectedBg("\x1b[44m")
	e.SetPlaceholder("hint")
	if e.placeText != "hint" {
		t.Fatalf("expected placeholder, got %q", e.placeText)
	}
	e.SetPlaceholderFn(func(s string) string { return s })
	e.SetMinRows(0) // clamped to 1
	if e.minRows != 1 {
		t.Fatalf("expected minRows 1, got %d", e.minRows)
	}
	e.SetMinRows(3)
	e.SetMaxRows(0) // clamped to 1
	if e.maxRows != 1 {
		t.Fatalf("expected maxRows 1, got %d", e.maxRows)
	}
	e.SetMaxRows(20)
	e.SetPaddingX(-1) // clamped to 0
	if e.padX != 0 {
		t.Fatalf("expected padX 0, got %d", e.padX)
	}
	e.SetPaddingX(2)
	e.SetFocusIndicator("▌")
	if e.focusIndicator != "▌" {
		t.Fatalf("unexpected focus indicator %q", e.focusIndicator)
	}
	e.SetFocused(true)
	if !e.IsFocused() {
		t.Fatal("expected focused")
	}
	e.SetFocused(false)
	if e.IsFocused() {
		t.Fatal("expected unfocused")
	}
	e.Invalidate() // no-op
}

func TestEditorSetValueAndClear(t *testing.T) {
	e := NewEditor(nil)
	e.SetValue("line1\nline2")
	if got := e.valueLocked(); got != "line1\nline2" {
		t.Fatalf("expected multi-line value, got %q", got)
	}
	if e.row != 1 {
		t.Fatalf("expected cursor row 1, got %d", e.row)
	}
	e.Clear()
	if got := e.valueLocked(); got != "" {
		t.Fatalf("expected empty after Clear, got %q", got)
	}
}

func TestEditorSubmitCallback(t *testing.T) {
	e := NewEditor(nil)
	e.SetValue("hello")
	submitted := make(chan string, 1)
	e.OnSubmit(func(s string) { submitted <- s })
	e.submit()
	select {
	case got := <-submitted:
		if got != "hello" {
			t.Fatalf("expected hello, got %q", got)
		}
	default:
		t.Fatal("expected submit callback")
	}
	// No callback — no panic.
	e2 := NewEditor(nil)
	e2.submit()
	// OnCancel / OnChange / OnCopy / OnPaste wiring.
	e.OnCancel(func() {})
	e.OnChange(func(string) {})
	e.OnCopy(func(string) {})
	e.OnPaste(func() core.Cmd { return nil })
}

func TestEditorAutocompleteActiveCheck(t *testing.T) {
	e := NewEditor(nil)
	if e.isAutocompleteActive() {
		t.Fatal("expected false with no check")
	}
	e.SetAutocompleteActiveCheck(func() bool { return true })
	if !e.isAutocompleteActive() {
		t.Fatal("expected true when check reports active")
	}
	e.SetAutocompleteActiveCheck(func() bool { return false })
	if e.isAutocompleteActive() {
		t.Fatal("expected false when check reports inactive")
	}
	e.SetAutocompleteActiveCheck(nil)
	if e.isAutocompleteActive() {
		t.Fatal("expected false after nil check")
	}
}

func TestEditorRedoEmptyFuture(t *testing.T) {
	e := NewEditor(nil)
	e.SetValue("abc")
	e.redo() // no future — no-op
	if got := e.valueLocked(); got != "abc" {
		t.Fatalf("expected unchanged value, got %q", got)
	}
}

func TestEditorRemoveBeforeCursorLocked(t *testing.T) {
	e := NewEditor(nil)
	// Single line: remove within the line (3 runes before the cursor at 5).
	e.SetValue("hello world")
	e.mu.Lock()
	e.row = 0
	e.col = 5
	e.removeBeforeCursorLocked(3)
	got := string(e.lines[0])
	e.mu.Unlock()
	if got != "he world" {
		t.Fatalf("expected 'he world', got %q", got)
	}

	// Removal spanning line breaks: "ab\ncd" with cursor at col 1 of row 1,
	// remove 3 -> deletes "b\nc" and merges rows, leaving "ad".
	e.SetValue("ab\ncd")
	e.mu.Lock()
	e.row = 1
	e.col = 1
	e.removeBeforeCursorLocked(3)
	got = e.valueLocked()
	e.mu.Unlock()
	if got != "ad" {
		t.Fatalf("expected 'ad' after cross-line removal, got %q", got)
	}

	// Removal beyond buffer start stops at the beginning.
	e.SetValue("ab")
	e.mu.Lock()
	e.row = 0
	e.col = 0
	e.removeBeforeCursorLocked(10)
	got = e.valueLocked()
	e.mu.Unlock()
	if got != "ab" {
		t.Fatalf("expected unchanged 'ab', got %q", got)
	}
}

func TestInputSetters(t *testing.T) {
	i := NewInput(nil)
	i.SetPrompt("> ")
	i.SetPromptFn(func(s string) string { return s })
	i.SetTextFn(func(s string) string { return s })
	i.SetSelectedBg("")
	i.SetPlaceholder("ph")
	i.SetPlaceholderFn(func(s string) string { return s })
	i.SetPaddingX(-2) // clamped to 0
	i.SetValue("val")
	if i.GetValue() != "val" {
		t.Fatalf("expected 'val', got %q", i.GetValue())
	}
	i.Clear()
	if i.GetValue() != "" {
		t.Fatalf("expected empty after Clear, got %q", i.GetValue())
	}
	i.SelectAll()
	if i.GetSelectedText() != "" {
		t.Fatalf("expected empty selection text, got %q", i.GetSelectedText())
	}
	i.SetValue("abc")
	i.SelectAll()
	if i.GetSelectedText() != "abc" {
		t.Fatalf("expected 'abc' selected, got %q", i.GetSelectedText())
	}
	i.ClearSelection()
	i.OnSubmit(func(string) {})
	i.OnChange(func(string) {})
	i.OnHistoryPrev(func() (string, bool) { return "", false })
	i.OnHistoryNext(func() (string, bool) { return "", false })
	i.SetFocused(true)
	if !i.IsFocused() {
		t.Fatal("expected focused")
	}
	i.SetFocused(false)
	if i.IsFocused() {
		t.Fatal("expected unfocused")
	}
}

// TestInputSetPaddingXNegative verifies padding clamp in Input.
func TestInputSetPaddingXNegative(t *testing.T) {
	i := NewInput(nil)
	i.SetPaddingX(-5)
	if i.paddingX != 0 {
		t.Fatalf("expected paddingX 0, got %d", i.paddingX)
	}
	i.SetPaddingX(3)
	if i.paddingX != 3 {
		t.Fatalf("expected paddingX 3, got %d", i.paddingX)
	}
}

// TestNewEditorWithGlobalKeybindings verifies the nil-arg path.
func TestNewEditorWithGlobalKeybindings(t *testing.T) {
	e := NewEditor(nil)
	if e.km == nil {
		t.Fatal("expected global keybindings wired")
	}
	_ = terminal.GetGlobalKeybindings()
}
