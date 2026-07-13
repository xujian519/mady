package compiler

import (
	"errors"
	"testing"
)

func TestRulePromoter_Promote_Success(t *testing.T) {
	var registered RuleCandidate
	registrar := func(c RuleCandidate) error {
		registered = c
		return nil
	}
	p := NewRulePromoter(NewRulePromotionGate(DefaultPromotionGateConfig()), registrar)

	c := RuleCandidate{
		ID:            "rc_001",
		StrategyID:    "oa_three_step",
		Samples:       10,
		SuccessRate:   0.9,
		HumanApproved: true,
		ShadowPassed:  true,
	}

	if err := p.Promote(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if registered.ID != "rc_001" {
		t.Errorf("expected registered ID rc_001, got %s", registered.ID)
	}
	logs := p.Logs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].CandidateID != "rc_001" {
		t.Errorf("expected log rc_001, got %s", logs[0].CandidateID)
	}
}

func TestRulePromoter_Promote_GateRejected(t *testing.T) {
	registrar := func(c RuleCandidate) error {
		t.Error("registrar should not be called for rejected candidate")
		return nil
	}
	p := NewRulePromoter(nil, registrar)

	c := RuleCandidate{
		ID:            "rc_001",
		Samples:       2,
		SuccessRate:   0.5,
		HumanApproved: false,
		ShadowPassed:  false,
	}

	err := p.Promote(c)
	if err == nil {
		t.Fatal("expected error for gate rejection")
	}
}

func TestRulePromoter_Promote_RegistrarError(t *testing.T) {
	registrar := func(c RuleCandidate) error {
		return errors.New("rule engine full")
	}
	p := NewRulePromoter(nil, registrar)

	c := RuleCandidate{
		ID:            "rc_001",
		Samples:       10,
		SuccessRate:   0.9,
		HumanApproved: true,
		ShadowPassed:  true,
	}

	err := p.Promote(c)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(p.Logs()) != 0 {
		t.Error("should not log failed promotion")
	}
}

func TestRulePromoter_PromoteBatch(t *testing.T) {
	count := 0
	registrar := func(c RuleCandidate) error {
		count++
		return nil
	}
	p := NewRulePromoter(nil, registrar)

	queue := NewReviewQueue(nil)
	queue.Enqueue(
		RuleCandidate{ID: "a", Status: CandidateDraft, HumanApproved: true, Samples: 10, SuccessRate: 0.9, ShadowPassed: true},
		RuleCandidate{ID: "b", Status: CandidateDraft},
		RuleCandidate{ID: "c", Status: CandidateDraft, HumanApproved: true, Samples: 8, SuccessRate: 0.85, ShadowPassed: true},
	)

	promoted, errs := p.PromoteBatch(queue)
	if promoted != 2 {
		t.Errorf("expected 2 promoted, got %d", promoted)
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
	}
	if count != 2 {
		t.Errorf("expected 2 registrations, got %d", count)
	}
	if queue.Pending() != 1 {
		t.Errorf("expected 1 remaining in queue, got %d", queue.Pending())
	}
}

func TestRulePromoter_PromoteBatch_PartialFailure(t *testing.T) {
	registrar := func(c RuleCandidate) error {
		if c.ID == "b" {
			return errors.New("registration failed")
		}
		return nil
	}
	p := NewRulePromoter(nil, registrar)

	queue := NewReviewQueue(nil)
	queue.Enqueue(
		RuleCandidate{ID: "a", Status: CandidateDraft, HumanApproved: true, Samples: 10, SuccessRate: 0.9, ShadowPassed: true},
		RuleCandidate{ID: "b", Status: CandidateDraft, HumanApproved: true, Samples: 10, SuccessRate: 0.9, ShadowPassed: true},
		RuleCandidate{ID: "c", Status: CandidateDraft, HumanApproved: true, Samples: 10, SuccessRate: 0.9, ShadowPassed: true},
	)

	promoted, errs := p.PromoteBatch(queue)
	if promoted != 2 {
		t.Errorf("expected 2 promoted, got %d", promoted)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}

func TestRulePromoter_Defaults(t *testing.T) {
	p := NewRulePromoter(nil, nil)
	if p.gate == nil {
		t.Error("expected non-nil gate")
	}
	c := RuleCandidate{
		Samples:       10,
		SuccessRate:   0.9,
		HumanApproved: true,
		ShadowPassed:  true,
	}
	err := p.Promote(c)
	if err == nil {
		t.Fatal("expected error for nil registrar")
	}
}

func TestPromoteFromCompiler(t *testing.T) {
	c := NewCompiler(Config{
		Strategies: []Strategy{
			{ID: "good", Successes: 9, Failures: 1, Guidance: "good rule"},
			{ID: "bad", Successes: 1, Failures: 9, Guidance: "bad rule"},
		},
	})
	queue := NewReviewQueue(nil)
	enqueued := PromoteFromCompiler(c, queue, 5, 0.7)
	if enqueued != 1 {
		t.Fatalf("expected 1 enqueued, got %d", enqueued)
	}
	if queue.Pending() != 1 {
		t.Errorf("expected 1 pending, got %d", queue.Pending())
	}
}

func TestPromotionLog_Fields(t *testing.T) {
	registrar := func(c RuleCandidate) error { return nil }
	p := NewRulePromoter(nil, registrar)

	c := RuleCandidate{
		ID:            "rc_test",
		StrategyID:    "strategy_x",
		Samples:       15,
		SuccessRate:   0.87,
		HumanApproved: true,
		ShadowPassed:  true,
		ReviewerNote:  "good rule",
	}

	p.Promote(c)
	logs := p.Logs()
	if len(logs) != 1 {
		t.Fatal("expected 1 log")
	}
	l := logs[0]
	if l.CandidateID != "rc_test" || l.StrategyID != "strategy_x" || l.Samples != 15 || l.SuccessRate != 0.87 {
		t.Errorf("unexpected log fields: %+v", l)
	}
	if l.Note != "good rule" {
		t.Errorf("unexpected note: %s", l.Note)
	}
	if l.PromotedAt.IsZero() {
		t.Error("expected non-zero promoted_at")
	}
}
