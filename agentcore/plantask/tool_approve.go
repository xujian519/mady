package plantask

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================================
// plan_approve — 用户批准计划，开始执行
// ============================================================================

// PlanApproveArgs 是 plan_approve 的输入。
type PlanApproveArgs struct {
	SessionID string `json:"session_id" jsonschema:"required,description=HCL 会话 ID"`
}

// PlanApproveResult 是 plan_approve 的输出。
type PlanApproveResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

// newPlanApproveTool 创建 plan_approve 工具。
func newPlanApproveTool(e *Extension) *agentcore.Tool {
	return agentcore.NewTypedTool(
		"plan_approve",
		"批准执行计划并开始执行。仅在等待批准状态下可用；批准后解除只读门控。",
		func(ctx context.Context, args PlanApproveArgs) (PlanApproveResult, error) {
			s, err := e.loadSession(ctx, args.SessionID)
			if err != nil {
				return PlanApproveResult{}, err
			}
			from := s.Status
			if err := s.Transition(StatusExecuting); err != nil {
				return PlanApproveResult{}, err
			}
			e.cfg.Gate.Deactivate()
			if err := e.save(ctx, s, from); err != nil {
				return PlanApproveResult{}, err
			}
			return PlanApproveResult{
				SessionID: s.ID,
				Status:    string(s.Status),
				Message:   "计划已批准，开始执行",
			}, nil
		},
	)
}
