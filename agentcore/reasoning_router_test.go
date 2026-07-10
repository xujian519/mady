package agentcore

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultClassifier(t *testing.T) {
	c := NewDefaultClassifier()
	cases := []struct {
		in   string
		want Complexity
	}{
		{"hi", ComplexityLow},
		{"分析这个专利的新颖性", ComplexityHigh},
		{"debug the crash", ComplexityHigh},
		{strings.Repeat("a", 300), ComplexityMedium},
		{strings.Repeat("a", 900), ComplexityHigh},
	}
	for _, tc := range cases {
		got := c.Classify(tc.in, nil)
		if got != tc.want {
			t.Errorf("Classify(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestDefaultClassifier_HistoryBump(t *testing.T) {
	c := NewDefaultClassifier()
	msgs := make([]Message, c.HistoryTurnsForHigh)
	got := c.Classify("short", msgs)
	if got != ComplexityMedium {
		t.Fatalf("long history should bump Low→Medium, got %s", got)
	}
}

func TestReasoningRouter_BeforeModelCall(t *testing.T) {
	router := NewReasoningRouter(NewDefaultClassifier())
	var seen Complexity
	router.Decision = func(_ int64, c Complexity) { seen = c }

	req := &ProviderRequest{Messages: []Message{{Role: RoleUser, Content: "分析法律问题"}}}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{Messages: req.Messages}

	if err := router.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	if seen != ComplexityHigh {
		t.Fatalf("decision = %s, want high", seen)
	}
	if mcc.Request.Thinking == nil || mcc.Request.Thinking.Effort != ThinkingEffortHigh {
		t.Fatalf("thinking effort not set to high: %+v", mcc.Request.Thinking)
	}
}

func TestReasoningRouter_PreservesExistingThinking(t *testing.T) {
	router := NewReasoningRouter(NewDefaultClassifier())
	req := &ProviderRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Thinking: &ThinkingConfig{IncludeThoughts: true, Budget: 5000},
	}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{Messages: req.Messages}

	if err := router.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	if !mcc.Request.Thinking.IncludeThoughts {
		t.Fatal("router cleared an explicitly configured IncludeThoughts")
	}
	if mcc.Request.Thinking.Effort != ThinkingEffortLow {
		t.Fatalf("low-complexity effort = %q, want low", mcc.Request.Thinking.Effort)
	}
}
