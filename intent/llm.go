package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// LLMClassifier uses an LLM provider for structured intent classification.
// Falls back to keyword classification on error or low confidence.
type LLMClassifier struct {
	Provider        agentcore.Provider
	Model           string
	Threshold       float64
	keywordFallback *KeywordClassifier
}

// NewLLMClassifier creates an LLMClassifier.
func NewLLMClassifier(provider agentcore.Provider) *LLMClassifier {
	return &LLMClassifier{
		Provider:        provider,
		Threshold:       0.7,
		keywordFallback: NewKeywordClassifier(),
	}
}

// Name returns the classifier identifier.
func (c *LLMClassifier) Name() string { return "llm" }

// Classify implements Classifier using LLM structured output.
// Uses a 5-second timeout to prevent indefinite blocking.
func (c *LLMClassifier) Classify(input string) IntentResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := c.classifyWithLLM(ctx, input)
	if err != nil || result.Confidence < c.threshold() {
		return c.keywordFallback.Classify(input)
	}
	result.Sources = append([]string{"llm"}, result.Sources...)
	return result
}

var llmClassificationSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"domain": map[string]any{
			"type":        "string",
			"enum":        []string{"chat", "assistant", "patent", "legal"},
			"description": "分类的目标领域",
		},
		"sub_intent": map[string]any{
			"type":        "string",
			"description": "细分意图（专利领域：invalidation/infringement/novelty/inventiveness/drafting/oa_response/reexamination/fto/enablement/general）",
		},
		"complexity": map[string]any{
			"type":        "string",
			"enum":        []string{"low", "medium", "high"},
			"description": "推理复杂度",
		},
		"confidence": map[string]any{
			"type":    "number",
			"minimum": 0,
			"maximum": 1,
		},
		"reasoning": map[string]any{
			"type": "string",
		},
	},
	"required": []string{"domain", "confidence"},
}

type llmClassificationResult struct {
	Domain     string  `json:"domain"`
	SubIntent  string  `json:"sub_intent"`
	Complexity string  `json:"complexity"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

func (c *LLMClassifier) classifyWithLLM(ctx context.Context, input string) (IntentResult, error) {
	if c.Provider == nil {
		return IntentResult{}, fmt.Errorf("llm classify: provider is nil")
	}

	systemPrompt := strings.Join([]string{
		"你是一个意图分类器。分析用户输入，将其归类到最匹配的领域。",
		"",
		"领域说明：",
		"- chat: 日常聊天、问候、情感交流等纯对话场景",
		"- assistant: 代码生成、文件操作、网页搜索、数据分析等技术任务",
		"- patent: 专利检索、权利要求分析、新颖性比对、专利申请文书",
		"- legal: 法条检索、判例检索、法律分析、合同审查",
		"",
		"细分意图（仅 patent/legal 领域需要）：",
		"- invalidation: 无效宣告",
		"- infringement: 侵权分析",
		"- novelty: 新颖性判断",
		"- inventiveness: 创造性判断",
		"- drafting: 专利撰写",
		"- oa_response: OA答复",
		"- reexamination: 复审请求",
		"- fto: 自由实施分析",
		"- enablement: 充分公开评估",
		"- general: 一般性咨询",
		"",
		"复杂度判断：",
		"- low: 简单问答、问候",
		"- medium: 需要一定分析的任务",
		"- high: 需要深度推理的复杂任务",
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
				Schema: llmClassificationSchema,
				Strict: true,
			},
		},
	}

	resp, err := c.Provider.Complete(ctx, req)
	if err != nil {
		return IntentResult{}, fmt.Errorf("llm classify: %w", err)
	}

	content := resp.Content
	if resp.Structured != nil {
		content = string(resp.Structured)
	}

	var result llmClassificationResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return IntentResult{}, fmt.Errorf("llm classify: parse response: %w", err)
	}

	return llmResultToIntent(result), nil
}

func llmResultToIntent(r llmClassificationResult) IntentResult {
	domain := Domain(r.Domain)
	subIntent := SubIntent(r.SubIntent)
	complexity := parseComplexity(r.Complexity)
	runMode := ModeFlexiblePlan

	// Judgment mode for specific sub-intents
	switch subIntent {
	case SubIntentNovelty, SubIntentInventiveness, SubIntentEnablement:
		runMode = ModeJudgment
	}

	return IntentResult{
		Domain:     domain,
		SubIntent:  subIntent,
		RunMode:    runMode,
		Complexity: complexity,
		Confidence: r.Confidence,
	}
}

func parseComplexity(s string) Complexity {
	switch strings.ToLower(s) {
	case "high":
		return ComplexityHigh
	case "medium":
		return ComplexityMedium
	default:
		return ComplexityLow
	}
}

func (c *LLMClassifier) threshold() float64 {
	if c.Threshold <= 0 {
		return 0.7
	}
	return c.Threshold
}
