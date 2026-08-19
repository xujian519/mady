package guardrails

import (
	"regexp"
	"testing"
)

// TestRuleNames verifies the Name() method of every rule type.
func TestRuleNames(t *testing.T) {
	rules := []Rule{
		&keywordBlocklistRule{name: "blocklist"},
		&patternAnalysisRule{name: "pattern"},
		&categoryDetectionRule{name: "category"},
		&sectionStructureRule{name: "structure"},
		&specificationRule{name: "spec"},
		&structuralAnalysisRule{name: "structural"},
		&scopeComparisonRule{name: "scope"},
		&claimClarityRule{name: "clarity"},
	}
	want := []string{"blocklist", "pattern", "category", "structure", "spec", "structural", "scope", "clarity"}
	for i, r := range rules {
		if got := r.Name(); got != want[i] {
			t.Errorf("rule %d Name() = %q, want %q", i, got, want[i])
		}
	}
}

// TestKeywordBlocklistRule verifies keyword blocklist matching and pass-through.
func TestKeywordBlocklistRule(t *testing.T) {
	r := &keywordBlocklistRule{
		name:     "blocklist",
		severity: SeverityError,
		action:   ActionBlock,
		keywords: []string{"恶意代码"},
	}

	if res := r.Check("这是恶意代码示例", nil); res.Passed || res.Action != ActionBlock {
		t.Errorf("blocked case: %+v", res)
	}
	if res := r.Check("正常内容", nil); !res.Passed {
		t.Errorf("pass case: %+v", res)
	}
	if r.Name() != "blocklist" {
		t.Errorf("Name = %q", r.Name())
	}
}

// TestPatternAnalysisRule verifies regex pattern matching.
func TestPatternAnalysisRule(t *testing.T) {
	r := &patternAnalysisRule{
		name:     "pattern",
		severity: SeverityWarning,
		action:   ActionInject,
		compiled: []*regexp.Regexp{regexp.MustCompile(`\d{6,}`)},
	}
	if res := r.Check("身份证号 123456789", nil); res.Passed {
		t.Errorf("pattern match should fail: %+v", res)
	}
	if res := r.Check("无敏感信息", nil); !res.Passed {
		t.Errorf("no match should pass: %+v", res)
	}
}

// TestCategoryDetectionRule verifies case-insensitive category detection.
func TestCategoryDetectionRule(t *testing.T) {
	r := &categoryDetectionRule{
		name:       "category",
		severity:   SeverityInfo,
		action:     ActionAlert,
		categories: []string{"PII", "机密"},
	}
	if res := r.Check("包含 pii 数据", nil); res.Passed {
		t.Errorf("category should match case-insensitively: %+v", res)
	}
	if res := r.Check("包含机密内容", nil); res.Passed {
		t.Errorf("category should match: %+v", res)
	}
	if res := r.Check("普通文本", nil); !res.Passed {
		t.Errorf("no category should pass: %+v", res)
	}
}

// TestSectionStructureRule verifies required-section completeness.
func TestSectionStructureRule(t *testing.T) {
	r := &sectionStructureRule{
		name:     "structure",
		severity: SeverityError,
		action:   ActionBlock,
		titles:   []string{"背景技术", "发明内容"},
		minCount: 2,
	}
	if res := r.Check("背景技术\n发明内容", nil); !res.Passed {
		t.Errorf("complete sections should pass: %+v", res)
	}
	if res := r.Check("只有背景技术", nil); res.Passed {
		t.Errorf("missing section should fail: %+v", res)
	}
}

// TestSpecificationRule verifies required-field completeness.
func TestSpecificationRule(t *testing.T) {
	r := &specificationRule{
		name:     "spec",
		severity: SeverityError,
		action:   ActionInject,
		required: []string{"技术领域", "有益效果"},
	}
	if res := r.Check("技术领域\n有益效果", nil); !res.Passed {
		t.Errorf("complete fields should pass: %+v", res)
	}
	if res := r.Check("只有技术领域", nil); res.Passed {
		t.Errorf("missing field should fail: %+v", res)
	}
}

// TestStructuralAnalysisRule verifies paragraph count bounds.
func TestStructuralAnalysisRule(t *testing.T) {
	r := &structuralAnalysisRule{
		name:     "structural",
		severity: SeverityWarning,
		action:   ActionAlert,
		minCount: 2,
		maxCount: 3,
	}
	if res := r.Check("a\n\nb", nil); !res.Passed {
		t.Errorf("within bounds should pass: %+v", res)
	}
	if res := r.Check("a", nil); res.Passed {
		t.Errorf("below min should fail: %+v", res)
	}
	if res := r.Check("a\n\nb\n\nc\n\nd", nil); res.Passed {
		t.Errorf("above max should fail: %+v", res)
	}
}

// TestScopeComparisonRule verifies scope-comparison detection.
func TestScopeComparisonRule(t *testing.T) {
	r := &scopeComparisonRule{
		name:      "scope",
		severity:  SeverityWarning,
		action:    ActionInject,
		stopWords: []string{"权利要求1"},
	}
	// Comparison words without a concrete scope description → fail.
	if res := r.Check("本方案范围涵盖所有实施方式", nil); res.Passed {
		t.Errorf("comparison without scope should fail: %+v", res)
	}
	// Comparison with a stop word (concrete scope) → pass.
	if res := r.Check("本方案范围涵盖权利要求1所述方案", nil); !res.Passed {
		t.Errorf("comparison with concrete scope should pass: %+v", res)
	}
	// No comparison words → pass.
	if res := r.Check("普通描述", nil); !res.Passed {
		t.Errorf("no comparison should pass: %+v", res)
	}
}

// TestClaimClarityRule verifies field-rule pairing.
func TestClaimClarityRule(t *testing.T) {
	r := &claimClarityRule{
		name:     "clarity",
		severity: SeverityError,
		action:   ActionBlock,
		fieldRules: map[string]string{
			"其特征在于": "包括",
		},
	}
	// Field present but expected term missing → fail.
	if res := r.Check("其特征在于：本装置", nil); res.Passed {
		t.Errorf("field without expected term should fail: %+v", res)
	}
	// Field + expected term → pass.
	if res := r.Check("其特征在于：包括本体", nil); !res.Passed {
		t.Errorf("field with expected term should pass: %+v", res)
	}
	// Field absent → pass.
	if res := r.Check("普通权利要求", nil); !res.Passed {
		t.Errorf("no field should pass: %+v", res)
	}
}
