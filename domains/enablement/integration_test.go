package enablement

import (
	"context"
	"testing"

	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Integration tests (P5.6): verify end-to-end data flow
// from disclosure pipeline output → enablement graph input → result
// =============================================================================

// buildEnablementInputFromReport mirrors server/enablement_events.go logic
// to verify the transformation without depending on the server package.
func buildEnablementInputFromReport(report *disclosure.AnalysisReport, evidenceCoverage string) *EnablementInput {
	cover := evidenceCoverage
	if cover == "" {
		cover = "partial"
	}
	input := &EnablementInput{
		EvidenceCoverage: cover,
	}
	if report == nil || report.Extraction == nil {
		return input
	}
	ext := report.Extraction

	for _, f := range ext.Features {
		input.Features = append(input.Features, TechFeature{
			ID:          f.ID,
			Description: f.Description,
			Category:    string(f.Category),
			Function:    f.Function,
			Importance:  f.Importance,
		})
	}
	for _, t := range ext.PFETriples {
		input.PFETriples = append(input.PFETriples, PFETriple{
			ID:         t.ID,
			Problem:    t.Problem,
			FeatureIDs: t.FeatureIDs,
			Effect:     t.Effect,
		})
	}
	input.Problems = ext.Problems
	input.Effects = ext.Effects

	if report.Document != nil {
		input.HasDrawings = report.Document.HasDrawings
		input.DocSections = make(map[string]string)
		for section, content := range report.Document.Sections {
			input.DocSections[string(section)] = content
		}
	}

	if len(input.Features) > 0 {
		input.EvidenceCoverage = "full"
	}
	return input
}

func TestIntegration_DisclosureReportToEnablementInput(t *testing.T) {
	// Simulate a complete disclosure report with all fields populated.
	report := &disclosure.AnalysisReport{
		ID: "test-report-001",
		Document: &disclosure.DisclosureDoc{
			ID:          "doc-001",
			Title:       "一种自动清洁滤网装置",
			Format:      "markdown",
			HasDrawings: true,
			Sections: map[disclosure.DocSection]string{
				disclosure.SecTechField:   "本实用新型涉及过滤设备技术领域",
				disclosure.SecBackground:  "现有滤网需要人工拆卸清洗，费时费力",
				disclosure.SecContent:     "本实用新型提供一种自动清洁滤网装置",
				disclosure.SecEmbodiments: "如图1所示，装置包括壳体(1)、滤网(2)、刮板(3)、电机(4)",
				disclosure.SecDrawings:    "图1为装置结构示意图",
			},
		},
		Extraction: &disclosure.ExtractionResult{
			Problems: []string{"现有滤网需要人工拆卸清洗，费时费力"},
			Features: []disclosure.TechFeature{
				{ID: "f1", Description: "壳体", Category: disclosure.CatStructure, Function: "容纳组件", Importance: "high"},
				{ID: "f2", Description: "滤网", Category: disclosure.CatStructure, Function: "过滤", Importance: "high"},
				{ID: "f3", Description: "电机驱动刮板往复运动", Category: disclosure.CatMethod, Function: "自动清洁", Importance: "high"},
			},
			Effects: []string{"实现自动清洁滤网，无需人工拆卸"},
			PFETriples: []disclosure.PFETriple{
				{ID: "t1", Problem: "现有滤网需要人工拆卸清洗", FeatureIDs: []string{"f1", "f2", "f3"}, Effect: "实现自动清洁"},
			},
		},
	}

	// Transform to enablement input.
	input := buildEnablementInputFromReport(report, "partial")

	// Verify transformation.
	if len(input.Features) != 3 {
		t.Errorf("expected 3 features, got %d", len(input.Features))
	}
	if len(input.PFETriples) != 1 {
		t.Errorf("expected 1 PFE triple, got %d", len(input.PFETriples))
	}
	if len(input.Problems) != 1 {
		t.Errorf("expected 1 problem, got %d", len(input.Problems))
	}
	if input.EvidenceCoverage != "full" {
		t.Errorf("expected EvidenceCoverage=full, got %q", input.EvidenceCoverage)
	}
	if !input.HasDrawings {
		t.Error("expected HasDrawings=true")
	}
	if len(input.DocSections) == 0 {
		t.Error("expected non-empty DocSections")
	}

	// Verify the input passes loadInputNode validation.
	state := graph.PregelState{stateKeyInput: input}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode: %v", err)
	}
	// Should NOT have set result (valid input passes through).
	if _, ok := state[stateKeyResult]; ok {
		t.Error("loadInputNode should not set result for valid input")
	}
}

