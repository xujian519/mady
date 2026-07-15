package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

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
}

// InMemoryOption 是 InMemoryStore 的函数式配置选项。
type InMemoryOption func(*InMemoryStore)

// WithScoringConfig 设置复合评分参数。
func WithScoringConfig(cfg ScoringConfig) InMemoryOption {
	return func(s *InMemoryStore) { s.scoring = cfg }
}

// WithTokenBudget 设置记忆层的 token 预算。
func WithTokenBudget(budget TokenBudget) InMemoryOption {
	return func(s *InMemoryStore) { s.tokenBudget = budget }
}

// WithClock 注入时间函数（测试用）。
func WithClock(clock func() time.Time) InMemoryOption {
	return func(s *InMemoryStore) { s.now = clock }
}

// WithEmbeddingDimension 设置向量维度（默认 384）。
func WithEmbeddingDimension(dim int) InMemoryOption {
	return func(s *InMemoryStore) { s.dimension = dim }
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

// estimateTokens 粗略估计一段文本的 token 数（4 chars/token）。
func estimateTokens(content string) int64 {
	return int64(len([]rune(content)) / 4)
}

// recencyScore 计算新鲜度分。借鉴 CrewAI 指数衰减公式：
//
//	score = 0.5^(age_in_days / halfLifeDays)
func recencyScore(lastAccess time.Time, now time.Time, halfLifeDays float64) float64 {
	age := now.Sub(lastAccess).Hours() / 24 // 天
	if age <= 0 {
		return 1.0
	}
	return math.Pow(0.5, age/halfLifeDays)
}

// --- 关键词检索（Phase 1 Simple Fallback）---

// tokenize 将文本分词为小写词干列表。
// 对 CJK（中日韩）文字做单字拆分，对拉丁文字按空格/标点拆分。
func tokenize(text string) []string {
	var tokens []string
	var buf strings.Builder

	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}

	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			// CJK 字符：每个字独立作为 token
			flush()
			tokens = append(tokens, string(r))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			buf.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

// keywordScore 计算基于关键词匹配的语义相似度估计（0~1）。
// 使用 TF-like 加权：查询词在命中内容中的占比 + 覆盖度。
func keywordScore(query string, content string) float64 {
	if query == "" || content == "" {
		return 0
	}
	qTokens := tokenize(query)
	if len(qTokens) == 0 {
		return 0
	}
	cTokens := tokenize(content)
	if len(cTokens) == 0 {
		return 0
	}

	// 建 content 词频 map
	cFreq := make(map[string]int, len(cTokens))
	for _, t := range cTokens {
		cFreq[t]++
	}

	// 统计查询词命中数和权值
	hits := 0
	totalWeight := 0.0
	weightSum := 0.0
	for i, qt := range qTokens {
		w := 1.0 + float64(len(qTokens)-i)/float64(len(qTokens)) // 位置权值：越靠前权重越大
		weightSum += w
		if count, ok := cFreq[qt]; ok && count > 0 {
			hits++
			totalWeight += w
		}
	}

	coverage := float64(hits) / float64(len(qTokens)) // 覆盖度
	weightedScore := totalWeight / weightSum          // 加权匹配度
	return (coverage*0.4+weightedScore*0.6)*0.8 + 0.2 // 映射到 0.2~1.0 区间
}

// --- 复合评分 ---

// computeCompositeScore 计算单条记忆的复合评分。
func (s *InMemoryStore) computeCompositeScore(semantic, importance float64, lastAccess time.Time) float64 {
	now := s.nowTime()
	recency := recencyScore(lastAccess, now, s.scoring.RecencyHalfLife)

	return s.scoring.SemanticWeight*semantic +
		s.scoring.RecencyWeight*recency +
		s.scoring.ImportanceWeight*importance
}

// --- 机器生成的重要性估计（无 LLM 时使用）---

// estimateImportance 基于内容特征估算重要性（0~1）。
// 当 LLM 提取器不可用时作为 fallback。
func estimateImportance(content string) float64 {
	if content == "" {
		return 0
	}

	score := 0.3 // 基础分

	// 长度因子：过短的内容不太重要
	runes := []rune(content)
	lenScore := float64(len(runes)) / 500.0
	if lenScore > 0.4 {
		lenScore = 0.4
	}
	score += lenScore

	// 关键词启发：含有决策/事实等关键词的重要性更高
	importanceKeywords := []string{"决定", "决策", "重要", "关键", "必须", "一定要",
		"prefer", "favorite", "important", "critical", "decision",
		"like", "dislike", "want", "need", "记住", "记住我"}
	lower := strings.ToLower(content)
	for _, kw := range importanceKeywords {
		if strings.Contains(lower, kw) {
			score += 0.05
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
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
	s.mu.Unlock()

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
func (s *InMemoryStore) Get(ctx context.Context, id string) (*MemoryEntry, error) {
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
func (s *InMemoryStore) Update(ctx context.Context, id string, content string) error {
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
func (s *InMemoryStore) Forget(ctx context.Context, id string) error {
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
func (s *InMemoryStore) ForgetAll(ctx context.Context, filter MemoryFilter) error {
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
func (s *InMemoryStore) List(ctx context.Context, layer MemoryLayer, opts ListOptions) ([]MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	set, ok := s.byLayer[layer]
	if !ok {
		return nil, fmt.Errorf("memory: invalid layer %q", layer)
	}

	entries := make([]MemoryEntry, 0, len(set))
	for id := range set {
		if e, ok := s.entries[id]; ok {
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
func (s *InMemoryStore) Prune(ctx context.Context, layer MemoryLayer, threshold float64) (int64, error) {
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
func (s *InMemoryStore) Stats(ctx context.Context) MemoryStats {
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
