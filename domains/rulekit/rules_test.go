package rulekit

import (
	"strings"
	"sync"
	"testing"
)

// testRule 是 Rule 接口的测试实现。
type testRule struct {
	BaseRule
	severity Severity
	hit      bool // Check 命中时返回一条该严重度的违规
}

func newTestRule(name string, sev Severity, hit bool) *testRule {
	return &testRule{BaseRule: NewBaseRule(name, "测试规则", "专利法"), severity: sev, hit: hit}
}

func (r *testRule) Check(item, ctx any) []Violation {
	if !r.hit {
		return nil
	}
	return []Violation{{
		RuleName:   r.Name(),
		RuleBasis:  r.LegalBasis(),
		Severity:   r.severity,
		Message:    "命中",
		Suggestion: "修改",
	}}
}

func TestNewEngineAndRules(t *testing.T) {
	e := NewEngine[any, any]()
	if e == nil {
		t.Fatal("NewEngine 返回 nil")
	}
	if got := e.Rules(); len(got) != 0 {
		t.Fatalf("新引擎应无规则，got %d", len(got))
	}
}

func TestRegisterAndRegisterAll(t *testing.T) {
	e := NewEngine[any, any]()
	e.Register(newTestRule("r1", SeverityError, false))
	e.RegisterAll(
		newTestRule("r2", SeverityWarning, false),
		newTestRule("r3", SeverityInfo, false),
	)

	names := make([]string, 0)
	for _, r := range e.Rules() {
		names = append(names, r.Name())
	}
	want := "r1,r2,r3"
	if got := strings.Join(names, ","); got != want {
		t.Errorf("注册顺序应为 %s，got %s", want, got)
	}
}

func TestRulesReturnsCopy(t *testing.T) {
	e := NewEngine[any, any]()
	e.Register(newTestRule("r1", SeverityError, false))

	got := e.Rules()
	got[0] = newTestRule("hacked", SeverityInfo, false)

	names := make([]string, 0)
	for _, r := range e.Rules() {
		names = append(names, r.Name())
	}
	if got := strings.Join(names, ","); got != "r1" {
		t.Errorf("Rules 应返回副本，内部规则被外部篡改，got %s", got)
	}
}

func TestValidate(t *testing.T) {
	e := NewEngine[any, any]()
	e.RegisterAll(
		newTestRule("r1", SeverityError, true),
		newTestRule("r2", SeverityWarning, false), // 未命中，不产生违规
		newTestRule("r3", SeverityInfo, true),
	)

	got := e.Validate(nil, nil)
	if len(got) != 2 {
		t.Fatalf("应命中 2 条规则，got %d", len(got))
	}
	if got[0].RuleName != "r1" || got[1].RuleName != "r3" {
		t.Errorf("违规应按注册顺序返回，got %v %v", got[0].RuleName, got[1].RuleName)
	}
	if got[0].RuleBasis != "专利法" {
		t.Errorf("RuleBasis 应来自 LegalBasis，got %q", got[0].RuleBasis)
	}
}

func TestValidateAndGroup(t *testing.T) {
	e := NewEngine[any, any]()
	e.RegisterAll(
		newTestRule("e1", SeverityError, true),
		newTestRule("w1", SeverityWarning, true),
		newTestRule("i1", SeverityInfo, true),
		newTestRule("skip", SeverityError, false),
	)

	errs, warns, infos := e.ValidateAndGroup(nil, nil)
	if len(errs) != 1 || errs[0].RuleName != "e1" {
		t.Errorf("errors 分组错误: %v", errs)
	}
	if len(warns) != 1 || warns[0].RuleName != "w1" {
		t.Errorf("warnings 分组错误: %v", warns)
	}
	if len(infos) != 1 || infos[0].RuleName != "i1" {
		t.Errorf("infos 分组错误: %v", infos)
	}
}

func TestBaseRule(t *testing.T) {
	r := NewBaseRule("clarity-wording", "用语清晰性", "专利法第26条第4款")
	if r.Name() != "clarity-wording" {
		t.Errorf("Name() 错误: %q", r.Name())
	}
	if r.Description() != "用语清晰性" {
		t.Errorf("Description() 错误: %q", r.Description())
	}
	if r.LegalBasis() != "专利法第26条第4款" {
		t.Errorf("LegalBasis() 错误: %q", r.LegalBasis())
	}
}

