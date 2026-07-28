package component

import (
	"strings"
	"testing"
)

func TestMarkdownHeadingsAndCode(t *testing.T) {
	md := NewMarkdown("# Title\n\nSome **bold** and `code`.\n\n```go\nfmt.Println(\"hi\")\n```")
	lines := md.Render(40)
	if len(lines) == 0 {
		t.Fatal("empty render")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Title") {
		t.Fatalf("missing title: %s", joined)
	}
	if !strings.Contains(joined, "fmt.Println") {
		t.Fatalf("missing code body: %s", joined)
	}
}

func TestMarkdownTable(t *testing.T) {
	md := NewMarkdown("| a | b |\n| --- | --- |\n| 1 | 2 |")
	lines := md.Render(20)
	if len(lines) < 4 {
		t.Fatalf("expected >=4 rows, got %d", len(lines))
	}
}

// TestRenderFenceBlockSoftWrap 是代码块超宽截断的回归测试。
// 修复前：renderFenceBlock 用 PadToWidth 处理超宽行（直接返回原样不换行），
// 导致超宽行被上层 normalizeLine 截断为省略号。
// 修复后：超宽代码行软换行，内容完整保留。
func TestRenderFenceBlockSoftWrap(t *testing.T) {
	theme := defaultMarkdownTheme()
	width := int64(20)
	// 代码行（含 2 字符缩进）远超 width，应软换行而非保留超宽行。
	code := "this_is_a_very_long_code_line_that_exceeds_width"
	lines := renderFenceBlock("", []string{code}, width, theme)
	if len(lines) < 2 {
		t.Fatalf("expected soft-wrapped multiple lines, got %d: %v", len(lines), lines)
	}
	for i, ln := range lines {
		if w := visibleWidthStripAnsi(ln); int64(w) > width {
			t.Errorf("line %d width=%d exceeds %d: %q", i, w, width, ln)
		}
	}
	joined := stripAnsiLocal(strings.Join(lines, ""))
	if !strings.Contains(joined, "exceeds_width") {
		t.Errorf("end of code line lost after wrapping: %q", joined)
	}
}

func TestMarkdownList(t *testing.T) {
	md := NewMarkdown("- one\n- two\n  - nested")
	lines := md.Render(20)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "one") || !strings.Contains(joined, "nested") {
		t.Fatalf("missing bullets: %s", joined)
	}
}
