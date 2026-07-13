package compiler

import (
	"testing"
)

func TestRuleCandidateExtractor_ExtractFromCompiler(t *testing.T) {
	c := NewCompiler(Config{
		Strategies: []Strategy{
			{
				ID:          "high_perf",
				Description: "高性能策略",
				Guidance:    "使用三步法分析",
				Successes:   9,
				Failures:    1,
			},
			{
				ID:          "low_perf",
				Description: "低性能策略",
				Guidance:    "简单回答",
				Successes:   2,
				Failures:    8,
			},
			{
				ID:          "too_few_samples",
				Description: "样本不足",
				Guidance:    "快速分析",
				Successes:   3,
				Failures:    0,
			},
		},
	})

	extractor := NewRuleCandidateExtractor(5, 0.7)
	candidates := extractor.ExtractFromCompiler(c)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].StrategyID != "high_perf" {
		t.Errorf("expected high_perf, got %s", candidates[0].StrategyID)
	}
	if candidates[0].SuccessRate != 0.9 {
		t.Errorf("expected 0.9, got %f", candidates[0].SuccessRate)
	}
	if candidates[0].Samples != 10 {
		t.Errorf("expected 10 samples, got %d", candidates[0].Samples)
	}
	if candidates[0].Status != CandidateDraft {
		t.Errorf("expected draft status, got %s", candidates[0].Status)
	}
	if candidates[0].DraftRuleText == "" {
		t.Error("expected non-empty draft rule text")
	}
}

func TestRuleCandidateExtractor_EmptyCompiler(t *testing.T) {
	c := NewCompiler(Config{})
	extractor := NewRuleCandidateExtractor(5, 0.7)
	candidates := extractor.ExtractFromCompiler(c)
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates from default strategies (untested), got %d", len(candidates))
	}
}

func TestRuleCandidateExtractor_Defaults(t *testing.T) {
	e := NewRuleCandidateExtractor(0, 0)
	if e.MinSamples != 5 {
		t.Errorf("expected default minSamples 5, got %d", e.MinSamples)
	}
	if e.MinSuccessRate != 0.7 {
		t.Errorf("expected default minSuccessRate 0.7, got %f", e.MinSuccessRate)
	}
}

func TestRulePromotionGate_Evaluate_Ready(t *testing.T) {
	gate := NewRulePromotionGate(DefaultPromotionGateConfig())
	c := RuleCandidate{
		Samples:       10,
		SuccessRate:   0.9,
		HumanApproved: true,
		ShadowPassed:  true,
	}
	result := gate.Evaluate(c)
	if !result.Ready {
		t.Errorf("expected ready, got reasons: %v", result.Reasons)
	}
}

func TestRulePromotionGate_Evaluate_NotReady(t *testing.T) {
	gate := NewRulePromotionGate(DefaultPromotionGateConfig())
	c := RuleCandidate{
		Samples:       3,
		SuccessRate:   0.6,
		HumanApproved: false,
		ShadowPassed:  false,
	}
	result := gate.Evaluate(c)
	if result.Ready {
		t.Error("expected not ready")
	}
	if len(result.Reasons) != 4 {
		t.Errorf("expected 4 reasons, got %d: %v", len(result.Reasons), result.Reasons)
	}
}

func TestRulePromotionGate_PartialRequirements(t *testing.T) {
	gate := NewRulePromotionGate(DefaultPromotionGateConfig())
	c := RuleCandidate{
		Samples:       10,
		SuccessRate:   0.9,
		HumanApproved: true,
		ShadowPassed:  false,
	}
	result := gate.Evaluate(c)
	if result.Ready {
		t.Error("expected not ready")
	}
	if len(result.Reasons) != 1 {
		t.Errorf("expected 1 reason, got %d: %v", len(result.Reasons), result.Reasons)
	}
}

func TestRuleCandidate_MarkHumanApproval(t *testing.T) {
	c := RuleCandidate{
		Status: CandidateDraft,
	}
	c.MarkHumanApproval(true, "审核通过，规则合理")
	if !c.HumanApproved {
		t.Error("expected approved")
	}
	if c.Status != CandidateApproved {
		t.Errorf("expected approved status, got %s", c.Status)
	}
	if c.ReviewedAt == nil {
		t.Error("expected reviewed_at to be set")
	}
	if c.ReviewerNote != "审核通过，规则合理" {
		t.Errorf("unexpected note: %s", c.ReviewerNote)
	}
}

func TestRuleCandidate_MarkHumanRejection(t *testing.T) {
	c := RuleCandidate{
		Status: CandidateDraft,
	}
	c.MarkHumanApproval(false, "规则描述不够精确")
	if c.HumanApproved {
		t.Error("expected not approved")
	}
	if c.Status != CandidateRejected {
		t.Errorf("expected rejected status, got %s", c.Status)
	}
}

func TestRuleCandidate_MarkShadowResult(t *testing.T) {
	c := RuleCandidate{}
	c.MarkShadowResult(true)
	if !c.ShadowPassed {
		t.Error("expected shadow passed")
	}
}

func TestDefaultPromotionGateConfig(t *testing.T) {
	cfg := DefaultPromotionGateConfig()
	if cfg.MinSamples != 5 {
		t.Errorf("expected minSamples 5, got %d", cfg.MinSamples)
	}
	if cfg.MinSuccessRate != 0.8 {
		t.Errorf("expected minSuccessRate 0.8, got %f", cfg.MinSuccessRate)
	}
	if !cfg.RequireHumanApproval {
		t.Error("expected require human approval")
	}
	if !cfg.RequireShadowEval {
		t.Error("expected require shadow eval")
	}
}
