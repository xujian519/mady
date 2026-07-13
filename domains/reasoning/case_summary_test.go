package reasoning

import (
	"strings"
	"testing"
)

func TestExtractCaseSummary_WithBlackboard(t *testing.T) {
	bb := NewFactBlackboard("case-001", CasePatentability, "G06F")
	bb.AddFact(FactEntry{Content: "对比文件1公开了技术特征A"})
	bb.AddFact(FactEntry{Content: "权利要求1还包含特征B"})
	bb.SetWorkflowID("wf-patent-001")

	cp := &StageCheckpoint{
		CheckpointID: "cp-001",
		CaseID:       "case-001",
		CaseType:     CasePatentability,
		CurrentStage: 3,
		Blackboard:   bb,
	}

	s := ExtractCaseSummary(cp)
	if s.CaseID != "case-001" {
		t.Errorf("CaseID = %s, want case-001", s.CaseID)
	}
	if s.CaseType != string(CasePatentability) {
		t.Errorf("CaseType = %s, want %s", s.CaseType, CasePatentability)
	}
	if s.TechnicalField != "G06F" {
		t.Errorf("TechnicalField = %s, want G06F", s.TechnicalField)
	}
	if s.CurrentStage != 3 {
		t.Errorf("CurrentStage = %d, want 3", s.CurrentStage)
	}
	if s.FactCount != 2 {
		t.Errorf("FactCount = %d, want 2", s.FactCount)
	}
	if s.WorkflowID != "wf-patent-001" {
		t.Errorf("WorkflowID = %s, want wf-patent-001", s.WorkflowID)
	}
}

func TestExtractCaseSummary_NilBlackboard(t *testing.T) {
	cp := &StageCheckpoint{
		CheckpointID: "cp-002",
		CaseID:       "case-002",
		CaseType:     CaseNoveltySearch,
		CurrentStage: 1,
		Blackboard:   nil,
	}

	s := ExtractCaseSummary(cp)
	if s.CaseID != "case-002" {
		t.Errorf("CaseID = %s, want case-002", s.CaseID)
	}
	if s.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", s.FactCount)
	}
	if s.TechnicalField != "" {
		t.Errorf("TechnicalField should be empty for nil blackboard")
	}
}

func TestCaseSummary_String(t *testing.T) {
	s := CaseSummary{
		CaseID:         "case-003",
		CaseType:       "patentability",
		TechnicalField: "H04L",
		CurrentStage:   2,
		FactCount:      5,
		WorkflowID:     "wf-003",
		CreatedAt:      "2026-07-13T10:00:00Z",
		UpdatedAt:      "2026-07-13T11:00:00Z",
	}

	str := s.String()
	if str == "" {
		t.Fatal("String() returned empty")
	}
	if !strings.Contains(str, "case-003") {
		t.Errorf("String() missing case ID: %s", str)
	}
	if !strings.Contains(str, "H04L") {
		t.Errorf("String() missing technical field: %s", str)
	}
	if !strings.Contains(str, "5条") {
		t.Errorf("String() missing fact count: %s", str)
	}
}
