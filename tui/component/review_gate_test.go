package component

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestReviewGateRender(t *testing.T) {
	g := newTestGate()
	lines := g.Render(60)
	joined := strings.Join(lines, "\n")

	for _, want := range []string{
		"复核门",
		"当前判断",
		"初步结论已形成",
		"置信度",
		"主要依据",
		"证据A",
		"复核清单",
		"已列明主要依据",
		"风险提示",
		"通过复核",
		"返回补证据",
		"标记阻塞",
		"Esc",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in review gate output:\n%s", want, joined)
		}
	}
}

func TestReviewGateConfidenceBar(t *testing.T) {
	tests := []struct {
		conf    float64
		checkFn func(string) bool
	}{
		{0.9, func(s string) bool { return strings.Contains(s, "90%") && strings.Contains(s, "高") }},
		{0.5, func(s string) bool { return strings.Contains(s, "50%") && strings.Contains(s, "中") }},
		{0.2, func(s string) bool { return strings.Contains(s, "20%") && strings.Contains(s, "低") }},
		{-1, func(s string) bool { return !strings.Contains(s, "置信度") }},
	}
	for _, tt := range tests {
		g := NewReviewGate("test", tt.conf, nil, nil, nil)
		joined := strings.Join(g.Render(60), "\n")
		if !tt.checkFn(joined) {
			t.Fatalf("confidence bar check failed for conf=%.1f:\n%s", tt.conf, joined)
		}
	}
}

func TestReviewGateNavigateDownThroughSections(t *testing.T) {
	g := newTestGate()
	g.SetFocused(true)

	// Start: focusEvidences, evidenceIdx=0
	g.mu.RLock()
	if g.focusSec != focusEvidences || g.evidenceIdx != 0 {
		t.Fatalf("initial: focusSec=%d evidenceIdx=%d", g.focusSec, g.evidenceIdx)
	}
	g.mu.RUnlock()

	// Down 1 → evidenceIdx=1 (second evidence)
	g.Update(core.KeyMsg{Data: "j"})
	g.mu.RLock()
	if g.focusSec != focusEvidences || g.evidenceIdx != 1 {
		t.Fatalf("after down1: focusSec=%d evidenceIdx=%d", g.focusSec, g.evidenceIdx)
	}
	g.mu.RUnlock()

	// Down 2 → focusChecklist, checklistIdx=0
	g.Update(core.KeyMsg{Data: "j"})
	g.mu.RLock()
	if g.focusSec != focusChecklist || g.checklistIdx != 0 {
		t.Fatalf("after down2: focusSec=%d checklistIdx=%d", g.focusSec, g.checklistIdx)
	}
	g.mu.RUnlock()

	// Down 3 → checklistIdx=1
	g.Update(core.KeyMsg{Data: "j"})
	g.mu.RLock()
	if g.focusSec != focusChecklist || g.checklistIdx != 1 {
		t.Fatalf("after down3: focusSec=%d checklistIdx=%d", g.focusSec, g.checklistIdx)
	}
	g.mu.RUnlock()

	// Keep going to end of checklist, should wrap to actions
	nChecklist := len(g.checklist)
	for i := 0; i < (nChecklist - 1); i++ {
		g.Update(core.KeyMsg{Data: "j"})
	}
	g.mu.RLock()
	if g.focusSec != focusActions {
		t.Fatalf("after reaching end of checklist: focusSec=%d, want focusActions", g.focusSec)
	}
	g.mu.RUnlock()

	// One more down wraps back to first evidence
	g.Update(core.KeyMsg{Data: "j"})
	g.mu.RLock()
	if g.focusSec != focusEvidences || g.evidenceIdx != 0 {
		t.Fatalf("after wrap: focusSec=%d evidenceIdx=%d", g.focusSec, g.evidenceIdx)
	}
	g.mu.RUnlock()
}

func TestReviewGateNavigateUpThroughSections(t *testing.T) {
	g := newTestGate()
	g.SetFocused(true)

	// Navigate up from start wraps to actions
	g.Update(core.KeyMsg{Data: "k"})
	g.mu.RLock()
	if g.focusSec != focusActions {
		t.Fatalf("after up from start: focusSec=%d, want focusActions", g.focusSec)
	}
	g.mu.RUnlock()

	// Up from actions goes to checklist last item
	g.Update(core.KeyMsg{Data: "k"})
	g.mu.RLock()
	if g.focusSec != focusChecklist || g.checklistIdx != len(g.checklist)-1 {
		t.Fatalf("after up from actions: focusSec=%d checklistIdx=%d", g.focusSec, g.checklistIdx)
	}
	g.mu.RUnlock()
}

func TestReviewGateChecklistToggle(t *testing.T) {
	g := newTestGate()
	g.SetFocused(true)

	// Set focus to checklist item 1 directly
	g.mu.Lock()
	g.focusSec = focusChecklist
	g.checklistIdx = 1
	g.mu.Unlock()

	// Item 1 starts as checked (true). Toggle → unchecked.
	g.Update(core.KeyMsg{Data: " "})

	g.mu.RLock()
	checked := g.checklist[1].Checked
	g.mu.RUnlock()
	if checked {
		t.Fatal("expected checklist item 1 to be unchecked after Space (was initially true)")
	}

	// Toggle again → checked again
	g.Update(core.KeyMsg{Data: " "})

	g.mu.RLock()
	checked = g.checklist[1].Checked
	g.mu.RUnlock()
	if !checked {
		t.Fatal("expected checklist item 1 to be checked after second Space")
	}

	// Item 2 starts as unchecked (false). Navigate then toggle.
	g.mu.Lock()
	g.checklistIdx = 2
	g.mu.Unlock()

	g.Update(core.KeyMsg{Data: " "})

	g.mu.RLock()
	checked = g.checklist[2].Checked
	g.mu.RUnlock()
	if !checked {
		t.Fatal("expected checklist item 2 (initially unchecked) to be checked after Space")
	}
}

