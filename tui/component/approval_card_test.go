package component

import (
	"strings"
	"testing"
)

// testApprovalTheme returns a simple ApprovalCardTheme for testing.
func testApprovalTheme() ApprovalCardTheme {
	id := func(s string) string { return s }
	return ApprovalCardTheme{
		Title:         id,
		Border:        id,
		Warning:       id,
		Dim:           id,
		Action:        id,
		Body:          id,
		MarkdownTheme: DefaultMarkdownTheme(),
	}
}

func TestRenderApprovalConfBar(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
		width      int64
		wantPct    string
	}{
		{"zero confidence", 0.0, 40, "0%"},
		{"full confidence", 1.0, 40, "100%"},
		{"half confidence", 0.5, 40, "50%"},
		{"quarter confidence", 0.25, 40, "25%"},
		{"three quarters", 0.75, 40, "75%"},
		{"tiny width", 0.9, 10, "90%"},
		{"negative clamped to zero", -0.1, 40, "0%"},
		{"overflow clamped to 100", 1.5, 40, "100%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := testApprovalTheme()
			got := renderApprovalConfBar(tt.confidence, tm, tt.width)
			if !strings.Contains(got, tt.wantPct) {
				t.Errorf("renderApprovalConfBar(%v) = %q, want containing %q",
					tt.confidence, got, tt.wantPct)
			}
			// Verify the bar characters are present
			if !strings.Contains(got, "█") && !strings.Contains(got, "░") {
				t.Errorf("bar should contain block characters: %q", got)
			}
		})
	}
}

func TestRenderApprovalCard_Header(t *testing.T) {
	dm := &DomainMessage{
		Type: DomainMsgTypeApprovalPrompt,
		Body: "专利结论：该发明具有新颖性",
	}
	tm := testApprovalTheme()
	width := int64(60)
	lines := RenderApprovalCard(dm, tm, width)
	joined := strings.Join(lines, "\n")

	// Must contain the approval header
	if !strings.Contains(joined, "人 工 审 核 关 卡") {
		t.Errorf("approval card missing header, got:\n%s", joined)
	}
	// Must contain the body text
	if !strings.Contains(joined, "专利结论：该发明具有新颖性") {
		t.Errorf("approval card missing body, got:\n%s", joined)
	}
}

func TestRenderApprovalCard_Actions(t *testing.T) {
	dm := &DomainMessage{
		Type: DomainMsgTypeApprovalPrompt,
		Body: "需要审核",
		Actions: []DomainAction{
			{Label: "确认并继续", Command: "/approve"},
			{Label: "拒绝并要求修改", Command: "/reject"},
		},
	}
	tm := testApprovalTheme()
	lines := RenderApprovalCard(dm, tm, 60)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "/approve") {
		t.Errorf("approval card missing /approve action, got:\n%s", joined)
	}
	if !strings.Contains(joined, "/reject") {
		t.Errorf("approval card missing /reject action, got:\n%s", joined)
	}
}

func TestRenderApprovalCard_Title(t *testing.T) {
	dm := &DomainMessage{
		Type:  DomainMsgTypeApprovalPrompt,
		Title: "新颖性评估结论",
		Body:  "权利要求1-5具备新颖性",
	}
	tm := testApprovalTheme()
	lines := RenderApprovalCard(dm, tm, 60)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "新颖性评估结论") {
		t.Errorf("approval card missing title, got:\n%s", joined)
	}
}

func TestRenderApprovalCard_EvidenceSummary(t *testing.T) {
	dm := &DomainMessage{
		Type:  DomainMsgTypeApprovalPrompt,
		Title: "侵权分析结论",
		Spans: []EvidenceRef{
			{Snippet: "对比文件D1公开了特征A", Direction: DirectionSupporting},
			{Snippet: "对比文件D2公开了特征B", Direction: DirectionContradicting},
			{Snippet: "对比文件D3公开了特征C", Direction: DirectionSupporting},
		},
	}
	tm := testApprovalTheme()
	lines := RenderApprovalCard(dm, tm, 60)
	joined := strings.Join(lines, "\n")

	// Should show evidence counts
	if !strings.Contains(joined, "支持证据: 2") {
		t.Errorf("approval card missing support count, got:\n%s", joined)
	}
	if !strings.Contains(joined, "反对证据: 1") {
		t.Errorf("approval card missing contradict count, got:\n%s", joined)
	}
	// Should show first evidence snippet
	if !strings.Contains(joined, "对比文件D1公开了特征A") {
		t.Errorf("approval card missing first snippet, got:\n%s", joined)
	}
}

func TestRenderApprovalCard_Dividers(t *testing.T) {
	dm := &DomainMessage{
		Type:  DomainMsgTypeApprovalPrompt,
		Title: "法律意见",
		Body:  "最终法律意见",
		Spans: []EvidenceRef{
			{Snippet: "依据专利法第22条", Direction: DirectionSupporting},
		},
		Actions: []DomainAction{
			{Label: "确认", Command: "/approve"},
		},
	}
	tm := testApprovalTheme()
	width := int64(40)
	lines := RenderApprovalCard(dm, tm, width)

	// Should have divider lines (═ or ─)
	dividerCount := 0
	for _, line := range lines {
		stripped := stripString(line)
		if strings.Contains(stripped, "═") || strings.Contains(stripped, "─") {
			dividerCount++
		}
	}
	if dividerCount < 2 {
		t.Errorf("expected at least 2 divider lines, got %d: %v", dividerCount, lines)
	}
}

// stripString returns a plain string (no-op for non-styled renderers).
func stripString(s string) string {
	_ = s
	return s
}
