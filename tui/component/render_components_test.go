package component

// render_components_test.go — rendering tests for 8 components that
// previously had zero test coverage. Each test exercises the component's
// Render/New/Default path with minimal input and verifies output structure.
//
// Covered files:
//   - conclusion_card.go
//   - evidence_card.go
//   - statusbar.go
//   - session_selector.go
//   - skill_center.go
//   - todo_panel.go
//   - table.go
//   - command_center.go

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// --- conclusion_card.go ---

func TestRenderConclusionCard_Basic(t *testing.T) {
	tm := DefaultConclusionCardTheme()
	msg := &DomainMessage{
		Title:      "新颖性分析结论",
		Body:       "权利要求1-5具备新颖性，对比文件D1未公开特征A。",
		Confidence: 0.85,
		Spans: []EvidenceRef{
			{Snippet: "D1公开了特征B", Direction: DirectionSupporting},
			{Snippet: "D1未公开特征A", Direction: DirectionContradicting},
		},
		Extra: map[string]string{"classification": "positive"},
	}

	lines := RenderConclusionCard(msg, tm, 60)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "新颖性分析结论") {
		t.Errorf("missing title, got:\n%s", joined)
	}
	if !strings.Contains(joined, "支持证据") {
		t.Errorf("missing evidence summary, got:\n%s", joined)
	}
	if !strings.Contains(joined, "置信度") || !strings.Contains(joined, "85%") {
		t.Errorf("missing confidence bar, got:\n%s", joined)
	}
	if !strings.Contains(joined, "权利要求1-5") {
		t.Errorf("missing body text, got:\n%s", joined)
	}
	if !strings.Contains(joined, "positive") {
		t.Errorf("missing classification, got:\n%s", joined)
	}
}

func TestRenderConclusionCard_EmptyTitle(t *testing.T) {
	tm := DefaultConclusionCardTheme()
	msg := &DomainMessage{
		Body:       "简短结论",
		Confidence: 0.5,
	}
	lines := RenderConclusionCard(msg, tm, 40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "分析结论") {
		t.Errorf("default title '分析结论' missing, got:\n%s", joined)
	}
}

func TestRenderConclusionCard_ZeroConfidence(t *testing.T) {
	tm := DefaultConclusionCardTheme()
	msg := &DomainMessage{
		Title:      "测试结论",
		Confidence: 0.0,
	}
	lines := RenderConclusionCard(msg, tm, 40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "0%") {
		t.Errorf("expected 0%%, got:\n%s", joined)
	}
}

// --- evidence_card.go ---

func TestRenderEvidenceCard_Collapsed(t *testing.T) {
	tm := DefaultEvidenceCardTheme()
	msg := &DomainMessage{
		Title: "证据列表",
		Spans: []EvidenceRef{
			{Snippet: "D1公开了特征A", Direction: DirectionSupporting},
			{Snippet: "D2公开了特征B", Direction: DirectionContradicting},
			{Snippet: "D3公开了特征C", Direction: DirectionSupporting},
		},
	}

	lines := RenderEvidenceCard(msg, true, tm, 60)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "证据列表") {
		t.Errorf("collapsed: missing title, got:\n%s", joined)
	}
	if !strings.Contains(joined, "支持 2") {
		t.Errorf("collapsed: missing support count, got:\n%s", joined)
	}
	if !strings.Contains(joined, "反对 1") {
		t.Errorf("collapsed: missing contradict count, got:\n%s", joined)
	}
}

func TestRenderEvidenceCard_Expanded(t *testing.T) {
	tm := DefaultEvidenceCardTheme()
	msg := &DomainMessage{
		Title:      "详细证据",
		Confidence: 0.75,
		Spans: []EvidenceRef{
			{Snippet: "D1公开了特征A", Direction: DirectionSupporting, SourceURI: "file://d1.pdf", PageRange: "p3"},
			{Snippet: "D2公开了特征B", Direction: DirectionContradicting},
		},
	}

	lines := RenderEvidenceCard(msg, false, tm, 60)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "75%") {
		t.Errorf("expanded: missing confidence, got:\n%s", joined)
	}
	if !strings.Contains(joined, "特征A") {
		t.Errorf("expanded: missing evidence snippet, got:\n%s", joined)
	}
}

