package domains

import "fmt"

// CaseStage 代表案件在业务层面的生命周期阶段。
// 与 ProjectRecord.Status（active/archived/unreachable）不同，CaseStage
// 表达的是专利/法律业务的办理进展，而非仓储状态。
type CaseStage string

const (
	// CaseStageDrafting 表示案件正在处理中。
	CaseStageDrafting CaseStage = "drafting"

	// CaseStageAmending 表示案件正在修改/补正中（如答复审查意见）。
	CaseStageAmending CaseStage = "amending"

	// CaseStageReviewing 表示案件正在审核中。
	CaseStageReviewing CaseStage = "reviewing"

	// CaseStageResponded 表示已答复（审查意见、复审等），等待官方裁决。
	CaseStageResponded CaseStage = "responded"

	// CaseStageGranted 表示已授权/已注册。
	CaseStageGranted CaseStage = "granted"

	// CaseStageRejected 表示被驳回。
	CaseStageRejected CaseStage = "rejected"

	// CaseStageAbandoned 表示已放弃。
	CaseStageAbandoned CaseStage = "abandoned"
)

// Valid returns true if the stage is a known value.
func (s CaseStage) Valid() bool {
	switch s {
	case CaseStageDrafting, CaseStageAmending, CaseStageReviewing,
		CaseStageResponded, CaseStageGranted, CaseStageRejected, CaseStageAbandoned:
		return true
	default:
		return false
	}
}

// TransitionAllowed checks whether moving from one stage to another is valid.
// Returns an error with reason if the transition is disallowed.
func TransitionAllowed(from, to CaseStage) error {
	// Allowed transitions map: from → set of allowed to-stages.
	allowed := map[CaseStage]map[CaseStage]bool{
		CaseStageDrafting: {
			CaseStageAmending:  true,
			CaseStageReviewing: true,
			CaseStageAbandoned: true,
		},
		CaseStageAmending: {
			CaseStageReviewing: true,
			CaseStageResponded: true,
			CaseStageAbandoned: true,
		},
		CaseStageReviewing: {
			CaseStageDrafting:  true, // 退回修改
			CaseStageAmending:  true,
			CaseStageResponded: true,
			CaseStageGranted:   true,
			CaseStageRejected:  true,
		},
		CaseStageResponded: {
			CaseStageReviewing: true, // 收到新通知后重新审核
			CaseStageGranted:   true,
			CaseStageRejected:  true,
		},
		CaseStageGranted: {
			CaseStageAbandoned: true, // 放弃已授权权利
		},
		CaseStageRejected: {
			CaseStageDrafting: true, // 重新起草
		},
		CaseStageAbandoned: {}, // 终止态，不可转换
	}

	if from == to {
		return nil // 同状态转换总是允许
	}
	if toSet, ok := allowed[from]; ok {
		if toSet[to] {
			return nil
		}
	}
	return fmt.Errorf("不允许从 %q 转换到 %q", from, to)
}

// Case 是业务的案件包装类型，组合 ProjectRecord 的案件元数据
// 与业务生命周期状态。它是"证据驱动的工作台"中案件管理的核心抽象。
type Case struct {
	// Record 是底层注册记录。
	Record ProjectRecord

	// Stage 是案件当前业务阶段。
	Stage CaseStage

	// Description 是案件摘要说明。
	Description string
}

// NewCase 从 ProjectRecord 创建 Case 实例，初始阶段为 CaseStageDrafting。
func NewCase(rec ProjectRecord) *Case {
	return &Case{
		Record: rec,
		Stage:  CaseStageDrafting,
	}
}

// TransitionTo 尝试将案件转换到新阶段。如果转换不被允许则返回错误。
func (c *Case) TransitionTo(stage CaseStage) error {
	if !stage.Valid() {
		return fmt.Errorf("未知的案件阶段: %q", stage)
	}
	if err := TransitionAllowed(c.Stage, stage); err != nil {
		return err
	}
	c.Stage = stage
	return nil
}

// CaseTypeLabel 返回案件类型的中文可读标签。
func (c *Case) CaseTypeLabel() string {
	switch c.Record.CaseType {
	case "发明专利":
		return "发明专利"
	case "实用新型":
		return "实用新型"
	case "外观设计":
		return "外观设计"
	case "商标":
		return "商标"
	case "著作权":
		return "著作权"
	default:
		if c.Record.CaseType == "" {
			return "未分类"
		}
		return c.Record.CaseType
	}
}

// Summary 返回案件的一行摘要。
func (c *Case) Summary() string {
	s := fmt.Sprintf("[%s] %s", c.CaseTypeLabel(), c.Record.Alias)
	if c.Record.FilingNumber != "" {
		s += fmt.Sprintf(" (%s)", c.Record.FilingNumber)
	}
	s += fmt.Sprintf(" — %s", c.Stage)
	return s
}
