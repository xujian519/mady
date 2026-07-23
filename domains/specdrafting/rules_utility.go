package specdrafting

import "strings"

// =============================================================================
// 实用新型特有规则（3 条）
// =============================================================================

type utilityDrawingsRequiredRule struct{ baseRule }

func (r *utilityDrawingsRequiredRule) Check(spec *SpecOutput, input SpecInput) []Violation {
	if input.PatentType != PatentTypeUtilityModel {
		return nil
	}
	if !input.HasDrawings {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityError,
			Message:    "实用新型必须有附图",
			Suggestion: "请补充说明书附图，包含表示要求保护产品形状或构造的视图",
		}}
	}
	dwg := findSection(spec, SecDrawings)
	if dwg == "" {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityError, SectionName: string(SecDrawings),
			Message:    "实用新型必须有附图说明章节",
			Suggestion: "请添加附图说明，包含至少一幅结构或构造示意图",
		}}
	}
	return nil
}

type utilityProductOnlyRule struct{ baseRule }

func (r *utilityProductOnlyRule) Check(_ *SpecOutput, input SpecInput) []Violation {
	if input.PatentType != PatentTypeUtilityModel {
		return nil
	}
	var methodFeatures []string
	for _, f := range input.Features {
		if f.Category == "method" {
			methodFeatures = append(methodFeatures, f.Description)
		}
	}
	var methodProblems []string
	for _, p := range input.Problems {
		if containsAnyOf(p, []string{"方法", "工艺", "流程", "步骤", "算法"}) &&
			containsAnyOf(p, []string{"制备", "制造", "生产", "处理", "检测", "控制"}) {
			methodProblems = append(methodProblems, p)
		}
	}
	if len(methodFeatures) > 0 || len(methodProblems) > 0 {
		msg := "实用新型仅保护产品的形状、构造，不允许方法特征"
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityError,
			Message:    msg,
			Suggestion: "如确实包含方法发明，建议申请发明专利或同日申请",
		}}
	}
	return nil
}

type utilitySingleIndependentRule struct{ baseRule }

func (r *utilitySingleIndependentRule) Check(_ *SpecOutput, input SpecInput) []Violation {
	if input.PatentType != PatentTypeUtilityModel {
		return nil
	}
	if len(input.Claims) <= 1 {
		return nil
	}
	indCount := 0
	for _, c := range input.Claims {
		if strings.Contains(c, "其特征在于") {
			indCount++
		}
	}
	if indCount > 1 {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityWarning,
			Message:    "实用新型应当只有一个独立权利要求",
			Suggestion: "多余独立权利要求应删除或改为从属权利要求，或分案申请",
		}}
	}
	return nil
}
