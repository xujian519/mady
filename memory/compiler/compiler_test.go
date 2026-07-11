package compiler

import (
	"math/rand"
	"testing"
	"time"
)

func TestStrategy_SuccessRate(t *testing.T) {
	s := Strategy{Successes: 7, Failures: 3}
	if rate := s.SuccessRate(); rate != 0.7 {
		t.Errorf("SuccessRate()=%.2f want 0.7", rate)
	}

	s2 := Strategy{} // untested
	if rate := s2.SuccessRate(); rate != 0.5 {
		t.Errorf("untested SuccessRate()=%.2f want 0.5", rate)
	}
}

func TestStrategy_MatchesGoal(t *testing.T) {
	s := Strategy{Preconditions: []string{"审查意见", "OA"}}
	if !s.MatchesGoal("请帮我答复审查意见") {
		t.Error("should match 审查意见")
	}
	if !s.MatchesGoal("handle this OA response") {
		t.Error("should match OA (case insensitive)")
	}
	if s.MatchesGoal("write a patent application") {
		t.Error("should not match unrelated goal")
	}
}

func TestSelectStrategy_Exploitation(t *testing.T) {
	strategies := []Strategy{
		{ID: "a", Preconditions: []string{"test"}, Successes: 3, Failures: 7},
		{ID: "b", Preconditions: []string{"test"}, Successes: 8, Failures: 2},
		{ID: "c", Preconditions: []string{"test"}, Successes: 5, Failures: 5},
	}

	rng := rand.New(rand.NewSource(42)) // deterministic
	// explorationRate=0 → always exploitation
	pick := SelectStrategy("test this", strategies, 0, rng)
	if pick.Strategy == nil || pick.Strategy.ID != "b" {
		t.Errorf("expected strategy b (highest success rate), got %+v", pick.Strategy)
	}
	if pick.Explored {
		t.Error("should not be exploration with rate=0")
	}
}

func TestSelectStrategy_NoMatch(t *testing.T) {
	strategies := []Strategy{
		{ID: "a", Preconditions: []string{"patent"}},
	}
	pick := SelectStrategy("cook dinner", strategies, 10, nil)
	if pick.Strategy != nil {
		t.Error("should return nil for no matching strategies")
	}
}

func TestSelectStrategy_Exploration(t *testing.T) {
	strategies := []Strategy{
		{ID: "a", Preconditions: []string{"test"}, Successes: 1, Failures: 0},
		{ID: "b", Preconditions: []string{"test"}, Successes: 1, Failures: 0},
	}

	// With 100% exploration, should pick randomly
	rng := rand.New(rand.NewSource(1))
	pick := SelectStrategy("test", strategies, 100, rng)
	if pick.Strategy == nil {
		t.Fatal("expected non-nil strategy")
	}
	if !pick.Explored {
		t.Error("should be exploration with rate=100")
	}
}

func TestDefaultStrategies(t *testing.T) {
	s := DefaultStrategies()
	if len(s) < 5 {
		t.Errorf("expected at least 5 default strategies, got %d", len(s))
	}
	// Verify each has required fields
	for _, strat := range s {
		if strat.ID == "" || strat.Description == "" {
			t.Errorf("strategy missing required fields: %+v", strat)
		}
		if len(strat.Preconditions) == 0 {
			t.Errorf("strategy %s has no preconditions", strat.ID)
		}
	}
}

func TestCompiler_StartTurn(t *testing.T) {
	c := NewCompiler(Config{})

	guidance, strategyID := c.StartTurn("请帮我答复审查意见")
	if guidance == "" {
		t.Error("expected non-empty guidance for matching goal")
	}
	if strategyID == "" {
		t.Error("expected non-empty strategy ID")
	}
}

func TestCompiler_StartTurn_NoMatch(t *testing.T) {
	c := NewCompiler(Config{})

	guidance, strategyID := c.StartTurn("cook dinner")
	if guidance != "" || strategyID != "" {
		t.Error("expected empty guidance for non-matching goal")
	}
}

func TestCompiler_FinishTurn_Success(t *testing.T) {
	c := NewCompiler(Config{ExplorationRate: 0})

	// Select a strategy
	_, sid := c.StartTurn("审查意见答复")
	if sid == "" {
		t.Fatal("expected strategy selection")
	}

	// Get initial success count
	strat, _ := c.StrategyByID(sid)
	initialSuccess := strat.Successes

	// Record success
	trace := NewTrace("t1", "审查意见答复", sid, 1)
	trace.Complete(OutcomeSuccess, 3, 0)
	c.FinishTurn(trace)

	// Verify stats updated
	strat2, _ := c.StrategyByID(sid)
	if strat2.Successes != initialSuccess+1 {
		t.Errorf("successes=%d want %d", strat2.Successes, initialSuccess+1)
	}
}

