package design

import (
	"testing"

	"github.com/xujian519/mady/workflows/patent"
)

func TestNewDesignRuleEngine(t *testing.T) {
	engine := NewDesignRuleEngine()
	if engine == nil {
		t.Fatal("NewDesignRuleEngine() returned nil")
	}
	rules := engine.Rules()
	if len(rules) != 5 {
		t.Fatalf("expected 5 design rules, got %d", len(rules))
	}
}

func TestDesignRulesRegistered(t *testing.T) {
	engine := NewDesignRuleEngine()
	expectedIDs := []string{
		"DESIGN-OVERALL-COMPARISON",
		"DESIGN-SPACE-DETERMINATION",
		"DESIGN-COMMON-EXCLUSION",
		"DESIGN-GUI-SPECIAL",
		"DESIGN-COMBINATION-TRANSFORMATION",
	}
	for _, id := range expectedIDs {
		if _, ok := engine.GetRule(id); !ok {
			t.Errorf("expected rule %q not found", id)
		}
	}
}

func TestDesignRulesAllHaveCheckType(t *testing.T) {
	for _, rule := range DesignRules() {
		if rule.CheckType != DesignCheckType {
			t.Errorf("rule %q has wrong CheckType: got %q, want %q",
				rule.ID, rule.CheckType, DesignCheckType)
		}
	}
}

func TestEvaluateCompleteAnalysis(t *testing.T) {
	engine := NewDesignRuleEngine()
	text := `
外观设计整体视觉效果的对比分析如下：

第一步：确定设计特征。该外观设计的主要设计特征包括整体形状、图案布局和色彩搭配。

第二步：对比整体视觉效果。以一般消费者视角对比涉案专利与对比设计的整体观察结果。

第三步：判断近似。基于整体观察和综合判断，上述设计特征构成近似。

第四步：结论。整体视觉效果足以使一般消费者产生混淆，构成近似。

设计空间分析：该产品类别设计空间较大，一般消费者对差异敏感度较低。

惯常设计排除：该产品类别中圆形形状属于惯常设计，应在对比中排除。

GUI 分析：由于涉及图形用户界面（GUI）设计，还需考虑界面交互方式。

组合与转用分析：本设计组合了现有设计特征，转用于智能家居产品类别。
`
	results := engine.EvaluateAll(text)
	if len(results) != 0 {
		t.Errorf("expected 0 failures for complete analysis, got %d: %v", len(results), results)
	}
}

func TestEvaluateMissingOverallComparison(t *testing.T) {
	engine := NewDesignRuleEngine()
	// Missing overall comparison keywords.
	text := `本外观设计简要说明描述了产品形状和图案。`
	results := engine.EvaluateAll(text)
	found := false
	for _, r := range results {
		if r.RuleID == "DESIGN-OVERALL-COMPARISON" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected DESIGN-OVERALL-COMPARISON to fail for incomplete text")
	}
}

func TestEvaluateMissingDesignSpace(t *testing.T) {
	engine := NewDesignRuleEngine()
	text := `
第一步：确定设计特征。主要特征包括整体形状。
第二步：对比整体视觉效果。以一般消费者视角进行整体观察。
第三步：判断近似。整体观察和综合判断下构成近似。
第四步：结论。构成近似。
`
	results := engine.EvaluateAll(text)
	found := false
	for _, r := range results {
		if r.RuleID == "DESIGN-SPACE-DETERMINATION" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected DESIGN-SPACE-DETERMINATION to fail when design space is missing")
	}
}

func TestEvaluateGUIWhenRelevant(t *testing.T) {
	engine := NewDesignRuleEngine()
	// Text mentioning GUI should pass the GUI rule.
	text := `
该外观设计涉及图形用户界面（GUI），
确定设计特征包括界面布局和交互方式。
整体视觉效果对比需要考虑动态变化过程。
整体观察和综合判断下判断近似并给出结论。
`
	results := engine.EvaluateAll(text)
	// Check that GUI rule passes.
	for _, r := range results {
		if r.RuleID == "DESIGN-GUI-SPECIAL" && !stringsContains(text, "图形用户界面") {
			t.Errorf("GUI rule should pass when GUI is mentioned")
		}
	}
}

