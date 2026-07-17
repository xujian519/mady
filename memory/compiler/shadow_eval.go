package compiler

import (
	"fmt"
	"strings"
	"time"

	"github.com/xujian519/mady/workflows/patent"
)

// DefaultShadowEval creates a ShadowEvalFunc that performs basic
// conflict and overlap detection with existing rules in the engine.
//
// Phase 3 implementation: keyword-overlap based heuristic.
// Phase 4 upgrade: run candidate against Golden Benchmark via
//
//	agentcore/evaluate/benchmark.RunStatic.
//
// The evaluation checks:
//  1. Keyword overlap with existing rules (>70% → potential duplicate)
//  2. Empty or trivial rule text (reject)
//
// Returns passed=true if no conflicts are detected.
func DefaultShadowEval(engine *patent.RuleEngine) ShadowEvalFunc {
	return func(c RuleCandidate) (ShadowEvalResult, error) {
		if c.DraftRuleText == "" || len(strings.TrimSpace(c.DraftRuleText)) < 5 {
			return ShadowEvalResult{
				Passed: false,
				Score:  0,
				Detail: "规则文本为空或过短",
				RunAt:  time.Now(),
			}, nil
		}

		// Build keyword set for the candidate.
		candidateKW := make(map[string]bool)
		for _, kw := range extractKeywordsFromCandidate(c) {
			candidateKW[strings.ToLower(kw)] = true
		}

		if len(candidateKW) == 0 {
			return ShadowEvalResult{
				Passed: true,
				Score:  1.0,
				Detail: "候选规则无关键词，跳过冲突检测",
				RunAt:  time.Now(),
			}, nil
		}

		// Check overlap with each existing rule.
		for _, rule := range engine.Rules() {
			overlap := keywordOverlap(candidateKW, rule.RequiredElements)
			if overlap > 0.7 {
				return ShadowEvalResult{
					Passed: false,
					Score:  1.0 - overlap,
					Detail: fmt.Sprintf("与已有规则 %s 关键词重叠度 %d%% 过高，可能重复",
						rule.ID, int(overlap*100)),
					RunAt: time.Now(),
				}, nil
			}
		}

		return ShadowEvalResult{
			Passed: true,
			Score:  1.0,
			Detail: fmt.Sprintf("无冲突（已对比 %d 条已有规则）", len(engine.Rules())),
			RunAt:  time.Now(),
		}, nil
	}
}

// keywordOverlap computes the Jaccard-like overlap ratio between a candidate
// keyword set and a list of rule keywords.
func keywordOverlap(candidateKW map[string]bool, ruleKWs []string) float64 {
	if len(candidateKW) == 0 || len(ruleKWs) == 0 {
		return 0
	}

	matched := 0
	for _, rk := range ruleKWs {
		if candidateKW[strings.ToLower(rk)] {
			matched++
		}
	}

	// Jaccard-like overlap: union denominator prevents narrow candidates from
	// being incorrectly flagged as duplicates against broad existing rules.
	denom := len(candidateKW) + len(ruleKWs) - matched
	if denom <= 0 {
		return 0
	}

	return float64(matched) / float64(denom)
}