func TestCompiler_FinishTurn_Failure(t *testing.T) {
	c := NewCompiler(Config{ExplorationRate: 0})

	_, sid := c.StartTurn("审查意见")
	strat, _ := c.StrategyByID(sid)
	initialFailures := strat.Failures

	trace := NewTrace("t1", "审查意见", sid, 1)
	trace.Complete(OutcomeFailure, 5, 3)
	c.FinishTurn(trace)

	strat2, _ := c.StrategyByID(sid)
	if strat2.Failures != initialFailures+1 {
		t.Errorf("failures=%d want %d", strat2.Failures, initialFailures+1)
	}
}

func TestCompiler_TraceBuffer(t *testing.T) {
	c := NewCompiler(Config{MaxTraces: 5})

	for i := 0; i < 10; i++ {
		trace := NewTrace("t", "goal", "", int64(i))
		trace.Complete(OutcomeSuccess, 0, 0)
		c.FinishTurn(trace)
	}

	traces := c.Traces(0)
	if len(traces) != 5 {
		t.Errorf("expected 5 traces (circular buffer), got %d", len(traces))
	}
}

func TestCompiler_Stats(t *testing.T) {
	c := NewCompiler(Config{})

	c.FinishTurn(ExecutionTrace{Outcome: OutcomeSuccess})
	c.FinishTurn(ExecutionTrace{Outcome: OutcomeFailure})
	c.FinishTurn(ExecutionTrace{Outcome: OutcomeSuccess})

	stats := c.Stats()
	if stats.TotalTraces != 3 {
		t.Errorf("TotalTraces=%d want 3", stats.TotalTraces)
	}
	if stats.SuccessTraces != 2 {
		t.Errorf("SuccessTraces=%d want 2", stats.SuccessTraces)
	}
	if stats.FailureTraces != 1 {
		t.Errorf("FailureTraces=%d want 1", stats.FailureTraces)
	}
}

func TestClassifyQuality(t *testing.T) {
	tests := []struct {
		trace ExecutionTrace
		want  Quality
	}{
		{ExecutionTrace{Outcome: OutcomeSuccess, ToolCalls: 3, ToolErrors: 0}, QualityHigh},
		{ExecutionTrace{Outcome: OutcomeSuccess, ToolCalls: 3, ToolErrors: 1}, QualityMedium},
		{ExecutionTrace{Outcome: OutcomeFailure, ToolCalls: 4, ToolErrors: 1}, QualityHigh},
		{ExecutionTrace{Outcome: OutcomeFailure, ToolCalls: 4, ToolErrors: 3}, QualityMedium},
		{ExecutionTrace{Outcome: OutcomeAborted}, QualityNoise},
		{ExecutionTrace{Outcome: OutcomePartial}, QualityMedium},
	}
	for _, tt := range tests {
		got := ClassifyQuality(tt.trace)
		if got != tt.want {
			t.Errorf("ClassifyQuality(outcome=%s, errors=%d/%d)=%s want %s",
				tt.trace.Outcome, tt.trace.ToolErrors, tt.trace.ToolCalls, got, tt.want)
		}
	}
}

func TestDecayedConfidence(t *testing.T) {
	cfg := DefaultDecayConfig()

	// Recent — no decay
	recent := time.Now()
	conf := DecayedConfidence(0.8, recent, cfg)
	if conf < 0.79 || conf > 0.81 {
		t.Errorf("recent confidence=%.3f want ~0.8", conf)
	}

	// Old — significant decay
	old := time.Now().AddDate(0, 0, -28) // 4 weeks ago
	conf = DecayedConfidence(0.8, old, cfg)
	if conf > 0.7 {
		t.Errorf("old confidence=%.3f should be < 0.7", conf)
	}

	// Very old — floor
	veryOld := time.Now().AddDate(-1, 0, 0)
	conf = DecayedConfidence(0.8, veryOld, cfg)
	if conf > 0.1 {
		t.Errorf("very old confidence=%.3f should hit floor", conf)
	}
}

func TestExecutionTrace_Complete(t *testing.T) {
	trace := NewTrace("t1", "goal", "strategy1", 1)
	trace.Complete(OutcomeSuccess, 5, 1)

	if trace.Outcome != OutcomeSuccess {
		t.Error("outcome not set")
	}
	if trace.ToolCalls != 5 {
		t.Error("tool calls not set")
	}
	if trace.DurationMs < 0 {
		t.Error("duration should be non-negative")
	}
	if trace.CompletedAt.IsZero() {
		t.Error("completed time not set")
	}
}
