package component

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

// ---------------------------------------------------------------------------
// keyhelp.go
// ---------------------------------------------------------------------------

func TestKeyHelpSettersAndClose(t *testing.T) {
	h := NewKeyHelp(nil)
	h.SetTitle("帮助")
	if h.title != "帮助" {
		t.Fatalf("expected title, got %q", h.title)
	}
	h.SetFilter("ctrl")
	if h.filter != "ctrl" {
		t.Fatalf("expected filter, got %q", h.filter)
	}
	h.SetMaxRows(5)
	if h.maxRows != 5 {
		t.Fatalf("expected maxRows 5, got %d", h.maxRows)
	}
	closed := 0
	h.OnClose(func() { closed++ })
	h.Close()
	if closed != 1 {
		t.Fatalf("expected 1 close, got %d", closed)
	}
	h.SetOnClose(func() { closed++ }) // alias — replaces the previous fn
	h.Close()
	if closed != 2 {
		t.Fatalf("expected 2 closes, got %d", closed)
	}
	// Close with no callback — no panic.
	h2 := NewKeyHelp(nil)
	h2.Close()
}

func TestKeyHelpScrollAndRender(t *testing.T) {
	h := NewKeyHelp(terminal.GetGlobalKeybindings())
	rendered := 0
	h.SetOnRequestRender(func() { rendered++ })
	h.ScrollBy(2)
	if h.scroll != 2 {
		t.Fatalf("expected scroll 2, got %d", h.scroll)
	}
	if rendered != 1 {
		t.Fatalf("expected requestRender, got %d", rendered)
	}
	h.ScrollBy(-10) // clamped to 0
	if h.scroll != 0 {
		t.Fatalf("expected scroll 0, got %d", h.scroll)
	}
	if rendered != 2 {
		t.Fatalf("expected second requestRender, got %d", rendered)
	}
	lines := h.Render(60)
	if len(lines) == 0 {
		t.Fatal("expected render")
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 60 {
			t.Fatalf("line width %d > 60 (line=%q)", w, ln)
		}
	}
	h.Invalidate() // no-op
	h.Update(core.KeyMsg{Data: "\x1b[B"})
}

// ---------------------------------------------------------------------------
// loader.go — covers Loader paths not exercised by loader_test.go:
// SetTheme, Dispose, Update, plus CancellableLoader OnAbort / Focusable.
// ---------------------------------------------------------------------------

func TestLoaderThemeDisposeUpdate(t *testing.T) {
	l := NewLoader(func() {}, "处理中…")
	l.SetTheme(LoaderTheme{})
	l.Start()
	l.Stop()
	l.Dispose() // Stop again — no panic
	if l.IsRunning() {
		t.Fatal("expected stopped after Dispose")
	}
	l.Invalidate() // no-op
	if cmd := l.Update(core.TickMsg{}); cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
}

func TestCancellableLoaderCallbacks(t *testing.T) {
	cl := NewCancellableLoader(func() {}, "取消?")
	aborted := false
	cl.OnAbort(func() { aborted = true })
	cl.SetFocused(true) // no-op
	cl.Start()
	if !cl.IsFocused() {
		t.Fatal("expected IsFocused while running")
	}
	// Escape aborts: marks aborted, cancels the context, fires the callback.
	cl.Update(core.KeyMsg{Data: "\x1b"})
	if !aborted || !cl.Aborted() {
		t.Fatal("expected abort callback on Escape")
	}
	if cl.Context().Err() == nil {
		t.Fatal("expected context canceled after abort")
	}
	// Second Escape is a no-op (already aborted).
	cl.Update(core.KeyMsg{Data: "\x1b"})
	cl.Stop() // abort cancels the context; Stop halts the animation goroutine
	// WindowSizeMsg invalidates.
	cl.Update(core.WindowSizeMsg{Width: 100, Height: 30})
	// Stop without start — no panic.
	cl2 := NewCancellableLoader(func() {}, "x")
	cl2.Stop()
}

// ---------------------------------------------------------------------------
// judgment_view.go
// ---------------------------------------------------------------------------

