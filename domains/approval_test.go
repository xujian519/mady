package domains

import (
	"strings"
	"testing"
)

func TestApprovalGate_KeywordTrigger(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		keywords  []string
		shouldTrigger bool
	}{
		{
			name:      "triggers on patent keyword",
			content:   "根据分析，专利结论为具有新颖性。",
			keywords:  []string{"专利结论", "侵权判断"},
			shouldTrigger: true,
		},
		{
			name:      "triggers on legal keyword",
			content:   "我们的法律意见是建议和解。",
			keywords:  []string{"法律意见", "诉讼策略"},
			shouldTrigger: true,
		},
		{
			name:      "does not trigger without keyword",
			content:   "这是一份专利检索结果摘要。",
			keywords:  []string{"专利结论", "侵权判断"},
			shouldTrigger: false,
		},
		{
			name:      "empty content does not trigger",
			content:   "",
			keywords:  []string{"风险", "建议"},
			shouldTrigger: false,
		},
		{
			name:      "nil keywords uses defaults so triggers",
			content:   "风险评估结论：低风险",
			keywords:  nil,
			shouldTrigger: true, // DefaultApprovalConfig includes "风险评估"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := NewApprovalGate(ApprovalConfig{
				RequireApprovalFor: tt.keywords,
			})
			result := gate.needsApproval(tt.content)
			if result != tt.shouldTrigger {
				t.Errorf("needsApproval(%q) = %v, want %v", tt.content, result, tt.shouldTrigger)
			}
		})
	}
}

func TestApprovalGate_SkipIfNoTools(t *testing.T) {
	t.Run("needsApproval returns true with keyword", func(t *testing.T) {
		gate := NewApprovalGate(ApprovalConfig{
			RequireApprovalFor: []string{"专利结论"},
		})
		if !gate.needsApproval("专利结论：具有新颖性") {
			t.Error("should trigger on keyword")
		}
	})

	t.Run("needsApproval returns false without keyword", func(t *testing.T) {
		gate := NewApprovalGate(ApprovalConfig{
			RequireApprovalFor: []string{"专利结论"},
		})
		if gate.needsApproval("这是一份普通的检索报告") {
			t.Error("should not trigger without keyword")
		}
	})

	t.Run("SkipIfNoTools is preserved in config", func(t *testing.T) {
		gate := NewApprovalGate(ApprovalConfig{
			RequireApprovalFor: []string{"专利结论"},
			SkipIfNoTools:      true,
		})
		if !gate.config.SkipIfNoTools {
			t.Error("SkipIfNoTools should be true")
		}
	})
}

func TestApprovalGate_BuildMessage(t *testing.T) {
	gate := NewApprovalGate(DefaultApprovalConfig())

	msg := gate.buildApprovalMessage("这是一份很长的专利分析报告，详细说明了各方面的技术特征和创新点。经过与现有技术的比对，我们认为该发明具有创造性。")
	if !strings.Contains(msg, "人 工 审 核 关 卡") {
		t.Errorf("message missing approval header")
	}
	if !strings.Contains(msg, "确认") {
		t.Errorf("message missing confirm instruction")
	}
}

func TestApprovalGate_BuildMessageTruncates(t *testing.T) {
	gate := NewApprovalGate(DefaultApprovalConfig())

	// Create content longer than 500 chars.
	longContent := strings.Repeat("专利技术分析内容，", 100)
	msg := gate.buildApprovalMessage(longContent)

	if !strings.Contains(msg, "...") {
		t.Errorf("long message should be truncated with '...'")
	}
	if len(msg) > len(longContent)+500 {
		t.Errorf("message unexpectedly long: %d chars", len(msg))
	}
}

func TestDefaultApprovalConfig(t *testing.T) {
	cfg := DefaultApprovalConfig()

	if len(cfg.RequireApprovalFor) == 0 {
		t.Error("RequireApprovalFor should not be empty")
	}
	if cfg.TimeoutMsg == "" {
		t.Error("TimeoutMsg should not be empty")
	}

	// Verify key approval keywords are present.
	expected := []string{"专利结论", "法律意见", "风险评估"}
	for _, kw := range expected {
		found := false
		for _, ak := range cfg.RequireApprovalFor {
			if ak == kw {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing approval keyword %q", kw)
		}
	}
}

func TestNewApprovalGate_EmptyConfigUsesDefaults(t *testing.T) {
	gate := NewApprovalGate(ApprovalConfig{})
	if len(gate.config.RequireApprovalFor) == 0 {
		t.Error("empty config should use default approval keywords")
	}
	if gate.config.TimeoutMsg == "" {
		t.Error("empty config should use default timeout msg")
	}
}

func TestRequireApproval(t *testing.T) {
	err := RequireApproval("final patent claim draft", map[string]any{
		"claim_number": "1",
	})
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "人工审核") {
		t.Errorf("error message missing '人工审核': %v", err)
	}
}
