package memory

import (
	"context"
)

// ExtractedFact 是从对话中提取的单条原子事实。
type ExtractedFact struct {
	Content    string         `json:"content"`            // 事实内容
	Scope      MemoryScope    `json:"scope"`              // 作用域
	Layer      MemoryLayer    `json:"layer"`              // 推荐存放层级
	Importance float64        `json:"importance"`         // 重要性 (0~1)
	Metadata   map[string]any `json:"metadata,omitempty"` // 来源信息等
}

// Extractor 从对话中提取结构化记忆。
// 借鉴 Mem0 的 LLM 提取管线：原始文本 → 原子事实 → 去重 → 冲突解决。
type Extractor struct {
	llm FactExtractorLLM // LLM 接口，为 nil 时使用规则提取
	cfg ExtractorConfig
}

// FactExtractorLLM 是提取器需要的 LLM 接口。
// 在生产环境中接入 agentcore.Provider 或任何 LLM 服务。
type FactExtractorLLM interface {
	ExtractFacts(ctx context.Context, conversation string) ([]string, error)
}

// ExtractorConfig 控制提取行为。
type ExtractorConfig struct {
	// MaxFacts 每次提取的最大事实数 (default: 5)
	MaxFacts int `json:"max_facts"`
	// MinContentLen 最小内容长度（过短的内容不提取）
	MinContentLen int `json:"min_content_len"`
	// EnableConflictResolution 是否启用冲突解决
	EnableConflictResolution bool `json:"enable_conflict_resolution"`
}

// DefaultExtractorConfig 返回默认配置。
func DefaultExtractorConfig() ExtractorConfig {
	return ExtractorConfig{
		MaxFacts:                 5,
		MinContentLen:            10,
		EnableConflictResolution: false, // Phase 1 关闭
	}
}

// NewExtractor 创建提取器。
// llm 为 nil 时使用基于规则的简易提取（仅关键词匹配）。
func NewExtractor(llm FactExtractorLLM, cfg ExtractorConfig) *Extractor {
	return &Extractor{
		llm: llm,
		cfg: cfg,
	}
}

// Extract 从对话内容中提取记忆事实。
// 有 LLM 时使用 LLM 提取，否则使用规则回退。
func (e *Extractor) Extract(ctx context.Context, conversation string, scope MemoryScope) ([]ExtractedFact, error) {
	if e.llm != nil {
		return e.extractWithLLM(ctx, conversation, scope)
	}
	return e.extractWithRules(conversation, scope), nil
}

func (e *Extractor) extractWithLLM(ctx context.Context, conversation string, scope MemoryScope) ([]ExtractedFact, error) {
	texts, err := e.llm.ExtractFacts(ctx, conversation)
	if err != nil {
		return nil, err
	}

	var facts []ExtractedFact
	for _, text := range texts {
		if len([]rune(text)) < e.cfg.MinContentLen {
			continue
		}
		facts = append(facts, ExtractedFact{
			Content:    text,
			Scope:      scope,
			Layer:      LayerLongTerm,
			Importance: estimateImportance(text),
			Metadata:   map[string]any{"source": "llm_extraction"},
		})
		if len(facts) >= e.cfg.MaxFacts {
			break
		}
	}
	return facts, nil
}

func (e *Extractor) extractWithRules(_ string, _ MemoryScope) []ExtractedFact {
	// Phase 1 简易规则提取：按换行拆分 + 关键句子
	// 在实际使用中，建议配置 LLM 以获得更好的提取质量。
	return nil // 返回空，由 Manager 使用原文保存 fallback
}
