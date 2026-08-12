package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/retrieval"
)

// InMemoryStore 是 MemoryStore 的纯内存实现。
// 使用 sync.RWMutex 保证线程安全，支持简单的关键词检索 + 复合评分。
// Phase 1 实现：无外部依赖，适合开发/测试和中小规模部署。
type InMemoryStore struct {
	mu      sync.RWMutex
	entries map[string]*MemoryEntry
	byLayer map[MemoryLayer]map[string]struct{} // layer → entryID set

	scoring     ScoringConfig
	tokenBudget TokenBudget
	dimension   int // embedding 维度
	embedder    retrieval.Embedder
	now         func() time.Time

	// capacityWarned 记录已告警的最高阈值，确保每个阈值只告警一次。
	capacityWarned atomic.Int64
}

// InMemoryOption 是 InMemoryStore 的函数式配置选项。
type InMemoryOption func(*InMemoryStore)

// WithClock 注入时间函数（测试用）。
func WithClock(clock func() time.Time) InMemoryOption {
	return func(s *InMemoryStore) { s.now = clock }
}

// WithEmbedder 注入向量编码器，启用语义检索。
// 当 embedder 非 nil 时，Remember/RememberBatch 自动生成 embedding，
// Recall 使用向量相似度替代关键词匹配。
func WithEmbedder(emb retrieval.Embedder) InMemoryOption {
	return func(s *InMemoryStore) { s.embedder = emb }
}

// NewInMemoryStore 创建一个新的内存记忆存储。
func NewInMemoryStore(opts ...InMemoryOption) *InMemoryStore {
	s := &InMemoryStore{
		entries:     make(map[string]*MemoryEntry),
		byLayer:     make(map[MemoryLayer]map[string]struct{}),
		scoring:     DefaultScoringConfig(),
		tokenBudget: DefaultMemoryTokenBudget(),
		now:         time.Now,
		dimension:   384,
	}
	for _, opt := range opts {
		opt(s)
	}
	// 初始化各层索引
	for _, l := range ValidLayers() {
		if s.byLayer[l] == nil {
			s.byLayer[l] = make(map[string]struct{})
		}
	}
	return s
}

// --- ID 生成 ---

var idCounter atomic.Int64

func nextMemoryID() string {
	n := idCounter.Add(1)
	return fmt.Sprintf("mem_%d_%d", time.Now().UnixMilli(), n)
}

// --- 工具函数 ---

func (s *InMemoryStore) nowTime() time.Time {
	return s.now()
}

// --- MemoryStore 接口实现 ---

// Remember 存入一条记忆。
func (s *InMemoryStore) Remember(ctx context.Context, content string, scope MemoryScope, layer MemoryLayer, metadata map[string]any) (string, error) {
	if content == "" {
		return "", fmt.Errorf("memory: content is empty")
	}
	if !layer.IsValid() {
		return "", fmt.Errorf("memory: invalid layer %q", layer)
	}

	id := nextMemoryID()
	now := s.nowTime()

	entry := &MemoryEntry{
		ID:          id,
		Scope:       scope,
		Layer:       layer,
		Content:     content,
		Importance:  estimateImportance(content),
		AccessCount: 0,
		CreatedAt:   now,
		UpdatedAt:   now,
		LastAccess:  now,
		DecayFactor: 0.95,
		Metadata:    metadata,
	}

	if s.embedder != nil {
		if vecs, err := s.embedder.Embed(ctx, []string{content}); err == nil && len(vecs) > 0 {
			entry.Embedding = vecs[0]
		}
	}

	s.mu.Lock()
	s.entries[id] = entry
	s.byLayer[layer][id] = struct{}{}
	entryCount := len(s.entries)
	s.mu.Unlock()

	// 容量监控：超过阈值时发出告警（仅一次）。
	s.maybeWarnCapacity(entryCount)

	return id, nil
}

// RememberBatch 批量存入。
func (s *InMemoryStore) RememberBatch(ctx context.Context, entries []MemoryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	now := s.nowTime()

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range entries {
		e := &entries[i]
		if e.ID == "" {
			e.ID = nextMemoryID()
		}
		if e.CreatedAt.IsZero() {
			e.CreatedAt = now
		}
		if e.UpdatedAt.IsZero() {
			e.UpdatedAt = now
		}
		if e.LastAccess.IsZero() {
			e.LastAccess = now
		}
		if e.DecayFactor == 0 {
			e.DecayFactor = 0.95
		}
		if len(e.Embedding) == 0 && s.embedder != nil {
			if vecs, err := s.embedder.Embed(ctx, []string{e.Content}); err == nil && len(vecs) > 0 {
				e.Embedding = vecs[0]
			}
		}
		s.entries[e.ID] = &entries[i]
		if e.Layer.IsValid() {
			s.byLayer[e.Layer][e.ID] = struct{}{}
		}
	}
	return nil
}

