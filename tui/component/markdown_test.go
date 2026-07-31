package component

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
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
	// Check for code content. Glamour's chroma highlights individual tokens
	// with ANSI codes, so "fmt.Println" is split into "fmt" + "." + "Println".
	if !strings.Contains(joined, "fmt") || !strings.Contains(joined, "Println") {
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

// TestMarkdownParagraphDoesNotSwallowTable verifies that a table immediately
// following a paragraph is recognized as a table block rather than being merged
// into the preceding paragraph.
func TestMarkdownParagraphDoesNotSwallowTable(t *testing.T) {
	src := "intro paragraph\n| a | b |\n|---|---|\n| 1 | 2 |"
	md := NewMarkdown(src)
	lines := md.Render(40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "intro paragraph") {
		t.Errorf("paragraph text missing: %s", joined)
	}
	if !strings.Contains(joined, "+---+") {
		t.Errorf("table borders missing; paragraph swallowed the table:\n%s", joined)
	}
}

// TestMarkdownHeadingLenient parses LLM-friendly headings that place an emoji
// before the hash sequence or omit the space between hashes and text.
func TestMarkdownHeadingLenient(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"🏆### 一、Top 07", "🏆 一、Top 07"},
		{"🏆###一 Top07)", "🏆 一 Top07)"},
		{"⭐## Section", "⭐ Section"},
		// Ornamental Dingbats (1F650-1F67F) and Geometric Shapes Extended
		// (1F780-1F7FF) prefix decorations.
		{"❞## 装饰标题", "❞ 装饰标题"},
		{"🞄### 几何标题", "🞄 几何标题"},
	}
	for _, tc := range cases {
		md := NewMarkdown(tc.src)
		lines := md.Render(80)
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, tc.want) {
			t.Errorf("heading %q missing want %q; got:\n%s", tc.src, tc.want, joined)
		}
	}
}

// TestMarkdownTableVerticalFallback verifies that a table too wide for the
// viewport is rendered as key/value pairs instead of squeezing columns and
// truncating cell content with ellipsis.
func TestMarkdownTableVerticalFallback(t *testing.T) {
	src := "| 项目 | 配置情况 | 实际可用性 |\n|------|----------|------------|\n| MCP (filer) | ~/.mady/mcp.json里配了服务（python3 - filer.m_server） | ❌不可用 |"
	md := NewMarkdown(src)
	lines := md.Render(60)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "…") {
		t.Errorf("table should not truncate cells with ellipsis:\n%s", joined)
	}
	if !strings.Contains(joined, "配置情况:") {
		t.Errorf("expected vertical key/value layout with header labels, got:\n%s", joined)
	}
	if !strings.Contains(joined, "~/.mady/mcp.json里配了服务") {
		t.Errorf("cell content lost in fallback:\n%s", joined)
	}
}

// TestMarkdownTableVerticalFallbackAggressive verifies that tables switch to the
// vertical key/value layout early, before the horizontal ASCII borders get
// squeezed against the viewport edge.
func TestMarkdownTableVerticalFallbackAggressive(t *testing.T) {
	src := "| 维度 | 要点 |\n| --- | --- |\n| 新颖性 | 区别于现有技术 |"
	md := NewMarkdown(src)
	lines := md.Render(20)
	joined := core.StripAnsi(strings.Join(lines, "\n"))
	if strings.Contains(joined, "+---+") {
		t.Errorf("narrow table should use vertical fallback, got horizontal borders:\n%s", joined)
	}
	if !strings.Contains(joined, "维度:") || !strings.Contains(joined, "新颖性") {
		t.Errorf("vertical layout missing content:\n%s", joined)
	}
}

// TestMarkdownParagraphArtifactCleanup verifies that stray ATX hashes and
// unpaired emphasis markers inside paragraph lines are stripped rather than
// left raw on the terminal.
func TestMarkdownParagraphArtifactCleanup(t *testing.T) {
	src := "相关规定源自机#### 📖法> **如变压器\"\"放大器术语）。"
	md := NewMarkdown(src)
	lines := md.Render(80)
	joined := core.StripAnsi(strings.Join(lines, "\n"))
	if strings.Contains(joined, "####") {
		t.Errorf("inline ATX hashes should be stripped, got:\n%s", joined)
	}
	if strings.Contains(joined, "**") {
		t.Errorf("unpaired bold markers should be stripped, got:\n%s", joined)
	}
	if !strings.Contains(joined, "相关规定源自机") || !strings.Contains(joined, "放大器术语") {
		t.Errorf("paragraph content should be preserved, got:\n%s", joined)
	}
}

// TestMarkdownParagraphKeepsSingleHash verifies that legitimate single hashes
// (C#/F#/patent drawing references like "1#") survive paragraph cleanup — only
// hash runs of 2+ are treated as stray artifacts.
func TestMarkdownParagraphKeepsSingleHash(t *testing.T) {
	src := "C# 与 F# 语言，以及 1# 上盖（图纸引用）"
	md := NewMarkdown(src)
	lines := md.Render(80)
	joined := core.StripAnsi(strings.Join(lines, "\n"))
	for _, want := range []string{"C#", "F#", "1#"} {
		if !strings.Contains(joined, want) {
			t.Errorf("single hash %q should be preserved, got:\n%s", want, joined)
		}
	}
}
