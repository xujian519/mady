package domains

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestApprovalGate_KeywordTrigger(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		keywords      []string
		shouldTrigger bool
	}{
		{
			name:          "triggers on patent keyword",
			content:       "根据分析，专利结论为具有新颖性。",
			keywords:      []string{"专利结论", "侵权判断"},
			shouldTrigger: true,
		},
		{
			name:          "triggers on legal keyword",
			content:       "我们的法律意见是建议和解。",
			keywords:      []string{"法律意见", "诉讼策略"},
			shouldTrigger: true,
		},
		{
			name:          "does not trigger without keyword",
			content:       "这是一份专利检索结果摘要。",
			keywords:      []string{"专利结论", "侵权判断"},
			shouldTrigger: false,
		},
		{
			name:          "empty content does not trigger",
			content:       "",
			keywords:      []string{"风险", "建议"},
			shouldTrigger: false,
		},
		{
			name:          "nil keywords uses defaults so triggers",
			content:       "风险评估结论：低风险",
			keywords:      nil,
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

// ---------------------------------------------------------------------------
// B1 — Structured Approval Record tests
// ---------------------------------------------------------------------------

func TestMemoryApprovalStore_SaveAndList(t *testing.T) {
	store := NewMemoryApprovalStore()
	ctx := context.Background()

	r1 := ApprovalRecord{
		ID:             "r1",
		SessionID:      "sess-1",
		CaseID:         "case-1",
		Timestamp:      time.Now(),
		TriggerKeyword: "专利结论",
		OriginalOutput: "具有新颖性",
		Decision:       DecisionAdopted,
	}
	r2 := ApprovalRecord{
		ID:             "r2",
		SessionID:      "sess-1",
		CaseID:         "case-2",
		Timestamp:      time.Now(),
		TriggerKeyword: "法律意见",
		OriginalOutput: "建议和解",
		Decision:       DecisionModified,
		ModifiedOutput: "建议和解并支付许可费",
		Feedback:       "补充许可费条款",
	}
	r3 := ApprovalRecord{
		ID:             "r3",
		SessionID:      "sess-2",
		CaseID:         "case-1",
		Timestamp:      time.Now(),
		TriggerKeyword: "风险评估",
		OriginalOutput: "低风险",
		Decision:       DecisionRejected,
		Feedback:       "风险被低估",
	}

	for _, r := range []ApprovalRecord{r1, r2, r3} {
		if err := store.Save(ctx, r); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	got, err := store.List(ctx, "sess-1")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records for sess-1, got %d", len(got))
	}

	gotCase, err := store.ListByCase(ctx, "case-1")
	if err != nil {
		t.Fatalf("ListByCase failed: %v", err)
	}
	if len(gotCase) != 2 {
		t.Fatalf("expected 2 records for case-1, got %d", len(gotCase))
	}
}

func TestMemoryApprovalStore_Empty(t *testing.T) {
	store := NewMemoryApprovalStore()
	ctx := context.Background()

	got, err := store.List(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 records, got %d", len(got))
	}
}

func TestApprovalGate_RecordDecision(t *testing.T) {
	gate := NewApprovalGate(DefaultApprovalConfig())
	store := NewMemoryApprovalStore()
	WithApprovalStore(store)(gate)

	ctx := context.Background()
	original := "专利结论：该发明具备新颖性"

	err := gate.RecordDecision(ctx, "sess-1", "case-1", "专利结论", original,
		DecisionModified, "专利结论：该发明具备新颖性和创造性", "补充创造性分析")
	if err != nil {
		t.Fatalf("RecordDecision failed: %v", err)
	}

	records, err := store.List(ctx, "sess-1")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.Decision != DecisionModified {
		t.Errorf("expected modified, got %s", r.Decision)
	}
	if r.OriginalOutput != original {
		t.Errorf("original output mismatch")
	}
	if r.ModifiedOutput == "" {
		t.Error("modified output should not be empty")
	}
	if r.TriggerKeyword != "专利结论" {
		t.Errorf("trigger keyword mismatch: %s", r.TriggerKeyword)
	}
}

func TestApprovalGate_RecordDecision_NoStore(t *testing.T) {
	gate := NewApprovalGate(DefaultApprovalConfig())
	// No store attached — should be a silent no-op.
	err := gate.RecordDecision(context.Background(), "s", "c", "kw", "out",
		DecisionAdopted, "", "")
	if err != nil {
		t.Errorf("expected nil error without store, got %v", err)
	}
}

func TestApprovalGate_WithApprovalStore(t *testing.T) {
	gate := NewApprovalGate(DefaultApprovalConfig())
	if gate.store != nil {
		t.Error("store should be nil by default")
	}

	store := NewMemoryApprovalStore()
	WithApprovalStore(store)(gate)
	if gate.store == nil {
		t.Error("store should be set after WithApprovalStore")
	}
}
