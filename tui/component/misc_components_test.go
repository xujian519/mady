package component

import (
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// domain.go
// ---------------------------------------------------------------------------

func TestDomainMessageConfidence(t *testing.T) {
	msg := &DomainMessage{Confidence: 0.8}
	if msg.ConfidencePct() != 80 {
		t.Fatalf("expected 80, got %d", msg.ConfidencePct())
	}
	if msg.ConfidenceLevel() != "high" {
		t.Fatalf("expected high, got %q", msg.ConfidenceLevel())
	}
	msg.Confidence = 0.5
	if msg.ConfidenceLevel() != "medium" {
		t.Fatalf("expected medium, got %q", msg.ConfidenceLevel())
	}
	msg.Confidence = 0.1
	if msg.ConfidenceLevel() != "low" {
		t.Fatalf("expected low, got %q", msg.ConfidenceLevel())
	}
}

func TestDomainMessageSpanCounts(t *testing.T) {
	msg := &DomainMessage{Spans: []EvidenceRef{
		{Direction: DirectionSupporting},
		{Direction: DirectionSupporting},
		{Direction: DirectionContradicting},
		{Direction: DirectionNeutral},
	}}
	if msg.SupportingSpans() != 2 {
		t.Fatalf("expected 2 supporting, got %d", msg.SupportingSpans())
	}
	if msg.ContradictingSpans() != 1 {
		t.Fatalf("expected 1 contradicting, got %d", msg.ContradictingSpans())
	}
	empty := &DomainMessage{}
	if empty.SupportingSpans() != 0 || empty.ContradictingSpans() != 0 {
		t.Fatal("expected 0 counts for empty message")
	}
}

// ---------------------------------------------------------------------------
// fuzzy_provider.go
// ---------------------------------------------------------------------------

func TestNormalizeForMatch(t *testing.T) {
	// Passthrough to core.NormalizeForMatch — assert it does not panic and
	// preserves simple input.
	if got := NormalizeForMatch("hello world"); got != "hello world" {
		t.Fatalf("unexpected normalize result %q", got)
	}
}

func TestSubstringFuzzyMatch(t *testing.T) {
	start, end, ok := SubstringFuzzyMatch("The quick brown fox", "quick")
	if !ok || start < 0 || end <= start {
		t.Fatalf("expected match, got %v %v %v", start, end, ok)
	}
	if _, _, ok := SubstringFuzzyMatch("abc", "zzz"); ok {
		t.Fatal("expected no match")
	}
}

func TestSubstringFuzzyFilter(t *testing.T) {
	got := SubstringFuzzyFilter("qui", []string{"quick", "slow", "quilt"})
	if len(got) != 2 || got[0] != "quick" || got[1] != "quilt" {
		t.Fatalf("unexpected filter result %v", got)
	}
	// Empty query returns all candidates.
	got = SubstringFuzzyFilter("", []string{"a", "b"})
	if len(got) != 2 {
		t.Fatalf("expected all candidates for empty query, got %v", got)
	}
}

func TestFuzzyContentProviderTrigger(t *testing.T) {
	p := &FuzzyContentProvider{}
	if got := p.Trigger(); got != "#" {
		t.Fatalf("expected default #, got %q", got)
	}
	p.TriggerStr = ">"
	if got := p.Trigger(); got != ">" {
		t.Fatalf("expected >, got %q", got)
	}
}

func TestFuzzyContentProviderComplete(t *testing.T) {
	p := &FuzzyContentProvider{
		Entries: []ContentEntry{
			{Key: "doc1", Body: "patent claim drafting guide\nlong text", Tag: "tag1"},
			{Key: "doc2", Body: "legal analysis notes", Tag: "tag2"},
		},
	}
	// Empty token returns all with previews.
	got := p.Complete("")
	if len(got) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(got))
	}
	if got[0].Label != "doc1" || got[0].Tag != "tag1" {
		t.Fatalf("unexpected suggestion %+v", got[0])
	}
	if got[0].Description == "" {
		t.Fatal("expected description preview")
	}

	// Matching token returns excerpt with ellipses.
	got = p.Complete("claim")
	if len(got) != 1 || got[0].Label != "doc1" {
		t.Fatalf("expected doc1 match, got %v", got)
	}
	if !strings.Contains(got[0].Description, "claim") {
		t.Fatalf("expected excerpt containing token, got %q", got[0].Description)
	}

	// No match.
	if got := p.Complete("zzzz"); len(got) != 0 {
		t.Fatalf("expected no matches, got %v", got)
	}
}