// Recall 按语义检索记忆，返回按复合评分降序排列的结果。
// 当配置了 embedder 时使用向量相似度，否则退化为关键词匹配。
func (s *InMemoryStore) Recall(ctx context.Context, query string, filter MemoryFilter) ([]ScoredMemory, error) {
	s.mu.RLock()

	candidates := s.collectCandidates(filter)
	if len(candidates) == 0 {
		s.mu.RUnlock()
		return nil, nil
	}

	now := s.nowTime()

	var queryVec []float32
	if s.embedder != nil {
		if vecs, err := s.embedder.Embed(ctx, []string{query}); err == nil && len(vecs) > 0 {
			queryVec = vecs[0]
		}
	}

	scored := make([]ScoredMemory, 0, len(candidates))
	for _, entry := range candidates {
		var semantic float64
		if queryVec != nil && len(entry.Embedding) > 0 {
			semantic = retrieval.CosineSimilarity(queryVec, entry.Embedding)
			if semantic < 0 {
				semantic = 0
			}
		} else {
			semantic = keywordScore(query, entry.Content)
		}
		if semantic < 0.25 {
			continue
		}
		composite := s.computeCompositeScore(semantic, entry.Importance, entry.LastAccess)
		scored = append(scored, ScoredMemory{
			Entry:      entry.Clone(),
			Semantic:   semantic,
			Recency:    recencyScore(entry.LastAccess, now, s.scoring.RecencyHalfLife),
			Importance: entry.Importance,
			Composite:  composite,
		})
	}
	s.mu.RUnlock()

	sortScoredByComposite(scored)

	// 取 TopK
	topK := filter.EffectiveTopK()
	if len(scored) > topK {
		scored = scored[:topK]
	}
	for i := range scored {
		scored[i].Rank = i
	}

	// 在写锁下更新访问统计
	s.mu.Lock()
	for i := range scored {
		entryID := scored[i].Entry.ID
		if e, ok := s.entries[entryID]; ok {
			e.LastAccess = now
			e.AccessCount++
		}
	}
	s.mu.Unlock()

	return scored, nil
}

// RecallWithBudget 在 token 预算约束下检索。
func (s *InMemoryStore) RecallWithBudget(ctx context.Context, query string, filter MemoryFilter, maxTokens int64) ([]ScoredMemory, error) {
	results, err := s.Recall(ctx, query, filter)
	if err != nil {
		return nil, err
	}

	var filtered []ScoredMemory
	tokensUsed := int64(0)
	for _, r := range results {
		t := estimateTokens(r.Entry.Content)
		if tokensUsed+t > maxTokens {
			continue // 超出预算，跳过
		}
		tokensUsed += t
		filtered = append(filtered, r)
	}
	return filtered, nil
}

// Get 按 ID 获取单条记忆。
func (s *InMemoryStore) Get(_ context.Context, id string) (*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.entries[id]
	if !ok {
		return nil, fmt.Errorf("memory: entry %q not found", id)
	}
	clone := e.Clone()
	return &clone, nil
}

// Update 更新记忆内容。
func (s *InMemoryStore) Update(_ context.Context, id string, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return fmt.Errorf("memory: entry %q not found", id)
	}
	e.Content = content
	e.UpdatedAt = s.nowTime()
	return nil
}

// Forget 按 ID 删除。
func (s *InMemoryStore) Forget(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return fmt.Errorf("memory: entry %q not found", id)
	}
	delete(s.entries, id)
	if e.Layer.IsValid() {
		delete(s.byLayer[e.Layer], id)
	}
	return nil
}

// ForgetAll 按过滤条件批量删除。
func (s *InMemoryStore) ForgetAll(_ context.Context, filter MemoryFilter) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, e := range s.entries {
		if matchFilter(e, filter) {
			delete(s.entries, id)
			if e.Layer.IsValid() {
				delete(s.byLayer[e.Layer], id)
			}
		}
	}
	return nil
}

