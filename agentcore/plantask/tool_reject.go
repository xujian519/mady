package plantask

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================================
// plan_reject — 用户驳回计划，回到规划态重来
// ============================================================================

// PlanRejectArgs 是 plan_reject 的输入。reason 必填（驳回必带理由）。
type PlanRejectArgs struct {
	SessionID string `json:"session_id" jsonschema:"required,description=HCL 会话 ID"`
	Reason    string `json:"reason" jsonschema:"required,description=驳回理由（将注入规划上下文）"`
}

// PlanRejectResult 是 plan_reject 的输出。
type PlanRejectResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

// newPlanRejectTool 创建 plan_reject 工具。
func newPlanRejectTool(e *Extension) *agentcore.Tool {
	return agentcore.NewTypedTool(
		"plan_reject",
		"驳回执行计划，回到规划态重新生成。驳回理由将作为反馈记录并影响下一次规划。",
		func(ctx context.Context, args PlanRejectArgs) (PlanRejectResult, error) {
			if args.Reason == "" {
				return PlanRejectResult{}, ErrFeedbackEmpty
			}
			s, err := e.loadSession(ctx, args.SessionID)
			if err != nil {
				return PlanRejectResult{}, err
			}
			from := s.Status
			// 驳回理由入反馈日志（审计留痕），并作为重新规划的上下文。
			s.AddFeedback(args.Reason, "")
			if err := e.enterPlanning(ctx, s, from); err != nil {
				return PlanRejectResult{}, err
			}
			return PlanRejectResult{
				SessionID: s.ID,
				Status:    string(s.Status),
				Message:   "计划已驳回，请根据反馈重新规划",
			}, nil
		},
	)
}