func TestFuzzyContentProviderPreviewLen(t *testing.T) {
	p := &FuzzyContentProvider{Entries: []ContentEntry{{Key: "k", Body: "short"}}}
	if got := p.previewLen(); got != 48 {
		t.Fatalf("expected default preview 48, got %d", got)
	}
	p.MaxPreview = 10
	if got := p.previewLen(); got != 10 {
		t.Fatalf("expected preview 10, got %d", got)
	}
	got := p.Complete("")
	if got[0].Description != "short" {
		t.Fatalf("expected short description, got %q", got[0].Description)
	}
}

func TestPreviewTextTruncation(t *testing.T) {
	got := previewText(strings.Repeat("x", 100), 10)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
	// Newlines/tabs collapse to spaces.
	got = previewText("a\nb\tc", 100)
	if strings.ContainsAny(got, "\n\t") {
		t.Fatalf("expected whitespace collapsed, got %q", got)
	}
}

func TestMakeExcerpt(t *testing.T) {
	body := strings.Repeat("y", 100)
	// Match near the start: leading ellipsis only.
	got := makeExcerpt(body, 0, 5, 20)
	if !strings.HasSuffix(got, "…") || strings.HasPrefix(got, "…") {
		t.Fatalf("unexpected excerpt bounds %q", got)
	}
	// Match in the middle: both sides ellipsized.
	got = makeExcerpt(body, 40, 45, 20)
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("expected both ellipses, got %q", got)
	}
	// Long match caps the excerpt window: start=10, maxLen=20 -> end=30,
	// then widened to lo=0, hi=40, with a trailing ellipsis.
	got = makeExcerpt(body, 10, 90, 20)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected trailing ellipsis, got %q", got)
	}
	if len(got) != 43 { // 40 bytes + 3-byte ellipsis
		t.Fatalf("expected capped excerpt length 43, got %d (%q)", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// onboarding.go
// ---------------------------------------------------------------------------

func TestFirstRunWizardLifecycle(t *testing.T) {
	w := NewFirstRunWizard()
	if !w.IsVisible() {
		t.Fatal("expected wizard visible initially")
	}
	lines := w.Render(50)
	if len(lines) == 0 {
		t.Fatal("expected non-empty render")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"欢迎使用 Mady", "搜索对话历史", "按 Esc 关闭此引导"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in render, got %q", want, joined)
		}
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 50 {
			t.Fatalf("line width %d > 50 (line=%q)", w, ln)
		}
	}
}

func TestFirstRunWizardDismiss(t *testing.T) {
	w := NewFirstRunWizard()
	dismissed := false
	rendered := false
	w.SetOnDismiss(func() { dismissed = true })
	w.SetOnRequestRender(func() { rendered = true })
	w.Dismiss()
	if w.IsVisible() {
		t.Fatal("expected wizard hidden after Dismiss")
	}
	if !dismissed {
		t.Fatal("expected onDismiss callback")
	}
	if !rendered {
		t.Fatal("expected onRequestRender callback")
	}
	if lines := w.Render(50); lines != nil {
		t.Fatalf("expected nil render after dismiss, got %v", lines)
	}
}

func TestFirstRunWizardEscapeKey(t *testing.T) {
	w := NewFirstRunWizard()
	w.Update(core.KeyMsg{Data: "\x1b"})
	if w.IsVisible() {
		t.Fatal("expected wizard dismissed by Escape")
	}
	w.Invalidate() // no-op
	// Non-key message is a no-op.
	w2 := NewFirstRunWizard()
	w2.Update(core.WindowSizeMsg{Width: 100, Height: 30})
	if !w2.IsVisible() {
		t.Fatal("expected wizard still visible after WindowSizeMsg")
	}
}

func TestFirstRunWizardNarrowWidth(t *testing.T) {
	w := NewFirstRunWizard()
	lines := w.Render(10) // clamped to 30
	if len(lines) == 0 {
		t.Fatal("expected render")
	}
	for _, ln := range lines {
		if ln == "" {
			continue // spacer line
		}
		if w := core.VisibleWidth(ln); w < 30 {
			t.Fatalf("expected width clamped to 30, got %d (line=%q)", w, ln)
		}
	}
}

// ---------------------------------------------------------------------------
// approval_card.go / confidence_bar.go
// ---------------------------------------------------------------------------

func TestDefaultApprovalCardTheme(t *testing.T) {
	th := DefaultApprovalCardTheme()
	if th.Title == nil || th.Border == nil || th.Warning == nil || th.Dim == nil ||
		th.Action == nil || th.Body == nil {
		t.Fatal("expected all style fns non-nil")
	}
	if th.MarkdownTheme.HeadingFn[0] == nil || th.MarkdownTheme.EmphasisFn == nil {
		t.Fatal("expected markdown theme populated")
	}
}

func TestRenderApprovalCard(t *testing.T) {
	th := DefaultApprovalCardTheme()
	msg := &DomainMessage{
		Title:      "审 批 关 卡",
		Confidence: 0.8,
		Spans: []EvidenceRef{
			{Snippet: "对比文件 D1 公开了…", Direction: DirectionSupporting},
			{Snippet: "本申请具有创造性", Direction: DirectionContradicting},
		},
		Actions: []DomainAction{
			{Label: "批准", Command: "/approve"},
			{Label: "拒绝", Command: "/reject"},
		},
		Body: "正文 markdown 内容",
	}
	lines := RenderApprovalCard(msg, th, 60)
	if len(lines) == 0 {
		t.Fatal("expected non-empty render")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"人 工 审 核 关 卡", "审 批 关 卡", "支持证据: 1", "反对证据: 1", "/approve", "/reject", "正文 markdown 内容"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in render, got %q", want, joined)
		}
	}
}

