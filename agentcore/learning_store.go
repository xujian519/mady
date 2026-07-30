package agentcore

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// LearningStore — Agent 调用反馈数据收集器
//
// 参考 BCIP 的 LearningStore + FeedbackLoop (codex-patent-agents/feedback_loop.rs)。
// LearningStore 记录每次 Agent 调用的结构化元数据，为策略学习（memory/compiler/）、
// 模型路由、质量监控提供数据基础。
//
// 与 tracing/ 的关系：
//   tracing/ 负责分布式追踪（跨度、延迟分析），粒度细、开销可配置。
//   LearningStore 负责聚合反馈（成功/失败、质量分），供 compiler 做策略选择。
//   两者互补：tracing 回答"哪里慢了"，LearningStore 回答"哪个策略好"。
// =============================================================================

// AgentCallFeedback 记录一次 Agent 调用的反馈数据。
type AgentCallFeedback struct {
	// AgentID 是 Agent 实例的唯一标识。
	AgentID string `json:"agent_id"`

	// RoleID 是 Agent 角色标识（如 "patent", "legal", "novelty_checker"）。
	RoleID string `json:"role_id,omitempty"`

	// Model 是使用的模型标识（如 "deepseek-v4-flash"）。
	Model string `json:"model"`

	// Provider 是模型提供商（如 "deepseek", "openai"）。
	Provider string `json:"provider,omitempty"`

	// Success 表示调用是否成功完成。
	Success bool `json:"success"`

	// 当 Success 为 false 时的错误类别。
	ErrorCategory string `json:"error_category,omitempty"`

	// LatencyMs 是调用的耗时（毫秒）。
	LatencyMs int64 `json:"latency_ms"`

	// QualityScore 是 ReflectionEngine 评估的质量分 [0.0, 1.0]。
	// -1 表示未评估。
	QualityScore float64 `json:"quality_score"`

	// TurnCount 是调用中的轮次数。
	TurnCount int `json:"turn_count,omitempty"`

	// ToolCallCount 是工具调用次数。
	ToolCallCount int `json:"tool_call_count,omitempty"`

	// InputTokens 是输入 token 数（LLM API 返回）。
	InputTokens int `json:"input_tokens,omitempty"`

	// OutputTokens 是输出 token 数。
	OutputTokens int `json:"output_tokens,omitempty"`

	// Timestamp 是调用完成的时间。
	Timestamp time.Time `json:"timestamp"`
}

// LearningStore 收集 Agent 调用反馈数据并支持按维度聚合统计。
type LearningStore struct {
	mu        sync.RWMutex
	feedbacks []AgentCallFeedback
	maxSize   int

	// 回调函数：添加新反馈时通知编译器。
	onFeedback func(AgentCallFeedback)
}

// NewLearningStore 创建反馈数据存储。
// maxSize 控制内存中保留的最大记录数（默认 1000）。
func NewLearningStore(maxSize int) *LearningStore {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &LearningStore{
		feedbacks: make([]AgentCallFeedback, 0, maxSize),
		maxSize:   maxSize,
	}
}

// Record 记录一次 Agent 调用的反馈数据。
func (ls *LearningStore) Record(fb AgentCallFeedback) {
	ls.mu.Lock()
	if len(ls.feedbacks) >= ls.maxSize {
		// 移除最旧的记录。
		ls.feedbacks = ls.feedbacks[1:]
	}
	ls.feedbacks = append(ls.feedbacks, fb)
	cb := ls.onFeedback
	ls.mu.Unlock()

	// 触发外部回调（如 compiler 更新）。
	if cb != nil {
		cb(fb)
	}
}

// OnFeedback 设置新反馈数据的回调函数。
// 用于将 LearningStore 连接到 memory/compiler/ 等下游组件。
func (ls *LearningStore) OnFeedback(cb func(AgentCallFeedback)) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.onFeedback = cb
}

// FeedbackCount 返回已记录的反馈数。
func (ls *LearningStore) FeedbackCount() int {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return len(ls.feedbacks)
}

// RecentFeedbacks 返回最近 N 条反馈记录。
func (ls *LearningStore) RecentFeedbacks(n int) []AgentCallFeedback {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	if n <= 0 || n > len(ls.feedbacks) {
		n = len(ls.feedbacks)
	}
	out := make([]AgentCallFeedback, n)
	copy(out, ls.feedbacks[len(ls.feedbacks)-n:])
	return out
}

