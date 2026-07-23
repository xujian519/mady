// Package infringement implements patent infringement determination as a Pregel
// sub-graph with dual-perspective support (patentee/plaintiff vs accused/defendant).
//
// It covers four layers of analysis:
//
//	L1 - Core determination: claim scope → feature mapping → literal infringement →
//	     equivalence (means/function/effect) → estoppel → dedication → verdict
//	L2 - Defense review: prior art / prior use / legal source / exhaustion / rights conflict
//	L3 - Remedy assessment: damages (4-tier cascade) + injunction (5-factor test)
//	L4 - Strategy: litigation tactics / jurisdiction / settlement / invalidation routes
//
// The module follows the same architecture as domains/inventiveness/:
// Pregel sub-graph wrapped as an agentcore.Tool, with YAML rule engine
// integration and KnowledgeRetriever enhancement.
package infringement

// Perspective defines whose standpoint the analysis adopts.
type Perspective string

const (
	PerspectivePatentee  Perspective = "patentee"  // 专利权人/原告
	PerspectiveDefendant Perspective = "defendant" // 被控侵权人/被告
)

// PatentType restricts analysis to invention vs utility model.
// Design patents use a fundamentally different legal framework and
// will be handled in a future iteration.
type PatentType string

const (
	PatentTypeInvention    PatentType = "invention"
	PatentTypeUtilityModel PatentType = "utility_model"
)

// InfringementInput is the complete input for an infringement analysis.
type InfringementInput struct {
	PatentClaims       string         `json:"patent_claims"`
	PatentSpec         string         `json:"patent_spec,omitempty"`
	ProsecutionHistory string         `json:"prosecution_history,omitempty"`
	AccusedProduct     string         `json:"accused_product"`
	Perspective        Perspective    `json:"perspective"`
	PatentType         PatentType     `json:"patent_type"`
	PriorArtRefs       []string       `json:"prior_art_refs,omitempty"`
	LicenseInfo        *LicenseInfo   `json:"license_info,omitempty"`
	GuidelineRefs      []GuidelineRef `json:"guideline_refs,omitempty"`
	SimilarCases       []CaseRef      `json:"similar_cases,omitempty"`
}

// LicenseInfo captures the existence and scope of any license.
type LicenseInfo struct {
	HasLicense   bool    `json:"has_license"`
	LicenseType  string  `json:"license_type,omitempty"`
	LicenseScope string  `json:"license_scope,omitempty"`
	RoyaltyRate  float64 `json:"royalty_rate,omitempty"`
}

// InfringementOutput is the complete structured result of infringement analysis.
type InfringementOutput struct {
	Verdict           InfringementVerdict `json:"verdict"`
	ClaimScope        ClaimScopeResult    `json:"claim_scope"`
	FeatureMapping    []FeatureComparison `json:"feature_mapping"`
	LiteralResult     LiteralResult       `json:"literal_result"`
	EquivalenceResult EquivalenceResult   `json:"equivalence_result"`
	DefenseAnalysis   []DefenseAssessment `json:"defense_analysis"`
	RemedyAssessment  RemedyResult        `json:"remedy_assessment"`
	StrategyAdvice    StrategyResult      `json:"strategy_advice"`
	Confidence        float64             `json:"confidence"`
	Disclaimer        string              `json:"disclaimer"`
	CitationRefs      []CitationRef       `json:"citation_refs"`
}

// InfringementVerdict is the overall infringement conclusion.
type InfringementVerdict struct {
	Conclusion  string   `json:"conclusion"` // "infringed" | "not_infringed" | "uncertain"
	Likelihood  float64  `json:"likelihood"` // 0.0 - 1.0
	Basis       []string `json:"basis"`      // "literal" | "equivalence"
	KeyFindings []string `json:"key_findings"`
	RiskLevel   string   `json:"risk_level"` // "high" | "medium" | "low"
}

// ClaimScopeResult captures the interpreted protection scope.
type ClaimScopeResult struct {
	InterpretedScope      string           `json:"interpreted_scope"`
	KeyTerms              []TermDefinition `json:"key_terms"`
	DisclaimersIdentified []string         `json:"disclaimers_identified"`
}

// TermDefinition pairs a claim term with its interpretation.
type TermDefinition struct {
	Term           string `json:"term"`
	Interpretation string `json:"interpretation"`
	EvidenceSource string `json:"evidence_source"` // "intrinsic" | "extrinsic"
}

// FeatureComparison maps a claim feature to its accused product counterpart.
type FeatureComparison struct {
	ClaimFeature   string `json:"claim_feature"`
	ProductFeature string `json:"product_feature"`
	MatchType      string `json:"match_type"` // "literal" | "equivalent" | "missing"
	MatchReasoning string `json:"match_reasoning"`
}

// LiteralResult captures the all-elements rule analysis.
type LiteralResult struct {
	AllElementsMet  bool     `json:"all_elements_met"`
	MissingFeatures []string `json:"missing_features"`
	ExtraFeatures   []string `json:"extra_features"`
}

// EquivalenceResult captures the doctrine of equivalents analysis.
type EquivalenceResult struct {
	EquivalentFeatures []EquivalenceAssessment `json:"equivalent_features"`
	EstoppelApplied    bool                    `json:"estoppel_applied"`
	EstoppelDetails    string                  `json:"estoppel_details"`
	DedicationApplied  bool                    `json:"dedication_applied"`
	DedicationDetails  string                  `json:"dedication_details"`
}

// EquivalenceAssessment evaluates one feature pair under the means/function/effect test.
type EquivalenceAssessment struct {
	ClaimFeature   string `json:"claim_feature"`
	ProductFeature string `json:"product_feature"`
	SameMeans      bool   `json:"same_means"`
	SameFunction   bool   `json:"same_function"`
	SameEffect     bool   `json:"same_effect"`
	NonObviousness bool   `json:"non_obviousness"`
	IsEquivalent   bool   `json:"is_equivalent"`
	Reasoning      string `json:"reasoning"`
}

