package memory

import (
	"context"
	"fmt"
	"time"
)

// Manager 是记忆系统的核心协调器。
// 封装 MemoryStore，提供高层 API（从对话中提取记忆、智能检索、周期性衰减）。
type Manager struct {
	store     MemoryStore
	extractor *Extractor // 可为 nil（不使用 LLM 提取时）
	retriever *Retriever // 混合检索引擎
	clock     func() time.Time

	// 配置
	cfg ManagerConfig
}

// ManagerConfig 控制 Manager 的行为。
type ManagerConfig struct {
	// AutoExtract 开启时，每次 RememberFromTurn 自动调 LLM 提取事实。
	AutoExtract bool `json:"auto_extract"`

	// PruneInterval 定期清理间隔。0 = 禁用自动清理。
	PruneInterval time.Duration `json:"prune_interval"`

	// PruneThreshold 衰减清理阈值（低于此值的记忆被清理）。
	PruneThreshold float64 `json:"prune_threshold"`

	// DefaultTopK 不指定时的默认检索数量。
	DefaultTopK int `json:"default_top_k"`

	// MemoryBudgetRatio 记忆占总上下文的默认比例（0~1）。
	MemoryBudgetRatio float64 `json:"memory_budget_ratio"`
}

// DefaultManagerConfig 返回默认配置。
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		AutoExtract:       false, // Phase 1 默认关闭
		PruneInterval:     0,     // 默认不自动清理
		PruneThreshold:    0.15,
		DefaultTopK:       5,
		MemoryBudgetRatio: 0.15,
	}
}

// NewManager 创建新的记忆管理器。
//
// store 参数不能为 nil；extractor 和 retriever 可为 nil（retriever 为 nil 时将自动创建）。
func NewManager(store MemoryStore, extractor *Extractor, retriever *Retriever, cfg ManagerConfig) *Manager {
	if store == nil {
		store = NewInMemoryStore()
	}
	if retriever == nil {
		retriever = NewRetriever(DefaultRetrieverConfig())
	}

	return &Manager{
		store:     store,
		extractor: extractor,
		retriever: retriever,
		clock:     time.Now,
		cfg:       cfg,
	}
}

// Store 返回底层 MemoryStore。
func (m *Manager) Store() MemoryStore { return m.store }

// WithClock 设置时钟（测试用）。
func (m *Manager) WithClock(clock func() time.Time) *Manager {
	m.clock = clock
	return m
}

// ---------------------------------------------------------------------------
// Write Operations
// ---------------------------------------------------------------------------

// Remember 直接存入一条记忆。
func (m *Manager) Remember(ctx context.Context, content string, scope MemoryScope, layer MemoryLayer, metadata map[string]any) (string, error) {
	return m.store.Remember(ctx, content, scope, layer, metadata)
}

// RememberBatch 批量存入。
func (m *Manager) RememberBatch(ctx context.Context, entries []MemoryEntry) error {
	return m.store.RememberBatch(ctx, entries)
}

// RememberFromTurn 从一轮对话中提取并保存记忆。
// 如果 ManagerConfig.AutoExtract 为 true 且 extractor 不为 nil，
// 将使用 LLM 提取原子事实。否则将对话轮次原文作为一条记忆保存。
func (m *Manager) RememberFromTurn(ctx context.Context, userInput, assistantOutput string, scope MemoryScope) ([]string, error) {
	if userInput == "" && assistantOutput == "" {
		return nil, nil
	}

	var ids []string

	if m.cfg.AutoExtract && m.extractor != nil {
		// LLM 提取
		content := fmt.Sprintf("用户说: %s\n助手回答: %s", userInput, assistantOutput)
		facts, err := m.extractor.Extract(ctx, content, scope)
		if err != nil {
			// LLM 提取失败时 fallback 到原文保存
			id, fallbackErr := m.store.Remember(ctx, content, scope, LayerSession, map[string]any{"type": "fallback"})
			if fallbackErr == nil {
				ids = append(ids, id)
			}
			return ids, fmt.Errorf("memory: extract failed (fallback used): %w", err)
		}
		for _, fact := range facts {
			id, err := m.store.Remember(ctx, fact.Content, scope, LayerLongTerm, fact.Metadata)
			if err != nil {
				continue
			}
			ids = append(ids, id)
		}
	} else {
		// 原文保存到 Session 层
		var b string
		if userInput != "" {
			b = "用户: " + userInput
		}
		if assistantOutput != "" {
			if b != "" {
				b += "\n"
			}
			b += "助手: " + assistantOutput
		}
		id, err := m.store.Remember(ctx, b, scope, LayerSession, nil)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// ---------------------------------------------------------------------------
// Read Operations
// ---------------------------------------------------------------------------

// Search 检索与查询相关的记忆，返回按复合评分降序的结果。
func (m *Manager) Search(ctx context.Context, query string, filter MemoryFilter) ([]ScoredMemory, error) {
	if m.cfg.DefaultTopK > 0 && filter.TopK <= 0 {
		filter.TopK = m.cfg.DefaultTopK
	}
	return m.retriever.Search(ctx, m.store, query, filter)
}

// Get 按 ID 获取单条记忆。
func (m *Manager) Get(ctx context.Context, id string) (*MemoryEntry, error) {
	return m.store.Get(ctx, id)
}

// SearchAllLayers 跨所有层检索记忆。
func (m *Manager) SearchAllLayers(ctx context.Context, query string, topK int) ([]ScoredMemory, error) {
	if topK <= 0 {
		topK = m.cfg.DefaultTopK
	}
	perLayer := max(topK/3, 1)
	var all []ScoredMemory

	for _, layer := range ValidLayers() {
		results, err := m.retriever.Search(ctx, m.store, query, MemoryFilter{
			UserID:    "",
			SessionID: "",
			Layer:     layer,
			TopK:      perLayer,
		})
		if err != nil {
			continue
		}
		all = append(all, results...)
	}

	// 全局排序取 topK
	sortScoredByComposite(all)
	if len(all) > topK {
		all = all[:topK]
	}
	for i := range all {
		all[i].Rank = i
	}
	return all, nil
}

// SearchWithBudget 在 token 预算下检索记忆。
func (m *Manager) SearchWithBudget(ctx context.Context, query string, filter MemoryFilter, maxTokens int64) ([]ScoredMemory, error) {
	return m.store.RecallWithBudget(ctx, query, filter, maxTokens)
}

// ---------------------------------------------------------------------------
// Delete Operations
// ---------------------------------------------------------------------------

// Forget 删除单条记忆。
func (m *Manager) Forget(ctx context.Context, id string) error {
	return m.store.Forget(ctx, id)
}

// ForgetAll 按条件批量删除。
func (m *Manager) ForgetAll(ctx context.Context, filter MemoryFilter) error {
	return m.store.ForgetAll(ctx, filter)
}

// ---------------------------------------------------------------------------
// Maintenance
// ---------------------------------------------------------------------------

// Prune 清理低价值的记忆。
func (m *Manager) Prune(ctx context.Context, layer MemoryLayer, threshold float64) (int64, error) {
	if threshold <= 0 {
		threshold = m.cfg.PruneThreshold
	}
	return m.store.Prune(ctx, layer, threshold)
}

// Stats 返回统计信息。
func (m *Manager) Stats() MemoryStats {
	return m.store.Stats()
}

// Close 关闭管理器并释放资源。
func (m *Manager) Close() error {
	return m.store.Close()
}