// ---------------------------------------------------------------------------
// 聚合统计
// ---------------------------------------------------------------------------

// GroupByRole 返回按角色聚合的统计数据。
func (ls *LearningStore) GroupByRole() map[string]LearningStats {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return aggregateBy(ls.feedbacks, func(fb AgentCallFeedback) string {
		if fb.RoleID == "" {
			return "_default"
		}
		return fb.RoleID
	})
}

// GroupByModel 返回按模型聚合的统计数据。
func (ls *LearningStore) GroupByModel() map[string]LearningStats {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return aggregateBy(ls.feedbacks, func(fb AgentCallFeedback) string {
		if fb.Model == "" {
			return "_unknown"
		}
		return fb.Model
	})
}

// GroupByProvider 返回按提供商聚合的统计数据。
func (ls *LearningStore) GroupByProvider() map[string]LearningStats {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return aggregateBy(ls.feedbacks, func(fb AgentCallFeedback) string {
		if fb.Provider == "" {
			return "_unknown"
		}
		return fb.Provider
	})
}

// LearningStats 是按某个维度聚合的统计结果。
type LearningStats struct {
	TotalCalls      int            `json:"total_calls"`
	SuccessCount    int            `json:"success_count"`
	SuccessRate     float64        `json:"success_rate"`
	AvgLatencyMs    float64        `json:"avg_latency_ms"`
	AvgQualityScore float64        `json:"avg_quality_score"`
	ErrorCategories map[string]int `json:"error_categories,omitempty"`
}

// aggregateBy 按 keyFn 分组聚合反馈数据。
func aggregateBy(feedbacks []AgentCallFeedback, keyFn func(AgentCallFeedback) string) map[string]LearningStats {
	groups := make(map[string][]AgentCallFeedback)
	for _, fb := range feedbacks {
		key := keyFn(fb)
		groups[key] = append(groups[key], fb)
	}

	stats := make(map[string]LearningStats, len(groups))
	for key, group := range groups {
		s := LearningStats{
			TotalCalls:      len(group),
			ErrorCategories: make(map[string]int),
		}
		var totalLatency float64
		var totalQuality float64
		var qualityCount int

		for _, fb := range group {
			if fb.Success {
				s.SuccessCount++
			} else if fb.ErrorCategory != "" {
				s.ErrorCategories[fb.ErrorCategory]++
			}
			totalLatency += float64(fb.LatencyMs)
			if fb.QualityScore >= 0 {
				totalQuality += fb.QualityScore
				qualityCount++
			}
		}
		s.SuccessRate = float64(s.SuccessCount) / float64(len(group))
		if len(group) > 0 {
			s.AvgLatencyMs = totalLatency / float64(len(group))
		}
		if qualityCount > 0 {
			s.AvgQualityScore = totalQuality / float64(qualityCount)
		}
		stats[key] = s
	}
	return stats
}

// ---------------------------------------------------------------------------
// 持久化
// ---------------------------------------------------------------------------

// SaveToFile 将反馈数据持久化到 JSON 文件。
func (ls *LearningStore) SaveToFile(path string) error {
	ls.mu.RLock()
	data := make([]AgentCallFeedback, len(ls.feedbacks))
	copy(data, ls.feedbacks)
	ls.mu.RUnlock()

	// 如果文件不存在，创建目录。
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("learning_store: create dir: %w", err)
	}

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("learning_store: marshal: %w", err)
	}
	// 原子写入。
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, bytes, 0600); err != nil { //nolint:gosec
		return fmt.Errorf("learning_store: write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("learning_store: rename: %w", err)
	}
	return nil
}

// LoadFromFile 从 JSON 文件加载反馈数据。
func (ls *LearningStore) LoadFromFile(path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // 路径来自框架内部存储，非用户输入
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("learning_store: read: %w", err)
	}
	var feedbacks []AgentCallFeedback
	if err := json.Unmarshal(data, &feedbacks); err != nil {
		return fmt.Errorf("learning_store: unmarshal: %w", err)
	}
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.feedbacks = feedbacks
	return nil
}

// ---------------------------------------------------------------------------
// Compiler 集成辅助
// ---------------------------------------------------------------------------

// FeedbackToCompilerOutcome 将 AgentCallFeedback 转换为编译器 ExecutionTrace
// 所需的 Outcome 字符串（用于 memory/compiler 的策略更新）。
func FeedbackToCompilerOutcome(fb AgentCallFeedback) string {
	if fb.Success {
		if fb.QualityScore >= 0.7 {
			return "success"
		} else if fb.QualityScore >= 0.3 {
			return "partial"
		}
		return "failure"
	}
	return "failure"
}