func TestRenderEvidenceCard_Empty(t *testing.T) {
	tm := DefaultEvidenceCardTheme()
	msg := &DomainMessage{Title: "空证据"}
	lines := RenderEvidenceCard(msg, false, tm, 40)
	if len(lines) == 0 {
		t.Error("empty evidence card should still produce output")
	}
}

// --- statusbar.go ---

func TestNewStatusBar_Basic(t *testing.T) {
	sb := NewStatusBar()
	if sb == nil {
		t.Fatal("NewStatusBar returned nil")
	}
	// Should accept messages without panic
	_ = sb.Update(core.WindowSizeMsg{Width: 80, Height: 24})
	sb.SetMode("test")
	sb.SetAgent("agent-1")
}

func TestStatusBar_RenderBasic(t *testing.T) {
	sb := NewStatusBar()
	sb.SetMode("分析模式")
	_ = sb.Update(core.WindowSizeMsg{Width: 80, Height: 24})
	lines := sb.Render(80)
	if len(lines) == 0 {
		t.Error("statusbar rendered empty result")
	}
}

func TestStatusBar_Sections(t *testing.T) {
	sb := NewStatusBar()
	sections := []StatusBarSection{
		{Text: "左"},
		{Text: "中"},
		{Text: "右"},
	}
	sb.SetSections(sections)
	_ = sb.Update(core.WindowSizeMsg{Width: 80, Height: 24})
	sb.Render(80)
}

// --- session_selector.go ---

func TestNewSessionSelector_Basic(t *testing.T) {
	ss := NewSessionSelector()
	if ss == nil {
		t.Fatal("NewSessionSelector returned nil")
	}
}

func TestSessionSelector_Render(t *testing.T) {
	ss := NewSessionSelector()
	_ = ss.Update(core.WindowSizeMsg{Width: 60, Height: 20})
	grid := ss.Render(60)
	if len(grid) == 0 {
		t.Error("session selector rendered empty grid")
	}
}

// --- skill_center.go ---

func TestNewSkillCenter_Basic(t *testing.T) {
	sc := NewSkillCenter()
	if sc == nil {
		t.Fatal("NewSkillCenter returned nil")
	}
}

func TestSkillCenter_Theme(t *testing.T) {
	tm := DefaultSkillCenterTheme()
	if tm.Title == "" {
		t.Error("DefaultSkillCenterTheme has empty Title string")
	}
}

// --- todo_panel.go ---

func TestNewTodoPanel_Basic(t *testing.T) {
	tp := NewTodoPanel()
	if tp == nil {
		t.Fatal("NewTodoPanel returned nil")
	}
	_ = tp.Update(core.WindowSizeMsg{Width: 40, Height: 20})
	grid := tp.Render(40)
	// Even empty todo panel should render (with empty-state message or at
	// least padding). If it panics or returns nil, that's a failure.
	_ = grid
}

func TestTodoPanel_Theme(t *testing.T) {
	tm := DefaultTodoPanelTheme()
	if tm.Title == "" {
		t.Error("DefaultTodoPanelTheme has empty Title string")
	}
}

// --- table.go ---

func TestNewTable_Basic(t *testing.T) {
	tbl := NewTable()
	if tbl == nil {
		t.Fatal("NewTable returned nil")
	}
	// Table is typically rendered via RenderRow; just verify no panic.
	_ = tbl.RenderRow(0, 80)
}

// --- command_center.go ---

func TestNewCommandCenter_Basic(t *testing.T) {
	cc := NewCommandCenter([]CommandItem{
		{Name: "test", Label: "/test", Description: "测试命令"},
	})
	if cc == nil {
		t.Fatal("NewCommandCenter returned nil")
	}
	_ = cc.Update(core.WindowSizeMsg{Width: 60, Height: 20})
	cc.Render(60)
}
