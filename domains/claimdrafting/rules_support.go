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
// 增强版：同时检查特征类别多样性（结构+连接+功能各至少1类）。
type supportFunctionalRule struct{ baseRule }

func (r *supportFunctionalRule) Check(claims []Claim, input DraftInput) []Violation {
	var violations []Violation
	hasDetailedEmbodiment := len(input.Features) >= 3

	// 检查特征类别多样性
	catSet := make(map[string]bool)
	for _, f := range input.Features {
		catSet[f.Category] = true
	}
	hasCategoryDiversity := len(catSet) >= 2

	for _, c := range claims {
		text := c.String()
		if !strings.Contains(text, "用于") {
			continue
		}

		var hints []string
		if !hasDetailedEmbodiment {
			hints = append(hints, "特征数量较少（少于3个），建议补充更多具体实施方式")
		}
		if !hasCategoryDiversity {
			hints = append(hints, "特征类别单一，建议包含结构类、连接关系类和功能类特征各至少一项")
		}

		if len(hints) > 0 {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityWarning,
				ClaimNumber: c.Number,
				Message:     "功能性限定可能需要更充分的说明书支持： " + strings.Join(hints, "；"),
				Suggestion:  "1. 确保说明书中记载了实现该功能的具体结构或步骤；2. 若该功能所属领域技术人员不可预见其他替换方式，应同时记载结构特征",
			})
		}
	}
	return violations
}

// =============================================================================
// 支持性规则：功能性限定占比检查
// =============================================================================

// supportFunctionalVarietyRule 检查权利要求中功能性限定的占比是否过高。
// 依据：审查指南第二部分第二章§3.2.1——过度依赖功能性限定可能导致权利要求不清楚。
// 如果文本中功能性限定（以"用于"为代表）占比过高，且缺乏结构特征，则给出警告。
type supportFunctionalVarietyRule struct{ baseRule }

func (r *supportFunctionalVarietyRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		if c.Kind != "independent" {
			continue
		}
		text := c.Preamble + " " + c.Characterized
		if text == "" {
			continue
		}

		// 统计功能性限定关键词出现次数
		funcKeywords := []string{"用于", "以", "以便", "使得", "从而", "适于", "被配置为"}
		funcCount := 0
		for _, kw := range funcKeywords {
			funcCount += strings.Count(text, kw)
		}

		// 统计结构性关键词出现次数
		strucKeywords := []string{"包括", "包含", "由…组成", "连接", "设置", "安装", "固定", "所述"}
		strucCount := 0
		for _, kw := range strucKeywords {
			strucCount += strings.Count(text, kw)
		}

		// 如果功能性关键词数量大幅超过结构性关键词，警告
		if funcCount > 0 && strucCount == 0 {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityWarning,
				ClaimNumber: c.Number,
				Message:     "权利要求全部使用功能性限定，缺乏结构特征描述",
				Suggestion:  "在权利要求中补充至少一项结构性限定（如'所述装置包括……'、'所述模块与……连接'），以获得清楚的支持",
			})
		} else if funcCount > 2*strucCount && funcCount >= 3 {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityWarning,
				ClaimNumber: c.Number,
				Message:     "功能性限定占比过高，可能导致权利要求不清楚或得不到说明书支持",
				Suggestion:  "减少过多功能/效果描述，增加具体的结构特征（如组成部分、连接关系、位置配置等）",
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

// =============================================================================
// 支持性规则：马库什单一性检查
// =============================================================================

// supportMarkushUnityRule 检查马库什权利要求是否满足单一性要求。
// 依据：审查指南第二部分第十章§4.3——马库什权利要求中的可选化合物必须具有
// 共同结构（构成与现有技术的区别特征）并对共同性能或作用必不可少。
type supportMarkushUnityRule struct{ baseRule }

func (r *supportMarkushUnityRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		text := c.Preamble + " " + c.Characterized
		if !strings.Contains(text, "式(") && !strings.Contains(text, "式（") {
			continue // 不是马库什权利要求
		}

		// 检查是否有"其中"或"R1/R2"等取代基定义
		hasSubstituentDef := strings.Contains(text, "其中") &&
			(strings.Contains(text, "R1") || strings.Contains(text, "R2") ||
				strings.Contains(text, "R ") || strings.Contains(text, "X ") ||
				strings.Contains(text, "Y "))

		if !hasSubstituentDef {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityWarning,
				ClaimNumber: c.Number,
				Message:     "马库什权利要求缺少取代基定义（'其中R1…R2…'），可能无法满足单一性要求",
				Suggestion:  "请补充各取代基（R1、R2等的定义），确保所有可选择的化合物具有共同结构",
			})
		}

		// 检查是否只有一个通式（无共同结构提示）
		hasCoreStructure := strings.Contains(text, "选自由") || strings.Contains(text, "选自") ||
			strings.Contains(text, "相同") || strings.Contains(text, "不同") ||
			strings.Contains(text, "独立地")
		if !hasCoreStructure {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityInfo,
				ClaimNumber: c.Number,
				Message:     "马库什权利要求宜明示取代基的选择范围（如'R1选自由H和C1-C6烷基组成的组'）",
				Suggestion:  "通过'选自由…组成的组'或'独立地选自'等表述限定各取代基的范围",
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
