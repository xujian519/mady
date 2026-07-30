package chat

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestSelectionEmptyInitially(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "hello"})
	_ = h.Render(40)

	if !h.isSelectionEmptyLocked() {
		t.Fatal("expected empty selection initially")
	}
}

func TestGetSelectedTextNoSelection(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "hello"})
	_ = h.Render(40)

	got := h.GetSelectedText()
	if got != "" {
		t.Fatalf("expected empty selection, got %q", got)
	}
}

func TestClearSelection(t *testing.T) {
	h := NewChatHistory()
	h.selActive = true
	h.selStart = selectionPos{line: 0, col: 0}
	h.selEnd = selectionPos{line: 1, col: 5}
	h.ClearSelection()

	if h.selActive {
		t.Fatal("expected selActive=false after ClearSelection")
	}
	got := h.GetSelectedText()
	if got != "" {
		t.Fatalf("expected empty after clear, got %q", got)
	}
}

func TestRenderSelectionHighlight(t *testing.T) {
	h := NewChatHistory()
	h.maxRows = 10
	h.selActive = true
	h.selStart = selectionPos{line: 0, col: 0}
	h.selEnd = selectionPos{line: 0, col: 5}

	line := "\x1b[31mAB\x1b[0m\x1b[32mCD\x1b[0mE"
	lines := []string{line}
	h.applySelectionHighlightLocked(lines, 80)

	row := core.ParseLine(lines[0])
	if row.IsRaw() {
		t.Fatalf("expected parsed row, got raw")
	}
	if len(row.Cells) < 5 {
		t.Fatalf("unexpected rendered cell count: %d", len(row.Cells))
	}
	base := row.Cells[0].Style
	for i := 1; i < 5; i++ {
		if !row.Cells[i].Style.Equal(base) {
			t.Fatalf("selected styles are not uniform at col=%d", i)
		}
	}
}

func TestSelectionSnapshotHighlight(t *testing.T) {
	h := NewChatHistory()
	h.maxRows = 10

	line := "\x1b[31mAB\x1b[0m\x1b[32mCD\x1b[0mE"
	lines := []string{line}
	h.applySelectionHighlightSnapshot(lines, 80,
		selectionPos{line: 0, col: 0},
		selectionPos{line: 0, col: 5},
	)

	row := core.ParseLine(lines[0])
	if row.IsRaw() {
		t.Fatalf("expected parsed row, got raw")
	}
	if len(row.Cells) < 5 {
		t.Fatalf("unexpected rendered cell count: %d", len(row.Cells))
	}
	base := row.Cells[0].Style
	for i := 1; i < 5; i++ {
		if !row.Cells[i].Style.Equal(base) {
			t.Fatalf("selected styles are not uniform at col=%d", i)
		}
	}
}
