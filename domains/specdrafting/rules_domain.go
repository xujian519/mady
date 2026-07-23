package specdrafting

import (
	"strconv"
	"strings"
)

// =============================================================================
// 领域特定规则（5 条）
// =============================================================================

type domainMechanicalRule struct{ baseRule }

func (r *domainMechanicalRule) Check(spec *SpecOutput, input SpecInput) []Violation {
	if input.TechDomain != DomainMechanical || spec == nil {
		return nil
	}
	allText := ""
	for _, sec := range spec.Sections {
		allText += " " + sec.Content
	}
	if !containsAnyOf(allText, []string{"连接", "固定", "安装", "支撑", "设置", "壳体", "支架", "连杆", "齿轮", "弹簧"}) {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityWarning,
			Message:    "机械领域应描述零部件及其配置关系和联系形式",
			Suggestion: "请补充零部件的具体结构、连接关系和位置配置",
		}}
	}
	return nil
}

type domainElectricalRule struct{ baseRule }

func (r *domainElectricalRule) Check(spec *SpecOutput, input SpecInput) []Violation {
	if input.TechDomain != DomainElectrical || spec == nil {
		return nil
	}
	allText := ""
	for _, sec := range spec.Sections {
		allText += " " + sec.Content
	}
	if !containsAnyOf(allText, []string{"电路", "电压", "电流", "信号", "电极", "导线", "传感器", "电阻", "电容", "晶体管"}) {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityWarning,
			Message:    "电学领域应描述元器件、连接关系和电回路功能",
			Suggestion: "请补充电路元器件的具体类型、连接方式和信号路径",
		}}
	}
	return nil
}

type domainChemicalRule struct{ baseRule }

func (r *domainChemicalRule) Check(spec *SpecOutput, input SpecInput) []Violation {
	if input.TechDomain != DomainChemical || spec == nil {
		return nil
	}
	allText := ""
	for _, sec := range spec.Sections {
		allText += " " + sec.Content
	}
	var issues []Violation
	if !containsAnyOf(allText, []string{"组分", "含量", "百分比", "重量", "摩尔", "质量", "%"}) {
		issues = append(issues, Violation{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityWarning,
			Message:    "化学领域应公开组合物的组分及含量",
			Suggestion: "请补充各组分及其含量范围",
		})
	}
	if !containsAnyOf(allText, []string{"实验", "测试", "检测", "数据", "结果"}) {
		issues = append(issues, Violation{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityWarning,
			Message:    "化学领域通常需要实验数据证实技术效果",
			Suggestion: "请补充具体实施例的实验数据",
		})
	}
	return issues
}

type domainSoftwareRule struct{ baseRule }

func (r *domainSoftwareRule) Check(spec *SpecOutput, input SpecInput) []Violation {
	if input.TechDomain != DomainSoftware || spec == nil {
		return nil
	}
	allText := ""
	for _, sec := range spec.Sections {
		allText += " " + sec.Content
	}
	if !containsAnyOf(allText, []string{"步骤", "方法", "数据", "处理", "算法", "模块", "程序"}) {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityWarning,
			Message:    "软件领域应描述方法步骤或功能模块架构",
			Suggestion: "请补充具体的方法步骤流程或功能模块架构",
		}}
	}
	return nil
}

// =============================================================================
// 化学领域实施例规则
// =============================================================================

// domainChemicalEmbodimentRule 检查化学领域说明书是否包含足够的实施例。
//
// 法律依据：
//
//	专利法第26条第3款——说明书应当对发明作出清楚、完整的说明。
//	审查指南第二部分第十章§3.4——化学发明需用实验数据证实。
//
// 知识库要点：
//  1. 需足够数量的代表性实例支持权利要求范围（至少2个）
//  2. 需同时包含产品制备实施例和应用效果实施例
//  3. 效果用事实和数据说明，不能仅主观论断
type domainChemicalEmbodimentRule struct{ baseRule }

func (r *domainChemicalEmbodimentRule) Check(spec *SpecOutput, input SpecInput) []Violation {
	if input.TechDomain != DomainChemical || spec == nil {
		return nil
	}
	embContent := findSection(spec, SecEmbodiment)
	if embContent == "" {
		return nil
	}

	// 1) 检查实施例数量（至少2个，支持中文数字和阿拉伯数字）
	embCount := 0
	for i := 1; i <= 20; i++ {
		cnMarker := "实施例" + chineseNumeral(i)
		if strings.Contains(embContent, cnMarker) {
			embCount++
			continue
		}
		arMarker := "实施例" + strconv.Itoa(i)
		if strings.Contains(embContent, arMarker) {
			embCount++
		}
	}

	var issues []Violation

	if embCount < 2 {
		issues = append(issues, Violation{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityWarning, SectionName: string(SecEmbodiment),
			Message:    "化学领域通常需要至少2个代表性实施例来支持权利要求的范围",
			Suggestion: "建议增加至2个以上实施例，覆盖不同的组分比例或制备条件",
		})
	}

	// 2) 检查是否同时包含制备实施例和应用效果实施例
	hasPrepEmbodiment := containsAnyOf(embContent, []string{"制备", "合成", "配制", "混合", "反应", "称取"})
	hasAppEmbodiment := containsAnyOf(embContent, []string{"测试", "实验", "检测", "性能", "效果", "结果", "应用"})

	if hasPrepEmbodiment && !hasAppEmbodiment {
		issues = append(issues, Violation{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityInfo, SectionName: string(SecEmbodiment),
			Message:    "化学说明书建议同时包含应用效果实施例，用数据证实技术效果",
			Suggestion: "在制备实施例后补充应用效果测试（如强度、活性、稳定性等指标）",
		})
	} else if !hasPrepEmbodiment && hasAppEmbodiment {
		issues = append(issues, Violation{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityInfo, SectionName: string(SecEmbodiment),
			Message:    "化学说明书建议同时包含制备实施例，说明产品的具体实现方式",
			Suggestion: "补充产品的具体制备方法或合成过程实施例",
		})
	}

	return issues
}

// chineseNumeral 将数字转换为中文数字（1→一，2→二，…）。
func chineseNumeral(n int) string {
	if n < 1 || n > len(chineseNumerals) {
		return ""
	}
	return chineseNumerals[n-1]
}

var chineseNumerals = []string{
	"一", "二", "三", "四", "五", "六", "七", "八", "九", "十",
	"十一", "十二", "十三", "十四", "十五", "十六", "十七", "十八", "十九", "二十",
}
