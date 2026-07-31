package plantask

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================================
// plan_submit — LLM 提交 Plan 供用户批准
// ============================================================================

// PlanSubmitArgs 是 plan_submit 的输入。
// session_id 可空：空时创建新会话（规划态 → 提交态）。
type PlanSubmitArgs struct {
	SessionID string          `json:"session_id,omitempty" jsonschema:"description=HCL 会话 ID（空 = 新建会话）"`
	CaseID    string          `json:"case_id" jsonschema:"required,description=案件 ID"`
	CaseType  string          `json:"case_type" jsonschema:"required,description=案件类型（如 patentability/invalidation）"`
	Steps     []PlanStepInput `json:"steps" jsonschema:"required,description=计划步骤列表（按执行顺序）"`
	Intent    string          `json:"intent,omitempty" jsonschema:"description=分析意图说明（可选）"`
}

// PlanSubmitResult 是 plan_submit 的输出。
type PlanSubmitResult struct {
	SessionID string   `json:"session_id"`
	Status    string   `json:"status"`
	TaskIDs   []string `json:"task_ids"`
	Summary   string   `json:"summary"`
}

// newPlanSubmitTool 创建 plan_submit 工具。
func newPlanSubmitTool(e *Extension) *agentcore.Tool {
	return agentcore.NewTypedTool(
		"plan_submit",
		"提交执行计划供用户批准。将当前 Plan 步骤同步为任务清单并进入等待批准状态。"+
			"调用前请确保步骤完整、顺序正确；批准前系统处于只读规划态（禁止写操作）。",
		func(ctx context.Context, args PlanSubmitArgs) (PlanSubmitResult, error) {
			if len(args.Steps) == 0 {
				return PlanSubmitResult{}, ErrPlanEmpty
			}
			if args.CaseID == "" {
				return PlanSubmitResult{}, fmt.Errorf("plantask: case_id is required")
			}
			if args.CaseType == "" {
				return PlanSubmitResult{}, fmt.Errorf("plantask: case_type is required")
			}

			snapshots := make([]StepSnapshot, len(args.Steps))
			for i, s := range args.Steps {
				snapshots[i] = s.ToSnapshot()
			}

			// 复用已有会话（须处于 Planning）或新建。
			var (
				session *PlanTaskSession
				from    Status
				created bool
			)
			if args.SessionID != "" {
				s, err := e.loadSession(ctx, args.SessionID)
				if err != nil {
					return PlanSubmitResult{}, err
				}
				session = s
				from = s.Status
			} else {
				session = NewSession(e.sessionID(args.CaseID), args.CaseID, args.CaseType)
				from = StatusPlanning
				created = true
			}

			// 任务同步（Plan → tasklist）。
			taskIDs, err := SyncPlanToTasks(ctx, e.cfg.TaskStore, session, snapshots)
			if err != nil {
				return PlanSubmitResult{}, err
			}

			session.Plan = PlanSnapshot{Steps: snapshots}
			session.TaskIDs = taskIDs
			if created && args.Intent != "" {
				session.ReviseIntent = args.Intent
			}

			// 状态迁移：必须从 Planning 提交。
			if err := session.Transition(StatusAwaitingApproval); err != nil {
				return PlanSubmitResult{}, err
			}
			e.applyExpiry(session)
			if err := e.save(ctx, session, from); err != nil {
				return PlanSubmitResult{}, err
			}

			return PlanSubmitResult{
				SessionID: session.ID,
				Status:    string(session.Status),
				TaskIDs:   taskIDs,
				Summary:   formatPlanSummary(session),
			}, nil
		},
	)
}

// formatPlanSummary 生成供用户审阅的步骤摘要。
func formatPlanSummary(s *PlanTaskSession) string {
	out := fmt.Sprintf("案件 %s（%s）执行计划，共 %d 步：", s.CaseID, s.CaseType, len(s.Plan.Steps))
	for _, step := range s.Plan.Steps {
		out += fmt.Sprintf("\n%d. [%s] %s", step.Order, step.Strategy, step.Description)
	}
	out += "\n\n请输入 plan_approve 批准，或 plan_reject / plan_revise 修改。"
	return out
}