func TestIntegration_NilReport(t *testing.T) {
	input := buildEnablementInputFromReport(nil, "partial")
	if input == nil {
		t.Fatal("expected non-nil input from nil report")
	}
	if len(input.Features) != 0 {
		t.Error("expected 0 features from nil report")
	}
	if input.EvidenceCoverage != "partial" {
		t.Errorf("expected EvidenceCoverage=partial, got %q", input.EvidenceCoverage)
	}
}

func TestIntegration_NilExtraction(t *testing.T) {
	report := &disclosure.AnalysisReport{
		ID:         "test-002",
		Extraction: nil,
	}
	input := buildEnablementInputFromReport(report, "partial")
	if input == nil {
		t.Fatal("expected non-nil input from report with nil extraction")
	}
	if len(input.Features) != 0 {
		t.Error("expected 0 features when extraction is nil")
	}
}

func TestIntegration_EmptyExtraction(t *testing.T) {
	report := &disclosure.AnalysisReport{
		ID: "test-003",
		Extraction: &disclosure.ExtractionResult{
			Features:   []disclosure.TechFeature{},
			PFETriples: []disclosure.PFETriple{},
			Problems:   []string{},
			Effects:    []string{},
		},
	}
	input := buildEnablementInputFromReport(report, "partial")

	// This input should trigger skip behavior.
	state := graph.PregelState{stateKeyInput: input}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode: %v", err)
	}
	result, ok := state[stateKeyResult].(*EnablementResult)
	if !ok {
		t.Fatal("expected EnablementResult in state")
	}
	if !result.Skipped {
		t.Error("expected Skipped=true for empty extraction report")
	}
}

func TestIntegration_RoundTripWithMockGraph(t *testing.T) {
	// Test full round-trip: report → input → graph → result.
	report := &disclosure.AnalysisReport{
		ID: "test-roundtrip",
		Document: &disclosure.DisclosureDoc{
			ID:    "doc-r1",
			Title: "测试专利",
			Sections: map[disclosure.DocSection]string{
				disclosure.SecTechField: "测试技术领域",
			},
		},
		Extraction: &disclosure.ExtractionResult{
			Problems: []string{"测试问题"},
			Features: []disclosure.TechFeature{
				{ID: "f1", Description: "测试特征", Category: disclosure.CatStructure, Function: "测试功能", Importance: "high"},
			},
			Effects: []string{"测试效果"},
			PFETriples: []disclosure.PFETriple{
				{ID: "t1", Problem: "测试问题", FeatureIDs: []string{"f1"}, Effect: "测试效果"},
			},
		},
	}

	input := buildEnablementInputFromReport(report, "partial")

	// Build and run the graph with mock provider.
	compiled, err := BuildEnablementGraph(mockProvider{})
	if err != nil {
		t.Fatalf("BuildEnablementGraph: %v", err)
	}

	state := graph.PregelState{stateKeyInput: input}
	state, err = compiled.Run(context.Background(), state)
	if err != nil {
		t.Fatalf("graph run: %v", err)
	}

	result, ok := state[stateKeyResult].(*EnablementResult)
	if !ok {
		t.Fatal("expected EnablementResult in state after graph run")
	}
	if !result.Assessed {
		t.Error("expected Assessed=true after graph run")
	}
	if result.Skipped {
		t.Error("expected Skipped=false for valid input")
	}
	// With mock provider, LLM nodes return empty strings, so conclusion
	// should fall through to default parsing.
	t.Logf("Round-trip result: assessed=%v skipped=%v confidence=%q",
		result.Assessed, result.Skipped, result.Confidence)
}
