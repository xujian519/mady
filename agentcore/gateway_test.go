package agentcore

import (
	"context"
	"strings"
	"testing"
)

// countingClassifier records how many times Classify is invoked and returns
// a fixed complexity. It lets tests prove Gateway classifies exactly once
// per turn (the core unification guarantee) regardless of how many downstream
// concerns consume the result.
type countingClassifier struct {
	calls   int
	result  Complexity
	lastIn  string
	lastMsg int
}

func (c *countingClassifier) Classify(input string, messages []Message) Complexity {
	c.calls++
	c.lastIn = input
	c.lastMsg = len(messages)
	return c.result
}

// bigASCIIMessage builds a user message whose estimated tokens push the padded
// ratio past the blocking threshold for the given window.
func bigASCIIMessage(window int64) []Message {
	// ~4 ASCII chars ≈ 1 token. We want padded tokens (ceil(t*4/3)) well past
	// 0.95*window, so generate generous content.
	content := strings.Repeat("a", int(window)*8)
	return []Message{{Role: RoleUser, Content: content}}
}

// --- construction & defaults ---

func TestNewGateway_NilClassifierDefaults(t *testing.T) {
	g := NewGateway(nil)
	// A High-keyword input must classify as High via DefaultClassifier.
	d := g.Decide(&AgentRunContext{Input: "请对比分析这个专利的创造性", Messages: []Message{}})
	if d.Complexity != ComplexityHigh {
		t.Fatalf("expected ComplexityHigh via DefaultClassifier, got %s", d.Complexity)
	}
}

// --- the core guarantee: classify once ---

func TestGateway_Decide_ClassifiesOnce(t *testing.T) {
	cc := &countingClassifier{result: ComplexityMedium}
	g := NewGateway(cc)
	g.Fallback = NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityMedium: {"m-primary", "m-backup"},
		},
	}, nil, nil)
	g.Reasoning = NewReasoningRouter(nil) // also has its own classifier, must NOT run

	d := g.Decide(&AgentRunContext{Input: "hello", Turn: 3})
	if cc.calls != 1 {
		t.Fatalf("Gateway must classify exactly once per Decide, got %d calls", cc.calls)
	}
	if d.Turn != 3 {
		t.Errorf("Turn not propagated: got %d", d.Turn)
	}
	// Model selection and effort must both derive from the single classification.
	if d.Model != "m-primary" {
		t.Errorf("expected model from Medium candidates, got %q", d.Model)
	}
	if d.Effort != ThinkingEffortMedium {
		t.Errorf("expected medium effort for ComplexityMedium, got %s", d.Effort)
	}
	if d.Complexity != ComplexityMedium {
		t.Errorf("complexity mismatch: %s", d.Complexity)
	}
}

func TestGateway_BeforeModelCall_ClassifiesOnceAcrossConcerns(t *testing.T) {
	cc := &countingClassifier{result: ComplexityHigh}
	g := NewGateway(cc)
	g.Fallback = NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityHigh:   {"strong-model", "backup"},
			ComplexityMedium: {"medium-only"},
		},
	}, nil, nil)

	mcc := &ModelCallContext{Request: &ProviderRequest{Model: "default"}}
	if err := g.BeforeModelCall(context.Background(), &AgentRunContext{Input: "x"}, mcc); err != nil {
		t.Fatalf("BeforeModelCall: %v", err)
	}
	if cc.calls != 1 {
		t.Fatalf("expected single classify across model+effort, got %d", cc.calls)
	}
	if mcc.Request.Model != "strong-model" {
		t.Errorf("expected High candidate applied, got %q", mcc.Request.Model)
	}
	if mcc.Request.Thinking == nil || mcc.Request.Thinking.Effort != ThinkingEffortHigh {
		t.Errorf("expected High effort applied, got %+v", mcc.Request.Thinking)
	}
}

// --- budget evaluation ---

