package component

import (
	"strings"
	"testing"
)

func TestJudgmentViewRender(t *testing.T) {
	v := NewJudgmentView()
	v.SetPhase("复核阶段")
	v.SetStatus("awaiting_review")
	v.SetJudgment("某技术点可形成初步结论")
	v.SetConfidence(75)
	v.SetPending([]string{"引用来源 1 条", "对比材料 2 份"})
	v.SetContext([]string{"已归拢证据 6 条", "存在冲突证据 1 条"})
	v.SetActions([]JudgmentAction{
		{Key: "r", Label: "复核"},
		{Key: "e", Label: "证据"},
		{Key: "s", Label: "系统"},
	})

	lines := v.Render(60)
	joined := strings.Join(lines, "\n")

	checks := []struct{ name, want string }{
		{"phase", "复核阶段"},
		{"status label", "等待复核"},
		{"judgment text", "某技术点可形成初步结论"},
		{"confidence bar", "置信度"},
		{"confidence label 高", "高"},
		{"pending section", "仍待确认"},
		{"pending items", "引用来源"},
		{"context section", "当前上下文"},
		{"context item", "已归拢证据"},
		{"action item", "[r]"},
		{"action label", "复核"},
		{"action item e", "[e]"},
		{"action item s", "[s]"},
		{"action label 证据", "证据"},
		{"action label 系统", "系统"},
	}
	for _, c := range checks {
		if !strings.Contains(joined, c.want) {
			t.Errorf("missing %q (%s) in judgment view:\n%s", c.want, c.name, joined)
		}
	}
}

func TestJudgmentViewEmptyStates(t *testing.T) {
	v := NewJudgmentView()
	v.SetPhase("分析阶段")
	v.SetStatus("awaiting_review")
	v.SetJudgment("暂不能下结论")
	v.SetConfidence(-1) // hide confidence
	// pending and context are left empty

	lines := v.Render(60)
	joined := strings.Join(lines, "\n")

	// Must NOT contain confidence/pending/context sections
	for _, unwanted := range []string{"置信度", "仍待确认", "当前上下文"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("unexpected %q in empty-state rendering:\n%s", unwanted, joined)
		}
	}

	// But must still show judgment and phase
	for _, want := range []string{"分析阶段", "暂不能下结论"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in empty-state rendering:\n%s", want, joined)
		}
	}
}

func TestJudgmentViewConfidenceBar(t *testing.T) {
	tests := []struct {
		conf    int
		checkFn func(string) bool
	}{
		{90, func(s string) bool { return strings.Contains(s, "高") && strings.Contains(s, "置信度") }},
		{50, func(s string) bool { return strings.Contains(s, "中") && strings.Contains(s, "置信度") }},
		{20, func(s string) bool { return strings.Contains(s, "低") && strings.Contains(s, "置信度") }},
		{-1, func(s string) bool { return !strings.Contains(s, "置信度") }},
	}
	for _, tt := range tests {
		v := NewJudgmentView()
		v.SetPhase("test")
		v.SetStatus("awaiting_review")
		v.SetJudgment("test judgment")
		v.SetConfidence(tt.conf)
		joined := strings.Join(v.Render(60), "\n")
		if !tt.checkFn(joined) {
			t.Fatalf("confidence bar check failed for conf=%d:\n%s", tt.conf, joined)
		}
	}
}

func TestJudgmentViewPendingMax3(t *testing.T) {
	v := NewJudgmentView()
	v.SetPhase("t")
	v.SetStatus("awaiting_review")
	v.SetJudgment("test")
	v.SetConfidence(50)
	// Set more than 3 pending items
	v.SetPending([]string{"A", "B", "C", "D"})

	joined := strings.Join(v.Render(60), "\n")
	if !strings.Contains(joined, "A") {
		t.Errorf("missing A in pending")
	}
	if !strings.Contains(joined, "B") {
		t.Errorf("missing B in pending")
	}
	if !strings.Contains(joined, "C") {
		t.Errorf("missing C in pending")
	}
	// The 4th item may or may not be shown depending on truncation.
	// At minimum the first 3 must be present.
}

func TestJudgmentViewCollapsedModes(t *testing.T) {
	// idle and streaming should produce collapsed output (no expanded content)
	v := NewJudgmentView()
	v.SetPhase("分析阶段")
	v.SetStatus("streaming")
	v.SetJudgment("some judgment")
	v.SetConfidence(75)
	v.SetPending([]string{"A"})

	lines := v.Render(60)
	joined := strings.Join(lines, "\n")

	// Must show phase and status
	for _, want := range []string{"分析阶段", "输出中"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in streaming view:\n%s", want, joined)
		}
	}
}

