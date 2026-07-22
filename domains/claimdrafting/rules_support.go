package claimdrafting

import "strings"

// =============================================================================
// 支持性规则：以说明书为依据（概括不得超范围）
// =============================================================================

// supportEmbodimentRule 检查权利要求的概括是否得到说明书实施例支持。
// 依据：专利法第26条第4款——权利要求书应当以说明书为依据。
// 检查逻辑：如果 Description 为空但 Claims 使用了上位概括，给出警告。
type supportEmbodimentRule struct{ baseRule }

func (r *supportEmbodimentRule) Check(claims []Claim, input DraftInput) []Violation {
	var violations []Violation
	hasEmbodiment := strings.TrimSpace(input.Description) != "" || len(input.Features) > 0

	for _, c := range claims {
		// 检查独立权利要求是否使用了上位概念/功能性限定
		characterized := c.Characterized
		if c.Kind == "independent" && characterized != "" {
			// 检测是否使用了功能性限定特征
			if strings.Contains(characterized, "用于") && !hasEmbodiment {
				violations = append(violations, Violation{
					RuleName:    r.Name(),
					RuleBasis:   r.LegalBasis(),
					Severity:    SeverityWarning,
					ClaimNumber: c.Number,
					Message:     "独立权利要求使用了功能性限定（'用于……'），但可能缺少足够的实施例支持",
					Suggestion:  "在说明书中补充至少一个实现该功能的具体实施方式，并确保权利要求中的概括未超出说明书公开范围",
				})
			}
		}
	}
	return violations
}

// =============================================================================
// 支持性规则：功能性限定使用恰当
// =============================================================================

// supportFunctionalRule 检查功能性限定的使用是否恰当。
// 依据：审查指南第二部分第二章§3.2.1——功能性限定以说明书中有具体实施方式为前提。
type supportFunctionalRule struct{ baseRule }

func (r *supportFunctionalRule) Check(claims []Claim, input DraftInput) []Violation {
	var violations []Violation
	hasDetailedEmbodiment := len(input.Features) >= 3

	for _, c := range claims {
		text := c.String()
		if !strings.Contains(text, "用于") {
			continue
		}

		// 检测功能性限定的具体程度
		if !hasDetailedEmbodiment {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityWarning,
				ClaimNumber: c.Number,
				Message:     "功能性限定可能需要更充分的说明书支持",
				Suggestion:  "1. 确保说明书中记载了实现该功能的具体结构或步骤；2. 若该功能所属领域技术人员不可预见其他替换方式，应同时记载结构特征",
			})
		}
	}
	return violations
}

// =============================================================================
// 支持性规则：不得为纯功能性权利要求
// =============================================================================

// supportPureFunctionalRule 检查是否出现了纯功能性权利要求。
// 依据：审查指南第二部分第二章§3.2.1——纯功能性权利要求得不到说明书支持。
type supportPureFunctionalRule struct{ baseRule }

func (r *supportPureFunctionalRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		if c.Kind != "independent" {
			continue
		}
		text := c.Preamble + c.Characterized
		// 检测是否完全没有结构特征，仅用功能和效果描述
		if !hasStructuralFeature(text) {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityError,
				ClaimNumber: c.Number,
				Message:     "独立权利要求可能是纯功能性权利要求：未记载任何结构特征",
				Suggestion:  "在独立权利要求中补充实现所述功能所必需的结构特征（如'装置包括……'），至少应结合一定的结构特征进行限定",
			})
		}
	}
	return violations
}

// hasStructuralFeature 检查文本是否包含结构特征的表述。
var structuralIndicators = []string{
	"包括", "包含",
	"组成", "构成",
	"装置", "设备", "系统", "部件", "模块",
	"单元", "组件", "元件", "电路", "机构",
	"所述",
}

func hasStructuralFeature(text string) bool {
	for _, ind := range structuralIndicators {
		if strings.Contains(text, ind) {
			return true
		}
	}
	return false
}
