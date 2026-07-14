package domains

import (
	"testing"
)

func TestNewCase(t *testing.T) {
	rec := ProjectRecord{
		ProjectID:    "proj_001",
		Alias:        "智能伸缩支架案",
		CaseType:     "发明专利",
		FilingNumber: "CN202410123456.7",
		Jurisdiction: "CN",
	}
	c := NewCase(rec)
	if c.Stage != CaseStageDrafting {
		t.Errorf("initial stage=%q, want %q", c.Stage, CaseStageDrafting)
	}
	if c.Record.ProjectID != "proj_001" {
		t.Errorf("project ID=%q, want proj_001", c.Record.ProjectID)
	}
}

func TestCase_TransitionTo(t *testing.T) {
	rec := ProjectRecord{ProjectID: "proj_001", Alias: "测试案"}
	c := NewCase(rec)

	// Drafting → Reviewing (allowed)
	if err := c.TransitionTo(CaseStageReviewing); err != nil {
		t.Fatalf("Drafting→Reviewing: %v", err)
	}
	if c.Stage != CaseStageReviewing {
		t.Errorf("stage=%q, want reviewing", c.Stage)
	}

	// Reviewing → Granted (allowed)
	if err := c.TransitionTo(CaseStageGranted); err != nil {
		t.Fatalf("Reviewing→Granted: %v", err)
	}

	// Granted → Abandoned (allowed)
	if err := c.TransitionTo(CaseStageAbandoned); err != nil {
		t.Fatalf("Granted→Abandoned: %v", err)
	}
}

func TestCase_TransitionTo_Invalid(t *testing.T) {
	rec := ProjectRecord{ProjectID: "proj_001", Alias: "测试案"}
	c := NewCase(rec)

	// Drafting → Rejected (not allowed)
	if err := c.TransitionTo(CaseStageRejected); err == nil {
		t.Error("expected error for Drafting→Rejected")
	}

	// Drafting → Abandoned (allowed)
	if err := c.TransitionTo(CaseStageAbandoned); err != nil {
		t.Fatalf("Drafting→Abandoned: %v", err)
	}

	// Abandoned → anything (not allowed - terminal state)
	if err := c.TransitionTo(CaseStageDrafting); err == nil {
		t.Error("expected error from terminal state Abandoned")
	}
}

func TestCase_TransitionTo_SameState(t *testing.T) {
	rec := ProjectRecord{ProjectID: "proj_001", Alias: "测试案"}
	c := NewCase(rec)

	// Same state is always allowed
	if err := c.TransitionTo(CaseStageDrafting); err != nil {
		t.Errorf("same-state transition: %v", err)
	}
}

func TestCase_TransitionTo_Unknown(t *testing.T) {
	rec := ProjectRecord{ProjectID: "proj_001", Alias: "测试案"}
	c := NewCase(rec)

	if err := c.TransitionTo("未知阶段"); err == nil {
		t.Error("expected error for unknown stage")
	}
}

func TestCase_CaseTypeLabel(t *testing.T) {
	tests := []struct {
		caseType string
		label    string
	}{
		{"发明专利", "发明专利"},
		{"实用新型", "实用新型"},
		{"外观设计", "外观设计"},
		{"商标", "商标"},
		{"著作权", "著作权"},
		{"", "未分类"},
		{"其他", "其他"},
	}
	for _, tt := range tests {
		c := &Case{Record: ProjectRecord{CaseType: tt.caseType}}
		if got := c.CaseTypeLabel(); got != tt.label {
			t.Errorf("CaseTypeLabel(%q)=%q, want %q", tt.caseType, got, tt.label)
		}
	}
}

func TestCase_Summary(t *testing.T) {
	rec := ProjectRecord{
		ProjectID:    "proj_001",
		Alias:        "智能伸缩支架案",
		CaseType:     "发明专利",
		FilingNumber: "CN202410123456.7",
	}
	c := NewCase(rec)
	summary := c.Summary()
	if summary == "" {
		t.Fatal("empty summary")
	}
}

func TestTransitionAllowed(t *testing.T) {
	// Test the full transition matrix
	tests := []struct {
		from  CaseStage
		to    CaseStage
		allow bool
	}{
		{CaseStageDrafting, CaseStageReviewing, true},
		{CaseStageDrafting, CaseStageRejected, false},
		{CaseStageReviewing, CaseStageDrafting, true}, // 退回修改
		{CaseStageReviewing, CaseStageGranted, true},
		{CaseStageReviewing, CaseStageRejected, true},
		{CaseStageResponded, CaseStageGranted, true},
		{CaseStageResponded, CaseStageRejected, true},
		{CaseStageResponded, CaseStageDrafting, false},
		{CaseStageGranted, CaseStageAbandoned, true},
		{CaseStageGranted, CaseStageDrafting, false},
		{CaseStageRejected, CaseStageDrafting, true}, // 重新起草
		{CaseStageRejected, CaseStageGranted, false},
		{CaseStageAbandoned, CaseStageDrafting, false}, // 终止态
		{CaseStageAbandoned, CaseStageRejected, false},
	}
	for _, tt := range tests {
		err := TransitionAllowed(tt.from, tt.to)
		if tt.allow && err != nil {
			t.Errorf("TransitionAllowed(%q→%q) = %v, want nil", tt.from, tt.to, err)
		}
		if !tt.allow && err == nil {
			t.Errorf("TransitionAllowed(%q→%q) = nil, want error", tt.from, tt.to)
		}
	}
}
