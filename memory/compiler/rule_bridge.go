package compiler

import (
	"fmt"
	"time"
)

// CandidateStatus describes the review lifecycle of a rule candidate.
type CandidateStatus string

const (
	CandidateDraft    CandidateStatus = "draft"
	CandidateReviewed CandidateStatus = "reviewed"
	CandidateApproved CandidateStatus = "approved"
	CandidateRejected CandidateStatus = "rejected"
)

// RuleCandidate is a distilled rule proposal extracted from high-performing
// compiler strategies. It bridges the learning compiler (prompt-level guidance)
// toward the deterministic rule engine (structured CheckRule).
//
// Tier 3 candidates are NEVER auto-promoted — every candidate requires
// explicit human approval and shadow evaluation before becoming a live rule.
type RuleCandidate struct {
	ID            string          `json:"id"`
	StrategyID    string          `json:"strategy_id"`
	Description   string          `json:"description"`
	Guidance      string          `json:"guidance"`
	SuccessRate   float64         `json:"success_rate"`
	Samples       int             `json:"samples"`
	DraftRuleText string          `json:"draft_rule_text"`
	Status        CandidateStatus `json:"status"`
	HumanApproved bool            `json:"human_approved"`
	ShadowPassed  bool            `json:"shadow_passed"`
	ReviewerNote  string          `json:"reviewer_note,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	ReviewedAt    *time.Time      `json:"reviewed_at,omitempty"`
}

// PromotionGateConfig controls the thresholds for promoting a RuleCandidate.
// All thresholds must be met; human approval is always required regardless
// of statistical confidence.
type PromotionGateConfig struct {
	MinSamples           int
	MinSuccessRate       float64
	RequireHumanApproval bool
	RequireShadowEval    bool
}

// DefaultPromotionGateConfig returns the conservative default thresholds:
// at least 5 samples, 80% success rate, mandatory human approval, and
// mandatory shadow evaluation.
func DefaultPromotionGateConfig() PromotionGateConfig {
	return PromotionGateConfig{
		MinSamples:           5,
		MinSuccessRate:       0.8,
		RequireHumanApproval: true,
		RequireShadowEval:    true,
	}
}

// PromotionResult reports whether a candidate is ready for promotion
// and lists any unmet requirements.
type PromotionResult struct {
	Ready   bool
	Reasons []string
}

// RuleCandidateExtractor extracts rule candidates from a Compiler's
// strategy statistics. Only strategies meeting the minimum sample and
// success-rate thresholds are considered.
type RuleCandidateExtractor struct {
	MinSamples     int
	MinSuccessRate float64
}

// NewRuleCandidateExtractor creates an extractor with the given thresholds.
// Defaults to 5 samples and 0.7 success rate (lower than promotion gate
// to cast a wider net for review).
func NewRuleCandidateExtractor(minSamples int, minSuccessRate float64) *RuleCandidateExtractor {
	if minSamples <= 0 {
		minSamples = 5
	}
	if minSuccessRate <= 0 || minSuccessRate > 1 {
		minSuccessRate = 0.7
	}
	return &RuleCandidateExtractor{
		MinSamples:     minSamples,
		MinSuccessRate: minSuccessRate,
	}
}

// ExtractFromCompiler collects candidates from a Compiler's strategies.
// It snapshots the strategies and filters by the extractor's thresholds.
func (e *RuleCandidateExtractor) ExtractFromCompiler(c *Compiler) []RuleCandidate {
	strategies := c.Strategies()
	now := time.Now()
	var candidates []RuleCandidate
	for _, s := range strategies {
		if s.Samples() < e.MinSamples {
			continue
		}
		if s.SuccessRate() < e.MinSuccessRate {
			continue
		}
		candidates = append(candidates, RuleCandidate{
			ID:            fmt.Sprintf("rc_%s_%d", s.ID, now.Unix()),
			StrategyID:    s.ID,
			Description:   s.Description,
			Guidance:      s.Guidance,
			SuccessRate:   s.SuccessRate(),
			Samples:       s.Samples(),
			DraftRuleText: s.Guidance,
			Status:        CandidateDraft,
			CreatedAt:     now,
		})
	}
	return candidates
}

// RulePromotionGate evaluates whether a RuleCandidate meets all
// requirements for promotion to a live rule.
type RulePromotionGate struct {
	cfg PromotionGateConfig
}

// NewRulePromotionGate creates a gate with the given configuration.
func NewRulePromotionGate(cfg PromotionGateConfig) *RulePromotionGate {
	if cfg.MinSamples <= 0 {
		cfg.MinSamples = 5
	}
	if cfg.MinSuccessRate <= 0 || cfg.MinSuccessRate > 1 {
		cfg.MinSuccessRate = 0.8
	}
	return &RulePromotionGate{cfg: cfg}
}

// Evaluate checks all promotion requirements for a candidate.
func (g *RulePromotionGate) Evaluate(c RuleCandidate) PromotionResult {
	var reasons []string

	if c.Samples < g.cfg.MinSamples {
		reasons = append(reasons, fmt.Sprintf("样本数不足: %d < %d", c.Samples, g.cfg.MinSamples))
	}
	if c.SuccessRate < g.cfg.MinSuccessRate {
		reasons = append(reasons, fmt.Sprintf("成功率不足: %.0f%% < %.0f%%", c.SuccessRate*100, g.cfg.MinSuccessRate*100))
	}
	if g.cfg.RequireHumanApproval && !c.HumanApproved {
		reasons = append(reasons, "未经人工批准")
	}
	if g.cfg.RequireShadowEval && !c.ShadowPassed {
		reasons = append(reasons, "未通过影子评估")
	}

	return PromotionResult{
		Ready:   len(reasons) == 0,
		Reasons: reasons,
	}
}

// MarkHumanApproval sets the human approval flag on a candidate.
// This is the ONLY way to set HumanApproved — it cannot be done
// automatically by any extractor or gate logic.
func (c *RuleCandidate) MarkHumanApproval(approved bool, note string) {
	c.HumanApproved = approved
	c.ReviewerNote = note
	now := time.Now()
	c.ReviewedAt = &now
	if approved {
		c.Status = CandidateApproved
	} else {
		c.Status = CandidateRejected
	}
}

// MarkShadowResult records the shadow evaluation outcome.
func (c *RuleCandidate) MarkShadowResult(passed bool) {
	c.ShadowPassed = passed
}
