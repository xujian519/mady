package component

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

func testEvidenceItems() []EvidenceItem {
	return []EvidenceItem{
		{Title: "专利法第22条第3款", Source: "审查指南第二部分第四章", Score: 0.87, Excerpt: "创造性的判断…"},
		{Title: "专利法第26条第3款", Source: "", Score: -1, Excerpt: ""},
		{Title: "专利法第9条", Source: "审查指南", Score: 0.55, Excerpt: "同样的发明创造只能授予一项专利权"},
	}
}

func TestEvidenceOverlayBasics(t *testing.T) {
	e := NewEvidenceOverlay()
	if e.title != "引用证据详情" {
		t.Fatalf("expected default title, got %q", e.title)
	}
	e.SetTitle("自定义标题")
	if e.title != "自定义标题" {
		t.Fatalf("expected title set, got %q", e.title)
	}
	e.SetTitle("自定义标题") // same title — dirty unchanged
	e.SetItems(testEvidenceItems())
	if len(e.items) != 3 || e.scroll != 0 || e.cursor != 0 {
		t.Fatalf("unexpected state after SetItems")
	}
	e.SetKeybindings(terminal.GetGlobalKeybindings()) // no-op
	e.Invalidate()
}

func TestEvidenceOverlayRenderEmpty(t *testing.T) {
	e := NewEvidenceOverlay()
	lines := e.Render(50)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "暂无引用证据") {
		t.Fatalf("expected empty message, got %q", joined)
	}
	// Cache hit returns identical content.
	again := e.Render(50)
	if len(again) != len(lines) {
		t.Fatalf("expected cache hit, got %d vs %d lines", len(again), len(lines))
	}
}

func TestEvidenceOverlayRenderItems(t *testing.T) {
	e := NewEvidenceOverlay()
	e.SetItems(testEvidenceItems())
	lines := e.Render(60)
	if len(lines) == 0 {
		t.Fatal("expected render")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"专利法第22条第3款", "相关度: 0.87", "来源", "审查指南", "▶", "1/3", "↑↓ 浏览", "Esc 关闭"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in render, got %q", want, joined)
		}
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 60 {
			t.Fatalf("line width %d > 60 (line=%q)", w, ln)
		}
	}
}

func TestEvidenceOverlayRenderWideExcerptTruncated(t *testing.T) {
	e := NewEvidenceOverlay()
	long := strings.Repeat("这是一个很长的摘录内容", 20)
	e.SetItems([]EvidenceItem{{Title: "t", Excerpt: long}})
	lines := e.Render(30)
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 30 {
			t.Fatalf("line width %d > 30 (line=%q)", w, ln)
		}
	}
}

func TestEvidenceOverlayCloseOnEscape(t *testing.T) {
	e := NewEvidenceOverlay()
	closed := false
	e.SetOnClose(func() { closed = true })
	e.Update(core.KeyMsg{Data: "\x1b"})
	if !closed {
		t.Fatal("expected onClose on Escape")
	}
	// No callback — no panic.
	e2 := NewEvidenceOverlay()
	e2.Update(core.KeyMsg{Data: "\x1b"})
}

func TestEvidenceOverlayCursorMovement(t *testing.T) {
	e := NewEvidenceOverlay()
	e.SetItems(testEvidenceItems())
	e.Update(core.KeyMsg{Data: "\x1b[B"}) // down
	if e.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", e.cursor)
	}
	e.Update(core.KeyMsg{Data: "\x1b[B"})
	e.Update(core.KeyMsg{Data: "\x1b[B"}) // clamped at last
	if e.cursor != 2 {
		t.Fatalf("expected cursor clamped to 2, got %d", e.cursor)
	}
	e.Update(core.KeyMsg{Data: "j"}) // j also moves down (clamped)
	if e.cursor != 2 {
		t.Fatalf("expected cursor 2 after j, got %d", e.cursor)
	}
	e.Update(core.KeyMsg{Data: "\x1b[A"}) // up
	if e.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", e.cursor)
	}
	e.Update(core.KeyMsg{Data: "k"})
	e.Update(core.KeyMsg{Data: "k"})
	if e.cursor != 0 {
		t.Fatalf("expected cursor clamped to 0, got %d", e.cursor)
	}
	// Empty items — no-op.
	e2 := NewEvidenceOverlay()
	e2.Update(core.KeyMsg{Data: "\x1b[A"})
	if e2.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", e2.cursor)
	}
}

func TestEvidenceOverlayPageScroll(t *testing.T) {
	e := NewEvidenceOverlay()
	e.SetItems(testEvidenceItems())
	e.Update(core.KeyMsg{Data: "\x1b[6~"}) // pagedown
	if e.cursor != 2 {
		t.Fatalf("expected cursor 2 after pagedown, got %d", e.cursor)
	}
	e.Update(core.KeyMsg{Data: "\x1b[5~"}) // pageup
	if e.cursor != 0 {
		t.Fatalf("expected cursor 0 after pageup, got %d", e.cursor)
	}
	e2 := NewEvidenceOverlay()
	e2.Update(core.KeyMsg{Data: "\x1b[6~"}) // empty — no-op
	if e2.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", e2.cursor)
	}
}

func TestEvidenceOverlayClampScroll(t *testing.T) {
	e := NewEvidenceOverlay()
	e.SetItems(testEvidenceItems())
	e.cursor = 2
	e.scroll = 5
	e.clampScroll(16) // scroll > cursor -> pulled back
	if e.scroll > e.cursor {
		t.Fatalf("expected scroll <= cursor, got scroll=%d cursor=%d", e.scroll, e.cursor)
	}
	e.scroll = 0
	e.cursor = 2
	e.clampScroll(1) // cursor beyond scroll+itemsPerPage -> window moves
	if e.scroll != 2 {
		t.Fatalf("expected scroll 2, got %d", e.scroll)
	}
	// maxScroll clamp.
	e.cursor = 2
	e.scroll = 99
	e.clampScroll(1)
	if e.scroll != 2 {
		t.Fatalf("expected scroll clamped to 2, got %d", e.scroll)
	}
	// Empty items.
	e2 := NewEvidenceOverlay()
	e2.clampScroll(5)
	if e2.cursor != -1 {
		t.Fatalf("expected cursor -1 for empty, got %d", e2.cursor)
	}
}

func TestTruncateToWidth(t *testing.T) {
	// ASCII fits.
	if got := truncateToWidth("abc", 10); got != "abc" {
		t.Fatalf("expected 'abc', got %q", got)
	}
	// CJK counts double; truncation happens only when the NEXT rune would
	// exceed the limit (source behavior — the returned line may be one rune
	// over the stated max).
	if got := truncateToWidth("中文内容", 4); got != "中文…" {
		t.Fatalf("expected '中文…', got %q", got)
	}
	if got := truncateToWidth("中文内容", 8); got != "中文内容" {
		t.Fatalf("expected full string, got %q", got)
	}
	// Mixed.
	if got := truncateToWidth("ab中文", 4); got != "ab中…" {
		t.Fatalf("expected 'ab中…', got %q", got)
	}
	// CJK punctuation range (0x3000-0x303f).
	if got := truncateToWidth("、", 1); got != "…" {
		t.Fatalf("expected '…', got %q", got)
	}
	// Empty.
	if got := truncateToWidth("", 10); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