func TestJudgmentViewNormalModes(t *testing.T) {
	v := NewJudgmentView()
	v.SetPhase("分析阶段")
	v.SetStatus("done")
	v.SetJudgment("分析已完成")
	v.SetConfidence(80)
	v.SetPending([]string{"A"})
	v.SetContext([]string{"B"})

	lines := v.Render(60)
	joined := strings.Join(lines, "\n")

	// Done mode shows status line + judgment but NOT pending/context/actions
	if !strings.Contains(joined, "分析已完成") {
		t.Errorf("missing judgment in done view")
	}
	if !strings.Contains(joined, "已完成") {
		t.Errorf("missing status label in done view")
	}
}

func TestJudgmentViewDegradedMode(t *testing.T) {
	v := NewJudgmentView()
	v.SetPhase("分析阶段")
	v.SetStatus("awaiting_review")
	v.SetMode("degraded")
	v.SetJudgment("test")
	v.SetConfidence(50)

	joined := strings.Join(v.Render(60), "\n")
	if !strings.Contains(joined, "degraded") {
		t.Errorf("missing degraded mode tag:\n%s", joined)
	}
}

func TestJudgmentViewStatusLabels(t *testing.T) {
	tests := []struct {
		status string
		label  string
	}{
		{"idle", "空闲"},
		{"analyzing", "分析中"},
		{"running", "运行中"},
		{"streaming", "输出中"},
		{"awaiting_review", "等待复核"},
		{"blocked", "已阻塞"},
		{"done", "已完成"},
		{"failed", "失败"},
		{"degraded", "降级运行"},
	}
	for _, tt := range tests {
		v := NewJudgmentView()
		v.SetPhase("t")
		v.SetStatus(tt.status)
		v.SetJudgment("test")
		joined := strings.Join(v.Render(60), "\n")
		if !strings.Contains(joined, tt.label) {
			t.Errorf("status=%q: expected label %q, got:\n%s", tt.status, tt.label, joined)
		}
	}
}

func TestJudgmentViewSetMethods(t *testing.T) {
	v := NewJudgmentView()

	v.SetPhase("测试阶段")
	if got := v.Phase(); got != "测试阶段" {
		t.Errorf("Phase() = %q, want %q", got, "测试阶段")
	}

	v.SetStatus("running")
	if got := v.Status(); got != "running" {
		t.Errorf("Status() = %q, want %q", got, "running")
	}

	// SetPhase/SetStatus should not crash
	v.SetJudgment("judgment")
	v.SetConfidence(50)
	v.SetPending([]string{"x"})
	v.SetContext([]string{"y"})
	v.SetMode("degraded")
	v.SetActions([]JudgmentAction{{Key: "r", Label: "复核"}})

	// Render should not panic
	lines := v.Render(60)
	if len(lines) == 0 {
		t.Fatal("empty render after set methods")
	}
}

func TestJudgmentViewInvalidate(t *testing.T) {
	v := NewJudgmentView()
	v.SetPhase("t")
	v.SetStatus("awaiting_review")
	v.SetJudgment("test")
	v.SetConfidence(50)

	lines1 := v.Render(60)
	v.Invalidate()
	lines2 := v.Render(60)

	if len(lines1) != len(lines2) {
		t.Errorf("height changed after Invalidate: %d vs %d", len(lines1), len(lines2))
	}
}

func TestJudgmentViewIsEmpty(t *testing.T) {
	v := NewJudgmentView()
	if !v.IsEmpty() {
		t.Error("expected empty for fresh JudgmentView")
	}

	v.SetStatus("running")
	if v.IsEmpty() {
		t.Error("expected non-empty after SetStatus")
	}
}

func TestJudgmentViewSetStatusLabel(t *testing.T) {
	v := NewJudgmentView()
	v.SetPhase("test")
	v.SetStatus("running")
	v.SetStatusLabel("自定义标签")
	v.SetJudgment("test")

	joined := strings.Join(v.Render(60), "\n")
	if !strings.Contains(joined, "自定义标签") {
		t.Errorf("missing custom status label:\n%s", joined)
	}
	if strings.Contains(joined, "运行中") {
		t.Errorf("should not contain auto-derived label when override is set:\n%s", joined)
	}

	// Reset to auto-derived
	v.SetStatusLabel("")
	joined = strings.Join(v.Render(60), "\n")
	if !strings.Contains(joined, "运行中") {
		t.Errorf("missing auto-derived label after reset:\n%s", joined)
	}
}
