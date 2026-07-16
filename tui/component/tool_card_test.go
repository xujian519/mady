package component

// Tests for ToolCard: verifies the bar color heuristic, collapsed summary,
// and that the diff block body is appended beneath the header.

import (
	"strings"
	"testing"
)

func testToolCardTheme() ToolCardTheme {
	return ToolCardTheme{
		Border:        func(s string) string { return "[" + s + "]" },
		Success:       func(s string) string { return "<" + s + ">" },
		Error:         func(s string) string { return "!" + s + "!" },
		Title:         func(s string) string { return "*" + s + "*" },
		Dim:           func(s string) string { return "~" + s + "~" },
		MarkdownTheme: defaultMarkdownTheme(),
	}
}

func TestToolCardBarColorByStatus(t *testing.T) {
	theme := testToolCardTheme()
	cases := []struct {
		status string
		want   string // marker the rendered bar must contain
	}{
		{"✓ done", "<"},      // success
		{"✗ failed: x", "!"}, // error
		{"running", "["},     // default border
	}
	for _, c := range cases {
		lines := RenderToolCard(ToolCardConfig{Name: "t", Status: c.status}, theme, 40)
		if len(lines) == 0 {
			t.Fatalf("status %q: empty render", c.status)
		}
		if !strings.Contains(lines[0], c.want) {
			t.Errorf("status %q: expected bar marker %q in %q", c.status, c.want, lines[0])
		}
	}
}

func TestToolCardCollapsedSummary(t *testing.T) {
	theme := testToolCardTheme()
	lines := RenderToolCard(ToolCardConfig{
		Name: "edit", Status: "✓ done", Collapsed: true,
	}, theme, 40)
	if len(lines) != 1 {
		t.Fatalf("collapsed card should render exactly one line, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "[+]") {
		t.Errorf("collapsed card missing [+] marker: %q", lines[0])
	}
	if !strings.Contains(lines[0], "*edit*") {
		t.Errorf("collapsed card missing title: %q", lines[0])
	}
}

func TestToolCardLongStatusTruncatedInCollapsed(t *testing.T) {
	theme := testToolCardTheme()
	long := strings.Repeat("x", 200)
	lines := RenderToolCard(ToolCardConfig{
		Name: "t", Status: long, Collapsed: true,
	}, theme, 400)
	// Collapsed summary truncates to 117 + "...".
	if !strings.Contains(lines[0], "...") {
		t.Errorf("long status should be truncated with ...: %q", lines[0])
	}
}

func TestToolCardDiffBodyAppended(t *testing.T) {
	theme := testToolCardTheme()
	lines := RenderToolCard(ToolCardConfig{
		Name:     "edit_block",
		Status:   "✓ done",
		DiffText: "+added line\n-removed line",
	}, theme, 60)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "added line") {
		t.Errorf("diff body lost: %s", joined)
	}
	if !strings.Contains(joined, "removed line") {
		t.Errorf("diff body lost removed line: %s", joined)
	}
}
