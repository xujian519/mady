package autoresearch

import (
	"log/slog"
	"sync"
	"time"
)

// ResearchStatus 是研究任务的生命周期状态。
type ResearchStatus string

const (
	StatusIdle      ResearchStatus = "idle"
	StatusRunning   ResearchStatus = "running"
	StatusPaused    ResearchStatus = "paused"
	StatusCompleted ResearchStatus = "completed"
	StatusAborted   ResearchStatus = "aborted"
)

// validDomains 是受支持的研究领域集合。
// 不在此集合内的领域在构造时会被归一化为 "general"。
var validDomains = map[string]bool{
	"patent":  true,
	"legal":   true,
	"general": true,
}

// ResearchContract 定义一次自动研究任务的契约。
// 对齐 Reasonix autoresearch 的 TaskContract 设计。
type ResearchContract struct {
	mu sync.Mutex `json:"-"`

	// ID 是本次研究的唯一标识。
	ID string `json:"id"`
	// Goal 是研究目标（用户原始问题的提炼）。
	Goal string `json:"goal"`
	// Domain 是研究领域（patent/legal/general）。
	// 不在此集合内的值会被归一化为 "general"。
	Domain string `json:"domain"`

	// MaxRounds 是最大研究轮次（0 = 不限，受硬上限 100 约束）。
	MaxRounds int `json:"max_rounds"`
	// MaxDuration 是最大允许时长（0 = 不限）。
	MaxDuration time.Duration `json:"max_duration"`

	// Status 是当前状态。
	Status ResearchStatus `json:"status"`
	// CurrentRound 是当前轮次。
	CurrentRound int `json:"current_round"`
	// StartedAt 是开始时间。
	StartedAt time.Time `json:"started_at"`
	// PausedAt 是最近暂停时间（仅在 Status == paused 时有意义）。
	PausedAt *time.Time `json:"paused_at,omitempty"`
	// CompletedAt 是完成/中止时间。
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// SuccessCriteria 是研究成功标准列表。
	SuccessCriteria []SuccessCriterion `json:"success_criteria"`
	// Evidence 是各轮产出的证据汇总（上限为 MaxRounds 或 100）。
	Evidence []Evidence `json:"evidence"`
	// Directions 是方向变更记录。
	Directions []DirectionChange `json:"directions"`

	// AbortReason 记录中止原因（仅在 Status == aborted 时有意义）。
	AbortReason string `json:"abort_reason,omitempty"`
}

// CreateHeartbeat 创建与本研究合约绑定的心跳监控器。
// ContractID 自动从合约 ID 派生，消除字符串不匹配风险。
func (c *ResearchContract) CreateHeartbeat(interval, timeout time.Duration) *Heartbeat {
	return NewHeartbeat(c.ID, interval, timeout)
}

// ContractID 返回本研究合约的唯一标识，供 Heartbeat 等关联组件使用。
func (c *ResearchContract) ContractID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ID
}

// SuccessCriterion 定义一个研究成功条件。
type SuccessCriterion struct {
	Description string `json:"description"`
	Met         bool   `json:"met"`
	Evidence    string `json:"evidence,omitempty"`
}

// Evidence 是单轮研究的产出证据。
type Evidence struct {
	Round     int      `json:"round"`
	Summary   string   `json:"summary"`
	Findings  []string `json:"findings"`
	ToolsUsed []string `json:"tools_used"`
}

