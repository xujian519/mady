package claimdrafting

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ProviderAdapter 将 agentcore.Provider 适配为 claimdrafting.Provider 接口。
//
// claimdrafting 的 LLMDrafter 使用简化的 Complete(prompt) → (string, error) 接口，
// 而运行时注入的是 agentcore.Provider（完整的请求/响应模型）。
// 本 adapter 桥接两者，使 LLMDrafter 能复用全局的 LLM provider。
type ProviderAdapter struct {
	provider agentcore.Provider
	model    string
}

// NewProviderAdapter 创建一个基于 agentcore.Provider 的适配器。
// model 为空时由 provider 侧使用默认模型。
func NewProviderAdapter(provider agentcore.Provider, model string) *ProviderAdapter {
	return &ProviderAdapter{provider: provider, model: model}
}

// Available 实现 claimdrafting.Provider 接口。
func (a *ProviderAdapter) Available() bool {
	return a != nil && a.provider != nil
}

// Complete 发送单轮 prompt 并返回文本结果，实现 claimdrafting.Provider 接口。
func (a *ProviderAdapter) Complete(prompt string) (string, error) {
	resp, err := a.provider.Complete(context.Background(), &agentcore.ProviderRequest{
		Model:    a.model,
		Messages: []agentcore.Message{{Role: agentcore.RoleUser, Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
