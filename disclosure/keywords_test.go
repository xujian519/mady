package disclosure

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// mockKeywordProvider implements agentcore.Provider for keyword generation tests.
type mockKeywordProvider struct {
	respond func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error)
}

func (m *mockKeywordProvider) Complete(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	if m.respond != nil {
		return m.respond(req)
	}
	return &agentcore.ProviderResponse{
		Content: `{"keywords": ["传感器", "MEMS", "加速度计", "低功耗", "微机电系统"]}`,
	}, nil
}

func (m *mockKeywordProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, nil
}

func TestKeywordGenerator_RuleOnly(t *testing.T) {
	gen := NewKeywordGenerator(nil)
	ext := &ExtractionResult{
		Problems: []string{"现有方案功耗高", "精度不足"},
		Features: []TechFeature{
			{Description: "MEMS加速度计", Category: CatStructure, Importance: "high"},
			{Description: "低功耗电路", Category: CatParameter, Importance: "medium"},
		},
		Effects: []string{"降低功耗", "提高精度"},
	}

	keywords, err := gen.Generate(context.Background(), ext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(keywords) == 0 {
		t.Error("expected non-empty keywords from rule generation")
	}

	// Rule mode should extract terms from descriptions and problems.
	found := false
	for _, kw := range keywords {
		if kw == "MEMS加速度计" || kw == "低功耗电路" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("rule keywords %v should contain feature descriptions", keywords)
	}
}

func TestKeywordGenerator_LLMMode(t *testing.T) {
	expectedKeywords := []string{"传感器", "MEMS", "加速度计", "低功耗", "微机电系统"}
	mock := &mockKeywordProvider{
		respond: func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			// Verify proper request format
			if req.ResponseFormat == nil {
				t.Error("expected ResponseFormat to be set")
			}
			if req.ResponseFormat.Type != agentcore.ResponseFormatJSONSchema {
				t.Error("expected JSON Schema response format")
			}
			b, _ := json.Marshal(map[string]any{"keywords": expectedKeywords})
			return &agentcore.ProviderResponse{
				Content:    string(b),
				Structured: json.RawMessage(b),
			}, nil
		},
	}

	gen := NewKeywordGenerator(mock)
	ext := sampleExtractionResult()

	keywords, err := gen.Generate(context.Background(), ext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(keywords) != len(expectedKeywords) {
		t.Errorf("got %d keywords, want %d", len(keywords), len(expectedKeywords))
	}
	for i, kw := range keywords {
		if kw != expectedKeywords[i] {
			t.Errorf("keywords[%d] = %q, want %q", i, kw, expectedKeywords[i])
		}
	}
}

func TestKeywordGenerator_LLMFallback(t *testing.T) {
	// LLM fails → should fall back to rule mode
	mock := &mockKeywordProvider{
		respond: func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			return nil, assertAnError("api unavailable")
		},
	}

	gen := NewKeywordGenerator(mock)
	ext := sampleExtractionResult()

	keywords, err := gen.Generate(context.Background(), ext)
	if err != nil {
		t.Fatalf("fallback should not return error: %v", err)
	}

	if len(keywords) == 0 {
		t.Error("fallback should produce keywords from rule mode")
	}
}

func TestKeywordGenerator_EmptyExtraction(t *testing.T) {
	gen := NewKeywordGenerator(nil)
	ext := &ExtractionResult{}

	keywords, err := gen.Generate(context.Background(), ext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(keywords) != 0 {
		t.Errorf("empty extraction should produce 0 keywords, got %d", len(keywords))
	}
}

func TestKeywordGenerator_RulesOnlyFlag(t *testing.T) {
	mock := &mockKeywordProvider{
		respond: func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			b, _ := json.Marshal(map[string]any{"keywords": []string{"llm_keyword"}})
			return &agentcore.ProviderResponse{
				Content:    string(b),
				Structured: json.RawMessage(b),
			}, nil
		},
	}

	gen := NewKeywordGenerator(mock)
	gen.rulesOnly = true // Force rule mode even with provider
	ext := sampleExtractionResult()

	keywords, err := gen.Generate(context.Background(), ext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT contain the LLM keyword
	for _, kw := range keywords {
		if kw == "llm_keyword" {
			t.Error("rulesOnly mode should not use LLM output")
		}
	}
}

// sampleExtractionResult returns a test extraction result.
func sampleExtractionResult() *ExtractionResult {
	return &ExtractionResult{
		Problems: []string{"传统方案功耗过高", "测量精度不足"},
		Features: []TechFeature{
			{Description: "MEMS加速度计", Category: CatStructure, Importance: "high"},
			{Description: "低功耗信号处理电路", Category: CatParameter, Importance: "high"},
			{Description: "温度补偿算法", Category: CatMethod, Importance: "medium"},
		},
		Effects: []string{"显著降低系统功耗", "提高测量精度", "宽温区稳定工作"},
	}
}

// assertAnError returns a simple error for testing.
func assertAnError(msg string) error {
	return &testError{msg: msg}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
