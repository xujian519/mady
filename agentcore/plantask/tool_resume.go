package plantask

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================================
// workflow_resume — 用户无改动直接续跑（AwaitingFeedback → Executing）
// ============================================================================

// WorkflowResumeArgs 是 workflow_resume 的输入。
type WorkflowResumeArgs struct {
	SessionID string `json:"session_id" jsonschema:"required,description=HCL 会话 ID"`
}

// WorkflowResumeResult 是 workflow_resume 的输出。
type WorkflowResumeResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

// newWorkflowResumeTool 创建 workflow_resume 工具。
func newWorkflowResumeTool(e *Extension) *agentcore.Tool {
	return agentcore.NewTypedTool(
		"workflow_resume",
		"无改动直接恢复执行（用户触发）。仅等待反馈状态可用。",
		func(ctx context.Context, args WorkflowResumeArgs) (WorkflowResumeResult, error) {
			s, err := e.loadSession(ctx, args.SessionID)
			if err != nil {
				return WorkflowResumeResult{}, err
			}
			from := s.Status
			if err := s.Transition(StatusExecuting); err != nil {
				return WorkflowResumeResult{}, err
			}
			if err := e.save(ctx, s, from); err != nil {
				return WorkflowResumeResult{}, err
			}
			return WorkflowResumeResult{
				SessionID: s.ID,
				Status:    string(s.Status),
				Message:   "已恢复执行",
			}, nil
		},
	)
}

// ============================================================================
// workflow_feedback — 用户反馈 → 重新规划 → 增量续跑
// ============================================================================

// WorkflowFeedbackArgs 是 workflow_feedback 的输入。
type WorkflowFeedbackArgs struct {
	SessionID string `json:"session_id" jsonschema:"required,description=HCL 会话 ID"`
	Feedback  string `json:"feedback" jsonschema:"required,description=反馈文本；可用 \"重跑:步骤A,步骤B\" 显式要求重跑已完成步骤"`
	StepID    string `json:"step_id,omitempty" jsonschema:"description=反馈针对的步骤（可选）"`
}

// WorkflowFeedbackResult 是 workflow_feedback 的输出。
type WorkflowFeedbackResult struct {
	SessionID     string   `json:"session_id"`
	Status        string   `json:"status"`
	SkipHashes    []string `json:"skip_hashes,omitempty"`
	RemovedHashes []string `json:"removed_hashes,omitempty"`
	Message       string   `json:"message"`
}

// newWorkflowFeedbackTool 创建 workflow_feedback 工具。
// 流程：AwaitingFeedback → Replanning →（Replanner 执行合并）→ Executing。
// Replanner 未配置时停在 Replanning（等待装配层接入真实规划器）。
func newWorkflowFeedbackTool(e *Extension) *agentcore.Tool {
	return agentcore.NewTypedTool(
		"workflow_feedback",
		"注入用户反馈并重新规划执行路径（用户触发）。反馈将影响后续步骤生成，"+
			"已完成步骤默认保持完成状态（可用\"重跑:步骤A\"显式要求重跑）。",
		func(ctx context.Context, args WorkflowFeedbackArgs) (WorkflowFeedbackResult, error) {
			if args.Feedback == "" {
				return WorkflowFeedbackResult{}, ErrFeedbackEmpty
			}
			// 2000 rune 截断（03-design §5.4，按码点计数避免中文截断）。
			r := []rune(args.Feedback)
			if len(r) > 2000 {
				args.Feedback = string(r[:2000])
			}
			s, err := e.loadSession(ctx, args.SessionID)
			if err != nil {
				return WorkflowFeedbackResult{}, err
			}
			from := s.Status
			if err := s.Transition(StatusReplanning); err != nil {
				return WorkflowFeedbackResult{}, err
			}
			s.AddFeedback(args.Feedback, args.StepID)
			if e.agent != nil {
				e.agent.EmitEvent(agentcore.NewPlanTaskFeedbackAddedEvent(s.ID, args.Feedback, args.StepID))
			}

			var skip, removed []string
			if e.cfg.Replanner != nil {
				sk, rm, err := e.cfg.Replanner.Replan(ctx, s, args.Feedback)
				if err != nil {
					return WorkflowFeedbackResult{}, err
				}
				skip, removed = sk, rm
				// 合并完成 → 回执行态。
				if err := s.Transition(StatusExecuting); err != nil {
					return WorkflowFeedbackResult{}, err
				}
			}
			e.applyExpiry(s)
			if err := e.save(ctx, s, from); err != nil {
				return WorkflowFeedbackResult{}, err
			}
			return WorkflowFeedbackResult{
				SessionID:     s.ID,
				Status:        string(s.Status),
				SkipHashes:    skip,
				RemovedHashes: removed,
				Message:       "反馈已注入并重新规划",
			}, nil
		},
	)
}

// mergeSetsFrom 已废弃：合并结果由 Replanner.Replan 直接返回。
