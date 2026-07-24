package disclosure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/prompt"
)

// keywordExtractionSystemPromptFallback 是 disclosure-keyword-extraction 模板
// 未加载时的内联兜底提示词。
const keywordExtractionSystemPromptFallback = `你是一个专利检索关键词生成助手。根据技术交底书分析摘要，生成检索关键词。

要求：
- 生成 5-15 个关键词，覆盖技术问题、技术特征、技术效果的核心概念
- 关键词应包含上位概念和下位概念
- 适当包含同义词和近义词以扩大检索覆盖面
- 输出 JSON 格式：{ "keywords": ["关键词1", "关键词2", ...] }
- 每个关键词应当简洁（2-8 个字）
- 避免过于宽泛的常规词汇`

// =============================================================================
// KeywordGenerator — 检索关键词生成器（Phase 2: LLM 混合模式）
// =============================================================================

// KeywordGenerator 从技术交底书中生成检索关键词。
// 支持 LLM 驱动的智能关键词生成，确定性规则提取作为回退。
type KeywordGenerator struct {
	llmProvider agentcore.Provider // 非空时启用 LLM 模式
	model       string             // LLM 模型标识，空则用 provider 默认
	rulesOnly   bool               // 强制只使用规则模式
}

// NewKeywordGenerator 创建关键词生成器。
// provider 非空时启用 LLM 混合模式，为空则纯规则模式。
func NewKeywordGenerator(provider agentcore.Provider) *KeywordGenerator {
	return &KeywordGenerator{
		llmProvider: provider,
	}
}

// Generate 从提取结果生成检索关键词。
// 优先使用 LLM（若配置且未强制规则模式），失败时回退到确定性规则提取。
func (g *KeywordGenerator) Generate(ctx context.Context, ext *ExtractionResult) ([]string, error) {
	if g.llmProvider != nil && !g.rulesOnly {
		keywords, err := g.llmGenerate(ctx, ext)
		if err == nil && len(keywords) > 0 {
			return keywords, nil
		}
		// LLM 失败时静默回退到规则模式
	}
	return g.ruleGenerate(ext), nil
}

// llmGenerate 使用 LLM 调用生成关键词。
func (g *KeywordGenerator) llmGenerate(ctx context.Context, ext *ExtractionResult) ([]string, error) {
	// 构造输入摘要
	var sb strings.Builder
	sb.WriteString("技术交底书分析摘要：\n\n")

	if len(ext.Problems) > 0 {
		sb.WriteString("要解决的技术问题：\n")
		for _, p := range ext.Problems {
			fmt.Fprintf(&sb, "- %s\n", p)
		}
		sb.WriteString("\n")
	}

	if len(ext.Features) > 0 {
		sb.WriteString("技术特征：\n")
		for _, f := range ext.Features {
			fmt.Fprintf(&sb, "- [%s] %s（重要度：%s）\n", f.Category, f.Description, f.Importance)
		}
		sb.WriteString("\n")
	}

	if len(ext.Effects) > 0 {
		sb.WriteString("技术效果：\n")
		for _, e := range ext.Effects {
			fmt.Fprintf(&sb, "- %s\n", e)
		}
	}

	systemPrompt := prompt.ResolveSystemPromptOr("prompt://disclosure-keyword-extraction", keywordExtractionSystemPromptFallback)

	req := &agentcore.ProviderRequest{
		Model: g.model,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: systemPrompt},
			{Role: agentcore.RoleUser, Content: sb.String()},
		},
		MaxTokens:   300,
		Temperature: 0.1,
		ResponseFormat: &agentcore.ResponseFormat{
			Type: agentcore.ResponseFormatJSONSchema,
			JSONSchema: &agentcore.ResponseFormatJSONSchemaConfig{
				Name:   "keyword_generation",
				Schema: keywordSchema(),
				Strict: true,
			},
		},
	}

	resp, err := g.llmProvider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm keyword generate: %w", err)
	}

	content := resp.Content
	if resp.Structured != nil {
		content = string(resp.Structured)
	}

	var result struct {
		Keywords []string `json:"keywords"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("llm keyword parse: %w", err)
	}

	if len(result.Keywords) == 0 {
		return nil, fmt.Errorf("llm keyword generate: empty result")
	}

	return result.Keywords, nil
}

// keywordSchema 返回关键词生成的 JSON Schema。
func keywordSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keywords": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"minItems":    5,
				"maxItems":    15,
				"description": "检索关键词列表，每个词 2-8 个字",
			},
		},
		"required":             []string{"keywords"},
		"additionalProperties": false,
	}
}

// ruleGenerate 使用确定性规则从提取结果中提取关键词。
// 这是 Phase 1 的实现，也是 LLM 失败后的回退路径。
func (g *KeywordGenerator) ruleGenerate(ext *ExtractionResult) []string {
	return collectKeywordsFromExtraction(ext)
}

// =============================================================================
// Pregel 节点工厂
// =============================================================================

// generateKeywordsNodeWithLLM 返回检索关键词生成的 Pregel 节点（LLM 混合模式）。
// provider 非空时启用 LLM 增强的关键词生成，失败时自动回退到规则模式。
func generateKeywordsNodeWithLLM(provider agentcore.Provider) graph.PregelNode {
	return generateKeywordsNodeFromGenerator(provider)
}

// generateKeywordsNodeFromGenerator 内部构造工厂，统一两种模式。
func generateKeywordsNodeFromGenerator(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		gen := NewKeywordGenerator(provider)
		var keywords []string

		// 从提取结果中生成关键词
		if raw, ok := state[StateKeyExtraction]; ok {
			if ext, ok := raw.(*ExtractionResult); ok && ext != nil {
				var err error
				keywords, err = gen.Generate(ctx, ext)
				if err != nil {
					// 生成失败时使用默认关键词，不破坏管线
					keywords = collectKeywordsFromExtraction(ext)
				}
			}
		}

		if len(keywords) == 0 {
			keywords = []string{"技术交底书分析"}
		}

		state[StateKeySearchKeywords] = keywords
		return state, nil
	}
}