// List 按层分页列出记忆。
func (s *InMemoryStore) List(_ context.Context, layer MemoryLayer, opts ListOptions) ([]MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	set, ok := s.byLayer[layer]
	if !ok {
		return nil, fmt.Errorf("memory: invalid layer %q", layer)
	}

	entries := make([]MemoryEntry, 0, len(set))
	for id := range set {
		if e, ok := s.entries[id]; ok {
			// UserID scope 过滤（存储层完成，避免调用方客户端过滤）
			if opts.UserID != "" && e.Scope.UserID != opts.UserID {
				continue
			}
			entries = append(entries, e.Clone())
		}
	}

	// 按创建时间排序
	if opts.Asc {
		sortMemoryByCreatedAt(entries, true)
	} else {
		sortMemoryByCreatedAt(entries, false)
	}

	// 分页
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := opts.Offset
	if offset >= len(entries) {
		return nil, nil
	}
	end := min(offset+limit, len(entries))
	return entries[offset:end], nil
}

// Prune 清理低衰减/低重要性记忆。
func (s *InMemoryStore) Prune(_ context.Context, layer MemoryLayer, threshold float64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	set, ok := s.byLayer[layer]
	if !ok {
		return 0, fmt.Errorf("memory: invalid layer %q", layer)
	}

	now := s.nowTime()
	removed := int64(0)

	for id := range set {
		e, ok := s.entries[id]
		if !ok {
			continue
		}
		// 计算衰减后的复合评分
		recency := recencyScore(e.LastAccess, now, s.scoring.RecencyHalfLife)
		score := s.scoring.RecencyWeight*recency + s.scoring.ImportanceWeight*e.Importance

		if score < threshold {
			delete(s.entries, id)
			delete(set, id)
			removed++
		}
	}
	return removed, nil
}

// Stats 返回统计信息。
func (s *InMemoryStore) Stats(_ context.Context) MemoryStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stats MemoryStats
	stats.TotalEntries = int64(len(s.entries))
	for _, e := range s.entries {
		switch e.Layer {
		case LayerUser:
			stats.UserCount++
		case LayerSession:
			stats.SessionCount++
		case LayerLongTerm:
			stats.LongTermCnt++
		}
	}
	return stats
}

// Close 释放所有资源。
func (s *InMemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
	s.byLayer = make(map[MemoryLayer]map[string]struct{})
	return nil
}

// --- 内部辅助函数 ---

func (s *InMemoryStore) collectCandidates(filter MemoryFilter) []*MemoryEntry {
	var candidates []*MemoryEntry
	for _, e := range s.entries {
		if matchFilter(e, filter) {
			candidates = append(candidates, e)
		}
	}
	return candidates
}

func matchFilter(e *MemoryEntry, filter MemoryFilter) bool {
	if filter.UserID != "" && e.Scope.UserID != filter.UserID {
		return false
	}
	if filter.AgentID != "" && e.Scope.AgentID != filter.AgentID {
		return false
	}
	if filter.SessionID != "" && e.Scope.SessionID != filter.SessionID {
		return false
	}
	if filter.ProjectID != "" && e.Scope.ProjectID != filter.ProjectID {
		return false
	}
	if filter.Layer != "" && e.Layer != filter.Layer {
		return false
	}
	return true
}

// --- 排序辅助 ---

// sortScoredByComposite 按复合评分降序排列。
func sortScoredByComposite(s []ScoredMemory) {
	sort.Slice(s, func(i, j int) bool {
		return s[i].Composite > s[j].Composite
	})
}

// sortMemoryByCreatedAt 按创建时间排序。
func sortMemoryByCreatedAt(s []MemoryEntry, asc bool) {
	sort.Slice(s, func(i, j int) bool {
		if asc {
			return s[i].CreatedAt.Before(s[j].CreatedAt)
		}
		return s[i].CreatedAt.After(s[j].CreatedAt)
	})
}

// 编译时检查
var _ MemoryStore = (*InMemoryStore)(nil)

// capacityWarnThresholds 定义容量告警阈值（条目数）。
var capacityWarnThresholds = []int{1000, 5000, 10000, 50000}

// maybeWarnCapacity 在存储容量超过阈值时输出告警日志。
// 每个阈值只告警一次：capacityWarned 记录已告警的最高阈值，
// 后续只有更高阈值被超过时才会再次告警。
func (s *InMemoryStore) maybeWarnCapacity(entryCount int) {
	warned := int(s.capacityWarned.Load())
	for _, threshold := range capacityWarnThresholds {
		if entryCount >= threshold && threshold > warned {
			s.capacityWarned.Store(int64(threshold))
			slog.Warn("InMemoryStore: capacity threshold reached",
				"entries", entryCount,
				"threshold", threshold,
			)
			return
		}
	}
}
