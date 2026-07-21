package iface

import "context"

// =============================================================================
// LLM Provider 接口 — 轻量级抽象，不依赖 agentcore.ProviderRequest 等具体类型
// =============================================================================

// ChatMessage 是简化的 LLM 对话消息。
type ChatMessage struct {
	Role    string // "system", "user", "assistant", "tool"
	Content string
}

// ChatRequest 是 LLM 调用请求的简化表示。
type ChatRequest struct {
	Model       string
	Messages    []ChatMessage
	Temperature float64
	MaxTokens   int64
}

// ChatResponse 是 LLM 调用响应的简化表示。
type ChatResponse struct {
	Content string
	Model   string
	Usage   *Usage
}

// Usage 是 Token 使用统计。
type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

// ChatProvider 是 LLM 聊天补全的接口。
// 对应 agentcore.Provider，但使用 iface 类型而非 agentcore 类型。
type ChatProvider interface {
	Complete(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
