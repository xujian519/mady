package claimdrafting

import "strings"

// =============================================================================
// 领域特定规则：机械领域
// =============================================================================

// domainMechanicalRule 检查机械领域产品独立权利要求是否包含必要的要素。
// 依据：审查指南——机械领域产品独立权利要求应包含零部件、配置关系、联系形式。
type domainMechanicalRule struct{ baseRule }

func (r *domainMechanicalRule) Check(claims []Claim, input DraftInput) []Violation {
	if input.TechDomain != DomainMechanical {
		return nil // 仅适用于机械领域
	}

	var violations []Violation
	for _, c := range claims {
		if c.Kind != "independent" {
			continue
		}
		text := c.Preamble + c.Characterized

		// 检查是否包含"连接"、"安装"、"设置"等配置关系表述
		if !hasConfigurationRelation(text) {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityWarning,
				ClaimNumber: c.Number,
				Message:     "机械领域独立权利要求应说明零部件之间的配置关系",
				Suggestion:  "在独立权利要求中补充零部件之间的连接关系或位置关系（如'所述A与B连接'、'所述A设置在B上'）",
			})
		}
	}
	return violations
}

// hasConfigurationRelation 检查是否包含配置关系表述。
var relationIndicators = []string{
	"连接", "安装", "设置", "固定", "布置",
	"位于", "配置", "耦合", "结合", "支承",
	"接触", "邻接", "相对", "围绕",
}

func hasConfigurationRelation(text string) bool {
	for _, ind := range relationIndicators {
		if strings.Contains(text, ind) {
			return true
		}
	}
	return false
}

// =============================================================================
// 领域特定规则：电路领域
// =============================================================================

// domainElectricalRule 检查电路领域产品独立权利要求是否包含必要的要素。
// 依据：审查指南——电路产品应包含元器件、连接关系、电回路、功能描述。
type domainElectricalRule struct{ baseRule }

func (r *domainElectricalRule) Check(claims []Claim, input DraftInput) []Violation {
	if input.TechDomain != DomainElectrical {
		return nil
	}

	var violations []Violation
	for _, c := range claims {
		if c.Kind != "independent" {
			continue
		}
		text := c.Preamble + c.Characterized

		// 检查是否有连接关系的表述
		if !hasElectricalRelation(text) {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityWarning,
				ClaimNumber: c.Number,
				Message:     "电路领域独立权利要求应说明元器件之间的连接关系",
				Suggestion:  "补充元器件之间的导线连接关系或信号传递关系（如'所述A的输入端与B的输出端连接'）",
			})
		}
	}
	return violations
}

var electricalIndicators = []string{
	"输入", "输出", "连接", "电极", "端子",
	"信号", "电压", "电流", "电路", "回路",
	"接地", "电源",
}

func hasElectricalRelation(text string) bool {
	for _, ind := range electricalIndicators {
		if strings.Contains(text, ind) {
			return true
		}
	}
	return false
}

// =============================================================================
// 领域特定规则：化学领域
// =============================================================================

// domainChemicalRule 检查化学组合物独立权利要求是否包含组分及含量。
// 依据：审查指南——组合物独立权利要求应包含组分及含量，含量之和应为100%。
type domainChemicalRule struct{ baseRule }

func (r *domainChemicalRule) Check(claims []Claim, input DraftInput) []Violation {
	if input.TechDomain != DomainChemical {
		return nil
	}

	var violations []Violation
	for _, c := range claims {
		if c.Kind != "independent" {
			continue
		}
		text := c.Preamble + c.Characterized

		// 检查是否有含量百分比表述
		if !strings.Contains(text, "%") && !strings.Contains(text, "份") {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityWarning,
				ClaimNumber: c.Number,
				Message:     "化学组合物独立权利要求应记载各组分的含量或含量范围",
				Suggestion:  "在权利要求中补充各组分的重量/体积百分比或份数比例，并确保各组分含量之和不超过100%",
			})
		}
	}
	return violations
}

// =============================================================================
// 领域特定规则：计算机程序
// =============================================================================