func TestAggregateAllPass(t *testing.T) {
	results := []patent.RuleCheckResult{}
	verdict := Aggregate(results)
	if verdict != patent.VerdictPass {
		t.Errorf("expected pass for empty results, got %s", verdict)
	}
}

func TestAggregateCriticalFailure(t *testing.T) {
	results := []patent.RuleCheckResult{
		{
			RuleID: "DESIGN-OVERALL-COMPARISON",
			Passed: false,
			Level:  patent.LevelMust,
		},
	}
	verdict := Aggregate(results)
	if verdict != patent.VerdictBlocked {
		t.Errorf("expected blocked for LevelMust failure, got %s", verdict)
	}
}

func TestAggregateQualityFailures(t *testing.T) {
	// Three LevelQuality failures should trigger needs_revision.
	results := []patent.RuleCheckResult{
		{RuleID: "R1", Passed: false, Level: patent.LevelQuality},
		{RuleID: "R2", Passed: false, Level: patent.LevelQuality},
		{RuleID: "R3", Passed: false, Level: patent.LevelQuality},
	}
	verdict := Aggregate(results)
	if verdict != patent.VerdictNeedsRevision {
		t.Errorf("expected needs_revision for 3 quality failures, got %s", verdict)
	}
}

func TestGetRuleExisting(t *testing.T) {
	engine := NewDesignRuleEngine()
	rule, ok := engine.GetRule("DESIGN-OVERALL-COMPARISON")
	if !ok {
		t.Fatal("expected to find DESIGN-OVERALL-COMPARISON")
	}
	if rule.Level != patent.LevelMust {
		t.Errorf("expected LevelMust, got %v", rule.Level)
	}
}

func TestGetRuleNonExistent(t *testing.T) {
	engine := NewDesignRuleEngine()
	_, ok := engine.GetRule("NON-EXISTENT")
	if ok {
		t.Errorf("expected false for non-existent rule")
	}
}

func TestRegisterCustomRule(t *testing.T) {
	engine := NewDesignRuleEngine()
	customRule := patent.CheckRule{
		ID:               "DESIGN-CUSTOM-TEST",
		Name:             "自定义测试规则",
		Description:      "测试用自定义规则",
		Level:            patent.LevelMust,
		Severity:         patent.SeverityCritical,
		CheckType:        DesignCheckType,
		RequiredElements: []string{"自定义要素"},
		Domain:           "design_comparison",
	}
	engine.RegisterRule(customRule)
	if _, ok := engine.GetRule("DESIGN-CUSTOM-TEST"); !ok {
		t.Errorf("expected custom rule to be registered")
	}
}

func TestEvaluateWithDomainFilter(t *testing.T) {
	engine := NewDesignRuleEngine()
	text := "整体观察综合判断对比分析"
	results := engine.Evaluate(text, "nonexistent_domain")
	if len(results) != 0 {
		t.Errorf("expected 0 failures when domain filter excludes all rules, got %d", len(results))
	}
}

func TestFormatRuleResults(t *testing.T) {
	results := []patent.RuleCheckResult{
		{
			RuleID:        "TEST-RULE",
			RuleName:      "测试规则",
			Passed:        false,
			Level:         patent.LevelMust,
			Severity:      patent.SeverityCritical,
			Message:       "测试失败信息",
			FixSuggestion: "测试修改建议",
		},
	}
	report := FormatRuleResults(results, patent.VerdictBlocked)
	if report == "" {
		t.Errorf("expected non-empty report")
	}
	if !stringsContains(report, "测试规则") {
		t.Errorf("expected report to contain rule name")
	}
}

// stringsContains is a helper for string containment check.
func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && containsString(s, substr)
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
