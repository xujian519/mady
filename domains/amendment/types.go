package amendment

// =============================================================================
// 修改类型
// =============================================================================

// ModType 表示专利申请文件修改的类型。
type ModType string

const (
	ModActive    ModType = "active"     // 主动修改（实审请求时/进入实审3个月内）
	ModPassive   ModType = "passive"    // 被动修改（针对审查意见通知书）
	ModExOfficio ModType = "ex_officio" // 依职权修改（审查员自行修改明显错误）
)

// =============================================================================
// 修改检查输入
// =============================================================================

// CheckInput 是修改合规性检查的输入数据。
type CheckInput struct {
	OriginalClaims   string  `json:"original_claims"`
	OriginalSpec     string  `json:"original_specification"`
	AmendedClaims    string  `json:"amended_claims"`
	AmendedSpec      string  `json:"amended_specification"`
	ModificationType ModType `json:"modification_type"`
	OfficeActionText string  `json:"office_action_text,omitempty"` // 被动修改时填写
}

// =============================================================================
// 检查结果
// =============================================================================

// CheckResult 记录编译型规则检查的完整结果。
// 此结构既被 amendment 包内部使用，也被 handler 层（domains/rules）用作
// 格式化输出的基础。OriginalLength/AmendedLength/OfficeActionSummary
// 由 handler 层填充（非编译型规则的计算逻辑）。
type CheckResult struct {
	HasClaimChanges bool    `json:"has_claim_changes"`
	HasSpecChanges  bool    `json:"has_spec_changes"`
	ModType         ModType `json:"mod_type"`

	// 基本信息（由 handler 层设置）
	OriginalLength      int    `json:"original_length,omitempty"`
	AmendedLength       int    `json:"amended_length,omitempty"`
	OfficeActionSummary string `json:"office_action_summary,omitempty"`

	// 违规列表
	Violations      []Violation `json:"violations,omitempty"`
	TotalViolations int         `json:"total_violations"`

	// 综合判断
	IsCompliant bool   `json:"is_compliant"`
	Note        string `json:"note,omitempty"`
}

// Violation 记录一条修改违规信息。
type Violation struct {
	RuleName  string `json:"rule_name"`
	Severity  string `json:"severity"` // error / warning / info
	Message   string `json:"message"`
	Detail    string `json:"detail,omitempty"`
	Recommend string `json:"recommend,omitempty"`
}
