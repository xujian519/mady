package claimdrafting

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// =============================================================================
// 形式规范规则：编号顺序
// =============================================================================

// formalityNumberingRule 检查权利要求是否用阿拉伯数字顺序编号。
// 依据：细则第20条第1款——权利要求书有几项权利要求的，应当用阿拉伯数字顺序编号。
type formalityNumberingRule struct{ baseRule }

func (r *formalityNumberingRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for i, c := range claims {
		if c.Number != i+1 {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityError,
				ClaimNumber: c.Number,
				Message:     "权利要求未按阿拉伯数字顺序编号（应为" + strconv.Itoa(i+1) + "号）",
				Suggestion:  "请按1, 2, 3, ...的顺序重新编号权利要求",
			})
			continue
		}
	}
	return violations
}

// =============================================================================
// 形式规范规则：句号用法
// =============================================================================

// formalityPeriodRule 检查每一项权利要求是否只在其结尾处使用句号。
// 依据：细则第20条——每一项权利要求只允许在其结尾处使用句号。
type formalityPeriodRule struct{ baseRule }

func (r *formalityPeriodRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		text := c.String()
		// 检查是否在结尾之前有句号
		lastRune, _ := utf8.DecodeLastRuneInString(text)
		if lastRune != '。' {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityError,
				ClaimNumber: c.Number,
				Message:     "权利要求未以句号结尾",
				Suggestion:  "在权利要求末尾添加'。'",
			})
		}
	}
	return violations
}

// =============================================================================
// 形式规范规则：不得有插图
// =============================================================================

// formalityNoIllustrationRule 检查权利要求书中是否含有插图。
// 依据：细则第20条第3款——权利要求书中不得有插图。
type formalityNoIllustrationRule struct{ baseRule }

func (r *formalityNoIllustrationRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		text := c.String()
		// 检测常见插图标记
		if strings.Contains(text, "如图") || strings.Contains(text, "图") {
			// 允许引用附图标记（如"图1"在括号内）
			// 简单检查：不在括号内的"图"字视为引用不当
			hasIllegalFigRef := false
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				if strings.Contains(line, "如图") && !strings.Contains(line, "（") {
					hasIllegalFigRef = true
					break
				}
			}
			if hasIllegalFigRef {
				violations = append(violations, Violation{
					RuleName:    r.Name(),
					RuleBasis:   r.LegalBasis(),
					Severity:    SeverityError,
					ClaimNumber: c.Number,
					Message:     "权利要求中不得包含'如图……所示'等引用附图的表述（除非是括号内的附图标记）",
					Suggestion:  "删除'如图……所示'等表述，或将其替换为技术特征的直接描述",
				})
			}
		}
	}
	return violations
}

// =============================================================================
// 形式规范规则：多项从属限制
// =============================================================================

// formalityMultipleDependentRule 检查多项从属权利要求是否被另一项多项从属权利要求引用。
// 依据：细则第23条第2款——多项从属权利要求不得作为另一项多项从属权利要求的基础。
type formalityMultipleDependentRule struct{ baseRule }

func (r *formalityMultipleDependentRule) Check(claims []Claim, _ DraftInput) []Violation {
	// 构建多项从属集合
	multiDeps := make(map[int]bool)
	for _, c := range claims {
		if c.IsMultipleDependent() {
			multiDeps[c.Number] = true
		}
	}

	var violations []Violation
	for _, c := range claims {
		if c.Kind != "dependent" || !c.IsMultipleDependent() {
			continue
		}
		// 检查此多项从属是否引用了其他多项从属
		for _, dep := range c.DependsOn {
			if multiDeps[dep] {
				violations = append(violations, Violation{
					RuleName:    r.Name(),
					RuleBasis:   r.LegalBasis(),
					Severity:    SeverityError,
					ClaimNumber: c.Number,
					Message: "多项从属权利要求引用了另一项多项从属权利要求（权利要求" +
						strconv.Itoa(dep) + "），这是不允许的",
					Suggestion: "将多项从属权利要求改为仅引用独立权利要求或单引用的从属权利要求",
				})
			}
		}
	}
	return violations
}

// =============================================================================
// 形式规范规则：主题名称一致性
// =============================================================================

// formalityThemeConsistencyRule 检查从属权利要求的主题名称是否与其引用的权利要求一致。
// 依据：细则第22条第3款——从属权利要求的类型和主题名称应当与其引用权利要求的类型和主题名称一致。
type formalityThemeConsistencyRule struct{ baseRule }

