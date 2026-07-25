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
	if len(tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"judge_triple", "check_burden", "assess_standard", "detect_conflict", "judge_type_specific"} {
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
