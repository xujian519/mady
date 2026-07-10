package domains

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// mockProvider implements agentcore.Provider for testing.
type mockProvider struct {
	respond func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error)
}

func (m *mockProvider) Complete(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	if m.respond != nil {
		return m.respond(req)
	}
	return &agentcore.ProviderResponse{Content: `{"domain": "chat", "confidence": 0.95}`}, nil
}

func (m *mockProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, errors.New("streaming not implemented in mock")
}

func TestKeywordClassifier_Classify(t *testing.T) {
	classifier := &KeywordClassifier{}
	ctx := context.Background()

	tests := []struct {
		input string
		want  string
	}{
		{"帮我检索一下这个专利号", DomainPatent},
		{"权利要求分析", DomainPatent},
		{"新颖性判断", DomainPatent},
		{"prior art 检索", DomainPatent},
		{"PCT申请流程", DomainPatent},
		{"帮我查合同法第52条", DomainLegal},
		{"这个案件判例怎么找", DomainLegal},
		{"法律意见", DomainLegal},
		{"合同纠纷怎么处理", DomainLegal},
		{"刑法第264条", DomainLegal},
		{"今天天气怎么样", DomainChat},
		{"帮我写个Python脚本", DomainChat},
		{"你好", DomainChat},
		{"什么是AI", DomainChat},
		{"帮我分析这个文件", DomainChat},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, confidence, err := classifier.Classify(ctx, tt.input)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Classify(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if confidence != 1.0 {
				t.Errorf("KeywordClassifier confidence = %f, want 1.0", confidence)
			}
		})
	}

	// Verify domain constants are consistent.
	domain, _, _ := classifier.Classify(ctx, "专利相关")
	if domain != DomainPatent {
		t.Errorf("patent keyword classified as %q", domain)
	}
	if DomainPatent != "patent" {
		t.Errorf("DomainPatent = %q, want %q", DomainPatent, "patent")
	}
	if DomainLegal != "legal" {
		t.Errorf("DomainLegal = %q, want %q", DomainLegal, "legal")
	}
	if DomainChat != "chat" {
		t.Errorf("DomainChat = %q, want %q", DomainChat, "chat")
	}
}

func TestKeywordClassifier_EmptyInput(t *testing.T) {
	classifier := &KeywordClassifier{}
	ctx := context.Background()

	got, _, err := classifier.Classify(ctx, "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if got != DomainChat {
		t.Errorf("empty input should classify as chat, got %q", got)
	}
}

func TestLLMClassifier_PatentClassification(t *testing.T) {
	mock := &mockProvider{
		respond: func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			// Verify the request is well-formed.
			if req.ResponseFormat == nil {
				t.Error("expected ResponseFormat to be set")
			}
			if len(req.Messages) < 2 {
				t.Error("expected at least system + user messages")
			}
			result := classificationResult{Domain: "patent", Confidence: 0.95, Reasoning: "contains patent keywords"}
			b, _ := json.Marshal(result)
			return &agentcore.ProviderResponse{
				Content:    string(b),
				Structured: json.RawMessage(b),
			}, nil
		},
	}

	classifier := NewLLMClassifier(mock)
	ctx := context.Background()

	domain, confidence, err := classifier.Classify(ctx, "帮我检索这个发明专利的新颖性")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if domain != DomainPatent {
		t.Errorf("domain = %q, want %q", domain, DomainPatent)
	}
	if confidence < 0.9 {
		t.Errorf("confidence = %f, want >= 0.9", confidence)
	}
}

func TestLLMClassifier_LegalClassification(t *testing.T) {
	mock := &mockProvider{
		respond: func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			result := classificationResult{Domain: "legal", Confidence: 0.88, Reasoning: "legal question"}
			b, _ := json.Marshal(result)
			return &agentcore.ProviderResponse{
				Content:    string(b),
				Structured: json.RawMessage(b),
			}, nil
		},
	}

	classifier := NewLLMClassifier(mock)
	ctx := context.Background()

	domain, confidence, err := classifier.Classify(ctx, "合同纠纷如何处理")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if domain != DomainLegal {
		t.Errorf("domain = %q, want %q", domain, DomainLegal)
	}
	if confidence < 0.8 {
		t.Errorf("confidence = %f, want >= 0.8", confidence)
	}
}

