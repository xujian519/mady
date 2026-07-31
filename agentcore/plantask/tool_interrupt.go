package plantask

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================================
// workflow_interrupt — 用户/TUI 请求暂停执行（03-design §1.2）
//
// 实现为普通 Tool：先完成会话状态迁移（Executing → AwaitingFeedback），
// 再返回 agentcore.NewInterruptError 走既有中断通道（executor 捕获 r.Err
// → StatusInterrupted → Resume() 重放恢复）。
// ============================================================================

// WorkflowInterruptArgs 是 workflow_interrupt 的输入。
type WorkflowInterruptArgs struct {
	SessionID string `json:"session_id" jsonschema:"required,description=HCL 会话 ID"`
	StepID    string `json:"step_id,omitempty" jsonschema:"description=中断时的步骤（可选）"`
	Reason    string `json:"reason,omitempty" jsonschema:"description=中断原因（默认：用户请求暂停）"`
}

// WorkflowInterruptResult 是 workflow_interrupt 的输出（正常路径不会到达）。
type WorkflowInterruptResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

// newWorkflowInterruptTool 创建 workflow_interrupt 工具。
func newWorkflowInterruptTool(e *Extension) *agentcore.Tool {
	return agentcore.NewTypedTool(
		"workflow_interrupt",
		"请求暂停当前执行（用户触发）。会话进入等待反馈状态，执行可从断点恢复。",
		func(ctx context.Context, args WorkflowInterruptArgs) (WorkflowInterruptResult, error) {
			s, err := e.loadSession(ctx, args.SessionID)
			if err != nil {
				return WorkflowInterruptResult{}, err
			}
			from := s.Status
			if err := s.Transition(StatusAwaitingFeedback); err != nil {
				return WorkflowInterruptResult{}, err
			}
			reason := args.Reason
			if reason == "" {
				reason = "用户请求暂停"
			}
			s.Interrupt = &InterruptEntry{StepID: args.StepID, Reason: reason}
			if err := e.save(ctx, s, from); err != nil {
				return WorkflowInterruptResult{}, err
			}
			if e.agent != nil {
				e.agent.EmitEvent(agentcore.NewPlanTaskInterruptedEvent(s.ID, args.StepID, reason))
			}
			// 走既有中断通道（agent_run_tool.go:181,203-221）。
			return WorkflowInterruptResult{
					SessionID: s.ID,
					Status:    string(s.Status),
				}, agentcore.NewInterruptErrorWithData(reason, map[string]any{
					"plantask_session": s.ID,
					"step_id":          args.StepID,
				})
		},
	)
}
