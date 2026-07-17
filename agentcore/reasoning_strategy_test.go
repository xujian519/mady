package agentcore

import (
	"context"
	"testing"
)

func TestReasoningStrategy_String(t *testing.T) {
	tests := []struct {
		s    ReasoningStrategy
		want string
	}{
		{StrategyDefault, "default"},
		{StrategyStepByStep, "step_by_step"},
		{StrategyStructuredAnalysis, "structured_analysis"},
		{StrategyDebate, "debate"},
		{StrategyTreeOfThoughts, "tree_of_thoughts"},
		{StrategyVerifiedThinking, "verified_thinking"},
		{StrategyFirstPrinciples, "first_principles"},
		{ReasoningStrategy(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("ReasoningStrategy(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestStrategyHint_ContainsContent(t *testing.T) {
	strategies := []ReasoningStrategy{
		StrategyStepByStep,
		StrategyStructuredAnalysis,
		StrategyDebate,
		StrategyTreeOfThoughts,
		StrategyVerifiedThinking,
		StrategyFirstPrinciples,
	}
	for _, s := range strategies {
		hint := s.StrategyHint()
		if hint == "" {
			t.Errorf("StrategyHint() for %s returned empty string", s)
		}
	}
}

func TestStrategyHint_Default(t *testing.T) {
	if hint := StrategyDefault.StrategyHint(); hint != "" {
		t.Errorf("default strategy should have empty hint, got %q", hint)
	}
}

func TestDefaultFrameworks(t *testing.T) {
	fws := DefaultFrameworks()
	if len(fws) != 3 {
		t.Errorf("expected 3 frameworks, got %d", len(fws))
	}

	// Check each complexity level has a framework.
	for _, c := range []Complexity{ComplexityLow, ComplexityMedium, ComplexityHigh} {
		fw, ok := fws[c]
		if !ok {
			t.Errorf("missing framework for complexity %s", c)
			continue
		}
		if len(fw.Steps) == 0 {
			t.Errorf("framework %q has no steps", fw.Name)
		}
	}
}

func TestNewDefaultStrategySelector(t *testing.T) {
	s := NewDefaultStrategySelector()
	if s == nil {
		t.Fatal("expected non-nil selector")
	}
	if !s.StrategyHintInjection {
		t.Error("expected StrategyHintInjection to be true by default")
	}
	// Check each complexity maps to a strategy.
	for _, c := range []Complexity{ComplexityLow, ComplexityMedium, ComplexityHigh} {
		strategy := s.SelectStrategy(c)
		if strategy == StrategyDefault {
			t.Errorf("SelectStrategy(%s) returned default", c)
		}
		fw := s.GetFramework(c)
		if fw.Name == "" {
			t.Errorf("GetFramework(%s) returned empty name", c)
		}
	}
}

func TestStrategySelector_SelectStrategy_Custom(t *testing.T) {
	s := &StrategySelector{
		StrategyMap: map[Complexity]ReasoningStrategy{
			ComplexityLow: StrategyFirstPrinciples,
		},
	}
	if got := s.SelectStrategy(ComplexityLow); got != StrategyFirstPrinciples {
		t.Errorf("got %v, want %v", got, StrategyFirstPrinciples)
	}
	// Unmapped complexity returns default.
	if got := s.SelectStrategy(ComplexityHigh); got != StrategyDefault {
		t.Errorf("expected default for unmapped, got %v", got)
	}
}

func TestStrategySelector_NilMap(t *testing.T) {
	s := &StrategySelector{}
	if got := s.SelectStrategy(ComplexityHigh); got != StrategyDefault {
		t.Errorf("expected default for nil map, got %v", got)
	}
}

func TestNewReasoningStrategyRouter(t *testing.T) {
	classifier := NewDefaultClassifier()
	router := NewReasoningStrategyRouter(classifier, nil)
	if router == nil {
		t.Fatal("expected non-nil router")
	}
	if router.Selector == nil {
		t.Error("expected default selector when nil is passed")
	}
	if router.ReasoningRouter == nil {
		t.Error("expected reasoning router")
	}
}

func TestReasoningStrategyRouter_BeforeModelCall(t *testing.T) {
	classifier := NewDefaultClassifier()
	selector := NewDefaultStrategySelector()
	router := NewReasoningStrategyRouter(classifier, selector)

	// Create a model call context with a request containing a system message.
	mcc := &ModelCallContext{
		Request: &ProviderRequest{
			Messages: []Message{
				{Role: RoleSystem, Content: "你是一个专利助手。"},
				{Role: RoleUser, Content: "分析专利CN12345的新颖性"},
			},
		},
	}
	arc := &AgentRunContext{
		Input: "分析专利CN12345的新颖性",
		Messages: []Message{
			{Role: RoleUser, Content: "分析专利CN12345的新颖性"},
		},
	}

	if err := router.BeforeModelCall(context.TODO(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall failed: %v", err)
	}

	// The system message should now have a strategy hint appended.
	sysMsg := mcc.Request.Messages[0]
	if len(sysMsg.Content) <= len("你是一个专利助手。") {
		t.Error("expected strategy hint to be appended to system message")
	}
}

func TestReasoningStrategyRouter_NilMCC(t *testing.T) {
	classifier := NewDefaultClassifier()
	router := NewReasoningStrategyRouter(classifier, nil)
	if err := router.BeforeModelCall(context.TODO(), nil, nil); err != nil {
		t.Errorf("expected no error for nil mcc, got %v", err)
	}
}

func TestFrameworkStep_Output(t *testing.T) {
	fws := DefaultFrameworks()
	for c, fw := range fws {
		for i, step := range fw.Steps {
			if step.ID == "" {
				t.Errorf("framework %q step %d has empty ID", fw.Name, i)
			}
			if step.Instruction == "" {
				t.Errorf("framework %q step %d has empty Instruction", fw.Name, i)
			}
		}
		_ = c
	}
}
