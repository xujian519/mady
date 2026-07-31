package chat

// chat_app_plantask.go 处理 PlanTask HCL 事件的 UI 呈现：
// 状态迁移 / 反馈注入 / 执行中断在聊天流中以系统消息提示，
// 让用户随时感知人机协作闭环的进展。

import (
	"fmt"
)

// onPlanTaskStatusChanged 在 HCL 会话状态迁移时提示。
func (a *ChatApp) onPlanTaskStatusChanged(e ChatEvent) {
	ev, ok := e.(PlanTaskStatusChangedChatEvent)
	if !ok {
		return
	}
	a.PrintSystem(fmt.Sprintf("📋 HCL 状态 [%s]: %s → %s", ev.CaseID, ev.FromStatus, ev.ToStatus))
}

// onPlanTaskFeedbackAdded 在用户反馈注入时提示。
func (a *ChatApp) onPlanTaskFeedbackAdded(e ChatEvent) {
	ev, ok := e.(PlanTaskFeedbackAddedChatEvent)
	if !ok {
		return
	}
	a.PrintSystem(fmt.Sprintf("💬 反馈已注入: %s", ev.Text))
}

// onPlanTaskInterrupted 在执行中断时提示后续操作。
func (a *ChatApp) onPlanTaskInterrupted(e ChatEvent) {
	ev, ok := e.(PlanTaskInterruptedChatEvent)
	if !ok {
		return
	}
	reason := ev.Reason
	if reason == "" {
		reason = "用户请求暂停"
	}
	a.PrintSystem(fmt.Sprintf("⏸️ HCL 执行中断: %s（/feedback 提意见，/resume 直接续跑）", reason))
}