func (r *formalityThemeConsistencyRule) Check(claims []Claim, _ DraftInput) []Violation {
	// 构建权利要求的主题名称映射
	claimThemes := make(map[int]ClaimType)
	for _, c := range claims {
		claimThemes[c.Number] = c.ClaimType
	}

	var violations []Violation
	for _, c := range claims {
		if c.Kind != "dependent" {
			continue
		}
		for _, dep := range c.DependsOn {
			if parentType, ok := claimThemes[dep]; ok {
				if parentType != c.ClaimType {
					violations = append(violations, Violation{
						RuleName:    r.Name(),
						RuleBasis:   r.LegalBasis(),
						Severity:    SeverityError,
						ClaimNumber: c.Number,
						Message: "从属权利要求的类型（" + string(c.ClaimType) +
							"）与其引用的权利要求" + strconv.Itoa(dep) +
							"（" + string(parentType) + "）不一致",
						Suggestion: "确保从属权利要求的主题名称（产品/方法）与其引用的权利要求类型一致",
					})
				}
			}
		}
	}
	return violations
}

// =============================================================================
// 形式规范规则：保护范围递进
// =============================================================================

// formalityScopeNarrowingRule 检查从属权利要求的保护范围是否在引用权利要求的范围之内。
// 依据：审查指南——从属权利要求的保护范围应当比其引用权利要求的保护范围小。
// 注意：此规则为启发式检查，精确判断需要语义分析。
type formalityScopeNarrowingRule struct{ baseRule }

func (r *formalityScopeNarrowingRule) Check(claims []Claim, _ DraftInput) []Violation {
	// 目前做基本检查：从属权利要求不需要写与独立权利要求相同的特征开头
	// （从属权利要求自动包含被引用权利要求的全部技术特征）
	var violations []Violation
	for _, c := range claims {
		if c.Kind != "dependent" {
			continue
		}
		// 检查从属权利要求的限定部分是否为空
		if strings.TrimSpace(c.Limitation) == "" {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityError,
				ClaimNumber: c.Number,
				Message:     "从属权利要求的限定部分为空",
				Suggestion:  "在从属权利要求中写明附加的技术特征，对引用的权利要求作进一步限定",
			})
		}
	}
	return violations
}

// =============================================================================
// 形式规范规则：并列独立权利要求
// =============================================================================

type formalityParallelClaimRule struct{ baseRule }

func (r *formalityParallelClaimRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	parallelClaims := make(map[int]int)
	for _, c := range claims {
		if c.Kind != "independent" || c.Preamble == "" {
			continue
		}
		refNum := extractReferencedClaim(c.Preamble)
		if refNum > 0 {
			parallelClaims[c.Number] = refNum
		}
	}
	if len(parallelClaims) == 0 {
		return nil
	}
	// 检查引用链是否循环
	for pcNum, refNum := range parallelClaims {
		visited := make(map[int]bool)
		cur := refNum
		for {
			if visited[cur] {
				violations = append(violations, Violation{
					RuleName: r.Name(), RuleBasis: r.LegalBasis(),
					Severity: SeverityError, ClaimNumber: pcNum,
					Message:    "并列独立权利要求形成循环引用链",
					Suggestion: "检查并列独立权利要求的引用关系，避免循环依赖",
				})
				break
			}
			visited[cur] = true
			nextRef, ok := parallelClaims[cur]
			if !ok {
				break
			}
			cur = nextRef
		}
	}
	// 检查引用目标是否存在
	for pcNum, refNum := range parallelClaims {
		found := false
		for _, c := range claims {
			if c.Number == refNum {
				found = true
				break
			}
		}
		if !found {
			violations = append(violations, Violation{
				RuleName: r.Name(), RuleBasis: r.LegalBasis(),
				Severity: SeverityError, ClaimNumber: pcNum,
				Message:    "并列独立权利要求引用的权利要求" + strconv.Itoa(refNum) + "不存在",
				Suggestion: "确保引用的权利要求编号有效",
			})
		}
	}
	return violations
}

func extractReferencedClaim(preamble string) int {
	patterns := []string{"权利要求", "如权利要求", "根据权利要求"}
	for _, p := range patterns {
		idx := strings.Index(preamble, p)
		if idx < 0 {
			continue
		}
		after := preamble[idx+len([]rune(p)):]
		var numStr string
		for _, r := range after {
			if r >= '0' && r <= '9' {
				numStr += string(r)
			} else if numStr != "" {
				break
			}
		}
		if numStr != "" {
			if n, err := strconv.Atoi(numStr); err == nil {
				return n
			}
		}
	}
	return 0
}
