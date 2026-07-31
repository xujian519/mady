package plantask

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================================
// plan_revise — 用户表达修改意图，回到规划态由 LLM 重新生成（N2 建议结论）
// ============================================================================

// PlanReviseArgs 是 plan_revise 的输入。
// revise_intent 是用户的修改意图（自由文本），LLM 据此重新生成新 Plan。
type PlanReviseArgs struct {
	SessionID    string `json:"session_id" jsonschema:"required,description=HCL 会话 ID"`
	ReviseIntent string `json:"revise_intent" jsonschema:"required,description=修改意图（如：增加检索美国同族的步骤）"`
}

// PlanReviseResult 是 plan_revise 的输出。
type PlanReviseResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

// newPlanReviseTool 创建 plan_revise 工具。
func newPlanReviseTool(e *Extension) *agentcore.Tool {
	return agentcore.NewTypedTool(
		"plan_revise",
		"根据用户的修改意图回到规划态重新生成计划。调用后处于规划态（只读），"+
			"请按修改意图重新生成步骤并再次调用 plan_submit 提交。",
		func(ctx context.Context, args PlanReviseArgs) (PlanReviseResult, error) {
			if args.ReviseIntent == "" {
				return PlanReviseResult{}, ErrFeedbackEmpty
			}
			s, err := e.loadSession(ctx, args.SessionID)
			if err != nil {
				return PlanReviseResult{}, err
			}
			from := s.Status
			// 修改意图入反馈日志 + 会话字段，供重新规划使用。
			s.AddFeedback(args.ReviseIntent, "")
			s.ReviseIntent = args.ReviseIntent
			if err := e.enterPlanning(ctx, s, from); err != nil {
				return PlanReviseResult{}, err
			}
			return PlanReviseResult{
				SessionID: s.ID,
				Status:    string(s.Status),
				Message:   "已回到规划态，请按修改意图重新生成计划并提交",
			}, nil
		},
	)
}
