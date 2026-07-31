// Package plantask 提供 PlanTask HCL（Human-Collaborative Loop）闭环：
// 工作流执行前先构建 Plan + 任务清单，用户批准后执行，执行中可打断、
// 可反馈改进路径，实现真正的人机协作。
//
// 依赖方向（严格单向）：
//   - plantask → agentcore（Extension/Tool/Task/事件）
//   - plantask → agentcore/tasklist（FileStore 复用）
//   - plantask → agentcore/planmode（Gate 接口，由装配层注入）
//
// plantask 不依赖 domains/reasoning（分层红线）；Planner 类能力由
// 装配层经接口注入（阶段二 runner_adapter）。
package plantask

import (
	"errors"
	"fmt"
	"time"
)

// Status 是 HCL 会话的生命周期状态。
type Status string

const (
	// StatusPlanning 规划态：planmode 门控激活（只读），Agent 只允许产出 Plan。
	StatusPlanning Status = "planning"
	// StatusAwaitingApproval 等待用户批准。
	StatusAwaitingApproval Status = "awaiting_approval"
	// StatusExecuting 执行中（批准后）。
	StatusExecuting Status = "executing"
	// StatusAwaitingFeedback 执行中断，等待用户反馈。
	StatusAwaitingFeedback Status = "awaiting_feedback"
	// StatusReplanning 根据反馈重新规划中。
	StatusReplanning Status = "replanning"
	// StatusFinished 执行完成。
	StatusFinished Status = "finished"
	// StatusCanceled 用户取消。
	StatusCanceled Status = "canceled"
	// StatusExpired 会话超时。
	StatusExpired Status = "expired"
)

// String 返回状态的可读名称。
func (s Status) String() string { return string(s) }

// 语义错误（02-spec §6）。
var (
	// ErrNoActiveSession 工具调用时无匹配会话（ID 错误或已 Finished）。
	ErrNoActiveSession = errors.New("plantask: no active session")
	// ErrInvalidTransition 状态迁移不在白名单内。
	ErrInvalidTransition = errors.New("plantask: invalid state transition")
	// ErrSessionExpired 会话已超时。
	ErrSessionExpired = errors.New("plantask: session expired")
	// ErrPlanNotApproved Executing 状态下尝试批准类操作。
	ErrPlanNotApproved = errors.New("plantask: plan not approved")
	// ErrFeedbackEmpty workflow_feedback 文本为空。
	ErrFeedbackEmpty = errors.New("plantask: feedback text is empty")
	// ErrPlanEmpty plan_submit 提交的步骤为空。
	ErrPlanEmpty = errors.New("plantask: plan has no steps")
)

// InvalidTransitionError 携带 from/to 的非法迁移错误。
type InvalidTransitionError struct {
	From Status
	To   Status
}

func (e *InvalidTransitionError) Error() string {
	return fmt.Sprintf("%s: %s -> %s", ErrInvalidTransition, e.From, e.To)
}

func (e *InvalidTransitionError) Unwrap() error { return ErrInvalidTransition }

// StepSnapshot 是 Plan 步骤的持久化快照。
type StepSnapshot struct {
	Order       int    `json:"order"`
	Strategy    string `json:"strategy"`
	Description string `json:"description"`
	Hash        string `json:"hash"` // Order+Strategy+Description 的 SHA-256
}

// PlanSnapshot 是当前批准的 Plan 快照。
type PlanSnapshot struct {
	PlanID string         `json:"plan_id"`
	Steps  []StepSnapshot `json:"steps"`
}

// FeedbackEntry 是一条用户反馈（审计留痕，只增不改）。
type FeedbackEntry struct {
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
	StepID string    `json:"step_id"`
}

// InterruptEntry 是当前中断上下文。
type InterruptEntry struct {
	StepID string         `json:"step_id"`
	Reason string         `json:"reason"`
	Data   map[string]any `json:"data,omitempty"`
}

// PlanTaskSession 是一次 HCL 闭环的运行时实例。
type PlanTaskSession struct {
	ID           string          `json:"id"`
	CaseID       string          `json:"case_id"`
	CaseType     string          `json:"case_type"`
	Status       Status          `json:"status"`
	Plan         PlanSnapshot    `json:"plan"`
	TaskIDs      []string        `json:"task_ids"`
	CompletedIDs []string        `json:"completed_ids"`
	FeedbackLog  []FeedbackEntry `json:"feedback_log"`
	CheckpointID string          `json:"checkpoint_id"`
	Interrupt    *InterruptEntry `json:"interrupt,omitempty"`
	ReviseIntent string          `json:"revise_intent,omitempty"`
	ExpiresAt    *time.Time      `json:"expires_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// NewSession 创建一个处于 Planning 态的新会话。
func NewSession(id, caseID, caseType string) *PlanTaskSession {
	now := time.Now()
	return &PlanTaskSession{
		ID:        id,
		CaseID:    caseID,
		CaseType:  caseType,
		Status:    StatusPlanning,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Transition 校验并执行状态迁移（唯一状态写入路径）。
// 非法迁移返回 *InvalidTransitionError；会话已过期时返回 ErrSessionExpired。
func (s *PlanTaskSession) Transition(to Status) error {
	if s.Status == StatusExpired {
		return ErrSessionExpired
	}
	if s.ExpiresAt != nil && time.Now().After(*s.ExpiresAt) && s.Status != StatusFinished {
		s.Status = StatusExpired
		s.UpdatedAt = time.Now()
		return ErrSessionExpired
	}
	if !allowedTransition(s.Status, to) {
		return &InvalidTransitionError{From: s.Status, To: to}
	}
	s.Status = to
	s.UpdatedAt = time.Now()
	return nil
}

// AddFeedback 追加一条反馈记录（审计留痕）。
func (s *PlanTaskSession) AddFeedback(text, stepID string) {
	s.FeedbackLog = append(s.FeedbackLog, FeedbackEntry{
		At:     time.Now(),
		Text:   text,
		StepID: stepID,
	})
	s.UpdatedAt = time.Now()
}

// MarkCompleted 将步骤标记为已完成（replan 增量依据）。
func (s *PlanTaskSession) MarkCompleted(stepID string) {
	for _, id := range s.CompletedIDs {
		if id == stepID {
			return
		}
	}
	s.CompletedIDs = append(s.CompletedIDs, stepID)
	s.UpdatedAt = time.Now()
}

// SetExpiresAt 设置会话超时时间。
func (s *PlanTaskSession) SetExpiresAt(t time.Time) {
	s.ExpiresAt = &t
	s.UpdatedAt = time.Now()
}

// Clone 返回会话的深拷贝。
func (s *PlanTaskSession) Clone() *PlanTaskSession {
	if s == nil {
		return nil
	}
	cp := *s
	cp.TaskIDs = append([]string(nil), s.TaskIDs...)
	cp.CompletedIDs = append([]string(nil), s.CompletedIDs...)
	if s.FeedbackLog != nil {
		cp.FeedbackLog = append([]FeedbackEntry(nil), s.FeedbackLog...)
	}
	if s.Plan.Steps != nil {
		cp.Plan.Steps = append([]StepSnapshot(nil), s.Plan.Steps...)
	}
	if s.Interrupt != nil {
		it := *s.Interrupt
		cp.Interrupt = &it
	}
	if s.ExpiresAt != nil {
		t := *s.ExpiresAt
		cp.ExpiresAt = &t
	}
	return &cp
}
