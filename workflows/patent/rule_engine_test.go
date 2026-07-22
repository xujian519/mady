package patent

import (
	"strings"
	"testing"
)

func TestMatchKeyword(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		keyword string
		want    bool
	}{
		{"direct match", "本发明具有新颖性", "新颖性", true},
		{"synonym match", "该方案不属于现有技术", "新颖性", true},
		{"negated no match", "该方案不具有新颖性", "新颖性", false},
		{"no mention", "这是一个茶杯", "创造性", false},
		{"english synonym", "the inventive step is obvious", "创造性", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchKeyword(tt.text, tt.keyword)
			if got != tt.want {
				t.Errorf("matchKeyword(%q, %q) = %v, want %v", tt.text, tt.keyword, got, tt.want)
			}
		})
	}
}

func TestMatchKeywordsAll(t *testing.T) {
	if !matchKeywordsAll("新颖性和创造性", []string{"新颖性", "创造性"}) {
		t.Error("both present should match all")
	}
	if matchKeywordsAll("只有新颖性", []string{"新颖性", "创造性"}) {
		t.Error("missing one should not match all")
	}
}

func TestAggregate(t *testing.T) {
	tests := []struct {
		name    string
		results []RuleCheckResult
		want    Verdict
	}{
		{"empty", nil, VerdictPass},
		{"level0 fail", []RuleCheckResult{{Passed: false, Level: LevelMust}}, VerdictBlocked},
		{"level1 fail", []RuleCheckResult{{Passed: false, Level: LevelShould}}, VerdictBlocked},
		{"two level2", []RuleCheckResult{{Passed: false, Level: LevelQuality}, {Passed: false, Level: LevelQuality}}, VerdictPass},
		{"three level2", []RuleCheckResult{{Passed: false, Level: LevelQuality}, {Passed: false, Level: LevelQuality}, {Passed: false, Level: LevelQuality}}, VerdictNeedsRevision},
		{"all pass", []RuleCheckResult{{Passed: true, Level: LevelMust}}, VerdictPass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Aggregate(tt.results)
			if got != tt.want {
				t.Errorf("Aggregate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRuleEngineEvaluate_Novelty(t *testing.T) {
	engine := NewRuleEngine()
	engine.RegisterRules(DefaultPatentRules())

	// Text that violates single-comparison principle.
	badText := "本发明的对比文件1-3结合公开了所有技术特征，不具有新颖性。"
	results := engine.Evaluate(engine.Rules(), badText, "patent_novelty")

	foundViolation := false
	for _, r := range results {
		if r.RuleID == "NOVELTY-SINGLE-COMPARISON" {
			foundViolation = true
			if r.Passed {
				t.Error("single-comparison violation should fail")
			}
		}
	}
	if !foundViolation {
		t.Error("expected NOVELTY-SINGLE-COMPARISON rule to be evaluated")
	}
}

func TestRuleEngineEvaluate_Inventiveness(t *testing.T) {
	engine := NewRuleEngine()
	engine.RegisterRules(DefaultPatentRules())

	// Text with complete three-step method.
	goodText := "最接近的现有技术是文献D1，区别技术特征在于使用了超声波，本领域技术人员无法获得技术启示，具有创造性。"
	results := engine.Evaluate(engine.Rules(), goodText, "patent_inventiveness")

	for _, r := range results {
		if r.RuleID == "INVENTIVENESS-THREE-STEP" && !r.Passed {
			t.Errorf("complete three-step should pass: %s", r.Message)
		}
	}

	// Text missing steps.
	badText := "这个发明有创造性，非显而易见。"
	badResults := engine.Evaluate(engine.Rules(), badText, "patent_inventiveness")
	badFailed := false
	for _, r := range badResults {
		if r.RuleID == "INVENTIVENESS-THREE-STEP" {
			badFailed = true
		}
	}
	if !badFailed {
		t.Error("incomplete inventiveness should trigger failure")
	}
}

func TestRuleEngineEvaluate_DomainFilter(t *testing.T) {
	engine := NewRuleEngine()
	engine.RegisterRules(DefaultPatentRules())

	// Disclosure rule has Domain="patent_disclosure"; evaluating with
	// domain="patent_novelty" should skip it.
	results := engine.Evaluate(engine.Rules(), "充分公开 能够实现", "patent_novelty")
	for _, r := range results {
		if r.RuleID == "DISCLOSURE-SUFFICIENCY" {
			t.Error("disclosure rule should be filtered out for patent_novelty domain")
		}
	}
}

func TestDefaultPatentRules(t *testing.T) {
	rules := DefaultPatentRules()
	if len(rules) < 5 {
		t.Fatalf("expected at least 5 default rules, got %d", len(rules))
	}

	// Verify required rule IDs exist.
	ids := make(map[string]bool)
	for _, r := range rules {
		ids[r.ID] = true
	}
	for _, want := range []string{"NOVELTY-SINGLE-COMPARISON", "INVENTIVENESS-THREE-STEP", "DISCLOSURE-SUFFICIENCY"} {
		if !ids[want] {
			t.Errorf("missing rule %s", want)
		}
	}
}

func TestFormatRuleResults(t *testing.T) {
	engine := NewRuleEngine()
	engine.RegisterRules(DefaultPatentRules())
	results := engine.Evaluate(engine.Rules(), "这是一个简单的茶杯", "patent_novelty")
	verdict := Aggregate(results)

	out := FormatRuleResults(results, verdict)
	if out == "" {
		t.Fatal("output should not be empty")
	}
	if !strings.Contains(out, "规则引擎检查") {
		t.Error("should contain header")
	}
	if !strings.Contains(out, "检查结论") {
		t.Error("should contain verdict label")
	}
}

func TestRuleEngineRegisterRemove(t *testing.T) {
	engine := NewRuleEngine()
	rule := CheckRule{ID: "TEST-1", Name: "Test", CheckType: CheckNovelty}
	engine.RegisterRule(rule)
	if _, ok := engine.GetRule("TEST-1"); !ok {
		t.Error("rule should be registered")
	}
	engine.RemoveRule("TEST-1")
	if _, ok := engine.GetRule("TEST-1"); ok {
		t.Error("rule should be removed")
	}
}

func TestScenarioRuleSets(t *testing.T) {
	// Verify each scenario rule set is non-empty and has valid IDs.
	tests := []struct {
		name     string
		rules    []CheckRule
		expected []string // expected rule IDs
	}{
		{"NoveltyRules", NoveltyRules(), []string{"NOVELTY-SINGLE-COMPARISON", "NOVELTY-FEATURE-COVERAGE"}},
		{"InventivenessRules", InventivenessRules(), []string{"INVENTIVENESS-THREE-STEP", "INVENTIVENESS-TECHNICAL-PROBLEM"}},
		{"InfringementRules", InfringementRules(), []string{"INFRINGEMENT-FULL-COVERAGE", "INFRINGEMENT-EQUIVALENCE", "INFRINGEMENT-ESTOPPEL"}},
		{"InvalidationRules", InvalidationRules(), []string{"INVALID-NOVELTY-SINGLE-COMPARISON", "INVALID-COMBINATION-MOTIVATION"}},
		{"ReexaminationRules", ReexaminationRules(), []string{"REEXAM-GROUNDS-SCOPE", "REEXAM-NEW-EVIDENCE"}},
		{"DisclosureRules", DisclosureRules(), []string{"DISCLOSURE-SUFFICIENCY", "CLAIM-CLARITY-SUPPORT"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.rules) == 0 {
				t.Fatalf("%s returned empty slice", tt.name)
			}
			ids := make(map[string]bool)
			for _, r := range tt.rules {
				if r.ID == "" {
					t.Error("rule has empty ID")
				}
				ids[r.ID] = true
			}
			for _, want := range tt.expected {
				if !ids[want] {
					t.Errorf("%s missing rule %s", tt.name, want)
				}
			}
		})
	}
}

func TestInfringementRules_Evaluation(t *testing.T) {
	engine := NewRuleEngine()
	engine.RegisterRules(InfringementRules())

	// Text missing equivalence analysis.
	badText := "权利要求的技术特征为A、B、C，被控方案包含全部特征，构成全面覆盖侵权。"
	results := engine.Evaluate(engine.Rules(), badText, "patent_infringement")

	foundEquivalenceViolation := false
	for _, r := range results {
		if r.RuleID == "INFRINGEMENT-EQUIVALENCE" {
			foundEquivalenceViolation = true
		}
	}
	if !foundEquivalenceViolation {
		t.Error("expected EQUIVALENCE rule to fail when equivalence analysis is missing")
	}

	// Text with full analysis.
	goodText := "权利要求的技术特征为A、B、C。被控方案进行全面覆盖比对，包含全部技术特征。" +
		"对于区别特征，采用等同原则分析，手段/功能/效果基本相同。" +
		"经审查，不存在禁止反悔情形，也不适用捐献规则。"
	goodResults := engine.Evaluate(engine.Rules(), goodText, "patent_infringement")
	verdict := Aggregate(goodResults)
	if verdict == VerdictBlocked {
		t.Error("complete infringement analysis should not be blocked")
	}
}

func TestInvalidationRules_Evaluation(t *testing.T) {
	engine := NewRuleEngine()
	engine.RegisterRules(InvalidationRules())

	// Text violating combination motivation requirement.
	badText := "对比文件1、2、3结合公开了权利要求的全部技术特征，不具有新颖性。"
	results := engine.Evaluate(engine.Rules(), badText, "patent_invalidation")

	foundViolation := false
	for _, r := range results {
		if r.RuleID == "INVALID-NOVELTY-SINGLE-COMPARISON" && !r.Passed {
			foundViolation = true
		}
	}
	if !foundViolation {
		t.Error("expected single-comparison violation in invalidation context")
	}
}

func TestDefaultPatentRules_AggregatesAll(t *testing.T) {
	rules := DefaultPatentRules()

	// Should contain rules from all scenario sets.
	ids := make(map[string]bool)
	for _, r := range rules {
		ids[r.ID] = true
	}

	// Check at least one rule from each scenario.
	required := []string{
		"NOVELTY-SINGLE-COMPARISON",      // from NoveltyRules
		"INVENTIVENESS-THREE-STEP",       // from InventivenessRules
		"DISCLOSURE-SUFFICIENCY",         // from DisclosureRules
		"INFRINGEMENT-FULL-COVERAGE",     // from InfringementRules
		"INVALID-COMBINATION-MOTIVATION", // from InvalidationRules
		"REEXAM-GROUNDS-SCOPE",           // from ReexaminationRules
	}
	for _, want := range required {
		if !ids[want] {
			t.Errorf("DefaultPatentRules missing %s", want)
		}
	}
}
