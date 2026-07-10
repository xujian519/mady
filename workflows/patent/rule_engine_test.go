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
	results := engine.Evaluate(engine.Rules(), goodText, "patent_novelty")

	for _, r := range results {
		if r.RuleID == "INVENTIVENESS-THREE-STEP" && !r.Passed {
			t.Errorf("complete three-step should pass: %s", r.Message)
		}
	}

	// Text missing steps.
	badText := "这个发明有创造性，非显而易见。"
	badResults := engine.Evaluate(engine.Rules(), badText, "patent_novelty")
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
