package memory

import (
	"context"
	"math"
	"time"
)

// Retriever 提供混合检索引擎。
// 组合关键词检索 + 向量检索（Phase 2）+ BM25 稀疏检索（Phase 5）+ RRF 融合 + 复合评分。
type Retriever struct {
	cfg       RetrieverConfig
	bm25Index *BM25Index // BM25 索引，为 nil 时跳过稀疏检索
	rrfCfg    RRFConfig
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

	// EnableHybrid 启用混合检索（稠密向量 + BM25 稀疏 + RRF 融合）。
	// 当 BM25 索引不可用时自动退化为纯稠密检索。
	EnableHybrid bool `json:"enable_hybrid"`
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
	return &Retriever{
		cfg:    cfg,
		rrfCfg: DefaultRRFConfig(),
	}
}

// SetBM25Index 设置 BM25 稀疏检索引擎。
// 为 nil 时 HybridSearch 退化为纯稠密检索。
func (r *Retriever) SetBM25Index(idx *BM25Index) {
	r.bm25Index = idx
}

// Search 执行检索。
// 当 EnableHybrid 为 true 且 BM25 索引可用时，使用混合检索（稠密+稀疏+RRF）。
// 否则退化为纯稠密检索。
func (r *Retriever) Search(ctx context.Context, store MemoryStore, query string, filter MemoryFilter) ([]ScoredMemory, error) {
	if query == "" {
		return nil, nil
	}

	if r.cfg.EnableHybrid && r.bm25Index != nil && r.bm25Index.Size() > 0 {
		return r.HybridSearch(ctx, store, query, filter)
	}

	return store.Recall(ctx, query, filter)
}

// HybridSearch 执行混合检索：稠密向量 + BM25 稀疏 + RRF 融合。
//
// 流程：
//  1. 稠密检索（向量相似度 / 关键词匹配）→ 获取 top-K*3 候选
//  2. BM25 稀疏检索 → 获取 top-K*3 候选
//  3. RRF 融合双方排名
//  4. 对融合结果应用复合评分排序，截取 top-K
func (r *Retriever) HybridSearch(ctx context.Context, store MemoryStore, query string, filter MemoryFilter) ([]ScoredMemory, error) {
	topK := filter.EffectiveTopK()
	candidateK := topK * 3 // 每路取更多候选供 RRF 融合

	// 1. 稠密检索
	denseFilter := filter
	denseFilter.TopK = candidateK
	denseResults, denseErr := store.Recall(ctx, query, denseFilter)
	if denseErr != nil {
		// 稠密检索失败时退化为纯 BM25
		denseResults = nil
	}

	// 2. BM25 稀疏检索
	// 注：当前逐条 Get() 查询，候选数 ≤30 时可接受。
	// 未来可优化为批量查询或直接从预加载的候选集中匹配。
	var sparseResults []ScoredMemory
	if r.bm25Index != nil {
		bm25Hits := r.bm25Index.Search(query, candidateK)
		// 将 BM25 结果转换为 ScoredMemory（需要从 store 获取 MemoryEntry）
		for _, hit := range bm25Hits {
			entry, err := store.Get(ctx, hit.EntryID)
			if err != nil {
				continue
			}
			// 检查是否匹配 filter
			if !matchFilter(entry, filter) {
				continue
			}
			sparseResults = append(sparseResults, ScoredMemory{
				Entry:    *entry,
				Semantic: hit.Score, // BM25 分数放在 Semantic 字段
			})
		}
	}

	// 3. 若只有稠密结果，直接返回
	if len(sparseResults) == 0 {
		if len(denseResults) > topK {
			denseResults = denseResults[:topK]
		}
		for i := range denseResults {
			denseResults[i].Rank = i
		}
		return denseResults, nil
	}

	// 4. RRF 融合
	fused := RRFFusion(denseResults, sparseResults, r.rrfCfg)
	if len(fused) > topK {
		fused = fused[:topK]
	}

	return fused, nil
}

// matchEntryFilter delegates to store.go's matchFilter to avoid duplication.

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