// DirectionChange 记录研究方向变更。
type DirectionChange struct {
	Round  int    `json:"round"`
	Reason string `json:"reason"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// NewResearchContract 创建一个研究契约，初始化默认值。
// MaxRounds 默认为 20，MaxDuration 默认为 30 分钟。
// domain 不合法时会被归一化为 "general"。
func NewResearchContract(id, goal, domain string) *ResearchContract {
	if !validDomains[domain] {
		domain = "general"
	}
	return &ResearchContract{
		ID:          id,
		Goal:        goal,
		Domain:      domain,
		MaxRounds:   20,
		MaxDuration: 30 * time.Minute,
		Status:      StatusIdle,
	}
}

// Start 启动研究（idle → running）。
// 非 idle 状态调用为 no-op。
func (c *ResearchContract) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Status != StatusIdle {
		slog.Debug("autoresearch: Start ignored (not idle)",
			"contract_id", c.ID, "status", c.Status)
		return
	}
	c.Status = StatusRunning
	c.CurrentRound = 0
	c.StartedAt = time.Now()
	slog.Info("autoresearch: research started",
		"contract_id", c.ID, "goal", c.Goal, "domain", c.Domain)
}

// AdvanceRound 推进到下一轮。
func (c *ResearchContract) AdvanceRound() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CurrentRound++
	slog.Debug("autoresearch: round advanced",
		"contract_id", c.ID, "round", c.CurrentRound)
}

// Pause 暂停研究（running → paused）。
// 非 running 状态调用为 no-op。
func (c *ResearchContract) Pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Status != StatusRunning {
		slog.Debug("autoresearch: Pause ignored (not running)",
			"contract_id", c.ID, "status", c.Status)
		return
	}
	now := time.Now()
	c.PausedAt = &now
	c.Status = StatusPaused
	slog.Info("autoresearch: research paused",
		"contract_id", c.ID, "round", c.CurrentRound)
}

// Resume 恢复研究（paused → running）。
// 非 paused 状态调用为 no-op。
func (c *ResearchContract) Resume() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Status != StatusPaused {
		slog.Debug("autoresearch: Resume ignored (not paused)",
			"contract_id", c.ID, "status", c.Status)
		return
	}
	c.Status = StatusRunning
	slog.Info("autoresearch: research resumed",
		"contract_id", c.ID, "round", c.CurrentRound)
}

// Complete 标记研究完成（running/paused → completed）。
// 非 running/paused 状态调用为 no-op。
func (c *ResearchContract) Complete() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Status != StatusRunning && c.Status != StatusPaused {
		slog.Debug("autoresearch: Complete ignored (not running/paused)",
			"contract_id", c.ID, "status", c.Status)
		return
	}
	now := time.Now()
	c.CompletedAt = &now
	c.Status = StatusCompleted
	slog.Info("autoresearch: research completed",
		"contract_id", c.ID,
		"rounds", c.CurrentRound,
		"duration", time.Since(c.StartedAt).Round(time.Second))
}

// Abort 中止研究（running/paused → aborted）。
// 非 running/paused 状态调用为 no-op。
// 中止原因以 AbortReason 字段独立存储，不影响 SuccessCriteria。
func (c *ResearchContract) Abort(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Status != StatusRunning && c.Status != StatusPaused {
		slog.Debug("autoresearch: Abort ignored (not running/paused)",
			"contract_id", c.ID, "status", c.Status)
		return
	}
	now := time.Now()
	c.CompletedAt = &now
	c.Status = StatusAborted
	c.AbortReason = reason
	slog.Warn("autoresearch: research aborted",
		"contract_id", c.ID,
		"reason", reason,
		"round", c.CurrentRound)
}

// IsExpired 检查是否超过最大轮次或时长。
// 未启动（StartedAt 为零值）时返回 false。
func (c *ResearchContract) IsExpired() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.StartedAt.IsZero() {
		return false
	}
	if c.MaxRounds > 0 && c.CurrentRound >= c.MaxRounds {
		return true
	}
	if c.MaxDuration > 0 && time.Since(c.StartedAt) > c.MaxDuration {
		return true
	}
	return false
}

// AllCriteriaMet 检查是否所有成功标准都已满足。
// 中止状态直接返回 false；空标准列表视为未满足。
func (c *ResearchContract) AllCriteriaMet() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Status == StatusAborted {
		return false
	}
	if len(c.SuccessCriteria) == 0 {
		return false
	}
	for _, sc := range c.SuccessCriteria {
		if !sc.Met {
			return false
		}
	}
	return true
}

// maxEvidenceEntries 是证据记录硬上限。
const maxEvidenceEntries = 100

// AddEvidence 追加一轮证据。超出上限时自动淘汰最旧条目。
func (c *ResearchContract) AddEvidence(e Evidence) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Evidence = append(c.Evidence, e)

	// 用 MaxRounds 作为证据上限（有设定时），否则使用硬上限。
	limit := c.MaxRounds
	if limit <= 0 || limit > maxEvidenceEntries {
		limit = maxEvidenceEntries
	}
	if len(c.Evidence) > limit {
		dropped := len(c.Evidence) - limit
		c.Evidence = c.Evidence[len(c.Evidence)-limit:]
		slog.Debug("autoresearch: evidence trimmed",
			"contract_id", c.ID, "dropped", dropped, "remaining", limit)
	}
	slog.Debug("autoresearch: evidence added",
		"contract_id", c.ID, "round", e.Round,
		"summary", truncateString(e.Summary, 40))
}

// RecordDirectionChange 记录方向变更。
func (c *ResearchContract) RecordDirectionChange(from, to, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Directions = append(c.Directions, DirectionChange{
		Round:  c.CurrentRound,
		Reason: reason,
		From:   from,
		To:     to,
	})
}

// DirectionPivotCount 返回方向变更次数（用于检测频繁 pivoting）。
func (c *ResearchContract) DirectionPivotCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.Directions)
}

// PausedAtTime 返回暂停时间戳和有效标志。
// 若从未暂停则有效标志为 false。
func (c *ResearchContract) PausedAtTime() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.PausedAt == nil {
		return time.Time{}, false
	}
	return *c.PausedAt, true
}

// CompletedAtTime 返回完成时间戳和有效标志。
// 若从未完成/中止则有效标志为 false。
func (c *ResearchContract) CompletedAtTime() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.CompletedAt == nil {
		return time.Time{}, false
	}
	return *c.CompletedAt, true
}

// truncateString 截断字符串到指定长度（用于日志摘要，避免埋入过长的研究目标/摘要）。
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
