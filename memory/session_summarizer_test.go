package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestSessionSummarizer_RedactsSensitiveData(t *testing.T) {
	var capturedRequest *agentcore.ProviderRequest
	mock := &mockProvider{
		completeFn: func(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
			capturedRequest = req
			return &agentcore.ProviderResponse{Content: `{"facts": ["用户偏好红色"]}`}, nil
		},
	}

	summarizer := NewSessionSummarizer(mock, "test-model")
	memories := []MemoryEntry{
		{Content: "用户: 我的 api_key=sk-secret-12345"},
		{Content: "助手: 好的，我记下了。"},
	}

	_, err := summarizer.Summarize(context.Background(), memories)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedRequest == nil {
		t.Fatal("expected provider to be called")
	}
	var userMsg string
	for _, msg := range capturedRequest.Messages {
		if msg.Role == agentcore.RoleUser {
			userMsg = msg.Content
		}
		if strings.Contains(msg.Content, "sk-secret-12345") {
			t.Errorf("sensitive api_key found in provider request: %s", msg.Content)
		}
	}
	if !strings.Contains(userMsg, "api_key: ***") {
		t.Errorf("expected api_key to be masked in user message, got: %s", userMsg)
	}
}
