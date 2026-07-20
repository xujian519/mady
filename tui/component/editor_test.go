package component

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestEditorInsertAndNewline(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "hello"})
	e.Update(core.KeyMsg{Data: "\x1b\r"})
	e.Update(core.KeyMsg{Data: "world"})
	if got := e.GetValue(); got != "hello\nworld" {
		t.Fatalf("want %q, got %q", "hello\nworld", got)
	}
}

func TestEditorUndoRedo(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "abc"})
	if e.GetValue() != "abc" {
		t.Fatalf("initial: %q", e.GetValue())
	}
	e.Update(core.KeyMsg{Data: "\x1a"})
	v := e.GetValue()
	if v == "abc" {
		t.Fatalf("expected undo to shorten: %q", v)
	}
}

func TestEditorCursorRenderMarker(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "hi"})
	lines := e.Render(20)
	if len(lines) == 0 {
		t.Fatalf("expected render output")
	}
	found := false
	for _, l := range lines {
		if containsMarker(l) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cursor marker missing from %v", lines)
	}
}

func TestEditorSelectAllIsEditorScoped(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "hello"})
	e.Update(core.KeyMsg{Data: "\x1b[97;9u"}) // Kitty CSI-u: super+a

	if got := e.GetSelectedText(); got != "hello" {
		t.Fatalf("selected text: want %q, got %q", "hello", got)
	}

	e.Update(core.KeyMsg{Data: "x"})
	if got := e.GetValue(); got != "x" {
		t.Fatalf("typing should replace selected editor text, got %q", got)
	}
	if got := e.GetSelectedText(); got != "" {
		t.Fatalf("selection should clear after replacement, got %q", got)
	}
}

func TestEditorSelectAllDelete(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "hello"})
	e.Update(core.KeyMsg{Data: "\x1b\r"})
	e.Update(core.KeyMsg{Data: "world"})
	e.Update(core.KeyMsg{Data: "\x1b[97;9u"}) // Kitty CSI-u: super+a
	e.Update(core.KeyMsg{Data: "\x7f"})

	if got := e.GetValue(); got != "" {
		t.Fatalf("delete should clear selected editor text, got %q", got)
	}
}

func TestEditorMouseDragSelection(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "hello world"})
	e.Render(40) // populate lastVisuals; default prompt "> " is 2 cols wide

	e.Update(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 2})  // buffer col 0
	e.Update(core.MouseMsg{Action: core.MouseMotion, Row: 0, Col: 7}) // buffer col 5
	e.Update(core.MouseMsg{Action: core.MouseRelease, Row: 0, Col: 7})

	if got := e.GetSelectedText(); got != "hello" {
		t.Fatalf("want %q, got %q", "hello", got)
	}
}

func TestEditorMouseDragSelectionMultiline(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "foo"})
	e.Update(core.KeyMsg{Data: "\x1b\r"}) // hard newline
	e.Update(core.KeyMsg{Data: "bar"})
	e.Render(40)

	// Row 0: "> foo" (prompt 2 cols); Row 1: "  bar" (continuation prompt 2 cols).
	e.Update(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 3})  // after 'f' in "foo"
	e.Update(core.MouseMsg{Action: core.MouseMotion, Row: 1, Col: 4}) // after "ba" in "bar"
	e.Update(core.MouseMsg{Action: core.MouseRelease, Row: 1, Col: 4})

	if want, got := "oo\nba", e.GetSelectedText(); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestEditorMouseClickWithoutDragNoSelection(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "hello"})
	e.Render(40)

	e.Update(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 4})
	e.Update(core.MouseMsg{Action: core.MouseRelease, Row: 0, Col: 4})

	if got := e.GetSelectedText(); got != "" {
		t.Fatalf("expected empty selection for a plain click, got %q", got)
	}
}

func TestEditorMouseSelectionClearsOnKeystroke(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "hello"})
	e.Render(40)

	e.Update(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 2})
	e.Update(core.MouseMsg{Action: core.MouseMotion, Row: 0, Col: 5})
	e.Update(core.MouseMsg{Action: core.MouseRelease, Row: 0, Col: 5})
	if e.GetSelectedText() == "" {
		t.Fatalf("expected non-empty selection before keystroke")
	}

	e.Update(core.KeyMsg{Data: "!"})
	if got := e.GetSelectedText(); got != "" {
		t.Fatalf("expected selection cleared after keystroke, got %q", got)
	}
}

