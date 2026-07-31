package plantask

import (
	"context"
	"fmt"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName 是 HCL 扩展的注册名称。
const ExtensionName = "plantask"

// Gate 是 planmode 门控的最小接口（02-spec §7 边界契约）。
// agentcore/planmode.PlanModeExtension 天然满足；由装配层注入，
// 避免 plantask 与 planmode 强耦合。
type Gate interface {
	Activate()
	Deactivate()
	IsActive() bool
}

// Config 是 plantask Extension 的装配配置。
type Config struct {
	// Store 持久化 HCL 会话（文件/SQLite/内存）。
	Store Store
	// TaskStore 用于 Plan→tasklist 同步（agentcore/tasklist.Store 满足）。
	TaskStore TaskStore
	// Gate 是 planmode 门控（Planning 态激活、批准后关闭）。
	Gate Gate
	// NewSessionID 生成会话 ID；nil 时使用默认实现（case 前缀 + 纳秒时间戳）。
	NewSessionID func(caseID string) string
	// DefaultExpiry 未提供时设置会话超时（AwaitingApproval 24h / AwaitingFeedback 1h）。
	// nil 表示不启用超时。
	DefaultExpiry *SessionExpiry
	// Replanner 根据反馈重新生成计划并合并（03-design §3.3）。
	// 由装配层注入（bootstrap/plantask_bridge.go）；nil 时 workflow_feedback
	// 停在 Replanning 状态，等待外部接入。
	Replanner Replanner
	// AutoEnter 配置自动进入规划态门槛（02-spec §N4）；零值 = 关闭。
	AutoEnter AutoEnterConfig
}

// SessionExpiry 配置不同状态的默认超时时长（02-spec §2.3 / §9 N3）。
type SessionExpiry struct {
	Approval   time.Duration // AwaitingApproval 默认 24h
	Feedback   time.Duration // AwaitingFeedback 默认 1h
	Replanning time.Duration // Replanning 默认 30m
}

// DefaultExpirySettings 返回 02-spec N3 建议的默认超时时长。
func DefaultExpirySettings() SessionExpiry {
	return SessionExpiry{
		Approval:   24 * time.Hour,
		Feedback:   1 * time.Hour,
		Replanning: 30 * time.Minute,
	}
}

// Extension 将 PlanTask HCL 工具集注入 Agent。
//
// 通过 ToolProvider 贡献工具（阶段一：批准门 4 个）：
//   - plan_submit: 提交 Plan（→ AwaitingApproval）
//   - plan_approve: 批准（→ Executing，解除门控）
//   - plan_reject: 驳回（→ Planning，带理由）
//   - plan_revise: 修改意图（→ Planning，LLM 辅助重新生成）
//
// 阶段二追加：workflow_interrupt / workflow_resume / workflow_feedback。
type Extension struct {
	cfg       Config
	agent     *agentcore.Agent
	autoEnter autoEnterState
}

var (
	_ agentcore.Extension    = (*Extension)(nil)
	_ agentcore.ToolProvider = (*Extension)(nil)
)

// NewExtension 创建 HCL 扩展。
// AutoEnter.Rounds 语义：0 = 关闭自动进入（安全默认）；>0 = 连续 N 轮触发。
// 需要自动进入时由装配层显式开启（bootstrap 默认设为 2，见 02-spec §N4）。
func NewExtension(cfg Config) (*Extension, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("plantask: Store is required")
	}
	if cfg.Gate == nil {
		return nil, fmt.Errorf("plantask: Gate is required")
	}
	return &Extension{cfg: cfg}, nil
}

// Name 实现 agentcore.Extension。
func (e *Extension) Name() string { return ExtensionName }

// Init 实现 agentcore.Extension。
// 同一 Extension 实例可能被多个 Agent 共享（例如 handoff 子 Agent 继承父配置），
// 因此 agent 引用只设置一次，避免父 Agent 的事件引用被子 Agent 覆盖。
func (e *Extension) Init(_ context.Context, agent *agentcore.Agent) error {
	if e.agent == nil {
		e.agent = agent
	}
	return nil
}

