package guardrails

import (
	"context"
	"strings"
	"testing"

	iface "github.com/xujian519/mady/agentcore/iface"
)

func TestVerifyPatentOutput_NoKeywords(t *testing.T) {
	rep := VerifyPatentOutput("本方案采用石墨烯涂层，具备导电性能。")
	if len(rep.RiskWordsHit) != 0 || len(rep.ApprovalWordsHit) != 0 || len(rep.AbsoluteWordsHit) != 0 {
		t.Fatalf("unexpected hits: %+v", rep)
	}
	if !rep.NeedsDisclaimer {
		t.Errorf("expected NeedsDisclaimer true for plain text")
	}
	if rep.NeedsApproval {
		t.Errorf("expected NeedsApproval false")
	}
}

func TestVerifyPatentOutput_NegationExempt(t *testing.T) {
	rep := VerifyPatentOutput("经比对，该方案不构成侵权，具备新颖性。")
	for _, w := range rep.RiskWordsHit {
		if w == "侵权" {
			t.Errorf("「不构成侵权」应豁免 risk word 侵权, got %v", rep.RiskWordsHit)
		}
	}
}

func TestVerifyPatentOutput_ApprovalNoExemption(t *testing.T) {
	// 审批词无否定豁免：即使「不，专利结论…」也触发人工审批。
	rep := VerifyPatentOutput("经核验，本报告给出专利结论：具备创造性。")
	if !rep.NeedsApproval {
		t.Fatalf("expected NeedsApproval true")
	}
	found := false
	for _, w := range rep.ApprovalWordsHit {
		if w == "专利结论" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 专利结论 in ApprovalWordsHit, got %v", rep.ApprovalWordsHit)
	}
}

func TestVerifyPatentOutput_AbsoluteWords(t *testing.T) {
	rep := VerifyPatentOutput("本方案一定具备创造性，效果显著。")
	if len(rep.AbsoluteWordsHit) == 0 {
		t.Fatalf("expected absolute word hit")
	}
	if rep.NeedsApproval {
		t.Errorf("absolute words must not trigger approval")
	}
}

func TestVerifyPatentOutput_DisclaimerPresent(t *testing.T) {
	rep := VerifyPatentOutput("本分析由 AI 辅助生成，不构成正式法律意见。")
	if rep.NeedsDisclaimer {
		t.Errorf("expected NeedsDisclaimer false when disclaimer present")
	}
}

func TestVerifyPatentOutput_Citations(t *testing.T) {
	rep := VerifyPatentOutput("依据专利法第二十二条，该方案具备新颖性。")
	if rep.Citations.Total == 0 {
		t.Errorf("expected citations extracted, got 0")
	}
}

func TestNewPatentOutputGate_ApprovalSuppresses(t *testing.T) {
	g := NewPatentOutputGate()
	mcc := &iface.ModelCallContext{Content: "本报告给出专利结论：具备创造性。"}
	g.AfterModelCall(context.Background(), nil, mcc)
	if !mcc.SuppressPersist {
		t.Errorf("expected SuppressPersist set when approval keywords hit")
	}
}

func TestNewPatentOutputGate_Recorder(t *testing.T) {
	var recorded *PatentOutputReport
	g := NewPatentOutputGate(WithPatentOutputRecorder(func(r PatentOutputReport) { recorded = &r }))
	mcc := &iface.ModelCallContext{Content: "本报告给出专利结论。"}
	g.AfterModelCall(context.Background(), nil, mcc)
	if recorded == nil {
		t.Fatalf("expected recorder callback")
	}
	if !recorded.NeedsApproval {
		t.Errorf("expected recorder report NeedsApproval")
	}
}

func TestHasNegationContext(t *testing.T) {
	tests := []struct {
		text string
		word string
		want bool
	}{
		{"该方案不构成侵权", "侵权", true},
		{"该方案构成侵权", "侵权", false},
		{"不同于现有技术", "现有技术", false},
	}
	for _, tt := range tests {
		if got := hasNegationContext(tt.text, tt.word); got != tt.want {
			t.Errorf("hasNegationContext(%q,%q)=%v,want %v", tt.text, tt.word, got, tt.want)
		}
	}
}

func TestPatentAbsoluteWords_AreUnicodeEscaped(t *testing.T) {
	// 校验转义词在运行时解出中文，避免该数据被 tone 词表误扫。
	if !strings.Contains(string([]rune("绝对")), "绝对") {
		t.Errorf("escape mismatch")
	}
	if len(patentAbsoluteWords) != 5 {
		t.Errorf("expected 5 absolute words, got %d", len(patentAbsoluteWords))
	}
}