// domainSoftwareRule 检查计算机程序发明的权利要求形式是否恰当。
// 依据：审查指南第二部分第九章§5.2——可写为方法或产品权利要求。
type domainSoftwareRule struct{ baseRule }

func (r *domainSoftwareRule) Check(claims []Claim, input DraftInput) []Violation {
	if input.TechDomain != DomainSoftware {
		return nil
	}

	var violations []Violation
	for _, c := range claims {
		if c.Kind != "independent" {
			continue
		}
		text := c.Preamble + c.Characterized

		// 对于计算机程序发明，如果写成产品权利要求，需要包含存储介质或处理器等要素
		if c.ClaimType == ClaimTypeProduct {
			if !strings.Contains(text, "处理器") && !strings.Contains(text, "存储") &&
				!strings.Contains(text, "计算机") && !strings.Contains(text, "CPU") {
				violations = append(violations, Violation{
					RuleName:    r.Name(),
					RuleBasis:   r.LegalBasis(),
					Severity:    SeverityInfo,
					ClaimNumber: c.Number,
					Message:     "计算机程序发明的产品权利要求应包含处理器/存储器等硬件要素",
					Suggestion:  "在装置/系统权利要求中明确记载处理器（或微处理器）、存储器等硬件部件，并将程序步骤对应为功能模块",
				})
			}
		}
	}
	return violations
}

// =============================================================================
// 领域特定规则：实用新型
// =============================================================================

// domainUtilityModelRule 检查实用新型专利的权利要求类型是否恰当。
// 依据：专利法实施细则第2条——实用新型专利只能有产品权利要求。
type domainUtilityModelRule struct{ baseRule }

func (r *domainUtilityModelRule) Check(claims []Claim, input DraftInput) []Violation {
	// 若无 input，无法判断是否为实用新型，跳过
	if input.Title == "" {
		return nil
	}

	// 通过发明名称判断是否为实用新型
	isUtilityModel := strings.Contains(input.Title, "实用新型")

	var violations []Violation
	if !isUtilityModel {
		return violations
	}

	for _, c := range claims {
		if c.ClaimType == ClaimTypeMethod {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityError,
				ClaimNumber: c.Number,
				Message:     "实用新型专利不允许有方法权利要求",
				Suggestion:  "将方法权利要求修改为产品权利要求，或将方法权利要求从权利要求书中删除",
			})
		}
	}
	return violations
}

// =============================================================================
// 领域特定规则：软件方法→产品转换提示
// =============================================================================

// domainMethodToProductRule 提示软件领域可将方法权利要求同时表达为产品形式。
// 依据：审查指南第二部分第九章§5.2——含计算机程序的发明可以写成方法权利要求，
// 也可以写成装置权利要求（用步骤限定装置）。
type domainMethodToProductRule struct{ baseRule }

func (r *domainMethodToProductRule) Check(claims []Claim, input DraftInput) []Violation {
	var violations []Violation
	if input.TechDomain != DomainSoftware {
		return nil
	}

	// 检查是否同时有方法权利要求和对应的产品权利要求（用步骤限定装置）
	hasMethod := false
	hasApparatusForMethod := false
	for _, c := range claims {
		if c.ClaimType == ClaimTypeMethod && c.Kind == "independent" {
			hasMethod = true
		}
		if c.ClaimType == ClaimTypeProduct && c.Kind == "independent" {
			if strings.Contains(c.Preamble, "用于执行") {
				hasApparatusForMethod = true
			}
		}
	}

	if hasMethod && !hasApparatusForMethod {
		violations = append(violations, Violation{
			RuleName:    r.Name(),
			RuleBasis:   r.LegalBasis(),
			Severity:    SeverityInfo,
			ClaimNumber: 0,
			Message:     "软件领域建议增加'用步骤限定的装置'产品权利要求，以覆盖设备制造商的侵权场景",
			Suggestion:  "在方法权利要求之外，增加一项对应的产品权利要求（如'用于执行权利要求X所述方法的装置，包括用于……的模块'），保护范围不变，但可覆盖使用该方法的设备",
		})
	}
	return violations
}
