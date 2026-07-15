// Package memory 提供 Mady Agent 的长期记忆系统。
//
// 架构借鉴 Mem0、CrewAI、Letta、LangChain、LlamaIndex 等开源记忆系统的核心设计：
//   - 四维隔离作用域（Mem0 风格）：user / agent / session / project
//   - 三层记忆模型（CrewAI + Mem0 风格）：User / Session / Long-term
//   - 复合评分检索（CrewAI 风格）：语义 + 新鲜度 + 重要性
//   - Token 预算感知（LlamaIndex 风格）：chat_history_token_ratio
//   - Agent 自我编辑记忆（Letta 风格）：通过 Tool Calling 管理
//
// 集成方式：MemoryExtension 实现 agentcore.Extension 的 TransformContextProvider
// （注入记忆）+ ToolProvider（工具）+ LifecycleProvider（生命周期钩子），
// 复用 Mady 现有的 Extension 注册机制。
package memory

import (
	"context"
	"maps"
	"time"
)

// ---------------------------------------------------------------------------
// Scope — 四维隔离作用域
// ---------------------------------------------------------------------------

// MemoryScope 标识一条记忆属于谁。
// 借鉴 Mem0 的 user_id / agent_id / app_id / run_id 四维正交模型。
type MemoryScope struct {
	UserID    string `json:"user_id,omitempty"`    // 用户标识（跨会话持久）
	AgentID   string `json:"agent_id,omitempty"`   // Agent 角色/标识
	SessionID string `json:"session_id,omitempty"` // 会话标识（映射 Mady session）
	ProjectID string `json:"project_id,omitempty"` // 项目/工作区标识
}

// IsEmpty 返回是否所有字段均为空。
func (s MemoryScope) IsEmpty() bool {
	return s.UserID == "" && s.AgentID == "" && s.SessionID == "" && s.ProjectID == ""
}

// ---------------------------------------------------------------------------
// Layer — 记忆分层
// ---------------------------------------------------------------------------

// MemoryLayer 标识记忆的持久层级。
type MemoryLayer string

const (
	LayerUser     MemoryLayer = "user"      // 跨会话用户偏好/背景（持久）
	LayerSession  MemoryLayer = "session"   // 当前会话关键上下文（会话级）
	LayerLongTerm MemoryLayer = "long_term" // 跨会话持久事实/知识（持久）
)

// ValidLayers 返回所有有效层级的切片。
func ValidLayers() []MemoryLayer {
	return []MemoryLayer{LayerUser, LayerSession, LayerLongTerm}
}