func TestReviewGateActions(t *testing.T) {
	passCalled := false
	backCalled := false
	blockCalled := false
	closeCalled := false

	g := NewReviewGate("test", 0.8, nil, nil, nil)
	g.SetFocused(true)
	g.SetOnPass(func() { passCalled = true })
	g.SetOnBack(func() { backCalled = true })
	g.SetOnBlock(func() { blockCalled = true })
	g.SetOnClose(func() { closeCalled = true })

	// p → pass
	g.Update(core.KeyMsg{Data: "p"})
	if !passCalled {
		t.Fatal("expected onPass to be called on 'p'")
	}

	// b → back
	g.Update(core.KeyMsg{Data: "b"})
	if !backCalled {
		t.Fatal("expected onBack to be called on 'b'")
	}

	// f → block
	g.Update(core.KeyMsg{Data: "f"})
	if !blockCalled {
		t.Fatal("expected onBlock to be called on 'f'")
	}

	// Esc → close
	g.Update(core.KeyMsg{Data: "\x1b"})
	if !closeCalled {
		t.Fatal("expected onClose to be called on Escape")
	}
}

func TestReviewGateEvidenceExpand(t *testing.T) {
	evidences := []ReviewEvidence{
		{ID: "ev1", Title: "证据A", Summary: "这是证据A的摘要", Role: "核心证据", Status: EvidenceConfirmed},
	}
	checklist := []ReviewCheckItem{{Label: "测试项", Checked: false}}
	g := NewReviewGate("test", 0.8, evidences, checklist, []string{"风险项"})

	// Navigate to evidence and expand
	g.SetFocused(true)
	g.mu.Lock()
	g.focusSec = focusEvidences
	g.evidenceIdx = 0
	g.mu.Unlock()

	g.Update(core.KeyMsg{Data: " "}) // expand

	expanded := g.expanded["ev1"]
	if !expanded {
		t.Fatal("expected evidence ev1 to be expanded after Space")
	}

	joined := strings.Join(g.Render(60), "\n")
	if !strings.Contains(joined, "这是证据A的摘要") {
		t.Fatalf("expected expanded summary in render:\n%s", joined)
	}

	// Collapse
	g.Update(core.KeyMsg{Data: " "})
	if g.expanded["ev1"] {
		t.Fatal("expected evidence ev1 to be collapsed after second Space")
	}
}

func TestReviewGateEmptyRisks(t *testing.T) {
	g := NewReviewGate("test", 0.8, nil, nil, nil) // no risks
	joined := strings.Join(g.Render(60), "\n")
	if strings.Contains(joined, "风险提示") {
		t.Fatal("expected no risk section when risks are empty")
	}
}

func TestReviewGateEmptyEvidences(t *testing.T) {
	g := NewReviewGate("test", 0.8, nil, []ReviewCheckItem{{Label: "检查项", Checked: false}}, nil)
	joined := strings.Join(g.Render(60), "\n")
	if strings.Contains(joined, "主要依据") {
		t.Fatal("expected no evidence section when evidences are empty")
	}
	if !strings.Contains(joined, "复核清单") {
		t.Fatal("expected checklist section to be present")
	}
}

func TestReviewGateFocusIndicator(t *testing.T) {
	g := newTestGate()
	g.SetFocused(true)
	g.mu.Lock()
	g.focusSec = focusEvidences
	g.evidenceIdx = 0
	g.mu.Unlock()

	lines := g.Render(60)
	// The first evidence should have the focus marker (▸)
	found := false
	for _, line := range lines {
		if strings.Contains(line, "▸") && strings.Contains(line, "证据A") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected focus indicator ▸ on first evidence item:\n" + strings.Join(lines, "\n"))
	}
}

func TestReviewGateUpdateNoCrash(t *testing.T) {
	g := NewReviewGate("test", 0.8, nil, nil, nil)
	g.SetFocused(false)

	// Update should not crash regardless of focus state
	g.Update(core.KeyMsg{Data: "p"})
	g.Update(core.KeyMsg{Data: "b"})
	g.Update(core.KeyMsg{Data: "f"})
	g.Update(core.KeyMsg{Data: "\x1b"})
	g.Update(core.KeyMsg{Data: " "})
	g.Update(core.KeyMsg{Data: "\x1b[B"})
}

func TestReviewGateSetTitle(t *testing.T) {
	g := NewReviewGate("test", 0.8, nil, nil, nil)
	g.SetTitle("自定义复核")
	joined := strings.Join(g.Render(60), "\n")
	if !strings.Contains(joined, "自定义复核") {
		t.Fatal("expected custom title in render")
	}
}

// newTestGate creates a ReviewGate with representative test data.
func newTestGate() *ReviewGate {
	evidences := []ReviewEvidence{
		{ID: "ev1", Title: "证据A", Role: "核心证据", Summary: "证据A的核心摘要", Status: EvidenceConfirmed},
		{ID: "ev2", Title: "证据B", Role: "辅助证据", Summary: "证据B的补充信息", Status: EvidencePending},
	}
	checklist := []ReviewCheckItem{
		{Label: "已列明主要依据", Checked: true},
		{Label: "已标注不确定性", Checked: true},
		{Label: "冲突证据已处理", Checked: false},
	}
	risks := []string{"当前有 1 条引用待人工核验", "当前结论适合内部研判，不宜直接外发"}
	return NewReviewGate("初步结论已形成", 0.75, evidences, checklist, risks)
}
