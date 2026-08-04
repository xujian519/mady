package component

// Tests for ToolCard: verifies the bar color heuristic, collapsed summary,
// and that the diff block body is appended beneath the header.

import (
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
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
	long := strings.Repeat("x", 100)
	lines := RenderToolCard(ToolCardConfig{
		Name: "t", Status: long, Collapsed: true,
	}, theme, 40)
	// Collapsed summary is truncated to fit a single line.
	plain := core.StripAnsi(lines[0])
	if !strings.Contains(plain, "…") {
		t.Errorf("long status should be truncated with ellipsis: %q", plain)
	}
}

func TestToolCardCompactHeaderSingleLine(t *testing.T) {
	theme := testToolCardTheme()
	long := strings.Repeat("x", 200)
	lines := RenderToolCard(ToolCardConfig{
		Name: "bash", Status: long,
	}, theme, 40)
	if len(lines) != 1 {
		t.Fatalf("expanded tool card header should be one line, got %d: %v", len(lines), lines)
	}
	if core.VisibleWidth(lines[0]) > 40 {
		t.Errorf("header exceeds width: %q", lines[0])
	}
}

func TestToolCardHeaderNarrowWidth(t *testing.T) {
	theme := testToolCardTheme()
	// 极窄窗口：prefix（序号+工具名）+meta 本身已超过 width，status 无剩余空间。
	cfg := ToolCardConfig{
		Name:      "transfer_to_patent", // 18 列工具名
		Status:    "done: analysis complete",
		Duration:  1 * time.Second, // meta "(1s)"
		Collapsed: true,
	}
	for _, width := range []int64{10, 15, 20} {
		lines := RenderToolCard(cfg, theme, width)
		if len(lines) != 1 {
			t.Fatalf("width %d: expected 1 line, got %d", width, len(lines))
		}
		if vw := core.VisibleWidth(lines[0]); vw > width {
			t.Errorf("width %d: header exceeds width (got %d): %q", width, vw, core.StripAnsi(lines[0]))
		}
		plain := core.StripAnsi(lines[0])
		if !strings.Contains(plain, "[+]") {
			t.Errorf("width %d: marker lost in narrow render: %q", width, plain)
		}
	}
	// 普通宽度不受影响：状态完整可见。
	lines := RenderToolCard(cfg, theme, 80)
	plain := core.StripAnsi(lines[0])
	if !strings.Contains(plain, "done: analysis") {
		t.Errorf("normal width should keep status visible: %q", plain)
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
