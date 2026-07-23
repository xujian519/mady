package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestParseFactsFromResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int // expected number of facts
	}{
		{
			name:    "standard json",
			content: `{"facts": ["用户偏好使用表格", "用户从事专利代理工作"]}`,
			want:    2,
		},
		{
			name:    "json with markdown fences",
			content: "```json\n{\"facts\": [\"用户偏好使用表格\"]}\n```",
			want:    1,
		},
		{
			name:    "empty facts array",
			content: `{"facts": []}`,
			want:    0,
		},
		{
			name:    "filtered empty strings",
			content: `{"facts": ["用户偏好使用表格", "", "   ", "无"]}`,
			want:    1,
		},
		{
			name:    "invalid json",
			content: `not json at all`,
			want:    -1, // error expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := parseFactsFromResponse(tt.content)
			if tt.want == -1 {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(facts) != tt.want {
				t.Errorf("got %d facts, want %d", len(facts), tt.want)
			}
		})
	}
}

func TestParseFactsFromText(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{
			name:    "plain lines",
			content: "用户偏好使用表格\n用户从事专利代理工作\n助手建议三步法",
			want:    3,
		},
		{
			name:    "numbered lines",
			content: "1. 用户偏好使用表格\n2. 用户从事专利代理工作",
			want:    2,
		},
		{
			name:    "dash prefixed",
			content: "- 用户偏好使用表格\n- 用户从事专利代理工作",
			want:    2,
		},
		{
			name:    "empty and noise",
			content: "\n\n无\n用户偏好使用表格\n",
			want:    1,
		},
		{
			name:    "only noise",
			content: "无",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := parseFactsFromText(tt.content)
			if len(facts) != tt.want {
				t.Errorf("got %d facts, want %d: %v", len(facts), tt.want, facts)
			}
		})
	}
}

