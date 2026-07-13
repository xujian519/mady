package reasoning

import (
	"context"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// providerLlmClient adapts an agentcore.Provider to the LlmClient interface
// used by the EnhancedSyllogismChecker for Level 2 (logical consistency) and
// Level 3 (evidentiary sufficiency) validation.
type providerLlmClient struct {
	provider agentcore.Provider
	model    string
}

func (c *providerLlmClient) Chat(ctx context.Context, messages []LlmMessage) (string, error) {
	msgs := make([]agentcore.Message, len(messages))
	for i, m := range messages {
		role := agentcore.RoleUser
		switch m.Role {
		case "system":
			role = agentcore.RoleSystem
		case "assistant":
			role = agentcore.RoleAssistant
		}
		msgs[i] = agentcore.Message{Role: role, Content: m.Content}
	}

	resp, err := c.provider.Complete(ctx, &agentcore.ProviderRequest{
		Model:    c.model,
		Messages: msgs,
	})
	if err != nil {
		return "", err
	}

	// ProviderResponse.Content is the concatenated text from all text blocks.
	if resp.Content != "" {
		return resp.Content, nil
	}
	// Fall back to assembling from Blocks.
	var sb strings.Builder
	for _, block := range resp.Blocks {
		if block.Kind == agentcore.BlockKindText || block.Kind == agentcore.BlockKindThinking {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

// NewLlmClientFromProvider wraps an agentcore.Provider as an LlmClient.
// The model parameter identifies which model the provider should use for
// validation calls (typically the same model used by the main agent).
// Returns nil if p is nil — callers should check the return value.
//
// Usage:
//
//	var p agentcore.Provider = ...
//	llm := reasoning.NewLlmClientFromProvider(p, "deepseek-v4-pro")
//	runner := reasoning.NewWorkflowRunner(..., llm)
func NewLlmClientFromProvider(p agentcore.Provider, model string) LlmClient {
	if p == nil {
		return nil
	}
	return &providerLlmClient{provider: p, model: model}
}