func TestRenderApprovalCardMinimal(t *testing.T) {
	th := DefaultApprovalCardTheme()
	msg := &DomainMessage{Confidence: 0.3}
	lines := RenderApprovalCard(msg, th, 40)
	if len(lines) == 0 {
		t.Fatal("expected non-empty render for minimal message")
	}
}

func TestRenderConfidenceBar(t *testing.T) {
	got := RenderConfidenceBar(0.8, nil, 30, false)
	if !strings.Contains(got, "80%") {
		t.Fatalf("expected 80%% in bar, got %q", got)
	}
	// Colored with level label.
	colors := &ConfidenceBarColors{
		High:   func(s string) string { return s },
		Medium: func(s string) string { return s },
		Low:    func(s string) string { return s },
	}
	got = RenderConfidenceBar(0.8, colors, 30, true)
	if !strings.Contains(got, "(高)") {
		t.Fatalf("expected 高 label, got %q", got)
	}
	got = RenderConfidenceBar(0.5, colors, 30, true)
	if !strings.Contains(got, "(中)") {
		t.Fatalf("expected 中 label, got %q", got)
	}
	got = RenderConfidenceBar(0.2, colors, 30, true)
	if !strings.Contains(got, "(低)") {
		t.Fatalf("expected 低 label, got %q", got)
	}
	// Clamped out-of-range.
	got = RenderConfidenceBar(-0.5, nil, 10, false)
	if !strings.Contains(got, "0%") {
		t.Fatalf("expected clamped 0%%, got %q", got)
	}
	got = RenderConfidenceBar(1.5, nil, 10, false)
	if !strings.Contains(got, "100%") {
		t.Fatalf("expected clamped 100%% (percent), got %q", got)
	}
	// Partial colorization (nil High fn).
	got = RenderConfidenceBar(0.9, &ConfidenceBarColors{Medium: func(s string) string { return s }}, 30, false)
	if !strings.Contains(got, "90%") {
		t.Fatalf("unexpected bar %q", got)
	}
	if core.VisibleWidth(got) > 30 {
		t.Fatalf("bar wider than 30: %q", got)
	}
}

// ---------------------------------------------------------------------------
// statusbar.go
// ---------------------------------------------------------------------------

