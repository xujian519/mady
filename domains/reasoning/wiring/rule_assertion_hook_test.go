package wiring

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/reasoning"
)

func TestNewRuleAssertionHook_NoMustRules(t *testing.T) {
	// Only Should/Note rules → hook is nil (would be a no-op).
	rules := []reasoning.RuleConstraint{
		{ArticleID: "S1", Requirement: reasoning.ReqShould},
		{ArticleID: "N1", Requirement: reasoning.ReqNote},
	}
	if got := NewRuleAssertionHook(rules); got != nil {
		t.Fatalf("NewRuleAssertionHook with no Must rules = %v, want nil", got)
	}
}

func TestNewRuleAssertionHook_OnlyMustExtracted(t *testing.T) {
	rules := []reasoning.RuleConstraint{
		{ArticleID: "M1", Requirement: reasoning.ReqMust, ArticleName: "必须规则"},
		{ArticleID: "S1", Requirement: reasoning.ReqShould},
	}
	h := NewRuleAssertionHook(rules)
	if h == nil {
		t.Fatal("expected non-nil hook with Must rules")
	}
	if len(h.mustRules) != 1 || h.mustRules[0] != "M1" {
		t.Errorf("mustRules = %v, want [M1]", h.mustRules)
	}
}

func TestRuleAssertionHook_RecordsViolation(t *testing.T) {
	h := NewRuleAssertionHook([]reasoning.RuleConstraint{
		{ArticleID: "NOV-001", Requirement: reasoning.ReqMust},
		{ArticleID: "NOV-002", Requirement: reasoning.ReqMust},
	})

	// Output references NOV-001 but not NOV-002 → 1 violation.
	h.AfterModelCall(context.Background(),
		&agentcore.AgentRunContext{Turn: 1},
		&agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{Content: "根据 NOV-001 分析..."},
		})

	v := h.Violations()
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1", len(v))
	}
	if v[0].RuleID != "NOV-002" {
		t.Errorf("violation RuleID = %s, want NOV-002", v[0].RuleID)
	}
	if v[0].Turn != 1 {
		t.Errorf("violation Turn = %d, want 1", v[0].Turn)
	}
}

func TestRuleAssertionHook_NoViolationWhenAllReferenced(t *testing.T) {
	h := NewRuleAssertionHook([]reasoning.RuleConstraint{
		{ArticleID: "NOV-001", Requirement: reasoning.ReqMust},
	})
	h.AfterModelCall(context.Background(),
		&agentcore.AgentRunContext{Turn: 1},
		&agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{Content: "适用 NOV-001 判定"},
		})
	if h.HasViolations() {
		t.Error("expected no violations when rule is referenced")
	}
	if score := h.ComplianceScore(); score != 1.0 {
		t.Errorf("ComplianceScore = %v, want 1.0", score)
	}
}

func TestRuleAssertionHook_CaseInsensitive(t *testing.T) {
	h := NewRuleAssertionHook([]reasoning.RuleConstraint{
		{ArticleID: "NOV-001", Requirement: reasoning.ReqMust},
	})
	// Lowercase output should match uppercase rule ID.
	h.AfterModelCall(context.Background(), nil,
		&agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{Content: "checked nov-001"},
		})
	if h.HasViolations() {
		t.Error("case-insensitive match should not violate")
	}
}

func TestRuleAssertionHook_AccumulatesAcrossCalls(t *testing.T) {
	h := NewRuleAssertionHook([]reasoning.RuleConstraint{
		{ArticleID: "M1", Requirement: reasoning.ReqMust},
	})
	// Two calls, neither references M1.
	h.AfterModelCall(context.Background(), &agentcore.AgentRunContext{Turn: 1},
		&agentcore.ModelCallContext{Response: &agentcore.ProviderResponse{Content: "output1"}})
	h.AfterModelCall(context.Background(), &agentcore.AgentRunContext{Turn: 2},
		&agentcore.ModelCallContext{Response: &agentcore.ProviderResponse{Content: "output2"}})

	v := h.Violations()
	if len(v) != 2 {
		t.Fatalf("expected 2 violations across 2 calls, got %d", len(v))
	}
}

func TestRuleAssertionHook_ComplianceScore(t *testing.T) {
	h := NewRuleAssertionHook([]reasoning.RuleConstraint{
		{ArticleID: "A", Requirement: reasoning.ReqMust},
		{ArticleID: "B", Requirement: reasoning.ReqMust},
		{ArticleID: "C", Requirement: reasoning.ReqMust},
	})
	// Output references A and B but not C → 2/3 compliant.
	h.AfterModelCall(context.Background(), nil,
		&agentcore.ModelCallContext{Response: &agentcore.ProviderResponse{Content: "A and B"}})
	if score := h.ComplianceScore(); score < 0.66 || score > 0.67 {
		t.Errorf("ComplianceScore = %v, want ~0.667", score)
	}
}

func TestRuleAssertionHook_NilSafety(t *testing.T) {
	var h *RuleAssertionHook
	h.AfterModelCall(context.Background(), nil, nil) // must not panic
	if v := h.Violations(); v != nil {
		t.Errorf("nil Violations = %v, want nil", v)
	}
	if h.HasViolations() {
		t.Error("nil HasViolations should be false")
	}
	if score := h.ComplianceScore(); score != 1 {
		t.Errorf("nil ComplianceScore = %v, want 1", score)
	}
}

func TestRuleAssertionHook_NilResponse(t *testing.T) {
	h := NewRuleAssertionHook([]reasoning.RuleConstraint{
		{ArticleID: "X", Requirement: reasoning.ReqMust},
	})
	// nil Response must not panic.
	h.AfterModelCall(context.Background(), nil, &agentcore.ModelCallContext{})
	if h.HasViolations() {
		t.Error("nil Response should produce no violations")
	}
}