func TestGateway_Decide_NoBudgetManager_ZeroSnapshot(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityLow})
	d := g.Decide(&AgentRunContext{Input: "hi"})
	if d.Budget.MaxContextTokens != 0 {
		t.Errorf("expected zero budget snapshot without manager, got %+v", d.Budget)
	}
	if d.BudgetClamp {
		t.Error("should not clamp without budget manager")
	}
}

func TestGateway_Decide_BudgetOK_NoClamp(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	g.BudgetManager = NewTokenBudgetManager(DefaultBudgetConfig())
	g.ContextWindow = 10000
	d := g.Decide(&AgentRunContext{Input: "short", Messages: []Message{{Role: RoleUser, Content: "short"}}})
	if d.Budget.State != BudgetOK {
		t.Fatalf("expected OK, got %s (%s)", d.Budget.State, d.Budget)
	}
	if d.Effort != ThinkingEffortHigh {
		t.Errorf("High effort must survive when budget OK, got %s", d.Effort)
	}
	if d.BudgetClamp {
		t.Error("must not clamp when budget OK")
	}
}

func TestGateway_Decide_BudgetBlocking_ClampsEffort(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	g.BudgetManager = NewTokenBudgetManager(DefaultBudgetConfig())
	g.ContextWindow = 100
	d := g.Decide(&AgentRunContext{Input: "x", Messages: bigASCIIMessage(100)})
	if d.Budget.State != BudgetBlocking {
		t.Fatalf("expected Blocking for overflowing context, got %s (%s)", d.Budget.State, d.Budget)
	}
	if d.Effort != ThinkingEffortLow {
		t.Errorf("blocking must clamp effort to Low, got %s", d.Effort)
	}
	if !d.BudgetClamp {
		t.Error("BudgetClamp must be true when clamped")
	}
	if !strings.Contains(d.Reason, "clamped=low(blocking)") {
		t.Errorf("Reason must record the clamp, got %q", d.Reason)
	}
}

// --- model selection ---

func TestGateway_Decide_SelectsModelViaFallback(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityLow})
	g.Fallback = NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"cheap-fast"},
		},
	}, nil, nil)
	d := g.Decide(&AgentRunContext{Input: "hi"})
	if d.Model != "cheap-fast" {
		t.Fatalf("expected selected model, got %q", d.Model)
	}
}

func TestGateway_Decide_NoFallback_ModelEmpty(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	d := g.Decide(&AgentRunContext{Input: "x"})
	if d.Model != "" {
		t.Errorf("expected empty model without FallbackRouter, got %q", d.Model)
	}
}

func TestGateway_Decide_FallbackNoCandidatesForComplexity_ModelEmpty(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	g.Fallback = NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"only-low"},
		},
	}, nil, nil)
	d := g.Decide(&AgentRunContext{Input: "x"})
	if d.Model != "" {
		t.Errorf("expected empty model when no candidates for the complexity, got %q", d.Model)
	}
}

// --- effort resolution precedence ---

func TestGateway_Decide_EffortFromReasoning(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	rr := NewReasoningRouter(nil)
	rr.Efforts[ComplexityHigh] = ThinkingEffortMax
	g.Reasoning = rr
	d := g.Decide(&AgentRunContext{Input: "x"})
	if d.Effort != ThinkingEffortMax {
		t.Errorf("Reasoning.Efforts must take precedence, got %s", d.Effort)
	}
}

func TestGateway_Decide_EffortFromGatewayMap(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityMedium})
	g.Efforts = map[Complexity]ThinkingEffort{ComplexityMedium: ThinkingEffortMax}
	d := g.Decide(&AgentRunContext{Input: "x"})
	if d.Effort != ThinkingEffortMax {
		t.Errorf("Gateway.Efforts must apply when Reasoning is nil, got %s", d.Effort)
	}
}