// Dispose 实现 agentcore.Extension。
func (e *Extension) Dispose() error { return nil }

// Tools 实现 agentcore.ToolProvider，返回 HCL 工具集。
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		newPlanSubmitTool(e),
		newPlanApproveTool(e),
		newPlanRejectTool(e),
		newPlanReviseTool(e),
		newWorkflowInterruptTool(e),
		newWorkflowResumeTool(e),
		newWorkflowFeedbackTool(e),
	}
}

// LatestSession 返回最近创建的未终态会话（TUI 交互命令解析会话用）。
// 无活动会话时返回 ErrNoActiveSession。
func (e *Extension) LatestSession(ctx context.Context) (*PlanTaskSession, error) {
	pending, err := e.cfg.Store.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return nil, ErrNoActiveSession
	}
	return pending[len(pending)-1], nil
}

// Session 返回会话副本（供外部执行器读取当前计划与完成状态）。
func (e *Extension) Session(ctx context.Context, id string) (*PlanTaskSession, error) {
	return e.loadSession(ctx, id)
}

// Persist 保存外部执行器对会话的变更（如 MarkCompleted 后的进度回写）。
func (e *Extension) Persist(ctx context.Context, s *PlanTaskSession) error {
	if s == nil {
		return fmt.Errorf("plantask: nil session")
	}
	return e.cfg.Store.Save(ctx, s)
}

// sessionID 生成会话 ID（可注入覆盖）。
func (e *Extension) sessionID(caseID string) string {
	if e.cfg.NewSessionID != nil {
		return e.cfg.NewSessionID(caseID)
	}
	return fmt.Sprintf("%s_%d", caseID, time.Now().UnixNano())
}

// loadSession 按 ID 加载会话，统一错误映射。
func (e *Extension) loadSession(_ context.Context, id string) (*PlanTaskSession, error) {
	if id == "" {
		return nil, ErrNoActiveSession
	}
	s, err := e.cfg.Store.Load(context.Background(), id)
	if err != nil {
		return nil, ErrNoActiveSession
	}
	return s, nil
}

// save 持久化会话并在迁移时发射状态事件。
func (e *Extension) save(ctx context.Context, s *PlanTaskSession, from Status) error {
	if err := e.cfg.Store.Save(ctx, s); err != nil {
		return err
	}
	if from != s.Status && e.agent != nil {
		e.agent.EventBus().EmitMustDeliver(ctx, agentcore.NewPlanTaskStatusChangedEvent(
			s.ID, s.CaseID, string(from), string(s.Status)))
	}
	return nil
}

// enterPlanning 迁移到 Planning（保持门控激活）。
func (e *Extension) enterPlanning(ctx context.Context, s *PlanTaskSession, from Status) error {
	if err := s.Transition(StatusPlanning); err != nil {
		return err
	}
	e.cfg.Gate.Activate()
	return e.save(ctx, s, from)
}

// gateActive 报告门控是否激活。
func (e *Extension) gateActive() bool { return e.cfg.Gate.IsActive() }

// applyExpiry 按当前状态设置默认超时（仅在未设置时）。
func (e *Extension) applyExpiry(s *PlanTaskSession) {
	if e.cfg.DefaultExpiry == nil || s.ExpiresAt != nil {
		return
	}
	var d time.Duration
	switch s.Status {
	case StatusAwaitingApproval:
		d = e.cfg.DefaultExpiry.Approval
	case StatusAwaitingFeedback:
		d = e.cfg.DefaultExpiry.Feedback
	case StatusReplanning:
		d = e.cfg.DefaultExpiry.Replanning
	default:
		return
	}
	if d > 0 {
		s.SetExpiresAt(time.Now().Add(d))
	}
}
