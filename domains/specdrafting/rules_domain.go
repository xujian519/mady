package specdrafting

// =============================================================================
// 领域特定规则（4 条）
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
