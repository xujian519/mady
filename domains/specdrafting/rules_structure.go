package specdrafting

import "strings"

// =============================================================================
// 结构完整性规则（5 条）
// =============================================================================

type structureSectionsRule struct{ baseRule }

func (r *structureSectionsRule) Check(spec *SpecOutput, _ SpecInput) []Violation {
	if spec == nil || len(spec.Sections) == 0 {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityError, Message: "说明书缺少所有必要章节",
			Suggestion: "请按顺序撰写技术领域、背景技术、发明内容、附图说明和具体实施方式",
		}}
	}
	existing := make(map[SpecSectionName]bool)
	for _, sec := range spec.Sections {
		existing[sec.Name] = true
	}
	var missing []string
	for _, req := range requiredSections {
		if !existing[req] {
			missing = append(missing, string(req))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []Violation{{
		RuleName: r.Name(), RuleBasis: r.LegalBasis(),
		Severity: SeverityError, Message: "缺少必要章节：" + strings.Join(missing, "、"),
	}}
}

type structureTitleLengthRule struct{ baseRule }

func (r *structureTitleLengthRule) Check(spec *SpecOutput, input SpecInput) []Violation {
	title := spec.Title
	if title == "" {
		title = input.Title
	}
	if title == "" {
		return nil
	}
	if ChineseCharCount(title) > 25 {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityError, SectionName: string(SecTitle),
			Message:    "发明名称超过25字限制",
			Suggestion: "请缩短至25字以内，使用通用技术术语",
		}}
	}
	return nil
}

type structureAbstractLengthRule struct{ baseRule }

func (r *structureAbstractLengthRule) Check(spec *SpecOutput, _ SpecInput) []Violation {
	if spec.Abstract == "" {
		return nil
	}
	if ChineseCharCount(spec.Abstract) > 300 {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityError, SectionName: string(SecAbstract),
			Message:    "摘要超过300字限制",
			Suggestion: "请压缩至300字以内",
		}}
	}
	return nil
}

type structureContentTriadRule struct{ baseRule }

func (r *structureContentTriadRule) Check(spec *SpecOutput, _ SpecInput) []Violation {
	content := findSection(spec, SecContent)
	if content == "" {
		return nil
	}
	var missing []string
	if !containsAnyOf(content, []string{"技术问题", "要解决", "目的在于", "本发明的目的"}) {
		missing = append(missing, "要解决的技术问题")
	}
	if !containsAnyOf(content, []string{"技术方案", "技术方案如下", "采用如下技术方案"}) {
		missing = append(missing, "技术方案")
	}
	if !containsAnyOf(content, []string{"有益效果", "技术效果", "优点", "进步"}) {
		missing = append(missing, "有益效果")
	}
	if len(missing) == 0 {
		return nil
	}
	return []Violation{{
		RuleName: r.Name(), RuleBasis: r.LegalBasis(),
		Severity: SeverityError, SectionName: string(SecContent),
		Message:    "发明内容缺少以下要素：" + strings.Join(missing, "、"),
		Suggestion: "应包含要解决的技术问题、技术方案和有益效果三部分",
	}}
}

type structureEmbodimentDetailRule struct{ baseRule }

func (r *structureEmbodimentDetailRule) Check(spec *SpecOutput, _ SpecInput) []Violation {
	if spec == nil {
		return nil
	}
	emb := findSection(spec, SecEmbodiment)
	if emb == "" {
		return nil
	}
	var issues []string
	if len([]rune(emb)) < 50 {
		issues = append(issues, "内容过于简略")
	}
	if !containsAnyOf(emb, []string{"实施例", "具体", "例如", "优选"}) {
		issues = append(issues, "未给出具体实施例")
	}
	if !containsAnyOf(emb, []string{"步骤", "包括", "包含", "采用", "设置"}) {
		issues = append(issues, "缺少具体的结构/步骤描述")
	}
	if len(issues) == 0 {
		return nil
	}
	return []Violation{{
		RuleName: r.Name(), RuleBasis: r.LegalBasis(),
		Severity: SeverityWarning, SectionName: string(SecEmbodiment),
		Message:    "具体实施方式存在以下问题：" + strings.Join(issues, "；"),
		Suggestion: "请给出至少一个详细实施方式",
	}}
}

// =============================================================================
// 辅助函数
// =============================================================================

func findSection(spec *SpecOutput, name SpecSectionName) string {
	for _, sec := range spec.Sections {
		if sec.Name == name {
			return sec.Content
		}
	}
	return ""
}

func containsAnyOf(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
