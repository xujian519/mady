package infringement

// InfringementScorer computes a weighted multi-dimension likelihood score.
type InfringementScorer struct {
	weights map[string]float64
}

// DefaultScorerWeights returns the standard weighting scheme.
func DefaultScorerWeights() map[string]float64 {
	return map[string]float64{
		"literal_match":      0.25,
		"equivalence":        0.20,
		"estoppel_risk":      0.10,
		"dedication_risk":    0.05,
		"defense_strength":   0.20,
		"remedy_exposure":    0.10,
		"strategy_viability": 0.10,
	}
}

// NewInfringementScorer creates a scorer with custom or default weights.
func NewInfringementScorer(weights map[string]float64) *InfringementScorer {
	if weights == nil {
		weights = DefaultScorerWeights()
	}
	return &InfringementScorer{weights: weights}
}

// ScoreResult holds dimension-level and composite scores.
type ScoreResult struct {
	Dimensions map[string]float64 `json:"dimensions"`
	Composite  float64            `json:"composite"`
	RiskLevel  string             `json:"risk_level"`
}

// Score computes weighted scores from the analysis output.
func (s *InfringementScorer) Score(output *InfringementOutput) *ScoreResult {
	dim := map[string]float64{
		"literal_match":      s.scoreLiteralMatch(output),
		"equivalence":        s.scoreEquivalence(output),
		"estoppel_risk":      s.calcEstoppelRisk(output),
		"dedication_risk":    s.calcDedicationRisk(output),
		"defense_strength":   s.scoreDefenses(output),
		"remedy_exposure":    s.calcRemedyExposure(output),
		"strategy_viability": s.scoreStrategy(output),
	}
	var composite float64
	for k, v := range dim {
		composite += v * s.weights[k]
	}
	return &ScoreResult{
		Dimensions: dim,
		Composite:  composite,
		RiskLevel:  riskLevel(composite),
	}
}

func (s *InfringementScorer) scoreLiteralMatch(o *InfringementOutput) float64 {
	if o.LiteralResult.AllElementsMet {
		return 1.0
	}
	matched := 0
	for _, fc := range o.FeatureMapping {
		if fc.MatchType == "literal" {
			matched++
		}
	}
	total := len(o.FeatureMapping)
	if total == 0 {
		return 0
	}
	return float64(matched) / float64(total)
}

func (s *InfringementScorer) scoreEquivalence(o *InfringementOutput) float64 {
	equiv := o.EquivalenceResult.EquivalentFeatures
	if len(equiv) == 0 {
		return 0
	}
	equivalent := 0
	for _, ea := range equiv {
		if ea.IsEquivalent {
			equivalent++
		}
	}
	return float64(equivalent) / float64(len(equiv))
}

func (s *InfringementScorer) calcEstoppelRisk(o *InfringementOutput) float64 {
	if o.EquivalenceResult.EstoppelApplied {
		return 1.0
	}
	return 0
}

func (s *InfringementScorer) calcDedicationRisk(o *InfringementOutput) float64 {
	if o.EquivalenceResult.DedicationApplied {
		return 1.0
	}
	return 0
}

func (s *InfringementScorer) scoreDefenses(o *InfringementOutput) float64 {
	if len(o.DefenseAnalysis) == 0 {
		return 0
	}
	strong := 0
	for _, d := range o.DefenseAnalysis {
		if d.ViabilityRating == "high" || d.ViabilityRating == "medium" {
			strong++
		}
	}
	return float64(strong) / float64(len(o.DefenseAnalysis))
}

func (s *InfringementScorer) calcRemedyExposure(o *InfringementOutput) float64 {
	avg := (o.RemedyAssessment.DamageEstimate.RangeHigh + o.RemedyAssessment.DamageEstimate.RangeLow) / 2
	if avg <= 0 {
		return 0
	}
	normalized := avg / 10_000_000
	if normalized > 1.0 {
		return 1.0
	}
	return normalized
}

func (s *InfringementScorer) scoreStrategy(o *InfringementOutput) float64 {
	actions := o.StrategyAdvice.RecommendedActions
	if len(actions) == 0 {
		return 0.5
	}
	highPriority := 0
	for _, a := range actions {
		if a.Priority == "immediate" {
			highPriority++
		}
	}
	return float64(highPriority) / float64(len(actions))
}

func riskLevel(composite float64) string {
	switch {
	case composite >= 0.7:
		return "high"
	case composite >= 0.4:
		return "medium"
	default:
		return "low"
	}
}