func TestGateway_Decide_EffortDefaultByComplexity(t *testing.T) {
	cases := []struct {
		c   Complexity
		exp ThinkingEffort
	}{
		{ComplexityLow, ThinkingEffortLow},
		{ComplexityMedium, ThinkingEffortMedium},
		{ComplexityHigh, ThinkingEffortHigh},
	}
	for _, tc := range cases {
		g := NewGateway(&countingClassifier{result: tc.c})
		d := g.Decide(&AgentRunContext{Input: "x"})
		if d.Effort != tc.exp {
			t.Errorf("complexity %s: expected default effort %s, got %s", tc.c, tc.exp, d.Effort)
		}
	}
}

// --- BeforeModelCall application ---

func TestGateway_BeforeModelCall_AppliesModelAndEffort(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityMedium})
	g.Fallback = NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{ComplexityMedium: {"picked"}},
	}, nil, nil)
	mcc := &ModelCallContext{Request: &ProviderRequest{Model: "original"}}
	if err := g.BeforeModelCall(context.Background(), &AgentRunContext{Input: "x"}, mcc); err != nil {
		t.Fatalf("BeforeModelCall: %v", err)
	}
	if mcc.Request.Model != "picked" {
		t.Errorf("model not applied: %q", mcc.Request.Model)
	}
	if mcc.Request.Thinking == nil || mcc.Request.Thinking.Effort != ThinkingEffortMedium {
		t.Errorf("effort not applied: %+v", mcc.Request.Thinking)
	}
}

func TestGateway_BeforeModelCall_AppliesReasoningBudget(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	rr := NewReasoningRouter(nil)
	rr.Budgets[ComplexityHigh] = 4096
	g.Reasoning = rr
	mcc := &ModelCallContext{Request: &ProviderRequest{}}
	if err := g.BeforeModelCall(context.Background(), &AgentRunContext{Input: "x"}, mcc); err != nil {
		t.Fatalf("BeforeModelCall: %v", err)
	}
	if mcc.Request.Thinking == nil || mcc.Request.Thinking.Budget != 4096 {
		t.Errorf("reasoning budget not applied: %+v", mcc.Request.Thinking)
	}
}

func TestGateway_BeforeModelCall_ClampSkipsReasoningBudget(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	g.BudgetManager = NewTokenBudgetManager(DefaultBudgetConfig())
	g.ContextWindow = 100
	rr := NewReasoningRouter(nil)
	rr.Budgets[ComplexityHigh] = 4096
	g.Reasoning = rr
	mcc := &ModelCallContext{Request: &ProviderRequest{}}
	if err := g.BeforeModelCall(context.Background(),
		&AgentRunContext{Input: "x", Messages: bigASCIIMessage(100)}, mcc); err != nil {
		t.Fatalf("BeforeModelCall: %v", err)
	}
	if mcc.Request.Thinking == nil || mcc.Request.Thinking.Effort != ThinkingEffortLow {
		t.Errorf("expected clamped Low effort: %+v", mcc.Request.Thinking)
	}
	if mcc.Request.Thinking.Budget != 0 {
		t.Errorf("reasoning budget must NOT apply under clamp, got %d", mcc.Request.Thinking.Budget)
	}
}

func TestGateway_BeforeModelCall_NilRequest_NoPanic(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	g.Fallback = NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{ComplexityHigh: {"m"}},
	}, nil, nil)
	mcc := &ModelCallContext{Request: nil}
	if err := g.BeforeModelCall(context.Background(), &AgentRunContext{Input: "x"}, mcc); err != nil {
		t.Fatalf("BeforeModelCall with nil request: %v", err)
	}
}

func TestGateway_BeforeModelCall_NilMCC_NoPanic(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	if err := g.BeforeModelCall(context.Background(), &AgentRunContext{Input: "x"}, nil); err != nil {
		t.Fatalf("BeforeModelCall with nil mcc: %v", err)
	}
}

// --- AfterModelCall delegation ---

