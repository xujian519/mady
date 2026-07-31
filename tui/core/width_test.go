package core

import (
	"strings"
	"testing"
)

func TestVisibleWidth(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"hello", 5},
		{"中文", 4},
		{"a中b", 4},
		{"\x1b[31mhello\x1b[0m", 5},
		{"emoji: 🌍", 9}, // 7 ASCII + space=1 is already counted, emoji=2
		{"\x1b]8;;http://a\x07link\x1b]8;;\x07", 4},
	}
	for _, c := range cases {
		got := VisibleWidth(c.in)
		if got != c.want {
			t.Errorf("VisibleWidth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestTruncateToWidth(t *testing.T) {
	if got := TruncateToWidth("hello world", 8, "…"); VisibleWidth(got) > 8 {
		t.Errorf("truncation exceeded width: %q (width=%d)", got, VisibleWidth(got))
	}
	if got := TruncateToWidth("中文测试字符串", 6, "…"); VisibleWidth(got) > 6 {
		t.Errorf("cjk truncation exceeded width: %q (width=%d)", got, VisibleWidth(got))
	}
	if got := TruncateToWidth("short", 100, "…"); got != "short" {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestPadToWidth(t *testing.T) {
	got := PadToWidth("hi", 5)
	if VisibleWidth(got) != 5 {
		t.Errorf("PadToWidth visible width = %d, want 5 (got %q)", VisibleWidth(got), got)
	}
}

func TestWrapAnsi(t *testing.T) {
	lines := WrapAnsi("the quick brown fox jumps over the lazy dog", 10)
	for i, l := range lines {
		if VisibleWidth(l) > 10 {
			t.Errorf("line %d exceeds width: %q (%d cells)", i, l, VisibleWidth(l))
		}
	}
	if len(lines) < 2 {
		t.Errorf("expected multiple wrapped lines, got %d", len(lines))
	}
}

// TestWrapAnsiCJKNoDrop verifies that hard-wrapping CJK text never drops a
// wide (2-cell) rune at a width boundary. The pre-fix findBreakColumn
// returned the raw width even when it landed mid-glyph, and SliceByColumn
// then skipped that boundary rune entirely — every wrapped Chinese line lost
// its last character, i.e. "输出总是被截断".
func TestWrapAnsiCJKNoDrop(t *testing.T) {
	text := "中文换行测试文本内容较长用于验证宽度边界处是否丢字"
	width := int64(10)
	lines := WrapAnsi(text, width)
	var joined string
	for _, ln := range lines {
		joined += StripAnsi(ln)
	}
	if joined != text {
		t.Errorf("WrapAnsi dropped runes:\n got %q\nwant %q", joined, text)
	}
	for i, ln := range lines {
		if w := VisibleWidth(ln); w > width {
			t.Errorf("line %d width %d > %d: %q", i, w, width, ln)
		}
	}
}

// TestWrapAnsiCJKASCIINoDrop covers the common mixed ASCII+CJK reply text.
// Whitespace at wrap boundaries is intentionally trimmed (standard
// line-breaking), so the assertion compares the non-space character
// sequence: every glyph must survive, spaces may be dropped at breaks.
func TestWrapAnsiCJKASCIINoDrop(t *testing.T) {
	text := "根据专利法第26条第3款的规定，权利要求书应当以说明书为依据，claim 1 需要进一步限定。"
	width := int64(20)
	lines := WrapAnsi(text, width)
	joined := strings.Join(lines, "")
	noSpace := func(s string) string { return strings.ReplaceAll(StripAnsi(s), " ", "") }
	if noSpace(joined) != noSpace(text) {
		t.Errorf("WrapAnsi dropped runes:\n got %q\nwant %q", joined, text)
	}
	for i, ln := range lines {
		if w := VisibleWidth(ln); w > width {
			t.Errorf("line %d width %d > %d: %q", i, w, width, ln)
		}
	}
}

func TestSliceByColumn(t *testing.T) {
	if got := SliceByColumn("hello world", 6, 11); got != "world" {
		t.Errorf("SliceByColumn(6,11) = %q, want %q", got, "world")
	}
	if got := SliceByColumn("中文abc", 2, 5); got != "文a" {
		// "中"=2, "文"=2, "a"=1 -> columns [0,2)=中 [2,4)=文 [4,5)=a
		t.Errorf("SliceByColumn cjk = %q, want %q", got, "文a")
	}
}

// TestWrapAnsiSoftBreakOnPunctuation verifies that long tokens without
// whitespace are broken at punctuation boundaries (/, -, _, .) rather than
// arbitrary character boundaries. This keeps URLs and file paths readable.
func TestWrapAnsiSoftBreakOnPunctuation(t *testing.T) {
	text := "比如/skill:xxx命令严格说我没法自动激活它"
	width := int64(20)
	lines := WrapAnsi(text, width)
	for i, ln := range lines {
		if w := VisibleWidth(ln); w > width {
			t.Errorf("line %d width %d > %d: %q", i, w, width, ln)
		}
	}
	joined := strings.Join(lines, "")
	if StripAnsi(joined) != text {
		t.Errorf("WrapAnsi dropped runes:\n got %q\nwant %q", joined, text)
	}
	// The break must land at the '/' soft-break boundary, not inside "skill".
	first := StripAnsi(lines[0])
	if !strings.HasSuffix(first, "/") {
		t.Errorf("expected first wrapped line to end at '/', got %q", first)
	}
	if strings.Contains(first, "skill") {
		t.Errorf("expected first wrapped line to stop before 'skill', got %q", first)
	}
}
