package plantask

// ============================================================================
// 状态迁移矩阵（02-spec §2.2 合法迁移白名单）
// ============================================================================

// transitionMatrix 定义合法的 from→to 迁移。
// (init) 由 NewSession 处理（直接进入 Planning），不在矩阵内。
var transitionMatrix = map[Status]map[Status]bool{
	StatusPlanning: {
		StatusAwaitingApproval: true,
		StatusCanceled:         true,
		StatusExpired:          true,
	},
	StatusAwaitingApproval: {
		StatusPlanning:  true, // plan_reject / plan_revise（任何 Plan 变更回 Planning）
		StatusExecuting: true, // plan_approve
		StatusCanceled:  true,
		StatusExpired:   true,
	},
	StatusExecuting: {
		StatusAwaitingFeedback: true, // workflow_interrupt
		StatusFinished:         true, // 执行完成
		StatusCanceled:         true,
		StatusExpired:          true,
	},
	StatusAwaitingFeedback: {
		StatusExecuting:  true, // workflow_resume（无改动直接续跑）
		StatusReplanning: true, // workflow_feedback
		StatusCanceled:   true,
		StatusExpired:    true,
	},
	StatusReplanning: {
		StatusExecuting: true, // replan 完成 → 增量续跑
		StatusCanceled:  true,
		StatusExpired:   true,
	},
}

// allowedTransition 报告 from→to 是否为合法迁移。
func allowedTransition(from, to Status) bool {
	if to == StatusExpired {
		// 仅非终态可超时迁移（由 Transition 中的过期检查触发）；
		// 终态（Finished/Canceled/Expired）不参与任何迁移。
		return !isTerminal(from) && from != StatusExpired
	}
	dsts, ok := transitionMatrix[from]
	if !ok {
		return false
	}
	return dsts[to]
}