// IsValid 检查层级是否在有效集合中。
func (l MemoryLayer) IsValid() bool {
	switch l {
	case LayerUser, LayerSession, LayerLongTerm:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// MemoryEntry — 单条记忆
// ---------------------------------------------------------------------------

// MemoryEntry 是一条完整的记忆记录。
type MemoryEntry struct {
	ID        string      `json:"id"`
	Scope     MemoryScope `json:"scope"`
	Layer     MemoryLayer `json:"layer"`
	Content   string      `json:"content"`             // 自然语言记忆内容
	Embedding []float32   `json:"embedding,omitempty"` // 向量表示（语义检索用）

	// 复合评分字段（借鉴 CrewAI）
	Importance  float64   `json:"importance"`   // LLM 重要性评分 (0-1)
	AccessCount int64     `json:"access_count"` // 被检索次数（热度）
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastAccess  time.Time `json:"last_access"`
	DecayFactor float64   `json:"decay_factor"` // 衰减因子 (0~1, 默认 0.95/天)

	Metadata map[string]any `json:"metadata,omitempty"` // 扩展元数据
}

// Clone 返回 MemoryEntry 的深拷贝。
func (e MemoryEntry) Clone() MemoryEntry {
	cp := e
	if e.Embedding != nil {
		cp.Embedding = make([]float32, len(e.Embedding))
		copy(cp.Embedding, e.Embedding)
	}
	if e.Metadata != nil {
		cp.Metadata = make(map[string]any, len(e.Metadata))
		maps.Copy(cp.Metadata, e.Metadata)
	}
	return cp
}

// ---------------------------------------------------------------------------
// ScoredMemory — 检索结果（含复合评分）
// ---------------------------------------------------------------------------

// ScoredMemory 是经过复合评分排序的一条检索结果。
// 借鉴 CrewAI 的复合评分公式：
//
//	score = w_semantic × similarity + w_recency × decay + w_importance × importance
//
// 默认权重：w_semantic=0.5, w_recency=0.3, w_importance=0.2。
type ScoredMemory struct {
	Entry      MemoryEntry `json:"entry"`
	Semantic   float64     `json:"semantic"`   // 语义相似度 (cosine)
	Recency    float64     `json:"recency"`    // 新鲜度 (exponential decay)
	Importance float64     `json:"importance"` // 重要性（来自 LLM 标注）
	Composite  float64     `json:"composite"`  // 复合评分

	// Rank 在排序后赋值，0 = 最相关
	Rank int `json:"rank"`
}

// ---------------------------------------------------------------------------
// MemoryFilter — 检索过滤条件
// ---------------------------------------------------------------------------

// MemoryFilter 用于约束记忆检索的范围。
type MemoryFilter struct {
	UserID    string      `json:"user_id,omitempty"`
	AgentID   string      `json:"agent_id,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
	ProjectID string      `json:"project_id,omitempty"`
	Layer     MemoryLayer `json:"layer,omitempty"` // 空字符串 = 所有层

	// TopK 最大返回条数。0 = 使用默认值 (10)。
	TopK int `json:"top_k,omitempty"`
}

// EffectiveTopK 返回有效的 TopK 值。
func (f MemoryFilter) EffectiveTopK() int {
	if f.TopK <= 0 {
		return 10
	}
	if f.TopK > 100 {
		return 100
	}
	return f.TopK
}

// ---------------------------------------------------------------------------
// ScoringConfig — 复合评分配置
// ---------------------------------------------------------------------------

// ScoringConfig 控制复合评分中各维度的权重和参数。
type ScoringConfig struct {
	SemanticWeight   float64 `json:"semantic_weight"`   // 语义相似度权重 (default: 0.5)
	RecencyWeight    float64 `json:"recency_weight"`    // 新鲜度权重 (default: 0.3)
	ImportanceWeight float64 `json:"importance_weight"` // 重要性权重 (default: 0.2)
	RecencyHalfLife  float64 `json:"recency_half_life"` // 新鲜度半衰期（天, default: 30）
}

// DefaultScoringConfig 返回默认评分配置。
func DefaultScoringConfig() ScoringConfig {
	return ScoringConfig{
		SemanticWeight:   0.5,
		RecencyWeight:    0.3,
		ImportanceWeight: 0.2,
		RecencyHalfLife:  30.0, // 30 天半衰期
	}
}

// ---------------------------------------------------------------------------
// TokenBudget — Token 预算配置
// ---------------------------------------------------------------------------

// TokenBudget 借鉴 LlamaIndex 的 chat_history_token_ratio 机制，
// 控制记忆层在上下文中的 token 消耗上限。
type TokenBudget struct {
	// MaxTokens 是该层可用的最大 token 数。0 = 无限制。
	MaxTokens int64 `json:"max_tokens"`
	// Ratio 占总上下文的比例。当 MaxTokens 为 0 时使用 Ratio 计算。
	// 例如 0.15 = 最多占用 15% 的上下文窗口。
	Ratio float64 `json:"ratio"`
}

// DefaultMemoryTokenBudget 返回默认的记忆 token 预算（15% 的上下文窗口）。
func DefaultMemoryTokenBudget() TokenBudget {
	return TokenBudget{
		MaxTokens: 0,
		Ratio:     0.15,
	}
}

// ---------------------------------------------------------------------------
// MemoryStore — 记忆存储接口
// ---------------------------------------------------------------------------

// ListOptions 控制 List 操作的排序和分页。
type ListOptions struct {
	Limit  int  `json:"limit"`
	Offset int  `json:"offset"`
	Asc    bool `json:"asc"` // true = 按创建时间升序，false = 降序
}

// MemoryStats 是存储引擎的统计信息。
type MemoryStats struct {
	TotalEntries int64 `json:"total_entries"`
	UserCount    int64 `json:"user_count"`
	SessionCount int64 `json:"session_count"`
	LongTermCnt  int64 `json:"long_term_count"`
}

// MemoryStore 是所有记忆后端的统一接口。
type MemoryStore interface {
	// Remember 存入一条记忆。如果 content 为空则跳过。
	// 返回记忆 ID。实现在存入前应自动计算 importance。
	Remember(ctx context.Context, content string, scope MemoryScope, layer MemoryLayer, metadata map[string]any) (string, error)

	// RememberBatch 批量存入多条记忆（非阻塞语义，不保证原子性）。
	RememberBatch(ctx context.Context, entries []MemoryEntry) error

	// Recall 按语义相似度检索记忆，返回按复合评分降序排列的结果。
	Recall(ctx context.Context, query string, filter MemoryFilter) ([]ScoredMemory, error)

	// RecallWithBudget 在 token 预算约束下检索记忆。
	// 在达到 maxTokens 之前尽可能多地返回高评分结果。
	RecallWithBudget(ctx context.Context, query string, filter MemoryFilter, maxTokens int64) ([]ScoredMemory, error)

	// Get 按 ID 获取单条记忆。
	Get(ctx context.Context, id string) (*MemoryEntry, error)

	// Update 更新记忆内容和更新时间。不更新评分字段。
	Update(ctx context.Context, id string, content string) error

	// Forget 按 ID 删除单条记忆。
	Forget(ctx context.Context, id string) error

	// ForgetAll 按过滤条件批量删除。用于 GDPR 级数据清除。
	ForgetAll(ctx context.Context, filter MemoryFilter) error

	// List 按层列出记忆（分页）。
	List(ctx context.Context, layer MemoryLayer, opts ListOptions) ([]MemoryEntry, error)

	// Prune 清理低衰减/低重要性的记忆。
	// threshold: 复合评分阈值，低于此值的记忆将被删除。
	// 返回被清理的数量。
	Prune(ctx context.Context, layer MemoryLayer, threshold float64) (int64, error)

	// Stats 返回存储统计信息。
	Stats(ctx context.Context) MemoryStats

	// Close 释放所有资源。
	Close() error
}
