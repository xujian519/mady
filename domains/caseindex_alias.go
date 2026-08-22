package domains

import "github.com/xujian519/mady/domains/caseindex"

// 案件索引类型的别名桥接。实现已迁移至 domains/caseindex 子包（含 SQLite
// 驱动依赖），根包保留别名以维持既有调用方（bootstrap/cmd 及本包 case_ext_*）
// 的兼容性。别名是零成本转发，依赖方向单一：domains → domains/caseindex。

type (
	// CaseIndex 是案件索引库，实现见 domains/caseindex。
	CaseIndex = caseindex.CaseIndex
	// CaseRecord 是案件主记录。
	CaseRecord = caseindex.CaseRecord
	// CaseDocument 是案件关联文档记录。
	CaseDocument = caseindex.CaseDocument
	// CaseEvent 是案件生命周期事件记录。
	CaseEvent = caseindex.CaseEvent
	// CasePath 是案件关联路径记录。
	CasePath = caseindex.CasePath
	// CaseSearchQuery 是案件检索查询参数。
	CaseSearchQuery = caseindex.CaseSearchQuery
)

// NewCaseIndex 打开（必要时创建）案件索引数据库。
func NewCaseIndex(dbPath string) (*CaseIndex, error) { return caseindex.NewCaseIndex(dbPath) }

// 案件标识阶段常量，随实现迁至 caseindex 包。
const (
	StageDrafting  = caseindex.StageDrafting
	StageFiled     = caseindex.StageFiled
	StagePublished = caseindex.StagePublished

	CaseStatusActive   = caseindex.CaseStatusActive
	CaseStatusArchived = caseindex.CaseStatusArchived
	CaseStatusGranted  = caseindex.CaseStatusGranted
	CaseStatusRejected = caseindex.CaseStatusRejected

	DocConfirmation = caseindex.DocConfirmation
	DocFiling       = caseindex.DocFiling
	DocAcceptance   = caseindex.DocAcceptance
	DocPublication  = caseindex.DocPublication
	DocOfficeAction = caseindex.DocOfficeAction
	DocGrant        = caseindex.DocGrant
	DocRejection    = caseindex.DocRejection
	DocOther        = caseindex.DocOther
)