func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"```json\n{\"a\":1}\n```", "{\"a\":1}"},
		{"```\ntext\n```", "text"},
		{"plain text", "plain text"},
		{"```json\n{\"a\":1}", "{\"a\":1}"},
	}

	for _, tt := range tests {
		t.Run(tt.input[:min(20, len(tt.input))], func(t *testing.T) {
			got := stripMarkdownFences(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Mock provider for LLM extractor tests
// ---------------------------------------------------------------------------

type mockProvider struct {
	completeFn func(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error)
}

func (m *mockProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return m.completeFn(ctx, req)
}

func (m *mockProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, errors.New("stream not implemented")
}

// ---------------------------------------------------------------------------
// 边界条件: 空对话
// ---------------------------------------------------------------------------

func TestExtractFacts_EmptyConversation(t *testing.T) {
	extractor := NewProviderExtractor(nil, "test-model")

	facts, err := extractor.ExtractFacts(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error for empty conversation: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts from empty conversation, got %d", len(facts))
	}
}

// ---------------------------------------------------------------------------
// 敏感数据过滤
// ---------------------------------------------------------------------------

func TestExtractFacts_SensitiveDataFiltered(t *testing.T) {
	// The extractor should NOT send sensitive data to the provider.
	// We verify this by intercepting the request and checking that the
	// sensitive data is not present in the prompt sent to the model.
	var capturedRequest *agentcore.ProviderRequest
	mock := &mockProvider{
		completeFn: func(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			capturedRequest = req
			return &agentcore.ProviderResponse{Content: `{"facts": ["用户偏好红色"]}`}, nil
		},
	}

	extractor := NewProviderExtractor(mock, "test-model")

	conversation := `用户: 我的密码是 password=supersecret123, 请帮我处理这个文件.
助手: 好的,我帮你处理文件.`

	facts, err := extractor.ExtractFacts(context.Background(), conversation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) == 0 {
		t.Fatal("expected at least 1 fact")
	}

	// Verify the password was NOT sent to the LLM in plain text.
	// The providerExtractor sends the entire conversation as-is, so
	// we check that the captured request contains the conversation.
	// This test validates that extractFact's conversation input
	// has been sanitized before being passed to the LLM provider.
	if capturedRequest != nil {
		for _, msg := range capturedRequest.Messages {
			if strings.Contains(msg.Content, "supersecret123") {
				t.Errorf("sensitive data found in provider request: %s", msg.Content)
			}
		}
	}
}

func TestExtractFacts_SensitiveFieldsInInput(t *testing.T) {
	// Test with input containing various sensitive field patterns.
	sensitiveInput := `用户: API key=sk-1234567890abcdef, 帮我查询.
助手: 已查询完成, password=mysecret123不应该出现在记忆中.`

	mock := &mockProvider{
		completeFn: func(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			// Return a response that doesn't contain the sensitive data.
			return &agentcore.ProviderResponse{Content: `{"facts": ["用户请求查询操作"]}`}, nil
		},
	}

	extractor := NewProviderExtractor(mock, "test-model")
	facts, err := extractor.ExtractFacts(context.Background(), sensitiveInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) == 0 {
		t.Error("expected facts despite sensitive data in input")
	}
}

// ---------------------------------------------------------------------------
// LLM 错误路径: mock 返回 error
// ---------------------------------------------------------------------------

func TestExtractFacts_ProviderError_StructuredFailsThenFallback(t *testing.T) {
	// Provider returns error on structured output call.
	var callCount int
	mock := &mockProvider{
		completeFn: func(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			callCount++
			if callCount == 1 {
				// First call (structured) fails.
				return nil, errors.New("provider unavailable")
			}
			// Second call (fallback) succeeds.
			return &agentcore.ProviderResponse{
				Content: "用户偏好使用表格\n用户从事专利代理工作",
			}, nil
		},
	}

	extractor := NewProviderExtractor(mock, "test-model")
	facts, err := extractor.ExtractFacts(context.Background(), "用户: 我喜欢用表格\n助手: 好的")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) != 2 {
		t.Errorf("expected 2 facts from fallback, got %d: %v", len(facts), facts)
	}
	if callCount != 2 {
		t.Errorf("expected 2 provider calls (1 structured + 1 fallback), got %d", callCount)
	}
}

func TestExtractFacts_AllProvidersFail(t *testing.T) {
	mock := &mockProvider{
		completeFn: func(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			return nil, errors.New("provider permanently unavailable")
		},
	}

	extractor := NewProviderExtractor(mock, "test-model")
	facts, err := extractor.ExtractFacts(context.Background(), "用户: 你好\n助手: 你好")
	if err == nil {
		t.Fatal("expected error when all providers fail, got nil")
	}
	if facts != nil {
		t.Errorf("expected nil facts on error, got %d", len(facts))
	}
}

func TestExtractFacts_StructuredParseFailsThenFallback(t *testing.T) {
	// Structured output returns invalid JSON → fallback to text parsing.
	var callCount int
	mock := &mockProvider{
		completeFn: func(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			callCount++
			if callCount == 1 {
				// Structured output returns unparsable content.
				return &agentcore.ProviderResponse{
					Content: "not valid json at all",
				}, nil
			}
			// Fallback succeeds.
			return &agentcore.ProviderResponse{
				Content: "1. 用户偏好表格\n2. 用户从事专利工作",
			}, nil
		},
	}

	extractor := NewProviderExtractor(mock, "test-model")
	facts, err := extractor.ExtractFacts(context.Background(), "用户: 我用表格")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) != 2 {
		t.Errorf("expected 2 facts from fallback, got %d: %v", len(facts), facts)
	}
	if callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// 超长对话
// ---------------------------------------------------------------------------

func TestExtractFacts_LongConversation(t *testing.T) {
	mock := &mockProvider{
		completeFn: func(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			return &agentcore.ProviderResponse{
				Content: `{"facts": ["用户有许多偏好", "对话内容较丰富"]}`,
			}, nil
		},
	}

	extractor := NewProviderExtractor(mock, "test-model")

	// Generate a long conversation (100+ turns).
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("用户: 消息 ")
		sb.WriteByte('A' + byte(i%26))
		sb.WriteString("\n助手: 回复 ")
		sb.WriteByte('a' + byte(i%26))
		sb.WriteString("\n")
	}

	facts, err := extractor.ExtractFacts(context.Background(), sb.String())
	if err != nil {
		t.Fatalf("unexpected error for long conversation: %v", err)
	}
	if len(facts) != 2 {
		t.Errorf("expected 2 facts from long conversation, got %d", len(facts))
	}
}

// ---------------------------------------------------------------------------
// 原有测试
// ---------------------------------------------------------------------------

func TestNewProviderExtractor(t *testing.T) {
	// nil provider is acceptable — just won't be usable, but shouldn't panic.
	e := NewProviderExtractor(nil, "test-model")
	if e == nil {
		t.Fatal("expected non-nil extractor")
	}
	if e.model != "test-model" {
		t.Errorf("model = %q, want %q", e.model, "test-model")
	}

	// Empty model defaults to "default".
	e2 := NewProviderExtractor(nil, "")
	if e2.model != "default" {
		t.Errorf("model = %q, want %q", e2.model, "default")
	}
}
