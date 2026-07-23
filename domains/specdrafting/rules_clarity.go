package specdrafting

import "strings"

// =============================================================================
// 清楚性规则（4 条）
// =============================================================================

type clarityTerminologyRule struct{ baseRule }

func (r *clarityTerminologyRule) Check(spec *SpecOutput, _ SpecInput) []Violation {
	if spec == nil {
		return nil
	}
	allText := spec.Title + " " + spec.Abstract
	for _, sec := range spec.Sections {
		allText += " " + sec.Content
	}
	if allText == "" {
		return nil
	}
	if word, found := containsAny(allText, uncertainWords); found {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityWarning,
			Message:    "使用了不确定用语\"" + word + "\"",
			Suggestion: "请替换为明确的技术术语或给出具体数值范围",
		}}
	}
	return nil
}

type clarityForbiddenWordsRule struct{ baseRule }

func (r *clarityForbiddenWordsRule) Check(spec *SpecOutput, _ SpecInput) []Violation {
	if spec == nil {
		return nil
	}
	allText := ""
	for _, sec := range spec.Sections {
		allText += " " + sec.Content
	}
	if word, found := containsAny(allText, forbiddenWords); found {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityWarning,
			Message:    "使用了禁止用词\"" + word + "\"",
			Suggestion: "不得使用商业性宣传用语和含义不确定的用语",
		}}
	}
	return nil
}

type clarityPFEConsistencyRule struct{ baseRule }

func (r *clarityPFEConsistencyRule) Check(spec *SpecOutput, input SpecInput) []Violation {
	if spec == nil || (len(input.PFETriples) == 0 && len(input.Problems) == 0 && len(input.Effects) == 0) {
		return nil
	}
	content := findSection(spec, SecContent)
	if content == "" {
		return nil
	}
	var issues []Violation
	hasProblem := false
	for _, p := range input.Problems {
		if p != "" && strings.Contains(content, truncStr(p, 20)) {
			hasProblem = true
			break
		}
	}
	hasEffect := false
	for _, e := range input.Effects {
		if e != "" && strings.Contains(content, truncStr(e, 20)) {
			hasEffect = true
			break
		}
	}
	if !hasProblem && len(input.Problems) > 0 {
		issues = append(issues, Violation{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityWarning, SectionName: string(SecContent),
			Message:    "发明内容中未充分反映要解决的技术问题",
			Suggestion: "请确保发明内容中明确记载要解决的技术问题",
		})
	}
	if !hasEffect && len(input.Effects) > 0 {
		issues = append(issues, Violation{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityWarning, SectionName: string(SecContent),
			Message:    "发明内容中未充分反映有益效果",
			Suggestion: "请确保发明内容包含有益效果描述",
		})
	}
	return issues
}

type clarityTermConsistencyRule struct{ baseRule }

func (r *clarityTermConsistencyRule) Check(spec *SpecOutput, _ SpecInput) []Violation {
	if spec == nil || len(spec.Sections) < 2 {
		return nil
	}
	title := spec.Title
	if title == "" {
		return nil
	}
	citations := 0
	for _, sec := range spec.Sections {
		if sec.Content != "" && strings.Contains(sec.Content, truncStr(title, 10)) {
			citations++
		}
	}
	if citations == 0 && len(spec.Sections) >= 2 {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:   SeverityInfo,
			Message:    "发明名称在各章节中未被引用，可能导致主题不明确",
			Suggestion: "建议在各章节中统一引用发明名称",
		}}
	}
	return nil
}

func truncStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