func TestJudgmentViewModeAndExpansion(t *testing.T) {
	v := NewJudgmentView()
	v.SetStatus("awaiting_review")
	if !v.IsExpanded() {
		t.Fatal("expected expanded for awaiting_review")
	}
	v.SetStatus("blocked")
	if !v.IsExpanded() {
		t.Fatal("expected expanded for blocked")
	}
	v.SetStatus("idle")
	if v.IsExpanded() {
		t.Fatal("expected collapsed for idle")
	}
	if v.Mode() != "normal" && v.Mode() != "" {
		t.Fatalf("unexpected mode %q", v.Mode())
	}
	v.SetMode("degraded")
	if v.Mode() != "degraded" {
		t.Fatalf("expected degraded mode, got %q", v.Mode())
	}
	if h := v.Height(60); h != int64(len(v.Render(60))) {
		t.Fatalf("unexpected Height %d", h)
	}
}

func TestJudgmentViewSetters(t *testing.T) {
	v := NewJudgmentView()
	v.SetPhase("分析")
	v.SetStatus("analyzing")
	v.SetJudgment("具有创造性")
	v.SetConfidence(85)
	v.SetPending([]string{"item1", "item2"})
	v.SetContext([]string{"ctx1"})
	if v.phase != "分析" {
		t.Fatalf("unexpected phase %q", v.phase)
	}
	v.SetPhase("分析") // same value — dirty unchanged, no panic
	lines := v.Render(60)
	if len(lines) == 0 {
		t.Fatal("expected render")
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 60 {
			t.Fatalf("line width %d > 60 (line=%q)", w, ln)
		}
	}
	v.Invalidate()
	if lines := v.Render(60); len(lines) == 0 {
		t.Fatal("expected render after Invalidate")
	}
}

// ---------------------------------------------------------------------------
// markdown.go
// ---------------------------------------------------------------------------

func TestMarkdownSetSourceAndInvalidate(t *testing.T) {
	m := NewMarkdown("# 标题\n正文")
	m.SetSource("## 新标题")
	lines := m.Render(40)
	if len(lines) == 0 {
		t.Fatal("expected render")
	}
	m.Invalidate()
	if again := m.Render(40); len(again) == 0 {
		t.Fatal("expected render after Invalidate")
	}
	if cmd := m.Update(core.KeyMsg{Data: "x"}); cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
	// WindowSizeMsg invalidates.
	m.Update(core.WindowSizeMsg{Width: 100, Height: 30})
}

// ---------------------------------------------------------------------------
// footer.go
// ---------------------------------------------------------------------------

func TestFooterInvalidateNoOp(t *testing.T) {
	f := NewFooter()
	f.Invalidate() // no-op
}

// TestEvidenceOverlayKeybindingsNoOp covers the documented no-op setter.
func TestEvidenceOverlayKeybindingsNoOp(t *testing.T) {
	e := NewEvidenceOverlay()
	e.SetKeybindings(terminal.GetGlobalKeybindings()) // must not panic
}

// TestComponentInvalidateNoOps covers remaining no-op Invalidate
// implementations.
func TestComponentInvalidateNoOps(t *testing.T) {
	cc := NewCommandCenter(nil)
	cc.Invalidate()
	e := NewEditor(nil)
	e.Invalidate()
	sl := NewSelectList(nil)
	sl.Invalidate()
	tt := NewTruncatedText("x")
	tt.Invalidate()
	sp := NewSpacer(1)
	sp.Invalidate()
	v := NewViewport(3)
	v.Invalidate()
	toast := NewToast(0)
	toast.Invalidate()
	s := NewSkillCenter()
	s.Invalidate()
	tp := NewTodoPanel()
	tp.Invalidate()
	sb := NewStatusBar()
	sb.Invalidate()
	ss := NewSessionSelector()
	ss.Invalidate()
	sx := NewSyntax("func main() {}", "go")
	sx.Invalidate()
	if cmd := sx.Update(core.KeyMsg{Data: "x"}); cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
	sx.SetSource("package main")
	sx.SetTheme(DefaultSyntaxTheme())
}
