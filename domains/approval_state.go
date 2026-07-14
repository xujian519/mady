package domains

import "fmt"

// ApprovalState 表示审批流程的当前阶段。
//
// 状态转换图：
//
//	                     ┌──────────┐
//	                     │ drafted  │
//	                     └────┬─────┘
//	                          │ ───→ canceled
//	                     ┌────▼─────┐
//	                     │ pending  │
//	                     │ _approval│
//	                     └────┬─────┘
//	                    ┌─────┼──────┐
//	                    │     │      │
//	               ┌────▼┐ ┌─▼──┐ ┌──▼───┐
//	               │ap-  │ │mo- │ │rejec-│
//	               │pro- │ │dif-│ │ted   │
//	               │ved  │ │fied│ │      │
//	               └─────┘ └────┘ └──────┘
//
//	expired ←─── 以上任意状态均可超时过期
type ApprovalState string

const (
	StateNone            ApprovalState = ""
	StateDrafted         ApprovalState = "drafted"
	StatePendingApproval ApprovalState = "pending_approval"
	StateApproved        ApprovalState = "approved"
	StateModified        ApprovalState = "modified"
	StateRejected        ApprovalState = "rejected"
	StateCanceled        ApprovalState = "canceled"
	StateExpired         ApprovalState = "expired"
)

// Valid returns true if the state is a known value.
func (s ApprovalState) Valid() bool {
	switch s {
	case StateNone, StateDrafted, StatePendingApproval,
		StateApproved, StateModified, StateRejected,
		StateCanceled, StateExpired:
		return true
	default:
		return false
	}
}

// IsTerminal returns true if the state is a terminal (no further transitions).
func (s ApprovalState) IsTerminal() bool {
	switch s {
	case StateApproved, StateModified, StateRejected,
		StateCanceled, StateExpired:
		return true
	default:
		return false
	}
}

// TransitionAllowed checks whether moving from one approval state to another
// is valid according to the state machine.
func (s ApprovalState) TransitionAllowed(to ApprovalState) error {
	if !s.Valid() {
		return fmt.Errorf("未知的审批状态: %q", s)
	}
	if !to.Valid() {
		return fmt.Errorf("未知的目标状态: %q", to)
	}
	if s == to {
		return nil
	}
	if s.IsTerminal() {
		return fmt.Errorf("终止态 %q 不允许转换到 %q", s, to)
	}

	// Allowed transitions map: from → allowed to-states.
	allowed := map[ApprovalState]map[ApprovalState]bool{
		StateDrafted: {
			StatePendingApproval: true,
			StateCanceled:        true,
		},
		StatePendingApproval: {
			StateApproved: true,
			StateModified: true,
			StateRejected: true,
			StateExpired:  true,
			StateCanceled: true,
		},
	}

	if toSet, ok := allowed[s]; ok {
		if toSet[to] {
			return nil
		}
	}
	return fmt.Errorf("不允许从 %q 转换到 %q", s, to)
}

// ApprovalRecordState 是 ApprovalRecord 的审批状态包装，将状态机与审批记录关联。
type ApprovalRecordState struct {
	Record ApprovalRecord
	State  ApprovalState
}

// NewApprovalRecordState 从 ApprovalRecord 创建包含状态机的包装。
func NewApprovalRecordState(record ApprovalRecord) *ApprovalRecordState {
	state := StateDrafted
	if record.Decision != "" {
		switch record.Decision {
		case DecisionAdopted:
			state = StateApproved
		case DecisionModified:
			state = StateModified
		case DecisionRejected:
			state = StateRejected
		}
	}
	// 从 Record 的 State 字段恢复（持久化后重建）。
	// 优先使用显式存储的状态，因为它包含 canceled/expired 等 Decision 无法表达的状态。
	if record.State != "" {
		state = record.State
	}
	return &ApprovalRecordState{
		Record: record,
		State:  state,
	}
}

// SubmitForApproval 将状态转换到 pending_approval。
func (ars *ApprovalRecordState) SubmitForApproval() error {
	if err := ars.State.TransitionAllowed(StatePendingApproval); err != nil {
		return err
	}
	ars.State = StatePendingApproval
	return nil
}

// Approve 批准当前待审批记录。
func (ars *ApprovalRecordState) Approve() error {
	if err := ars.State.TransitionAllowed(StateApproved); err != nil {
		return err
	}
	ars.State = StateApproved
	ars.Record.Decision = DecisionAdopted
	return nil
}

// Modify 修改并批准（部分采纳）。
func (ars *ApprovalRecordState) Modify(modifiedOutput, feedback string) error {
	if err := ars.State.TransitionAllowed(StateModified); err != nil {
		return err
	}
	ars.State = StateModified
	ars.Record.Decision = DecisionModified
	ars.Record.ModifiedOutput = modifiedOutput
	ars.Record.Feedback = feedback
	return nil
}

// Reject 拒绝当前待审批记录。
func (ars *ApprovalRecordState) Reject(feedback string) error {
	if err := ars.State.TransitionAllowed(StateRejected); err != nil {
		return err
	}
	ars.State = StateRejected
	ars.Record.Decision = DecisionRejected
	ars.Record.Feedback = feedback
	return nil
}

// Cancel 取消审批流程。
func (ars *ApprovalRecordState) Cancel() error {
	if err := ars.State.TransitionAllowed(StateCanceled); err != nil {
		return err
	}
	ars.State = StateCanceled
	return nil
}
