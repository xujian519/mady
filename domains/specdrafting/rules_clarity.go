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

func (r *clarityTermConsistencyRule) Check(spec *SpecOutput, input SpecInput) []Violation {
	if spec == nil || len(spec.Sections) < 2 {
		return nil
	}
	var violations []Violation

	// 1) 发明名称一致性（已有检查）
	title := spec.Title
	if title != "" {
		citations := 0
		for _, sec := range spec.Sections {
			if sec.Content != "" && strings.Contains(sec.Content, truncStr(title, 10)) {
				citations++
			}
		}
		if citations == 0 && len(spec.Sections) >= 2 {
			violations = append(violations, Violation{
				RuleName: r.Name(), RuleBasis: r.LegalBasis(),
				Severity:   SeverityInfo,
				Message:    "发明名称在各章节中未被引用，可能导致主题不明确",
				Suggestion: "建议在各章节中统一引用发明名称",
			})
		}
	}

	// 2) 跨模块术语一致性：检查权利要求中的术语是否在说明书中出现
	if len(input.Claims) > 0 {
		allSpecText := ""
		for _, sec := range spec.Sections {
			allSpecText += " " + sec.Content
		}
		if allSpecText == "" {
			return violations
		}
		claimTerms := extractClaimTerms(input.Claims)
		var missingTerms []string
		for _, term := range claimTerms {
			if len([]rune(term)) < 2 {
				continue
			}
			if !strings.Contains(allSpecText, term) {
				missingTerms = append(missingTerms, term)
			}
		}
		if len(missingTerms) > 0 {
			if len(missingTerms) > 3 {
				missingTerms = missingTerms[:3]
			}
			violations = append(violations, Violation{
				RuleName: r.Name(), RuleBasis: r.LegalBasis(),
				Severity:   SeverityWarning,
				Message:    "以下权利要求中的术语在说明书中未出现：" + strings.Join(missingTerms, "、"),
				Suggestion: "确保说明书各章节使用与权利要求书一致的技术术语，删除权利要求中说明书未记载的术语",
			})
		}
	}

	return violations
}

// extractClaimTerms 从权利要求文本中提取用于一致性检查的关键技术术语。
// 提取策略：去掉编号和常见停用词，保留有实质含义的名词/动词。
func extractClaimTerms(claims []string) []string {
	stopWords := map[string]bool{
		"一种": true, "所述": true, "其特征在于": true,
		"包括": true, "包含": true, "具有": true,
		"和": true, "或": true, "的": true, "了": true, "是": true,
		"以及": true, "及其": true, "其中": true,
		"根据": true, "权利": true, "要求": true,
		"如": true, "第": true, "项": true,
		"用于": true, "为": true, "在": true, "于": true,
	}
	termSet := make(map[string]bool)
	for _, claim := range claims {
		cleaned := strings.TrimLeft(claim, "0123456789. ")
		// 按常见标点分割
		parts := strings.FieldsFunc(cleaned, func(r rune) bool {
			return r == '，' || r == '。' || r == '；' || r == '、' ||
				r == '：' || r == '（' || r == '）' || r == '(' || r == ')' ||
				r == ' ' || r == '\n' || r == '\t'
		})
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" || stopWords[part] {
				continue
			}
			runeLen := len([]rune(part))
			if runeLen >= 2 {
				termSet[part] = true
			}
		}
	}
	result := make([]string, 0, len(termSet))
	for t := range termSet {
		result = append(result, t)
	}
	if len(result) > 10 {
		result = result[:10]
	}
	return result
}

func truncStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// =============================================================================
// 清楚性规则：有益效果具体性（S3）
// =============================================================================

type clarityEffectsSpecificRule struct{ baseRule }

func (r *clarityEffectsSpecificRule) Check(spec *SpecOutput, _ SpecInput) []Violation {
	if spec == nil {
		return nil
	}
	content := findSection(spec, SecContent)
	if content == "" {
		return nil
	}
	effectSection := extractEffectPart(content)
	if effectSection == "" {
		return nil
	}

	vagueWords := []string{
		"结构简单", "使用方便", "操作简便",
		"成本低", "成本降低", "节约成本",
		"效率高", "效率提高", "提升效率", "提高效率",
		"性能好", "性能优良", "性能优异", "性能优越",
		"效果好", "效果良好", "效果显著",
		"稳定性好", "可靠性高",
		"寿命长", "延长寿命",
		"安全性高", "安全可靠",
	}
	hasCausalExplanation := containsAnyOf(effectSection, []string{
		"由于", "因为", "因此", "从而",
		"使得", "使", "由此",
		"实现了", "达到了", "获得了",
	})
	onlyVague := false
	vagueCount := 0
	for _, w := range vagueWords {
		if strings.Contains(effectSection, w) {
			vagueCount++
		}
	}
	if vagueCount >= 2 && !hasCausalExplanation {
		onlyVague = true
	}

	if onlyVague {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:    SeverityWarning,
			SectionName: string(SecContent),
			Message:     "有益效果仅列举了笼统的优点（如「结构简单、使用方便」），缺少因果解释",
			Suggestion:  "请明确各技术特征与效果之间的因果关系，如「由于采用了…结构，使得…从而实现…效果」",
		}}
	}
	return nil
}

// extractEffectPart 从发明内容中提取"有益效果"部分。
func extractEffectPart(content string) string {
	idx := strings.Index(content, "有益效果")
	if idx < 0 {
		idx = strings.Index(content, "技术效果")
	}
	if idx < 0 {
		idx = strings.Index(content, "优点")
	}
	if idx < 0 {
		return ""
	}
	return content[idx:]
}

// =============================================================================
// 清楚性规则：背景技术引证检查（S4）
// =============================================================================

type clarityCitationRule struct{ baseRule }

func (r *clarityCitationRule) Check(spec *SpecOutput, _ SpecInput) []Violation {
	if spec == nil {
		return nil
	}
	bgContent := findSection(spec, SecBackground)
	if bgContent == "" {
		return nil
	}

	hasPatentCitation := containsAnyOf(bgContent, []string{
		"CN", "US", "EP", "JP", "WO", "DE", "FR", "GB",
		"专利", "公开",
	})
	hasNonPatentCitation := containsAnyOf(bgContent, []string{
		"参见", "参考文献", "文献",
		"公开于", "记载于", "发表于",
	})

	if !hasPatentCitation && !hasNonPatentCitation && len([]rune(bgContent)) > 30 {
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity:    SeverityInfo,
			SectionName: string(SecBackground),
			Message:     "背景技术未引证反映现有技术的文件",
			Suggestion:  "在背景技术中引证最接近的现有技术文献（专利号如CNXXXXXX或非专利文献来源），以清楚界定本发明的改进基础",
		}}
	}
	return nil
}
