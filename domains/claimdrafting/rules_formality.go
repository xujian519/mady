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
		// 检测常见插图引用标记
		// 先拍平换行以应对跨行写法（如"如图 1\n所示"）
		flatText := strings.ReplaceAll(text, "\n", " ")
		if hasFigRefWithoutParens(flatText) {
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
	return violations
}

// figRefParens 是用于检测附图引用是否在括号内的括号字符串列表。
// 声明为包级别变量避免每次调用重新分配。
var figRefParens = []string{"（", "）", "(", ")", "[", "]"}

// hasFigRefWithoutParens 检测文本中是否含有未加括号括起的附图引用（如"如图1所示"）。
// 支持的中文括号：全角（）、英文括号()、方括号[]。
func hasFigRefWithoutParens(text string) bool {
	// 检测 "如图" 模式：后面跟数字/文字后接"所示"
	// 避免将"流程图""方框图"等误判
	if !strings.Contains(text, "如图") && !strings.Contains(text, "附图") {
		return false
	}

	// 检查是否在任意一种括号内
	for _, p := range figRefParens {
		if strings.Contains(text, p) {
			return false
		}
	}
	return true
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

// =============================================================================
// 形式规范规则：从属权利要求排序（金字塔型）
// =============================================================================

// formalityDependentOrderingRule 检查从属权利要求是否遵循"从宽到窄"的递进布局。
// 依据：审查指南——从属权利要求应当对引用的权利要求作进一步限定，
// 布局策略应遵循从重要到次要、从宽到窄的递进顺序。
type formalityDependentOrderingRule struct{ baseRule }

func (r *formalityDependentOrderingRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	// 找出所有从属权利要求
	var deps []Claim
	for _, c := range claims {
		if c.Kind == "dependent" {
			deps = append(deps, c)
		}
	}
	if len(deps) < 2 {
		return nil // 至少 2 项从属才有排序意义
	}

	// 检查引用链是否合理递进：
	//  前序从属应引用独立权利要求（保护范围较宽）
	//  后继从属可引用前序从属（递进限定，保护范围逐步收窄）
	indepNumbers := make(map[int]bool)
	for _, c := range claims {
		if c.Kind == "independent" {
			indepNumbers[c.Number] = true
		}
	}

	indirectCount := 0
	for _, d := range deps {
		allIndependent := true
		for _, dep := range d.DependsOn {
			if !indepNumbers[dep] {
				allIndependent = false
				break
			}
		}
		if !allIndependent {
			indirectCount++
		}
	}

	// 如果所有从属都直接引用独立权利要求，没有形成递进链：
	if indirectCount == 0 && len(deps) >= 3 {
		violations = append(violations, Violation{
			RuleName:    r.Name(),
			RuleBasis:   r.LegalBasis(),
			Severity:    SeverityInfo,
			ClaimNumber: 0,
			Message:     "所有从属权利要求均直接引用独立权利要求，未形成'从宽到窄'的递进保护链",
			Suggestion:  "建议将后几项从属权利要求改为引用前一项从属权利要求，形成逐步递进的保护层次（独立权利要求→从属1→从属2→从属3）",
		})
	}

	// 检查早期从属是否引用了后期从属（乱序）
	depOrder := make(map[int]int) // claimNumber → index
	for i, d := range deps {
		depOrder[d.Number] = i
	}
	for _, d := range deps {
		for _, ref := range d.DependsOn {
			if refIdx, ok := depOrder[ref]; ok && depOrder[d.Number] < refIdx {
				// 从属引用了在后面出现的从属 → 非典型但合法
			}
		}
	}

	return violations
}
