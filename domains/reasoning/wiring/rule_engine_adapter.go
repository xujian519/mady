package wiring

import (
	"context"
	"strings"

	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/domains/rules"
)

// RuleEngineAdapter wraps a *rules.Engine as a reasoning.RuleEngineSource,
// surfacing the deterministic YAML rules (NOV-001~004, etc.) as the
// highest-authority lane in Stage ② rule acquisition.
//
// caseType is mapped to one or more rule domains (e.g. patentability →
// patent_novelty + patent_inventiveness) so a single case pulls all relevant
// deterministic rules. When the caseType is unmapped, the adapter falls back
// to a keyword search over the engine so unknown case types still get rules
// rather than nothing.
type RuleEngineAdapter struct {
	engine *rules.Engine
}

// NewRuleEngineAdapter binds a rules engine. Returns nil when engine is nil
// so callers can let a nil adapter disable the deterministic-rules lane.
func NewRuleEngineAdapter(engine *rules.Engine) *RuleEngineAdapter {
	if engine == nil {
		return nil
	}
	return &RuleEngineAdapter{engine: engine}
}

// caseTypeDomains maps a reasoning CaseType to the rule domains whose
// deterministic rules apply. A case may span several domains (e.g.
// patentability covers both novelty and inventiveness checks).
var caseTypeDomains = map[string][]string{
	"novelty_search": {"patent_novelty"},
	"patentability":  {"patent_novelty", "patent_inventiveness"},
	"drafting":       {"patent_claims", "patent_disclosure"},
	"invalidation":   {"patent_novelty", "patent_inventiveness"},
	// infringement has no dedicated rule domain yet; falls back to keyword search.
}

// MatchRules implements reasoning.RuleEngineSource. It resolves the caseType
// to rule domains, converts each domain's rules via Engine.ToRuleConstraints,
// and wraps them as RetrievedRules with the highest authority tier.
func (a *RuleEngineAdapter) MatchRules(ctx context.Context, caseType string, queryCtx map[string]string) ([]reasoning.RetrievedRule, error) {
	if a == nil || a.engine == nil {
		return nil, nil
	}
	domains := caseTypeDomains[caseType]

	// Fallback: unmapped caseType → keyword search over all rules.
	if len(domains) == 0 {
		return a.matchByKeyword(queryCtx), nil
	}

	var out []reasoning.RetrievedRule
	seen := make(map[string]bool) // dedupe by ArticleID across domains
	for _, d := range domains {
		for _, rc := range a.engine.ToRuleConstraints(d) {
			if seen[rc.ArticleID] {
				continue
			}
			seen[rc.ArticleID] = true
			out = append(out, ruleConstraintToRetrieved(rc))
		}
	}
	return out, nil
}

// matchByKeyword falls back to SearchRules when the caseType has no domain
// mapping. The first non-empty keyword from queryCtx is used.
func (a *RuleEngineAdapter) matchByKeyword(queryCtx map[string]string) []reasoning.RetrievedRule {
	kw := strings.TrimSpace(queryCtx["keywords"])
	if kw == "" {
		return nil
	}
	// SearchRules does substring matching; use the first keyword token.
	if idx := strings.IndexByte(kw, ' '); idx > 0 {
		kw = kw[:idx]
	}
	matched := a.engine.SearchRules(kw)
	out := make([]reasoning.RetrievedRule, 0, len(matched))
	for _, r := range matched {
		// Reuse ToRuleConstraints logic by converting each matched rule.
		rcs := a.engine.ToRuleConstraints(r.Domain)
		for _, rc := range rcs {
			if rc.ArticleID == r.RuleID {
				out = append(out, ruleConstraintToRetrieved(rc))
				break
			}
		}
	}
	return out
}

// ruleConstraintToRetrieved wraps a RuleConstraint from the deterministic
// engine as a RetrievedRule. AuthorityScore is the highest tier (0.95):
// these are code-curated law-article rules, above retrieved corpus fragments
// (0.7) and wiki experience cards (0.4). Priority reflects the Requirement
// level the engine assigned (Must=1, Should=2, Note=3).
func ruleConstraintToRetrieved(rc reasoning.RuleConstraint) reasoning.RetrievedRule {
	priority := 3
	switch rc.Requirement {
	case reasoning.ReqMust:
		priority = 1
	case reasoning.ReqShould:
		priority = 2
	}
	return reasoning.RetrievedRule{
		Rule:           rc,
		Source:         reasoning.RuleSourceRules,
		Priority:       priority,
		AuthorityScore: 0.95, // highest tier: deterministic, code-curated
		Confidence:     1.0,  // deterministic rules are exact matches, not probabilistic
	}
}