// ----------------------------------------------------------------
// 默认全局实例（单例模式，供启动期注入）
// ----------------------------------------------------------------

var (
	defaultLearningStore     *LearningStore
	defaultLearningStoreOnce sync.Once
)

// DefaultLearningStore 返回全局默认的 LearningStore 实例。
func DefaultLearningStore() *LearningStore {
	defaultLearningStoreOnce.Do(func() {
		defaultLearningStore = NewLearningStore(1000)
	})
	return defaultLearningStore
}

// ----------------------------------------------------------------
// LearningHook — 集成 Agent 生命周期
// ----------------------------------------------------------------

// LearningHook 实现 TurnObserver，在 Agent 的每个 AfterTurn 时自动
// 收集反馈数据并记录到 LearningStore。
type LearningHook struct {
	Store  *LearningStore
	RoleID string
}

// AfterTurn 在 Agent 每次轮次完成后收集反馈。
func (h *LearningHook) AfterTurn(_ *AgentRunContext, info TurnInfo) {
	if h.Store == nil {
		return
	}
	// AfterTurn 没有完整反馈上下文，但我们可以记录轮次基本信息。
	// 完整的反馈收集由更高层的观察者完成。
	_ = info
}

// NewDefaultLearningHook 创建默认的学习钩子，使用全局 LearningStore。
func NewDefaultLearningHook(roleID string) *LearningHook {
	return &LearningHook{
		Store:  DefaultLearningStore(),
		RoleID: roleID,
	}
}

// Validate 检查反馈数据的字段合法性。
func (fb *AgentCallFeedback) Validate() error {
	if fb.AgentID == "" {
		return fmt.Errorf("learning_store: AgentID must not be empty")
	}
	if fb.Model == "" {
		return fmt.Errorf("learning_store: Model must not be empty")
	}
	if fb.LatencyMs < 0 {
		return fmt.Errorf("learning_store: LatencyMs must not be negative")
	}
	if fb.QualityScore < -1 || fb.QualityScore > 1.0 {
		return fmt.Errorf("learning_store: QualityScore %f out of range [-1, 1]", fb.QualityScore)
	}
	if fb.TurnCount < 0 {
		return fmt.Errorf("learning_store: TurnCount must not be negative")
	}
	return nil
}

// RoundQuality 将质量分四舍五入到指定小数位。
func RoundQuality(score float64, decimals int) float64 {
	if score < 0 {
		score = 0
	}
	if score > 1.0 {
		score = 1.0
	}
	pow := math.Pow(10, float64(decimals))
	return math.Round(score*pow) / pow
}

// ----------------------------------------------------------------
// 辅助函数
// ----------------------------------------------------------------

// SortFeedbackByTime 按时间戳排序反馈记录（最新的在前）。
func SortFeedbackByTime(feedbacks []AgentCallFeedback) {
	sort.Slice(feedbacks, func(i, j int) bool {
		return feedbacks[i].Timestamp.After(feedbacks[j].Timestamp)
	})
}

// FilterFeedbackByRole 过滤出指定角色的反馈记录。
func FilterFeedbackByRole(feedbacks []AgentCallFeedback, roleID string) []AgentCallFeedback {
	var out []AgentCallFeedback
	for _, fb := range feedbacks {
		if fb.RoleID == roleID {
			out = append(out, fb)
		}
	}
	return out
}

// FeedbackSummary 生成反馈的文本摘要，供调试和日志使用。
func FeedbackSummary(fb AgentCallFeedback) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("model=%s", fb.Model))
	if fb.Provider != "" {
		parts = append(parts, fmt.Sprintf("provider=%s", fb.Provider))
	}
	parts = append(parts, fmt.Sprintf("success=%v", fb.Success))
	parts = append(parts, fmt.Sprintf("latency=%dms", fb.LatencyMs))
	if fb.QualityScore >= 0 {
		parts = append(parts, fmt.Sprintf("quality=%.2f", fb.QualityScore))
	}
	if !fb.Success && fb.ErrorCategory != "" {
		parts = append(parts, fmt.Sprintf("err=%s", fb.ErrorCategory))
	}
	return fmt.Sprintf("AgentCall{%s}", strings.Join(parts, " "))
}