func TestStatusBarSettersAndRender(t *testing.T) {
	s := NewStatusBar()
	s.SetMode("normal")
	s.SetAgent("mady-agent")
	s.SetSections([]StatusBarSection{{Text: "已保存", Fn: nil}})
	s.SetUsage(100, 50, 1200)
	s.SetContext(4500, 10000)
	s.SetCaseInfo("专利案件 A")
	s.SetPendingReview(3)
	s.SetPersisted(true)
	s.SetWidth(120)
	s.Busy() // tok/s rate only renders while running

	lines := s.Render(120)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if w := core.VisibleWidth(lines[0]); w != 120 {
		t.Fatalf("line width %d != 120", w)
	}
	joined := lines[0]
	for _, want := range []string{"normal", "已保存", "专利案件 A", "3", "1.2k", "45%"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in status bar, got %q", want, joined)
		}
	}
	s.Idle()

	// Idle renders the agent indicator instead of the spinner.
	s.SetUsage(0, 0, 0)
	lines = s.Render(120)
	if !strings.Contains(lines[0], "mady-agent") {
		t.Fatalf("expected agent name when idle, got %q", lines[0])
	}
}

func TestStatusBarBusyIdle(t *testing.T) {
	s := NewStatusBar()
	s.Busy()
	if !s.running {
		t.Fatal("expected running after Busy")
	}
	time.Sleep(1100 * time.Millisecond)
	s.Idle()
	if s.running {
		t.Fatal("expected stopped after Idle")
	}
	if s.elapsed < time.Second {
		t.Fatalf("expected elapsed >= 1s, got %v", s.elapsed)
	}
	// Idle when not running keeps state.
	s.Idle()
	if s.elapsed < time.Second {
		t.Fatalf("elapsed regressed: %v", s.elapsed)
	}
}

func TestStatusBarRenderRunning(t *testing.T) {
	s := NewStatusBar()
	s.Busy()
	lines := s.Render(80)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	s.Idle()
}

func TestStatusBarNarrowWidth(t *testing.T) {
	s := NewStatusBar()
	s.SetCaseInfo("some case name that is long")
	s.SetPendingReview(7)
	lines := s.Render(40)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if w := core.VisibleWidth(lines[0]); w > 40 {
		t.Fatalf("line width %d > 40", w)
	}
	// Context bar hidden below width 100.
	s.SetContext(5000, 10000)
	lines = s.Render(80)
	if w := core.VisibleWidth(lines[0]); w > 80 {
		t.Fatalf("line width %d > 80", w)
	}
}

func TestStatusBarUpdateAndInvalidate(t *testing.T) {
	s := NewStatusBar()
	s.Invalidate() // no-op
	s.Update(core.WindowSizeMsg{Width: 100, Height: 30})
	s.Update(core.KeyMsg{Data: "x"}) // non-window msg
}

func TestFormatDuration(t *testing.T) {
	if got := formatDuration(65 * time.Second); got != "1m5s" {
		t.Fatalf("expected 1m5s, got %q", got)
	}
	if got := formatDuration(30 * time.Second); got != "30s" {
		t.Fatalf("expected 30s, got %q", got)
	}
}

func TestFormatTokenRate(t *testing.T) {
	if got := formatTokenRate(500); got != "500 tok/s" {
		t.Fatalf("expected '500 tok/s', got %q", got)
	}
	if got := formatTokenRate(1234); got != "1.2k tok/s" {
		t.Fatalf("expected '1.2k tok/s', got %q", got)
	}
}

func themeForContextBar() *theme.Palette { return theme.CurrentPalette() }

func TestRenderContextBar(t *testing.T) {
	pal := themeForContextBar()
	// total <= 0 -> empty.
	if got := renderContextBar(10, 0, pal); got != "" {
		t.Fatalf("expected empty for total 0, got %q", got)
	}
	// Normal occupancy.
	got := renderContextBar(4500, 10000, pal)
	if !strings.Contains(got, "45%") {
		t.Fatalf("expected 45%% in bar, got %q", got)
	}
	// Clamped values.
	got = renderContextBar(-10, 100, pal)
	if !strings.Contains(got, "0%") {
		t.Fatalf("expected 0%%, got %q", got)
	}
	got = renderContextBar(9999, 100, pal)
	if !strings.Contains(got, "100%") {
		t.Fatalf("expected 100%%, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// system_status.go
// ---------------------------------------------------------------------------

func TestSystemStatusSetKeybindings(t *testing.T) {
	s := NewSystemStatus()
	s.SetKeybindings(terminal.GetGlobalKeybindings())
	s.Invalidate()
	if lines := s.Render(60); len(lines) == 0 {
		t.Fatal("expected render with empty state")
	}
}
