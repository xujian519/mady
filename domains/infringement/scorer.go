package infringement

// InfringementScorer computes a weighted multi-dimension infringement likelihood score.
// The composite score represents how likely infringement is from the patentee's perspective:
// higher = more likely to be found infringing.
//
// Dimensions that LOWER infringement likelihood (estoppel applied, strong defenses)
// are inverted so they correctly reduce the composite.
type InfringementScorer struct {
	weights map[string]float64
}

// defaultWeights is the pre-allocated static weighting scheme, shared across all scorers.
var defaultWeights = map[string]float64{
	"literal_match":     0.25, // more literal matches → higher infringement
	"equivalence":       0.20, // more equivalents → higher infringement
	"estoppel_risk":     0.10, // inverted: estoppel applied → lower infringement
	"dedication_risk":   0.05, // inverted: dedication applied → lower infringement
	"defense_strength":  0.20, // inverted: strong defenses → lower infringement
	"remedy_exposure":   0.10, // post-infringement, informational
	"strategy_viability": 0.10, // informational, neutral
}

// defaultScorer is a shared singleton for common use.
var defaultScorer = &InfringementScorer{weights: defaultWeights}

// NewInfringementScorer creates a scorer with custom or default weights.
func NewInfringementScorer(weights map[string]float64) *InfringementScorer {
	if weights == nil {
		return defaultScorer
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
		"literal_match":     s.scoreLiteralMatch(output),
		"equivalence":       s.scoreEquivalence(output),
		"estoppel_risk":     s.calcEstoppelImpact(output),
		"dedication_risk":   s.calcDedicationImpact(output),
		"defense_strength":  s.calcDefenseImpact(output),
		"remedy_exposure":   s.calcRemedyExposure(output),
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

// calcEstoppelImpact: estoppel applied = patentee CANNOT expand via equivalence
// → infringement likelihood DECREASES. Return 1.0 when NOT applied (full scope available),
// 0 when applied (scope limited).
func (s *InfringementScorer) calcEstoppelImpact(o *InfringementOutput) float64 {
	if o.EquivalenceResult.EstoppelApplied {
		return 0 // estoppel limits the patentee — reduces infringement likelihood
	}
	return 1.0
}

// calcDedicationImpact: dedication applied = disclosed-but-unclaimed subject matter lost
// → infringement likelihood DECREASES. Return 1.0 when NOT applied, 0 when applied.
func (s *InfringementScorer) calcDedicationImpact(o *InfringementOutput) float64 {
	if o.EquivalenceResult.DedicationApplied {
		return 0
	}
	return 1.0
}

// calcDefenseImpact: stronger defenses → LESS likely to be found infringing.
// Returns 1.0 (full contribution) when no strong defenses exist,
// decreasing as more defenses show high/medium viability.
func (s *InfringementScorer) calcDefenseImpact(o *InfringementOutput) float64 {
	if len(o.DefenseAnalysis) == 0 {
		return 1.0 // no defense analysis → assume full infringement likelihood
	}
	strong := 0
	for _, d := range o.DefenseAnalysis {
		if d.ViabilityRating == "high" || d.ViabilityRating == "medium" {
			strong++
		}
	}
	// Invert: many strong defenses → lower infringement likelihood
	return 1.0 - float64(strong)/float64(len(o.DefenseAnalysis))
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