func TestGateway_AfterModelCall_DelegatesHealthToFallback(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityMedium})
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{ComplexityMedium: {"the-model"}},
	}, nil, nil)
	g.Fallback = fr

	mcc := &ModelCallContext{Request: &ProviderRequest{}}
	// BeforeModelCall selects the model and caches fr.lastModel.
	_ = g.BeforeModelCall(context.Background(), &AgentRunContext{Input: "x"}, mcc)

	// Success path: Request is nil (as runAfterModelCall does), Response set.
	g.AfterModelCall(context.Background(), &AgentRunContext{}, &ModelCallContext{
		Request: nil, Response: &ProviderResponse{Content: "ok"},
	})
	d := fr.HealthTracker.DetailOf("the-model")
	if d == nil {
		t.Fatal("expected health record after successful AfterModelCall")
	}
	if d.ConsecutiveSuccesses != 1 {
		t.Errorf("expected one success recorded via delegation, got %d", d.ConsecutiveSuccesses)
	}
}

func TestGateway_AfterModelCall_NoFallback_NoPanic(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityLow})
	g.AfterModelCall(context.Background(), &AgentRunContext{}, &ModelCallContext{Response: &ProviderResponse{}})
}

// --- LastDecision & Decision callback ---

func TestGateway_LastDecision_AfterBeforeModelCall(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	if (g.LastDecision() != GatewayDecision{}) {
		t.Error("zero-value expected before first Apply")
	}
	_ = g.BeforeModelCall(context.Background(), &AgentRunContext{Input: "x", Turn: 7},
		&ModelCallContext{Request: &ProviderRequest{}})
	ld := g.LastDecision()
	if ld.Turn != 7 || ld.Complexity != ComplexityHigh {
		t.Errorf("LastDecision not cached correctly: %+v", ld)
	}
}

func TestGateway_DecisionCallback_Fired(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	var seen *GatewayDecision
	g.Decision = func(d GatewayDecision) { cp := d; seen = &cp }
	_ = g.BeforeModelCall(context.Background(), &AgentRunContext{Input: "x"},
		&ModelCallContext{Request: &ProviderRequest{}})
	if seen == nil {
		t.Fatal("Decision callback not fired")
	}
	if seen.Complexity != ComplexityHigh {
		t.Errorf("callback got wrong complexity: %s", seen.Complexity)
	}
}

// --- purity ---

func TestGateway_Decide_DoesNotMutateArcMessages(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityLow})
	g.BudgetManager = NewTokenBudgetManager(DefaultBudgetConfig())
	g.ContextWindow = 1000
	msgs := []Message{{Role: RoleUser, Content: "frozen"}}
	arc := &AgentRunContext{Input: "x", Messages: msgs}
	_ = g.Decide(arc)
	if len(arc.Messages) != 1 || arc.Messages[0].Content != "frozen" {
		t.Errorf("Decide mutated arc messages: %+v", arc.Messages)
	}
}

// --- OnHighComplexity ---

// TestGateway_OnHighComplexity_HighOnly 验证 High 分类触发回调、非 High 不触发。
func TestGateway_OnHighComplexity_HighOnly(t *testing.T) {
	cc := &countingClassifier{result: ComplexityHigh}
	g := NewGateway(cc)
	var calls int
	var gotTurn int64
	g.OnHighComplexity = func(arc *AgentRunContext, d GatewayDecision) {
		calls++
		gotTurn = d.Turn
	}

	mcc := &ModelCallContext{Request: &ProviderRequest{}}
	arc := &AgentRunContext{Input: "分析权利要求", Turn: 7}
	if err := g.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || gotTurn != 7 {
		t.Errorf("expected 1 callback with turn 7, got calls=%d turn=%d", calls, gotTurn)
	}

	// 非 High → 不触发。
	cc.result = ComplexityLow
	if err := g.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("callback must not fire on Low complexity, got %d calls", calls)
	}
}

// TestGateway_OnHighComplexity_NilSafe 未设置回调时 High 分类不 panic。
func TestGateway_OnHighComplexity_NilSafe(t *testing.T) {
	g := NewGateway(&countingClassifier{result: ComplexityHigh})
	mcc := &ModelCallContext{Request: &ProviderRequest{}}
	if err := g.BeforeModelCall(context.Background(), &AgentRunContext{Input: "x"}, mcc); err != nil {
		t.Fatal(err)
	}
}