func TestLLMClassifier_LowConfidenceFallback(t *testing.T) {
	// LLM returns low confidence — should fall back to keyword classifier.
	mock := &mockProvider{
		respond: func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			result := classificationResult{Domain: "chat", Confidence: 0.3, Reasoning: "unclear"}
			b, _ := json.Marshal(result)
			return &agentcore.ProviderResponse{
				Content:    string(b),
				Structured: json.RawMessage(b),
			}, nil
		},
	}

	classifier := NewLLMClassifier(mock)
	ctx := context.Background()

	// Input with "专利" keyword should be caught by keyword fallback even though LLM says "chat" with low confidence.
	domain, _, err := classifier.Classify(ctx, "帮我看看这个专利")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if domain != DomainPatent {
		t.Errorf("low confidence should fall back to keyword; got %q, want %q", domain, DomainPatent)
	}
}

func TestLLMClassifier_ErrorFallback(t *testing.T) {
	mock := &mockProvider{
		respond: func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			return nil, errors.New("api unavailable")
		},
	}

	classifier := NewLLMClassifier(mock)
	ctx := context.Background()

	// Should fall back to keyword on error.
	domain, _, err := classifier.Classify(ctx, "专利检索")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if domain != DomainPatent {
		t.Errorf("error should fall back to keyword; got %q, want %q", domain, DomainPatent)
	}
}

func TestLLMClassifier_UnknownDomainFallback(t *testing.T) {
	mock := &mockProvider{
		respond: func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			result := classificationResult{Domain: "medical", Confidence: 0.95, Reasoning: "medical query"}
			b, _ := json.Marshal(result)
			return &agentcore.ProviderResponse{
				Content:    string(b),
				Structured: json.RawMessage(b),
			}, nil
		},
	}

	classifier := NewLLMClassifier(mock)
	ctx := context.Background()

	// Unknown domain with high confidence should still fall back to keyword.
	// No patent/legal keywords in input, so falls back to chat.
	domain, _, err := classifier.Classify(ctx, "我头痛怎么办")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if domain != DomainChat {
		t.Errorf("unknown domain should fall back to keyword; got %q, want %q", domain, DomainChat)
	}
}

func TestLLMClassifier_CustomThreshold(t *testing.T) {
	mock := &mockProvider{
		respond: func(req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			result := classificationResult{Domain: "patent", Confidence: 0.5, Reasoning: "uncertain patent"}
			b, _ := json.Marshal(result)
			return &agentcore.ProviderResponse{
				Content:    string(b),
				Structured: json.RawMessage(b),
			}, nil
		},
	}

	// Default threshold (0.7): should fall back.
	classifier := NewLLMClassifier(mock)
	ctx := context.Background()

	domain, _, err := classifier.Classify(ctx, "一般问题")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Confidence 0.5 < 0.7, falls back to keyword -> chat (input has no patent/legal keywords).
	if domain != DomainChat {
		t.Errorf("below default threshold should fall back; got %q, want %q", domain, DomainChat)
	}

	// Custom threshold 0.4: should accept.
	classifierLow := NewLLMClassifier(mock)
	classifierLow.Threshold = 0.4

	domain, confidence, err := classifierLow.Classify(ctx, "一般问题")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if domain != DomainPatent {
		t.Errorf("above custom threshold should accept LLM result; got %q, want %q", domain, DomainPatent)
	}
	if confidence != 0.5 {
		t.Errorf("confidence = %f, want 0.5", confidence)
	}
}

func TestLLMClassifier_NilProvider(t *testing.T) {
	classifier := NewLLMClassifier(nil)
	ctx := context.Background()

	// nil provider causes an error, which triggers keyword fallback.
	domain, _, err := classifier.Classify(ctx, "法律问题咨询")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if domain != DomainLegal {
		t.Errorf("nil provider should fall back to keyword; got %q, want %q", domain, DomainLegal)
	}
}
