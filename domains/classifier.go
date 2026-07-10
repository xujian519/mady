package domains

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// IntentClassifier classifies user input into a domain name.
// Returns the domain (chat/patent/legal), a confidence score (0.0-1.0),
// and any error that occurred.
type IntentClassifier interface {
	Classify(ctx context.Context, input string) (domain string, confidence float64, err error)
}

// classificationResult is the structured output from the LLM classifier.
type classificationResult struct {
	Domain     string  `json:"domain"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

// classificationSchema is the JSON Schema for structured classification output.
var classificationSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"domain": map[string]any{
			"type":        "string",
			"enum":        []string{"chat", "patent", "legal"},
			"description": "分类的目标领域: chat(通用聊天), patent(专利代理), legal(法律咨询)",
		},
		"confidence": map[string]any{
			"type":        "number",
			"minimum":     0,
			"maximum":     1,
			"description": "分类的置信度, 0.0-1.0",
		},
		"reasoning": map[string]any{
			"type":        "string",
			"description": "分类理由简述",
		},
	},
	"required": []string{"domain", "confidence"},
}

// DefaultClassificationThreshold is the minimum confidence score to accept an
// LLM classification result. Results below this threshold fall back to keyword
// classification.
const DefaultClassificationThreshold = 0.7

// --- KeywordClassifier ---

// KeywordClassifier classifies intent using keyword matching.
// It delegates to the existing ClassifyIntent function.
type KeywordClassifier struct{}

// Classify implements IntentClassifier using keyword matching.
// Confidence is always 1.0 for keyword matches as the rules are deterministic.
func (k *KeywordClassifier) Classify(_ context.Context, input string) (string, float64, error) {
	return ClassifyIntent(input), 1.0, nil
}

// --- LLMClassifier ---

// LLMClassifier uses an LLM provider for intent classification with
// keyword fallback on low confidence or error.
type LLMClassifier struct {
	// Provider is the LLM backend used for classification.
	Provider agentcore.Provider

	// Model is the model identifier for classification. If empty, the
	// provider default is used. A small/fast model (e.g. "gpt-4o-mini")
	// is recommended since classification requires only lightweight reasoning.
	Model string

	// Threshold is the minimum confidence to accept the LLM result.
	// If the LLM returns confidence below this, classification falls
	// back to keyword matching. Defaults to DefaultClassificationThreshold.
	Threshold float64

	// keywordFallback is used when LLM classification fails or has low confidence.
	keywordFallback *KeywordClassifier
}

// NewLLMClassifier creates an LLMClassifier with sensible defaults.
func NewLLMClassifier(provider agentcore.Provider) *LLMClassifier {
	return &LLMClassifier{
		Provider:        provider,
		Threshold:       DefaultClassificationThreshold,
		keywordFallback: &KeywordClassifier{},
	}
}

// Classify implements IntentClassifier using the LLM provider.
//
// Strategy:
//  1. Send a lightweight classification prompt with structured output.
//  2. If confidence >= threshold, return the LLM result.
//  3. Otherwise (or on any error), fall back to keyword classification.
func (c *LLMClassifier) Classify(ctx context.Context, input string) (string, float64, error) {
	domain, confidence, err := c.classifyWithLLM(ctx, input)
	if err != nil || confidence < c.threshold() {
		return c.keywordFallback.Classify(ctx, input)
	}
	return domain, confidence, nil
}

// classifyWithLLM sends a single-turn classification request to the LLM.
func (c *LLMClassifier) classifyWithLLM(ctx context.Context, input string) (string, float64, error) {
	if c.Provider == nil {
		return "", 0, fmt.Errorf("llm classify: provider is nil")
	}
	systemPrompt := strings.Join([]string{
		"你是一个意图分类器。分析用户输入，将其归类到最匹配的领域。",
		"",
		"领域说明：",
		"- chat: 通用聊天、日常问答、代码生成、信息查询、文件操作等非专业领域请求",
		"- patent: 专利检索、权利要求分析、新颖性比对、专利申请文书、现有技术检索等专利代理相关任务",
		"- legal: 法条检索、判例检索、法律分析、法律文书、合同审查、诉讼策略等法律相关任务",
		"",
		"分类规则：",
		"- 涉及专利、发明、权利要求、现有技术等关键词归为 patent",
		"- 涉及法律、法条、判例、诉讼、合同等关键词归为 legal",
		"- 无法明确分类的日常对话、技术问答、编程等归为 chat",
		"- 如果用户输入同时涉及多个领域，选择最核心的领域",
	}, " ")

	req := &agentcore.ProviderRequest{
		Model: c.Model,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: systemPrompt},
			{Role: agentcore.RoleUser, Content: input},
		},
		MaxTokens:   200,
		Temperature: 0,
		ResponseFormat: &agentcore.ResponseFormat{
			Type: agentcore.ResponseFormatJSONSchema,
			JSONSchema: &agentcore.ResponseFormatJSONSchemaConfig{
				Name:   "intent_classification",
				Schema: classificationSchema,
				Strict: true,
			},
		},
	}

	resp, err := c.Provider.Complete(ctx, req)
	if err != nil {
		return "", 0, fmt.Errorf("llm classify: %w", err)
	}

	content := resp.Content
	if resp.Structured != nil {
		content = string(resp.Structured)
	}

	var result classificationResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return "", 0, fmt.Errorf("llm classify: parse response: %w", err)
	}

	// Validate domain.
	switch result.Domain {
	case DomainChat, DomainPatent, DomainLegal:
	default:
		return "", 0, fmt.Errorf("llm classify: unknown domain %q", result.Domain)
	}

	return result.Domain, result.Confidence, nil
}

// threshold returns the configured threshold or the default.
func (c *LLMClassifier) threshold() float64 {
	if c.Threshold <= 0 {
		return DefaultClassificationThreshold
	}
	return c.Threshold
}
