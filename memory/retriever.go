package memory

import (
	"context"
	"math"
	"time"
)

// Retriever 提供混合检索引擎。
// 组合关键词检索 + 向量检索（Phase 2）+ 复合评分（CrewAI 公式）。
type Retriever struct {
	cfg RetrieverConfig
}

// RetrieverConfig 控制检索行为。
type RetrieverConfig struct {
	// MinSemanticScore 关键词检索的最低语义分（低于此值的结果被过滤）。
	MinSemanticScore float64 `json:"min_semantic_score"`

	// WeightSemantic 语义相似度在复合评分中的权重（同 ScoringConfig.SemanticWeight）。
	WeightSemantic float64 `json:"weight_semantic"`

	// WeightRecency 新鲜度在复合评分中的权重。
	WeightRecency float64 `json:"weight_recency"`

	// WeightImportance 重要性在复合评分中的权重。
	WeightImportance float64 `json:"weight_importance"`

	// RecencyHalfLife 新鲜度半衰期（天）。
	RecencyHalfLife float64 `json:"recency_half_life"`
}

// DefaultRetrieverConfig 返回默认检索配置。
func DefaultRetrieverConfig() RetrieverConfig {
	return RetrieverConfig{
		MinSemanticScore: 0.25,
		WeightSemantic:   0.5,
		WeightRecency:    0.3,
		WeightImportance: 0.2,
		RecencyHalfLife:  30.0,
	}
}

// NewRetriever 创建检索器。
func NewRetriever(cfg RetrieverConfig) *Retriever {
	return &Retriever{cfg: cfg}
}

// Search 执行混合检索。
// 使用 Rule-based 关键词匹配（Phase 1）+ 复合评分排序。
func (r *Retriever) Search(ctx context.Context, store MemoryStore, query string, filter MemoryFilter) ([]ScoredMemory, error) {
	if query == "" {
		return nil, nil
	}
	return store.Recall(ctx, query, filter)
}

// Score 计算单条记忆的复合评分。
// 公式：w1 × semantic + w2 × recency_decay + w3 × importance
func (r *Retriever) Score(semantic, importance float64, lastAccess time.Time, now time.Time) float64 {
	recency := recencyScore(lastAccess, now, r.cfg.RecencyHalfLife)
	return r.cfg.WeightSemantic*semantic +
		r.cfg.WeightRecency*recency +
		r.cfg.WeightImportance*importance
}

// EstimateBudgetTokens 估算指定条数记忆在上下文中的 token 消耗。
func (r *Retriever) EstimateBudgetTokens(results []ScoredMemory) int64 {
	var total int64
	for _, sr := range results {
		total += int64(math.Ceil(float64(len([]rune(sr.Entry.Content))) / 4.0))
	}
	return total
}
