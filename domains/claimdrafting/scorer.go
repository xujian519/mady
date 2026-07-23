package claimdrafting

import "sort"

// =============================================================================
// ClaimScorer 质量评分器
// =============================================================================

// ClaimScorer 基于规则引擎的输出，对权利要求书进行多维度质量评分。
type ClaimScorer struct {
	engine *RuleEngine
}

// NewClaimScorer 创建一个质量评分器。
func NewClaimScorer(engine *RuleEngine) *ClaimScorer {
	return &ClaimScorer{engine: engine}
}

// Score 对权利要求书进行综合评分，返回评分报告。
func (s *ClaimScorer) Score(claims []Claim, input DraftInput) *ScoreReport {
	if len(claims) == 0 {
		return &ScoreReport{
			OverallScore: 0,
			Grade:        "D",
			Suggestions:  []string{"权利要求书为空"},
		}
	}

	// 收集所有违规
	allViolations := s.engine.Validate(claims, input)

	// 按维度归类和计算得分
	dimensionScores := s.calcDimensionScores(claims, allViolations)

	// 计算总分（加权）
	overall := s.calcOverall(dimensionScores)

	// 生成修改建议
	suggestions := s.generateSuggestions(allViolations)

	// 评级
	grade := gradeFromScore(overall)

	return &ScoreReport{
		OverallScore:    overall,
		DimensionScores: dimensionScores,
		Violations:      allViolations,
		Suggestions:     suggestions,
		Grade:           grade,
	}
}

// severityPenalty 按违规严重程度定义扣分权重。
var severityPenalty = map[Severity]float64{
	SeverityError:   20,
	SeverityWarning: 10,
	SeverityInfo:    5,
}

// calcDimensionScores 计算各维度得分（0-100 分）。
// 按违规严重程度差异化扣分：error 扣 20、warning 扣 10、info 扣 5。
func (s *ClaimScorer) calcDimensionScores(_ []Claim, violations []Violation) map[string]float64 {
	// 按严重程度加权统计各规则的违规扣分
	rulePenalty := make(map[string]int)
	for _, v := range violations {
		penalty, ok := severityPenalty[v.Severity]
		if !ok {
			penalty = 20 // 未知严重程度按 Error 处理
		}
		rulePenalty[v.RuleName] += int(penalty)
	}

	// 维度与规则的对应关系
	dimensionRules := map[string][]string{
		DimClarity:   {"clarity-claim-type", "clarity-wording", "clarity-forbidden-words", "clarity-reference", "clarity-reference-chain"},
		DimSupport:   {"support-embodiment", "support-functional", "support-pure-functional"},
		DimNecessity: {"necessity-completeness", "necessity-non-essential"},
		DimFormality: {"formality-numbering", "formality-period",
			"formality-no-illustration", "formality-multiple-dependent", "formality-theme-consistency", "formality-scope-narrowing"},
		DimScope: {"domain-mechanical", "domain-electrical", "domain-chemical", "domain-software", "domain-utility-model",
			"scope-over-specification", "scope-equivalents-coverage"},
	}

	scores := make(map[string]float64)
	for dim, rules := range dimensionRules {
		var totalPenalty int
		for _, rule := range rules {
			totalPenalty += rulePenalty[rule]
		}
		scores[dim] = max(0, 100.0-float64(totalPenalty))
	}

	return scores
}

// calcOverall 计算加权总分。
func (s *ClaimScorer) calcOverall(dimensionScores map[string]float64) float64 {
	var total float64
	var weightSum float64
	for dim, weight := range DimensionWeights {
		if score, ok := dimensionScores[dim]; ok {
			total += score * weight
			weightSum += weight
		}
	}
	if weightSum == 0 {
		return 0
	}
	return total / weightSum
}

// generateSuggestions 从违规列表生成修改建议。
func (s *ClaimScorer) generateSuggestions(violations []Violation) []string {
	seen := make(map[string]bool)
	var suggestions []string
	for _, v := range violations {
		if v.Suggestion != "" && !seen[v.Suggestion] {
			suggestions = append(suggestions, v.Suggestion)
			seen[v.Suggestion] = true
		}
	}
	sort.Strings(suggestions)
	if len(suggestions) > 5 {
		suggestions = suggestions[:5]
	}
	return suggestions
}

// gradeFromScore 根据分数给出等级。
func gradeFromScore(score float64) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 75:
		return "B"
	case score >= 60:
		return "C"
	default:
		return "D"
	}
}