func TestContainsAny(t *testing.T) {
	cases := []struct {
		name  string
		s     string
		words []string
		wantW string
		wantB bool
	}{
		{"命中", "本发明涉及一种装置", []string{"装置", "方法"}, "装置", true},
		{"未命中", "本发明涉及一种装置", []string{"系统", "流程"}, "", false},
		{"空词表", "任意文本", nil, "", false},
		{"空串", "", []string{"x"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, ok := ContainsAny(tc.s, tc.words)
			if w != tc.wantW || ok != tc.wantB {
				t.Errorf("ContainsAny(%q, %v) = (%q, %v)，want (%q, %v)", tc.s, tc.words, w, ok, tc.wantW, tc.wantB)
			}
		})
	}
}

// TestEngineConcurrent 验证并发 Register/RegisterAll/Rules/Validate 无竞态。
func TestEngineConcurrent(t *testing.T) {
	e := NewEngine[any, any]()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r := newTestRule("r", SeverityError, j%2 == 0)
				r.BaseRule.name = string(rune('a'+n)) + "r"
				e.Register(r)
			}
		}(i)

		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = e.Validate(nil, nil)
				_ = e.Rules()
			}
		}()
	}
	wg.Wait()
}

// =============================================================================
// 判级聚合（verdict）
// =============================================================================

func TestAggregate_Empty(t *testing.T) {
	if got := AggregateWithDefault(nil); got != VerdictPass {
		t.Errorf("empty violations should pass, got %s", got)
	}
}

func TestAggregate_AnyErrorBlocks(t *testing.T) {
	vs := []Violation{
		{Severity: SeverityInfo, Message: "a"},
		{Severity: SeverityError, Message: "b"},
	}
	if got := AggregateWithDefault(vs); got != VerdictBlocked {
		t.Errorf("any error must block, got %s", got)
	}
}

func TestAggregate_WarningThresholds(t *testing.T) {
	one := []Violation{{Severity: SeverityWarning}, {Severity: SeverityInfo}, {Severity: SeverityInfo}}
	// DSH 默认：任一 Should 失败 → blocked。
	if got := AggregateWithDefault(one); got != VerdictBlocked {
		t.Errorf("single warning must block under default policy, got %s", got)
	}
	// 放宽 Should 阈值到 2：单 warning 不再阻断，info 不足 3 → pass。
	two := []Violation{{Severity: SeverityWarning}}
	if got := Aggregate(two, AggregatePolicy{ShouldBlockedAt: 2, InfoRevisionAt: 3}); got != VerdictPass {
		t.Errorf("relaxed policy: single warning should pass, got %s", got)
	}
	// 放宽 Should 阈值到 2 且两条 warning → blocked。
	tw := []Violation{{Severity: SeverityWarning}, {Severity: SeverityWarning}}
	if got := Aggregate(tw, AggregatePolicy{ShouldBlockedAt: 2, InfoRevisionAt: 3}); got != VerdictBlocked {
		t.Errorf("two warnings at threshold 2 must block, got %s", got)
	}
}

func TestAggregate_InfoRevisionThreshold(t *testing.T) {
	three := []Violation{{Severity: SeverityInfo}, {Severity: SeverityInfo}, {Severity: SeverityInfo}}
	if got := AggregateWithDefault(three); got != VerdictNeedsRevision {
		t.Errorf("3 infos must need revision, got %s", got)
	}
	two := three[:2]
	if got := AggregateWithDefault(two); got != VerdictPass {
		t.Errorf("2 infos should pass, got %s", got)
	}
}

func TestAggregate_ZeroPolicyFallsBackToDefault(t *testing.T) {
	// 阈值 ≤0 视为配置错误，回退默认，不得全放行。
	vs := []Violation{{Severity: SeverityWarning}}
	if got := Aggregate(vs, AggregatePolicy{}); got != VerdictBlocked {
		t.Errorf("zero policy must fall back to default (block on warning), got %s", got)
	}
}