func TestEditorClearSelectionResetsMouseDrag(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "hello"})
	e.Render(40)

	e.Update(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 2})
	e.Update(core.MouseMsg{Action: core.MouseMotion, Row: 0, Col: 5})
	e.Update(core.MouseMsg{Action: core.MouseRelease, Row: 0, Col: 5})
	e.ClearSelection()

	if got := e.GetSelectedText(); got != "" {
		t.Fatalf("expected empty selection after ClearSelection, got %q", got)
	}
}

// TestEditorCopyViaOnCopy verifies that handleCopy fires onCopy with the
// selected text when Select All is active.
func TestEditorCopyViaOnCopy(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "hello world"})

	var copied string
	e.OnCopy(func(text string) { copied = text })

	e.SelectAll()
	e.handleCopy()
	if copied != "hello world" {
		t.Fatalf("onCopy: want %q, got %q", "hello world", copied)
	}
}

// TestEditorCopyNoSelection verifies handleCopy does not fire onCopy when
// there is no selection.
func TestEditorCopyNoSelection(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "hello world"})

	fired := false
	e.OnCopy(func(text string) { fired = true })

	e.handleCopy()
	if fired {
		t.Fatal("onCopy should not fire without a selection")
	}
}

// TestEditorInsertText verifies insertText correctly inserts text at the
// cursor position, handling newlines.
func TestEditorInsertText(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.Update(core.KeyMsg{Data: "ab"})

	// Cursor is at position 2 ("ab|"). Insert "xy" in the middle.
	e.SetValue("ab")
	// Move cursor to position 1 (between 'a' and 'b').
	e.mu.Lock()
	e.col = 1
	e.mu.Unlock()

	e.insertText("XY")
	if got := e.GetValue(); got != "aXYb" {
		t.Fatalf("insertText single-line: want %q, got %q", "aXYb", got)
	}
}

// TestEditorInsertTextMultiline verifies insertText correctly inserts
// multi-line text, creating new rows.
func TestEditorInsertTextMultiline(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.SetValue("ab")

	// Cursor at position 1 (between 'a' and 'b').
	e.mu.Lock()
	e.col = 1
	e.mu.Unlock()

	e.insertText("X\nY")
	if got := e.GetValue(); got != "aX\nYb" {
		t.Fatalf("insertText multi-line: want %q, got %q", "aX\nYb", got)
	}
}

// TestEditorInsertTextReplacesSelection verifies insertText replaces
// selected text when allSelected is true.
func TestEditorInsertTextReplacesSelection(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.SetValue("hello world")
	e.SelectAll()

	e.insertText("hi")
	if got := e.GetValue(); got != "hi" {
		t.Fatalf("insertText replace all: want %q, got %q", "hi", got)
	}
}

// TestEditorPasteViaAsyncCmd verifies that handlePaste stores a Cmd in
// pastePendingCmd, that executing the Cmd produces a PasteMsg, and that
// feeding the PasteMsg back via Update inserts the text.
func TestEditorPasteViaAsyncCmd(t *testing.T) {
	e := NewEditor(nil)
	e.SetFocused(true)
	e.SetValue("ab")
	e.mu.Lock()
	e.col = 1
	e.mu.Unlock()

	var cmdRan bool
	e.OnPaste(func() core.Cmd {
		return func() core.Msg {
			cmdRan = true
			return core.PasteMsg{Text: "PASTE"}
		}
	})

	// handlePaste should store the Cmd in pastePendingCmd.
	e.handlePaste()

	e.mu.RLock()
	pending := e.pastePendingCmd
	e.mu.RUnlock()
	if pending == nil {
		t.Fatal("pastePendingCmd should be non-nil after handlePaste")
	}

	// Simulate what the TUI event loop does: execute the Cmd and feed the
	// result back to Update.
	msg := pending()
	if msg == nil {
		t.Fatal("paste Cmd returned nil Msg")
	}
	pm, ok := msg.(core.PasteMsg)
	if !ok {
		t.Fatalf("expected PasteMsg, got %T", msg)
	}
	if pm.Text != "PASTE" {
		t.Fatalf("PasteMsg.Text: want %q, got %q", "PASTE", pm.Text)
	}

	// Feed PasteMsg back to Update — should insert text via insertText.
	cmd2 := e.Update(msg)
	if cmd2 != nil {
		t.Fatal("Update returned non-nil Cmd after PasteMsg")
	}
	if got := e.GetValue(); got != "aPASTEb" {
		t.Fatalf("paste result: want %q, got %q", "aPASTEb", got)
	}
	if !cmdRan {
		t.Fatal("paste Cmd was not executed")
	}
}

func containsMarker(s string) bool {
	for i := 0; i+len(core.CursorMarker) <= len(s); i++ {
		if s[i:i+len(core.CursorMarker)] == core.CursorMarker {
			return true
		}
	}
	return false
}
