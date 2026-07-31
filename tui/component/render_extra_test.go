package component

import (
	"testing"
)

// TestEditorRenderPlaceholder covers the empty-editor placeholder path
// (renderPlaceholder) for focused and unfocused editors with minRows growth.
func TestEditorRenderPlaceholder(t *testing.T) {
	e := NewEditor(nil)
	e.SetPlaceholder("输入消息…")
	e.SetMinRows(2)
	e.SetPaddingX(1)
	lines := e.Render(30)
	if len(lines) == 0 {
		t.Fatal("expected render")
	}
	if !contains(stringsJoin(lines), "输入消息") {
		t.Fatalf("expected placeholder text, got %q", stringsJoin(lines))
	}
	if len(lines) < 2 {
		t.Fatalf("expected minRows growth, got %d lines", len(lines))
	}
}

func stringsJoin(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l
	}
	return out
}

// TestEditorRenderEmptyNoPlaceholder: without placeholder the render still
// produces prompt lines (structure only).
func TestEditorRenderEmptyNoPlaceholder(t *testing.T) {
	e := NewEditor(nil)
	lines := e.Render(30)
	if len(lines) == 0 {
		t.Fatal("expected render")
	}
}

// TestEditorHandleCopySelectedText covers handleCopy with an active
// mouse selection.
func TestEditorHandleCopySelectedText(t *testing.T) {
	e := NewEditor(nil)
	e.SetValue("hello world")
	var copied string
	e.OnCopy(func(s string) { copied = s })
	// Select "hello" via internal selection state.
	e.mu.Lock()
	e.selActive = true
	e.selStart = editorSelPos{row: 0, col: 0}
	e.selEnd = editorSelPos{row: 0, col: 5}
	e.mu.Unlock()
	e.handleCopy()
	if copied != "hello" {
		t.Fatalf("expected 'hello' copied, got %q", copied)
	}
	// No selection and no callback — no-op.
	e.ClearSelection()
	e.OnCopy(nil)
	e.handleCopy()
}

// TestEditorHandleCopyAllSelected covers the allSelected branch.
func TestEditorHandleCopyAllSelected(t *testing.T) {
	e := NewEditor(nil)
	e.SetValue("line1\nline2")
	var copied string
	e.OnCopy(func(s string) { copied = s })
	e.mu.Lock()
	e.allSelected = true
	e.mu.Unlock()
	e.handleCopy()
	if copied != "line1\nline2" {
		t.Fatalf("expected full value copied, got %q", copied)
	}
}

// TestFooterInvalidate covers Footer.Invalidate no-op.
func TestFooterInvalidate(t *testing.T) {
	f := NewFooter()
	f.Invalidate() // no-op
	if lines := f.Render(80); len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}
