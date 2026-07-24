package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/prompt"
)

// providerExtractor 通过 agentcore.Provider 调用 LLM 从对话中提取原子事实。
// 适配模式参考 domains/reasoning/ 中的 NewLlmClientFromProvider。
//
// FactExtractorLLM 接口签名（接收 conversation 字符串，返回事实列表）与
// agentcore.Provider 接口签名（接收 ProviderRequest，返回 ProviderResponse）不同，
// providerExtractor 负责桥接两者的差异。
type providerExtractor struct {
	provider agentcore.Provider
	model    string
}

// NewProviderExtractor 创建一个基于 agentcore.Provider 的事实提取器。
// provider 和 model 均不能为空。
func NewProviderExtractor(provider agentcore.Provider, model string) *providerExtractor {
	if model == "" {
		model = "default"
	}
	return &providerExtractor{provider: provider, model: model}
}

// ExtractFacts implements FactExtractorLLM.
// 参考 Mem0 的 extraction prompt：从对话中提取原子事实，每条事实独立、完整、
// 可单独理解。优先使用 structured output（JSON Schema），fallback 到文本逐行解析。
func (p *providerExtractor) ExtractFacts(ctx context.Context, conversation string) ([]string, error) {
	if conversation == "" {
		return nil, nil
	}

	conversation = sensitiveDataFilter(conversation)

	req := &agentcore.ProviderRequest{
		Model: p.model,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: prompt.ResolveSystemPromptOr("prompt://memory-fact-extraction", factExtractionSystemPromptFallback)},
			{Role: agentcore.RoleUser, Content: conversation},
		},
		Temperature: 0.1, // 低温度保证提取稳定性
		MaxTokens:   1024,
		ResponseFormat: agentcore.NewJSONSchemaResponseFormat(
			"extracted_facts",
			factExtractionSchema,
		),
	}

	resp, err := p.provider.Complete(ctx, req)
	if err != nil {
		// 如果 structured output 失败，重试不带 schema（fallback）
		return p.extractFallback(ctx, conversation)
	}

	facts, err := parseFactsFromResponse(resp.Content)
	if err != nil {
		return p.extractFallback(ctx, conversation)
	}

	return facts, nil
}

// extractFallback 在 structured output 失败时使用纯文本提取。
// 不要求 JSON 格式，逐行解析非空行作为事实。
func (p *providerExtractor) extractFallback(ctx context.Context, conversation string) ([]string, error) {
	conversation = sensitiveDataFilter(conversation)
	req := &agentcore.ProviderRequest{
		Model: p.model,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: prompt.ResolveSystemPromptOr("prompt://memory-fact-extraction-fallback", factExtractionFallbackPromptFallback)},
			{Role: agentcore.RoleUser, Content: conversation},
		},
		Temperature: 0.1,
		MaxTokens:   1024,
	}

	resp, err := p.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("memory/extractor: both structured and fallback extraction failed: %w", err)
	}

	return parseFactsFromText(resp.Content), nil
}

// ---------------------------------------------------------------------------
// 敏感数据过滤
// ---------------------------------------------------------------------------

var (
	credentialPattern = regexp.MustCompile(`(?i)(password|api[_-]?key|secret|token|bearer)\s*[:=]\s*\S+`)
	jwtPattern        = regexp.MustCompile(`eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`)
)

// sensitiveDataFilter 在对话文本送入 LLM 之前过滤敏感信息。
// 替换密码、API Key、密钥、令牌等凭据，以及 JWT 令牌。
func sensitiveDataFilter(text string) string {
	text = credentialPattern.ReplaceAllString(text, "$1: ***")
	text = jwtPattern.ReplaceAllString(text, "[JWT TOKEN]")
	return text
}

// ---------------------------------------------------------------------------
// Prompt 模板
// ---------------------------------------------------------------------------

// factExtractionSystemPromptFallback 是 memory-fact-extraction 模板未加载时
// 的内联兜底提示词。
const factExtractionSystemPromptFallback = `你是一个记忆提取器。你的任务是从用户与助手的对话中提取值得长期保存的原子事实。

提取原则：
1. 每条事实必须独立、完整、可脱离上下文单独理解
2. 使用第三人称描述（如"用户偏好使用表格展示数据"而非"我喜欢表格"）
3. 优先提取：用户偏好、决策、重要背景信息、领域知识
4. 忽略：寒暄、一次性问答、纯礼貌用语
5. 每条事实一句话即可，简洁明了

请严格按 JSON Schema 返回结果。`

// factExtractionFallbackPromptFallback 是 memory-fact-extraction-fallback 模板
// 未加载时的内联兜底提示词。
const factExtractionFallbackPromptFallback = `你是一个记忆提取器。从以下对话中提取值得长期保存的原子事实。

每条事实一行，使用第三人称描述。忽略寒暄和一次性问答。
只输出事实，不要编号、不要前缀、不要额外解释。如果没有值得提取的事实，输出"无"。

示例输出格式：
用户偏好使用表格展示数据分析结果
用户从事专利代理工作，主要处理机械领域案件
助手建议在答复审查意见时使用三步法策略`

// factExtractionSchema 定义事实提取的 JSON Schema（OpenAI structured output 兼容）。
var factExtractionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"facts": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "string",
			},
			"description": "从对话中提取的原子事实列表。每条事实独立、完整、使用第三人称描述。",
		},
	},
	"required": []string{"facts"},
}

// ---------------------------------------------------------------------------
// 响应解析
// ---------------------------------------------------------------------------

// parseFactsFromResponse 从 structured output 响应中解析事实列表。
func parseFactsFromResponse(content string) ([]string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("memory/extractor: empty response")
	}

	// 去除可能的 markdown 代码块包装
	content = stripMarkdownFences(content)

	var result struct {
		Facts []string `json:"facts"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("memory/extractor: parse structured response: %w", err)
	}

	// 过滤空事实
	var filtered []string
	for _, f := range result.Facts {
		f = strings.TrimSpace(f)
		if f != "" && f != "无" {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}

// parseFactsFromText 从纯文本响应中逐行解析事实。
func parseFactsFromText(content string) []string {
	var facts []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "无" {
			continue
		}
		// 去除可能的编号前缀（"1. "、"- "、"• "等）
		line = strings.TrimLeft(line, "0123456789. -•·*_")
		line = strings.TrimSpace(line)
		if line != "" {
			facts = append(facts, line)
		}
	}
	return facts
}

// stripMarkdownFences 去除 markdown 代码块标记（```json ... ```）。
func stripMarkdownFences(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