// DefenseAssessment evaluates a single defense theory.
type DefenseAssessment struct {
	DefenseType     string   `json:"defense_type"`
	Applicable      bool     `json:"applicable"`
	ViabilityRating string   `json:"viability_rating"`
	Analysis        string   `json:"analysis"`
	EvidenceNeeded  []string `json:"evidence_needed"`
	LegalBasis      string   `json:"legal_basis"`
}

// RemedyResult aggregates damages and injunction analysis.
type RemedyResult struct {
	DamageEstimate     DamageEstimate     `json:"damage_estimate"`
	InjunctionAnalysis InjunctionAnalysis `json:"injunction_analysis"`
	PunitiveRisk       *PunitiveRisk      `json:"punitive_risk,omitempty"`
}

// DamageEstimate follows the four-tier cascade of Article 65.
type DamageEstimate struct {
	Method           string  `json:"method"`
	EstimatedAmount  float64 `json:"estimated_amount"`
	RangeLow         float64 `json:"range_low"`
	RangeHigh        float64 `json:"range_high"`
	CalculationBasis string  `json:"calculation_basis"`
}

// InjunctionAnalysis covers both preliminary and permanent injunctions.
type InjunctionAnalysis struct {
	PreliminaryInjunction *InjunctionFactors `json:"preliminary_injunction"`
	PermanentInjunction   *InjunctionFactors `json:"permanent_injunction"`
}

// InjunctionFactors covers the five-factor test for injunctive relief.
type InjunctionFactors struct {
	LikelihoodOfSuccess string  `json:"likelihood_of_success"`
	IrreparableHarm     string  `json:"irreparable_harm"`
	BalanceOfHardships  string  `json:"balance_of_hardships"`
	PublicInterest      string  `json:"public_interest"`
	BondRequired        float64 `json:"bond_required"`
}

// PunitiveRisk assesses exposure to punitive damages (2020 amendment).
type PunitiveRisk struct {
	Willfulness    string  `json:"willfulness"`
	MultiplierLow  float64 `json:"multiplier_low"`
	MultiplierHigh float64 `json:"multiplier_high"`
	Analysis       string  `json:"analysis"`
}

// StrategyResult provides role-specific litigation strategy advice.
type StrategyResult struct {
	RecommendedActions   []StrategyAction      `json:"recommended_actions"`
	JurisdictionAnalysis *JurisdictionAdvice   `json:"jurisdiction_analysis,omitempty"`
	Timeline             []TimelineMilestone   `json:"timeline"`
	SettlementAssessment *SettlementAdvice     `json:"settlement_assessment,omitempty"`
	InvalidationRoute    *InvalidationStrategy `json:"invalidation_route,omitempty"`
}

// StrategyAction is a single recommended action with priority.
type StrategyAction struct {
	Action    string `json:"action"`
	Priority  string `json:"priority"`
	Rationale string `json:"rationale"`
	RiskLevel string `json:"risk_level"`
}

// JurisdictionAdvice covers venue selection strategy.
type JurisdictionAdvice struct {
	RecommendedVenues []string `json:"recommended_venues"`
	Rationale         string   `json:"rationale"`
}

// TimelineMilestone is a key event in the litigation timeline.
type TimelineMilestone struct {
	Event       string `json:"event"`
	Timeframe   string `json:"timeframe"`
	Criticality string `json:"criticality"`
}

// SettlementAdvice assesses settlement feasibility.
type SettlementAdvice struct {
	Recommendation string   `json:"recommendation"`
	RangeLow       float64  `json:"range_low"`
	RangeHigh      float64  `json:"range_high"`
	KeyFactors     []string `json:"key_factors"`
}

// InvalidationStrategy outlines patent invalidity attack paths.
type InvalidationStrategy struct {
	Grounds       []string `json:"grounds"`
	PriorArtRefs  []string `json:"prior_art_refs"`
	SuccessChance string   `json:"success_chance"`
	Timeline      string   `json:"timeline"`
}

// GuidelineRef is a reference to examination guidelines.
type GuidelineRef struct {
	Source  string `json:"source"`
	Section string `json:"section"`
	Content string `json:"content"`
}

// CaseRef is a reference to a judicial case.
type CaseRef struct {
	CaseName   string `json:"case_name"`
	CaseNumber string `json:"case_number"`
	Court      string `json:"court"`
	Holding    string `json:"holding"`
	Relevance  string `json:"relevance"`
}

// CitationRef is a verified legal citation.
type CitationRef struct {
	Article   string `json:"article"`
	Content   string `json:"content"`
	Relevance string `json:"relevance"`
}

// State keys for the Pregel shared state.
const (
	StateInput             = "inf_input"
	StatePerspective       = "inf_perspective"
	StateClaimScope        = "inf_claim_scope"
	StateClaimFeatures     = "inf_claim_features"
	StateProductFeatures   = "inf_product_features"
	StateFeatureMapping    = "inf_feature_mapping"
	StateLiteralResult     = "inf_literal_result"
	StateEquivalenceResult = "inf_equivalence_result"
	StateVerdict           = "inf_verdict"
	StateDefenseAnalysis   = "inf_defense_analysis"
	StateRemedyAssessment  = "inf_remedy_assessment"
	StateStrategy          = "inf_strategy"
	StateOutput            = "inf_output"
	StateSkipped = "inf_skipped"
)

// Rule severity levels, matching domains/rules convention.
const (
	SeverityBlock  = "block"
	SeverityMust   = "must"
	SeverityShould = "should"
	SeverityMay    = "may"
)
