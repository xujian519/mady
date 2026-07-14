package disclosure

import (
	"testing"

	"github.com/xujian519/mady/agentcore/evidence"
)

func TestEvidenceBuilder_FromReport(t *testing.T) {
	report := &AnalysisReport{
		ID: "rpt_test_001",
		Document: &DisclosureDoc{
			ID:      "doc_001",
			RawText: "测试文档",
		},
		Extraction: &ExtractionResult{
			Features: []TechFeature{
				{ID: "F1", Description: "智能伸缩臂", PriorArtStatus: "unknown", Importance: "high"},
				{ID: "F2", Description: "压力传感器", PriorArtStatus: "known", Importance: "medium"},
			},
		},
		Novelty: &NoveltyResult{
			Assessed:   true,
			Conclusion: "部分特征可能具有新颖性",
		},
	}

	cb := BuildEvidencePackage(report, "session_001")
	if cb.SpanCount() == 0 {
		t.Fatal("expected at least 1 evidence span")
	}

	ev := cb.GetEvidence("claim_feat_F1")
	if len(ev) == 0 {
		t.Error("expected evidence for claim_feat_F1")
	}
	if len(ev) > 0 && ev[0].Direction != evidence.DirectionSupporting {
		t.Errorf("F1 direction=%q, want supporting", ev[0].Direction)
	}

	ev2 := cb.GetEvidence("claim_feat_F2")
	if len(ev2) == 0 {
		t.Error("expected evidence for claim_feat_F2")
	}
	if len(ev2) > 0 && ev2[0].Direction != evidence.DirectionContradicting {
		t.Errorf("F2 direction=%q, want contradicting", ev2[0].Direction)
	}
}

func TestEvidenceBuilder_NilReport(t *testing.T) {
	cb := BuildEvidencePackage(nil, "session_001")
	if cb.SpanCount() != 0 {
		t.Errorf("expected 0 spans for nil report, got %d", cb.SpanCount())
	}
}

func TestEvidenceBuilder_NilExtraction(t *testing.T) {
	report := &AnalysisReport{ID: "rpt_test_002"}
	cb := BuildEvidencePackage(report, "session_001")
	if cb.SpanCount() != 0 {
		t.Errorf("expected 0 spans for nil extraction, got %d", cb.SpanCount())
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"a": 1}`, `{"a": 1}`},
		{`text {"a": 1} text`, `{"a": 1}`},
		{`no json here`, ``},
		{`{incomplete`, ``},
	}
	for _, tt := range tests {
		got := extractJSON(tt.input)
		if got != tt.want {
			t.Errorf("extractJSON(%q)=%q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNoveltySchema(t *testing.T) {
	schema := noveltySchema()
	if schema == nil {
		t.Fatal("noveltySchema() returned nil")
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema missing properties")
	}
	if _, ok := props["conclusion"]; !ok {
		t.Error("schema missing 'conclusion'")
	}
	if _, ok := props["feature_assessments"]; !ok {
		t.Error("schema missing 'feature_assessments'")
	}
}

func TestBuildNoveltyInput(t *testing.T) {
	state := map[string]any{
		StateKeyExtraction: &ExtractionResult{
			Features: []TechFeature{
				{ID: "F1", Description: "测试特征", Category: CatStructure, Importance: "high"},
			},
			Problems: []string{"测试问题"},
			Effects:  []string{"测试效果"},
		},
		StateKeySearchKeywords: []string{"测试关键词"},
	}
	input := buildNoveltyInput(state)
	if input == "" {
		t.Fatal("expected non-empty input")
	}
}
