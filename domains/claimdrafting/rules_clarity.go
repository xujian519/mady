package claimdrafting

import "strings"

// =============================================================================
// 清楚性规则：权利要求的类型应当清楚
// =============================================================================

// clarityClaimTypeRule 检查权利要求的类型是否清楚。
// 依据：细则第20条第2款——类型应当清楚，产品/方法不得混合。
type clarityClaimTypeRule struct{ baseRule }

func (r *clarityClaimTypeRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		if c.ClaimType != ClaimTypeProduct && c.ClaimType != ClaimTypeMethod {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityError,
				ClaimNumber: c.Number,
				Message:     "权利要求的类型不清楚：未明确指定是产品权利要求还是方法权利要求",
				Suggestion:  "请在主题名称中明确使用'一种……装置'（产品）或'一种……方法'（方法）",
			})
		}
		// 检查主题名称是否包含混合类型表述
		lowerPreamble := strings.ToLower(c.Preamble)
		if strings.Contains(lowerPreamble, "及其") &&
			(strings.Contains(lowerPreamble, "方法") || strings.Contains(lowerPreamble, "装置")) {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityError,
				ClaimNumber: c.Number,
				Message:     "权利要求的类型不清楚：不得使用混合类型的表述（如'一种……装置及其方法'）",
				Suggestion:  "分别撰写产品权利要求和方法权利要求，不得合并为一项权利要求",
			})
		}
	}
	return violations
}

// =============================================================================
// 清楚性规则：不得使用不确定用语
// =============================================================================

// clarityWordingRule 检查是否使用了含义不确定的用语。
// 依据：专利法第26条第4款——保护范围应当清楚。
type clarityWordingRule struct{ baseRule }

func (r *clarityWordingRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		text := c.String()
		if word, found := containsUncertainWord(text); found {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityWarning,
				ClaimNumber: c.Number,
				Message:     "使用了含义不确定的用语：'" + word + "'",
				Suggestion:  "建议使用具体数值或可量化的技术特征替代'" + word + "'，如将'高温'替换为'温度大于200℃'",
			})
		}
	}
	return violations
}

// =============================================================================
// 清楚性规则：不得使用非限定性用语
// =============================================================================

// clarityForbiddenWordsRule 检查是否使用了"例如""最好是"等非限定性用语。
// 依据：审查指南第二部分第二章§3.2.2。
type clarityForbiddenWordsRule struct{ baseRule }

func (r *clarityForbiddenWordsRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		text := c.String()
		if word, found := containsForbiddenWord(text); found {
			severity := SeverityWarning
			if word == "等" || word == "或类似物" {
				severity = SeverityError
			}
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    severity,
				ClaimNumber: c.Number,
				Message:     "使用了非限定性用语：'" + word + "'",
				Suggestion:  "删除'" + word + "'，或将所列举的技术特征完整写入权利要求",
			})
		}
	}
	return violations
}

// =============================================================================
// 清楚性规则：引用关系清楚
// =============================================================================

// clarityReferenceRule 检查从属权利要求的引用关系是否清楚。
// 依据：细则第23条第2款——多项从属只能择一引用（用"或"），不得用"和"。
type clarityReferenceRule struct{ baseRule }

func (r *clarityReferenceRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		if c.Kind != "dependent" {
			continue
		}
		// 检查是否引用了在后的权利要求
		for _, dep := range c.DependsOn {
			if dep >= c.Number {
				violations = append(violations, Violation{
					RuleName:    r.Name(),
					RuleBasis:   r.LegalBasis(),
					Severity:    SeverityError,
					ClaimNumber: c.Number,
					Message:     "从属权利要求引用了在后的权利要求，这是不允许的",
					Suggestion:  "从属权利要求只能引用在前的权利要求",
				})
			}
		}
		// 检查多项从属是否引用了自身
		if c.IsMultipleDependent() {
			for _, dep := range c.DependsOn {
				if dep == c.Number {
					violations = append(violations, Violation{
						RuleName:    r.Name(),
						RuleBasis:   r.LegalBasis(),
						Severity:    SeverityError,
						ClaimNumber: c.Number,
						Message:     "权利要求不能引用自身",
						Suggestion:  "删除对自身的引用",
					})
				}
			}
		}
	}
	return violations
}

// =============================================================================
// 清楚性规则：引用关系不得形成循环
// =============================================================================

// clarityReferenceChainRule 检查引用关系是否形成循环依赖。
// 依据：专利法第26条第4款——引用关系应当清楚正确。
type clarityReferenceChainRule struct{ baseRule }

func (r *clarityReferenceChainRule) Check(claims []Claim, _ DraftInput) []Violation {
	// 构建引用关系图并检测循环
	graph := make(map[int][]int)
	for _, c := range claims {
		if c.Kind == "dependent" {
			graph[c.Number] = c.DependsOn
		}
	}

	var violations []Violation
	visited := make(map[int]bool)
	recStack := make(map[int]bool)

	var dfs func(int) bool
	dfs = func(node int) bool {
		if recStack[node] {
			return true
		}
		if visited[node] {
			return false
		}
		visited[node] = true
		recStack[node] = true
		for _, dep := range graph[node] {
			if dfs(dep) {
				return true
			}
		}
		recStack[node] = false
		return false
	}

	for _, c := range claims {
		if c.Kind == "dependent" && !visited[c.Number] {
			if dfs(c.Number) {
				violations = append(violations, Violation{
					RuleName:    r.Name(),
					RuleBasis:   r.LegalBasis(),
					Severity:    SeverityError,
					ClaimNumber: c.Number,
					Message:     "权利要求的引用关系形成循环依赖",
					Suggestion:  "检查从属权利要求的引用链，确保每一级引用指向在前的权利要求且不形成循环",
				})
			}
		}
	}
	return violations
}
