package evidence

import (
	"context"
	"encoding/json"
	"testing"
)

func TestEvidenceDomainExtension_Name(t *testing.T) {
	ext := NewDomainExtension(nil)
	if ext.Name() != ExtensionNameDomain {
		t.Errorf("expected %q, got %q", ExtensionNameDomain, ext.Name())
	}
}

func TestEvidenceDomainExtension_InitDispose(t *testing.T) {
	ext := NewDomainExtension(nil)
	if err := ext.Init(context.Background(), nil); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := ext.Dispose(); err != nil {
		t.Fatalf("Dispose: %v", err)
	}
}

func TestEvidenceDomainExtension_Tools(t *testing.T) {
	ext := NewDomainExtension(nil)
	tools := ext.Tools()
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"judge_triple", "check_burden", "assess_standard", "determine_standard", "detect_conflict", "judge_type_specific", "assess_credibility"} {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestJudgeTripleTool_Success(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTripleTool(engine)

	args := `{"source_uri":"patent:CN12345678A","snippet":"权利要求1公开了一种图像识别方法"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	for _, key := range []string{"relevance", "legality", "authenticity", "overall_score", "reasoning"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in result", key)
		}
	}
	if overall, ok := m["overall_score"].(float64); ok {
		if overall < 0 || overall > 1 {
			t.Errorf("overall %f outside [0,1]", overall)
		}
	}

	// Verify semantic content
	if confidence, ok := m["confidence"].(float64); !ok || confidence <= 0 {
		t.Errorf("expected positive confidence, got %v", m["confidence"])
	}
	if reasoning, ok := m["reasoning"].(string); !ok || reasoning == "" {
		t.Error("expected non-empty reasoning")
	}
}

func TestJudgeTripleTool_MissingSnippet(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTripleTool(engine)

	args := `{"source_uri":"patent:CN12345678A"}`
	_, err := tool.Func(context.Background(), json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing snippet")
	}
}

func TestJudgeTripleTool_InvalidJSON(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTripleTool(engine)

	_, err := tool.Func(context.Background(), json.RawMessage(`{bad json}`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestCheckBurdenTool_ValidScenario(t *testing.T) {
	tool := newBurdenTool()

	args := `{"scenario":"patent_invalidation"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["holder"] != "请求人" {
		t.Errorf("expected holder 请求人, got %v", m["holder"])
	}
	if m["standard"] != "优势证据" {
		t.Errorf("expected standard 优势证据, got %v", m["standard"])
	}
}

func TestCheckBurdenTool_InvalidScenario(t *testing.T) {
	tool := newBurdenTool()

	args := `{"scenario":"nonexistent"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["holder"] != "主张方" {
		t.Errorf("expected fallback holder 主张方, got %v", m["holder"])
	}
}

func TestCheckBurdenTool_MissingScenario(t *testing.T) {
	tool := newBurdenTool()

	args := `{}`
	_, err := tool.Func(context.Background(), json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing scenario")
	}
}

func TestAssessStandardTool_Met(t *testing.T) {
	tool := newStandardTool()
	args := `{"standard":"preponderance","supporting_count":7,"total_count":10}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["met"] != true {
		t.Errorf("expected met=true, got %v", m["met"])
	}
}

func TestAssessStandardTool_NotMet(t *testing.T) {
	tool := newStandardTool()
	args := `{"standard":"high_probability","supporting_count":3,"total_count":10}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["met"] != false {
		t.Errorf("expected met=false, got %v", m["met"])
	}
}

func TestAssessStandardTool_EmptyTotal(t *testing.T) {
	tool := newStandardTool()
	args := `{"standard":"high_probability","supporting_count":0,"total_count":0}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["met"] != false {
		t.Errorf("expected met=false for zero total, got %v", m["met"])
	}
}

func TestDetectConflictTool_DirectionConflict(t *testing.T) {
	tool := newConflictTool(nil)
	args := `{"claims":[{"claim_id":"特征A","supporting":["ev1"],"contradicting":["ev2"]},{"claim_id":"特征B","supporting":["ev3"],"contradicting":[]}]}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	conflicts := m["conflicts"].([]map[string]any)
	if len(conflicts) < 1 {
		t.Fatal("expected at least 1 conflict")
	}
}

func TestDetectConflictTool_NoConflict(t *testing.T) {
	tool := newConflictTool(nil)
	args := `{"claims":[{"claim_id":"特征C","supporting":["ev4"],"contradicting":[]}]}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	conflicts := m["conflicts"].([]map[string]any)
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts, got %d", len(conflicts))
	}
}

func TestDetectConflictTool_EmptyClaims(t *testing.T) {
	tool := newConflictTool(nil)
	args := `{"claims":[]}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	conflicts := m["conflicts"].([]map[string]any)
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts, got %d", len(conflicts))
	}
}

func TestJudgeTypeSpecificTool_InternetPublic(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTypeSpecificTool(engine)
	// Use web_pub: prefix to infer internet_publication type
	args := `{"source_uri":"web_pub:https://example.com/product-page"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	et, _ := m["evidence_type"].(string)
	if et != string(EvTypeInternetPublication) {
		t.Errorf("expected evidence_type=%q, got %q", EvTypeInternetPublication, et)
	}
}

func TestJudgeTypeSpecificTool_Patent(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTypeSpecificTool(engine)
	args := `{"source_uri":"patent:CN12345678A"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if _, ok := m["evidence_type"]; !ok {
		t.Error("missing evidence_type")
	}
}

func TestJudgeTypeSpecificTool_EmptyURI(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTypeSpecificTool(engine)
	args := `{"source_uri":""}`
	_, err := tool.Func(context.Background(), json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for empty source_uri")
	}
}

func TestDetermineStandardTool_Infringement(t *testing.T) {
	tool := newDetermineStandardTool()
	args := `{"scenario":"patent_infringement"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["recommended_standard"] != string(StandardHighProbability) {
		t.Errorf("expected high_probability for infringement, got %v", m["recommended_standard"])
	}
	if m["scenario"] != "patent_infringement" {
		t.Errorf("expected scenario patent_infringement, got %v", m["scenario"])
	}
}

func TestDetermineStandardTool_InvalidScenario(t *testing.T) {
	tool := newDetermineStandardTool()
	args := `{"scenario":"invalid_scenario"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["recommended_standard"] == "" {
		t.Error("expected fallback standard for invalid scenario")
	}
}

func TestDetermineStandardTool_EmptyScenario(t *testing.T) {
	tool := newDetermineStandardTool()
	_, err := tool.Func(context.Background(), json.RawMessage(`{"scenario":""}`))
	if err == nil {
		t.Error("expected error for empty scenario")
	}
}

func TestCredibilityTool_GovernmentDomain(t *testing.T) {
	tool := newCredibilityTool()
	args := `{"uri":"https://www.cnipa.gov.cn/patent/12345"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["platform_level"] != "high" {
		t.Errorf("expected platform_level=high for government domain, got %v", m["platform_level"])
	}
	if m["platform_score"].(float64) != 0.95 {
		t.Errorf("expected platform_score=0.95 for government domain, got %v", m["platform_score"])
	}
}

func TestCredibilityTool_VerifiedCopy(t *testing.T) {
	tool := newCredibilityTool()
	args := `{"uri":"https://weibo.com/user/post123","is_verified_copy":true}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["combined_level"] != "medium_high" && m["combined_level"] != "medium" {
		t.Logf("verified copy of social media: combined_level=%v", m["combined_level"])
	}
	if m["is_verified_copy"] != true {
		t.Error("expected is_verified_copy=true")
	}
}

func TestCredibilityTool_EmptyURI(t *testing.T) {
	tool := newCredibilityTool()
	_, err := tool.Func(context.Background(), json.RawMessage(`{"uri":""}`))
	if err == nil {
		t.Error("expected error for empty uri")
	}
}
