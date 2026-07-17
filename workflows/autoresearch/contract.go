package autoresearch

import "time"

// ResearchStatus 是研究任务的生命周期状态。
type ResearchStatus string

const (
	StatusIdle      ResearchStatus = "idle"
	StatusRunning   ResearchStatus = "running"
	StatusPaused    ResearchStatus = "paused"
	StatusCompleted ResearchStatus = "completed"
	StatusAborted   ResearchStatus = "aborted"
)

// ResearchContract 定义一次自动研究任务的契约。
// 对齐 Reasonix autoresearch 的 TaskContract 设计。
type ResearchContract struct {
	// ID 是本次研究的唯一标识。
	ID string `json:"id"`
	// Goal 是研究目标（用户原始问题的提炼）。
	Goal string `json:"goal"`
	// Domain 是研究领域（patent/legal）。
	Domain string `json:"domain"`
	// MaxRounds 是最大研究轮次（0 = 不限，最多 20）。
	MaxRounds int `json:"max_rounds"`
	// MaxDuration 是最大允许时长（0 = 不限，最多 30 分钟）。
	MaxDuration time.Duration `json:"max_duration"`

	// Status 是当前状态。
	Status ResearchStatus `json:"status"`
	// CurrentRound 是当前轮次。
	CurrentRound int `json:"current_round"`
	// StartedAt 是开始时间。
	StartedAt time.Time `json:"started_at"`
	// PausedAt 是最近暂停时间。
	PausedAt *time.Time `json:"paused_at,omitempty"`
	// CompletedAt 是完成时间。
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// SuccessCriteria 是研究成功标准列表。
	SuccessCriteria []SuccessCriterion `json:"success_criteria"`
	// Evidence 是各轮产出的证据汇总。
	Evidence []Evidence `json:"evidence"`
	// Directions 是方向变更记录。
	Directions []DirectionChange `json:"directions"`
}

// SuccessCriterion 定义一个研究成功条件。
type SuccessCriterion struct {
	Description string `json:"description"`
	Met         bool   `json:"met"`
	Evidence    string `json:"evidence,omitempty"`
}

// Evidence 是单轮研究的产出证据。
type Evidence struct {
	Round    int       `json:"round"`
	Summary  string    `json:"summary"`
	Findings []string  `json:"findings"`
	ToolsUsed []string `json:"tools_used"`
}

// DirectionChange 记录研究方向变更。
type DirectionChange struct {
	Round  int    `json:"round"`
	Reason string `json:"reason"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// NewResearchContract 创建一个研究契约。
func NewResearchContract(id, goal, domain string) *ResearchContract {
	return &ResearchContract{
		ID:        id,
		Goal:      goal,
		Domain:    domain,
		MaxRounds: 20,
		Status:    StatusIdle,
	}
}

// Start 启动研究（idle → running）。
func (c *ResearchContract) Start() {
	c.Status = StatusRunning
	c.CurrentRound = 0
	c.StartedAt = time.Now()
}

// AdvanceRound 推进到下一轮。
func (c *ResearchContract) AdvanceRound() {
	c.CurrentRound++
}

// Pause 暂停研究。
func (c *ResearchContract) Pause() {
	now := time.Now()
	c.PausedAt = &now
	c.Status = StatusPaused
}

// Resume 恢复研究。
func (c *ResearchContract) Resume() {
	c.Status = StatusRunning
}

// Complete 标记研究完成。
func (c *ResearchContract) Complete() {
	now := time.Now()
	c.CompletedAt = &now
	c.Status = StatusCompleted
}

// Abort 中止研究。
func (c *ResearchContract) Abort(reason string) {
	now := time.Now()
	c.CompletedAt = &now
	c.Status = StatusAborted
	c.SuccessCriteria = append(c.SuccessCriteria, SuccessCriterion{
		Description: "中止原因",
		Met:         false,
		Evidence:    reason,
	})
}

// IsExpired 检查是否超过最大轮次或时长。
func (c *ResearchContract) IsExpired() bool {
	if c.MaxRounds > 0 && c.CurrentRound >= c.MaxRounds {
		return true
	}
	if c.MaxDuration > 0 && time.Since(c.StartedAt) > c.MaxDuration {
		return true
	}
	return false
}

// AllCriteriaMet 检查是否所有成功标准都已满足。
func (c *ResearchContract) AllCriteriaMet() bool {
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

// AddEvidence 追加一轮证据。
func (c *ResearchContract) AddEvidence(e Evidence) {
	c.Evidence = append(c.Evidence, e)
}

// RecordDirectionChange 记录方向变更。
func (c *ResearchContract) RecordDirectionChange(from, to, reason string) {
	c.Directions = append(c.Directions, DirectionChange{
		Round:  c.CurrentRound,
		Reason: reason,
		From:   from,
		To:     to,
	})
}

// DirectionPivotCount 返回方向变更次数（用于检测频繁 pivoting）。
func (c *ResearchContract) DirectionPivotCount() int {
	return len(c.Directions)
}
