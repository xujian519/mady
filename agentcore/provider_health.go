package agentcore

import (
	"sync"
	"time"
)

// ============================================================================
// ProviderHealthTracker — 模型级健康状态追踪
//
// 追踪每个模型的调用成功率，在连续失败超过阈值时临时降级，
// 避免向已故障的模型反复发送请求。
//
// 参考：PilotDeck ProviderHealthTracker 设计
// ============================================================================

// HealthConfig 控制健康检测的阈值参数。
type HealthConfig struct {
	// ConsecutiveFailThreshold 触发降级的连续失败次数。0 = 默认 3 次。
	ConsecutiveFailThreshold int `json:"consecutive_fail_threshold,omitempty" yaml:"consecutive_fail_threshold,omitempty"`

	// DegradeDuration 降级持续时长。0 = 默认 30 秒。
	DegradeDuration time.Duration `json:"degrade_duration,omitempty" yaml:"degrade_duration,omitempty"`

	// SuccessRecoveryCount 恢复健康所需的连续成功次数。0 = 默认 2 次。
	SuccessRecoveryCount int `json:"success_recovery_count,omitempty" yaml:"success_recovery_count,omitempty"`
}

// DefaultHealthConfig 返回默认的健康检测配置。
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		ConsecutiveFailThreshold: 3,
		DegradeDuration:          30 * time.Second,
		SuccessRecoveryCount:     2,
	}
}

// ProviderHealth 记录单个模型的健康状态。
type ProviderHealth struct {
	// Model 是模型标识符。
	Model string `json:"model"`

	// ConsecutiveFails 是连续失败次数。
	ConsecutiveFails int `json:"consecutive_fails"`

	// ConsecutiveSuccesses 是连续成功次数（用于恢复）。
	ConsecutiveSuccesses int `json:"consecutive_successes"`

	// TotalCalls 是历史总调用次数。
	TotalCalls int64 `json:"total_calls"`

	// TotalFailures 是历史总失败次数。
	TotalFailures int64 `json:"total_failures"`

	// DegradedUntil 为 nil 时表示健康；非 nil 表示降级到此时间点为止。
	DegradedUntil *time.Time `json:"degraded_until,omitempty"`

	// LastFailure 是最近一次失败的时间。
	LastFailure *time.Time `json:"last_failure,omitempty"`
}

// IsHealthy 返回模型当前是否可用（不在降级期内）。
func (h *ProviderHealth) IsHealthy() bool {
	if h.DegradedUntil == nil {
		return true
	}
	return time.Now().After(*h.DegradedUntil)
}

// HealthStatus 是模型健康状态的枚举。
type HealthStatus int

const (
	HealthStatusHealthy HealthStatus = iota
	HealthStatusDegraded
)

func (s HealthStatus) String() string {
	switch s {
	case HealthStatusHealthy:
		return "healthy"
	case HealthStatusDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// ProviderHealthTracker 管理多个模型的健康状态。
//
// 线程安全：内部使用 sync.RWMutex 保护状态。
type ProviderHealthTracker struct {
	mu     sync.RWMutex
	health map[string]*ProviderHealth
	config HealthConfig
}

// NewProviderHealthTracker 创建健康检测器，nil 配置使用默认值。
func NewProviderHealthTracker(cfg *HealthConfig) *ProviderHealthTracker {
	c := DefaultHealthConfig()
	if cfg != nil {
		if cfg.ConsecutiveFailThreshold > 0 {
			c.ConsecutiveFailThreshold = cfg.ConsecutiveFailThreshold
		}
		if cfg.DegradeDuration > 0 {
			c.DegradeDuration = cfg.DegradeDuration
		}
		if cfg.SuccessRecoveryCount > 0 {
			c.SuccessRecoveryCount = cfg.SuccessRecoveryCount
		}
	}
	return &ProviderHealthTracker{
		health: make(map[string]*ProviderHealth),
		config: c,
	}
}

// getOrCreate 获取或创建模型健康记录。
func (t *ProviderHealthTracker) getOrCreate(model string) *ProviderHealth {
	h, ok := t.health[model]
	if !ok {
		h = &ProviderHealth{Model: model}
		t.health[model] = h
	}
	return h
}

// RecordSuccess 记录一次模型调用成功。连续成功累积到阈值后自动恢复健康。
func (t *ProviderHealthTracker) RecordSuccess(model string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	h := t.getOrCreate(model)
	h.TotalCalls++
	h.ConsecutiveFails = 0
	h.ConsecutiveSuccesses++

	// 连续成功达到阈值 → 退出降级状态
	if h.ConsecutiveSuccesses >= t.config.SuccessRecoveryCount && h.DegradedUntil != nil {
		h.DegradedUntil = nil
	}
}

// RecordFailure 记录一次模型调用失败。连续失败超过阈值则进入降级。
// nonRetryable 表示不可重试的错误（如认证失败、模型不存在），这类错误立即触发降级。
func (t *ProviderHealthTracker) RecordFailure(model string, nonRetryable bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	h := t.getOrCreate(model)
	h.TotalCalls++
	h.TotalFailures++
	h.ConsecutiveFails++
	h.ConsecutiveSuccesses = 0
	now := time.Now()
	h.LastFailure = &now

	// 不可重试错误 → 立即降级
	if nonRetryable {
		until := now.Add(t.config.DegradeDuration)
		h.DegradedUntil = &until
		return
	}

	// 连续失败超过阈值 → 降级
	if h.ConsecutiveFails >= t.config.ConsecutiveFailThreshold {
		until := now.Add(t.config.DegradeDuration)
		h.DegradedUntil = &until
	}
}

// IsHealthy 查询模型是否健康。
func (t *ProviderHealthTracker) IsHealthy(model string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	h, ok := t.health[model]
	if !ok {
		return true // 无历史记录的健康模型
	}
	return h.IsHealthy()
}

// HealthOf 返回模型的健康状态。
// 等价于 HealthStatus 枚举版本的 IsHealthy，用于需要结构化输出的场景。
func (t *ProviderHealthTracker) HealthOf(model string) HealthStatus {
	if t.IsHealthy(model) {
		return HealthStatusHealthy
	}
	return HealthStatusDegraded
}

// Snapshot 返回所有模型健康状态的快照。
// 返回的 map 对并发读安全，但不会反映快照之后的状态变更。
func (t *ProviderHealthTracker) Snapshot() map[string]HealthStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	snap := make(map[string]HealthStatus, len(t.health))
	for model, h := range t.health {
		if h.DegradedUntil == nil || time.Now().After(*h.DegradedUntil) {
			snap[model] = HealthStatusHealthy
		} else {
			snap[model] = HealthStatusDegraded
		}
	}
	return snap
}

// DetailOf 返回模型的详细健康记录副本。
// 返回的副本包含独立的时间值副本，调用方修改不会影响内部状态。
func (t *ProviderHealthTracker) DetailOf(model string) *ProviderHealth {
	t.mu.RLock()
	defer t.mu.RUnlock()

	h, ok := t.health[model]
	if !ok {
		return nil
	}
	// 深度复制：结构体值复制 + 指针字段独立副本（*time.Time 是共享引用，
	// 浅复制后多调用方仍共享同一时间值）。时间值用后即弃从不原地修改，
	// 但深度复制是最安全的防御策略。
	cp := *h
	if h.DegradedUntil != nil {
		t := *h.DegradedUntil
		cp.DegradedUntil = &t
	}
	if h.LastFailure != nil {
		t := *h.LastFailure
		cp.LastFailure = &t
	}
	return &cp
}
