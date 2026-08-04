package core

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

// TestRuneWidthAmbiguousInCJKMode verifies that characters whose East Asian
// Width property is Ambiguous are counted as 2 cells when CJK mode is active.
// These are the most common cause of "text runs into the scrollbar" bugs.
func TestRuneWidthAmbiguousInCJKMode(t *testing.T) {
	// Save and restore the global mode so we don't leak state to other tests.
	old := IsCJKMode()
	defer SetCJKMode(old)

	ambiguous := "—…·“”‘’" // em dash, ellipsis, middle dot, curly quotes
	SetCJKMode(true)
	for _, r := range ambiguous {
		if got := RuneWidth(r); got != 2 {
			t.Errorf("CJK mode: RuneWidth(%q) = %d, want 2", r, got)
		}
	}

	SetCJKMode(false)
	for _, r := range ambiguous {
		if got := RuneWidth(r); got != 1 {
			t.Errorf("non-CJK mode: RuneWidth(%q) = %d, want 1", r, got)
		}
	}
}

// TestVisibleWidthCJKAmbiguous verifies the full width pipeline with ambiguous
// characters under CJK mode.
func TestVisibleWidthCJKAmbiguous(t *testing.T) {
	old := IsCJKMode()
	defer SetCJKMode(old)

	SetCJKMode(true)
	// "中—中" in a CJK terminal: 2 + 2 + 2 = 6.
	if got := VisibleWidth("中—中"); got != 6 {
		t.Errorf("VisibleWidth(\"中—中\") in CJK mode = %d, want 6", got)
	}
	SetCJKMode(false)
	if got := VisibleWidth("中—中"); got != 5 {
		t.Errorf("VisibleWidth(\"中—中\") in non-CJK mode = %d, want 5", got)
	}
}

// TestWrapAnsiCJKBreaksAtPunctuation verifies that CJK-aware wrapping prefers
// breaking after full-width punctuation instead of splitting a sentence at an
// arbitrary character boundary.
func TestWrapAnsiCJKBreaksAtPunctuation(t *testing.T) {
	old := IsCJKMode()
	defer SetCJKMode(old)
	SetCJKMode(true)

	text := "环境准备—稳定运行：本地云服务器AWS阿里Kubernetes集群安装基础依赖"
	width := int64(20)
	lines := WrapAnsiCJK(text, width)
	for i, ln := range lines {
		if w := VisibleWidth(ln); w > width {
			t.Errorf("line %d width %d > %d: %q", i, w, width, ln)
		}
	}
	// The first line should end at a punctuation boundary, not split a word.
	first := StripAnsi(lines[0])
	if !strings.HasSuffix(first, "—") && !strings.HasSuffix(first, "：") {
		t.Errorf("expected first line to end at CJK punctuation, got %q", first)
	}
}

// TestWrapAnsiCJKNoLeadingPunctuation verifies that a CJK punctuation is not
// left alone at the start of a wrapped line when the previous line has room.
func TestWrapAnsiCJKNoLeadingPunctuation(t *testing.T) {
	old := IsCJKMode()
	defer SetCJKMode(old)
	SetCJKMode(true)

	// "中文，中文..." forced narrow so the comma is near the boundary.
	text := "中文，中文中文中文中文中文中文中文中文中文中文"
	width := int64(10)
	lines := WrapAnsiCJK(text, width)
	for i := 1; i < len(lines); i++ {
		if strings.Contains(lines[i], "\x1b") {
			continue
		}
		r, _ := utf8.DecodeRuneInString(lines[i])
		if isCJKPunctuation(r) {
			t.Errorf("line %d starts with punctuation %q: %q", i, r, lines[i])
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

// TestWrapAnsiWidthOneCJKNoHang is the regression test for the wrapOneLine
// infinite loop: when width == 1 and the line starts with a 2-cell CJK rune,
// findBreakColumn previously returned `width` (1), SliceByColumn(0, 1) cut
// the wide rune and returned an empty left slice, so `cur` never advanced and
// the loop spun forever. This test fails fast via a watchdog if it hangs, and
// asserts no rune is dropped.
func TestWrapAnsiWidthOneCJKNoHang(t *testing.T) {
	text := "中文"
	done := make(chan []string, 1)
	go func() {
		done <- WrapAnsi(text, 1)
	}()
	select {
	case lines := <-done:
		var joined string
		for _, ln := range lines {
			joined += StripAnsi(ln)
		}
		if joined != text {
			t.Errorf("WrapAnsi(%q, 1) dropped runes: got %q", text, joined)
		}
		if len(lines) == 0 {
			t.Errorf("WrapAnsi(%q, 1) returned no lines", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("WrapAnsi(%q, 1) hung (infinite loop)", text)
	}
}

// TestWrapAnsiWidthOneMixed covers a wide rune in the middle of the line plus
// a standalone over-wide emoji at width 1 — both must terminate and preserve
// every glyph.
func TestWrapAnsiWidthOneMixed(t *testing.T) {
	for _, text := range []string{"a中b", "✅ok", "中文", "中"} {
		done := make(chan []string, 1)
		go func(src string) { done <- WrapAnsi(src, 1) }(text)
		select {
		case lines := <-done:
			var joined string
			for _, ln := range lines {
				joined += StripAnsi(ln)
			}
			if joined != text {
				t.Errorf("WrapAnsi(%q, 1) dropped runes: got %q", text, joined)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("WrapAnsi(%q, 1) hung (infinite loop)", text)
		}
	}
}
