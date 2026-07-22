package specdrafting

import "sort"

// SpecScorer 基于规则引擎输出进行多维度质量评分。
type SpecScorer struct {
	engine *RuleEngine
}

// NewSpecScorer 创建评分器。
// engine 不能为 nil，否则 panic。
func NewSpecScorer(engine *RuleEngine) *SpecScorer {
	if engine == nil {
		panic("specdrafting: NewSpecScorer 的 engine 参数不能为 nil")
	}
	return &SpecScorer{engine: engine}
}

// Score 进行综合评分，返回评分报告。
func (s *SpecScorer) Score(spec *SpecOutput, input SpecInput) *ScoreReport {
	if spec == nil || len(spec.Sections) == 0 {
		return &ScoreReport{OverallScore: 0, Grade: "D", Suggestions: []string{"说明书为空"}}
	}

	allViolations := s.engine.Validate(spec, input)
	dimensionScores := s.calcDimensionScores(allViolations)
	overall := s.calcOverall(dimensionScores)
	suggestions := s.generateSuggestions(allViolations)

	return &ScoreReport{
		OverallScore:    overall,
		DimensionScores: dimensionScores,
		Violations:      allViolations,
		Suggestions:     suggestions,
		Grade:           gradeFromScore(overall),
	}
}

func (s *SpecScorer) calcDimensionScores(violations []Violation) map[string]float64 {
	ruleViolations := make(map[string]int)
	for _, v := range violations {
		ruleViolations[v.RuleName]++
	}

	dimensionRules := map[string][]string{
		DimCompleteness:     {"structure-sections", "structure-content-triad"},
		DimClarity:          {"clarity-terminology", "clarity-forbidden-words", "clarity-pfe-consistency", "clarity-term-consistency"},
		DimSupport:          {"structure-embodiment-detail"},
		DimFormality:        {"structure-title-length", "structure-abstract-length"},
		DimDomainAdaptation: {"domain-mechanical", "domain-electrical", "domain-chemical", "domain-software", "utility-drawings-required", "utility-product-only"},
	}

	scores := make(map[string]float64)
	for dim, rules := range dimensionRules {
		v := 0
		for _, r := range rules {
			v += ruleViolations[r]
		}
		score := 100.0 - float64(v)*20.0
		if score < 0 {
			score = 0
		}
		scores[dim] = score
	}
	return scores
}

func (s *SpecScorer) calcOverall(dimensionScores map[string]float64) float64 {
	var total, weightSum float64
	for dim, weight := range SpecDimensionWeights {
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

func (s *SpecScorer) generateSuggestions(violations []Violation) []string {
	seen := make(map[string]bool)
	var ss []string
	for _, v := range violations {
		if v.Suggestion != "" && !seen[v.Suggestion] {
			ss = append(ss, v.Suggestion)
			seen[v.Suggestion] = true
		}
	}
	sort.Strings(ss)
	if len(ss) > 5 {
		ss = ss[:5]
	}
	return ss
}

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
