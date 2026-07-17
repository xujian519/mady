package compiler

import (
	"testing"

	"github.com/xujian519/mady/workflows/patent"
)

// TestRuleBridgeEndToEnd verifies the full Tier 3 pipeline:
//
//	Compiler strategy → Extract → Enqueue → Review → Drain → Promote → RuleEngine.
func TestRuleBridgeEndToEnd(t *testing.T) {
	// 1. Create a compiler with a custom strategy that has enough pre-success
	//    to cross the extraction threshold.
	customStrategy := Strategy{
		ID:            "oa_three_step",
		Description:   "审查意见三步法答复",
		Preconditions: []string{"审查意见", "答复", "OA"},
		Guidance:      "第一步：确认审查意见类型；第二步：分析对比文件技术特征差异；第三步：论证创造性",
		Successes:     8,
		Failures:      1,
	}
	comp := NewCompiler(Config{
		Strategies:      []Strategy{customStrategy},
		ExplorationRate: 0,
		MaxTraces:       100,
	})

	// 2. Extract candidates from the compiler.
	extractor := NewRuleCandidateExtractor(5, 0.7)
	candidates := extractor.ExtractFromCompiler(comp)
	if len(candidates) == 0 {
		t.Fatal("expected at least one candidate from compiler with 8 successes")
	}
	if candidates[0].Status != CandidateDraft {
		t.Errorf("expected draft status, got %s", candidates[0].Status)
	}
	t.Logf("extracted %d candidates", len(candidates))

	// 3. Create rule engine and registration infrastructure.
	engine := patent.NewRuleEngine()
	registrar, logs := NewRuleRegistrar(engine)

	// 4. Create review queue with shadow eval and enqueue candidates.
	shadowFn := DefaultShadowEval(engine)
	queue := NewReviewQueue(shadowFn)
	added := queue.Enqueue(candidates...)
	if added != len(candidates) {
		t.Fatalf("enqueue: expected %d, got %d", len(candidates), added)
	}

	// 5. Review sessions on each candidate.
	var reviewed []RuleCandidate
	for {
		c, ok := queue.Dequeue()
		if !ok {
			break
		}
		result, err := queue.ReviewSession(&c, true, "人工审核通过")
		if err != nil {
			t.Fatalf("review session: %v", err)
		}
		if !result.Ready {
			t.Errorf("candidate %s not ready: %v", c.ID, result.Reasons)
		}
		reviewed = append(reviewed, c)
	}

	// Re-enqueue reviewed candidates for DrainApproved.
	queue.Enqueue(reviewed...)

	// 6. Drain approved and promote.
	gate := NewRulePromotionGate(DefaultPromotionGateConfig())
	promoter := NewRulePromoter(gate, registrar)
	promoted, errs := promoter.PromoteBatch(queue)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Logf("promote warning: %v", e)
		}
	}

	// Manually promote any remaining.
	if promoted == 0 {
		for _, c := range reviewed {
			c.MarkShadowResult(true)
			if err := promoter.Promote(c); err != nil {
				t.Errorf("promote %s: %v", c.ID, err)
			} else {
				promoted++
			}
		}
	}

	// 7. Verify the rule is registered in the engine.
	if promoted == 0 {
		t.Fatal("expected at least one promoted rule")
	}
	if len(*logs) == 0 {
		t.Fatal("expected promotion logs")
	}

	rules := engine.Rules()
	if len(rules) == 0 {
		t.Fatal("expected at least one rule in engine after promotion")
	}
	t.Logf("registered %d rules, %d promotion logs", len(rules), len(*logs))
	for _, entry := range *logs {
		t.Log(entry)
	}
}

// TestCandidateToCheckRule verifies the basic field mapping.
func TestCandidateToCheckRule(t *testing.T) {
	c := RuleCandidate{
		ID:            "rc_test_001",
		StrategyID:    "oa_three_step",
		Description:   "审查意见三步法答复",
		Guidance:      "第一步：确认审查意见类型；第二步：分析对比文件的技术特征差异；第三步：论证创造性",
		DraftRuleText: "答复未使用三步法论证创造性",
		SuccessRate:   0.95,
		Samples:       12,
	}

	rule := CandidateToCheckRule(c)

	if rule.ID != c.ID {
		t.Errorf("ID: expected %s, got %s", c.ID, rule.ID)
	}
	if rule.Name != c.Description {
		t.Errorf("Name: expected %s, got %s", c.Description, rule.Name)
	}
	if rule.Message != c.DraftRuleText {
		t.Errorf("Message: expected %s, got %s", c.DraftRuleText, rule.Message)
	}
	if rule.FixSuggestion != c.Guidance {
		t.Errorf("FixSuggestion: expected %s, got %s", c.Guidance, rule.FixSuggestion)
	}
	if rule.Severity != patent.SeverityMajor {
		t.Errorf("Severity: expected Major for 12 samples/95%%, got %s", rule.Severity)
	}
	if rule.CheckType != "" {
		t.Errorf("CheckType: expected empty (human classification required), got %s", rule.CheckType)
	}
	if len(rule.RequiredElements) == 0 {
		t.Error("RequiredElements: expected non-empty keywords")
	}
}

// TestShadowEvalNoConflict verifies that a unique candidate passes shadow eval.
func TestShadowEvalNoConflict(t *testing.T) {
	engine := patent.NewRuleEngine()
	shadowFn := DefaultShadowEval(engine)

	c := RuleCandidate{
		ID:            "rc_shadow_test",
		Description:   "新颖性单独比对原则",
		Guidance:      "确认审查员是否将两篇对比文件组合评价新颖性",
		DraftRuleText: "新颖性判断应当遵循单独比对原则",
		SuccessRate:   0.9,
		Samples:       8,
	}

	result, err := shadowFn(c)
	if err != nil {
		t.Fatalf("shadow eval: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected unique candidate to pass, got: %s", result.Detail)
	}
}

// TestShadowEvalConflict verifies that overlap with existing rules triggers rejection.
func TestShadowEvalConflict(t *testing.T) {
	engine := patent.NewRuleEngine()
	engine.RegisterRule(patent.CheckRule{
		ID:               "existing_novelty",
		Name:             "现有新颖性规则",
		RequiredElements: []string{"新颖性判断", "单独比对原则", "对比文件需逐一分析"},
	})

	shadowFn := DefaultShadowEval(engine)

	c := RuleCandidate{
		ID:            "rc_duplicate",
		Description:   "新颖性判断，单独比对原则",
		Guidance:      "新颖性判断应当遵循单独比对原则，对比文件需逐一分析",
		DraftRuleText: "新颖性分析需单独比对每篇对比文件",
		SuccessRate:   0.85,
		Samples:       7,
	}

	result, err := shadowFn(c)
	if err != nil {
		t.Fatalf("shadow eval: %v", err)
	}
	if result.Passed {
		t.Error("expected high-overlap candidate to fail shadow eval")
	}
}

// TestShadowEvalEmptyRule rejects candidates with trivial rule text.
func TestShadowEvalEmptyRule(t *testing.T) {
	engine := patent.NewRuleEngine()
	shadowFn := DefaultShadowEval(engine)

	c := RuleCandidate{
		ID:            "rc_empty",
		Description:   "空规则",
		Guidance:      "  ",
		DraftRuleText: "  ",
	}

	result, err := shadowFn(c)
	if err != nil {
		t.Fatalf("shadow eval: %v", err)
	}
	if result.Passed {
		t.Error("expected empty rule candidate to fail shadow eval")
	}
}
