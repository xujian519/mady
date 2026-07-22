package compiler

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
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
	for _, st := range s {
		if st.ID == "" || st.Description == "" {
			t.Errorf("strategy missing required fields: %+v", st)
		}
		if len(st.Preconditions) == 0 {
			t.Errorf("strategy %s has no preconditions", st.ID)
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
	st, _ := c.StrategyByID(sid)
	initialSuccess := st.Successes

	// Record success
	trace := NewTrace("t1", "审查意见答复", sid, 1)
	trace.Complete(OutcomeSuccess, 3, 0)
	c.FinishTurn(trace)

	// Verify stats updated
	st2, _ := c.StrategyByID(sid)
	if st2.Successes != initialSuccess+1 {
		t.Errorf("successes=%d want %d", st2.Successes, initialSuccess+1)
	}
}

func TestCompiler_FinishTurn_Failure(t *testing.T) {
	c := NewCompiler(Config{ExplorationRate: 0})

	_, sid := c.StartTurn("审查意见")
	st, _ := c.StrategyByID(sid)
	initialFailures := st.Failures

	trace := NewTrace("t1", "审查意见", sid, 1)
	trace.Complete(OutcomeFailure, 5, 3)
	c.FinishTurn(trace)

	st2, _ := c.StrategyByID(sid)
	if st2.Failures != initialFailures+1 {
		t.Errorf("failures=%d want %d", st2.Failures, initialFailures+1)
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

// ---------------------------------------------------------------------------
// S1: Decay-aware strategy selection
// ---------------------------------------------------------------------------

func TestSelectStrategyWithDecay_PrefersRecent(t *testing.T) {
	// Two strategies with same success rate, but one used recently and one long ago.
	now := time.Now()
	strategies := []Strategy{
		{ID: "old", Preconditions: []string{"test"}, Successes: 8, Failures: 2, LastUsedAt: now.AddDate(0, 0, -60)},
		{ID: "new", Preconditions: []string{"test"}, Successes: 8, Failures: 2, LastUsedAt: now},
	}

	cfg := DefaultDecayConfig()
	rng := rand.New(rand.NewSource(42))
	pick := SelectStrategyWithDecay("test", strategies, 0, cfg, rng)
	if pick.Strategy == nil {
		t.Fatal("expected non-nil pick")
	}
	if pick.Strategy.ID != "new" {
		t.Errorf("expected 'new' (recently used) to be preferred, got %q", pick.Strategy.ID)
	}
}

func TestSelectStrategyWithDecay_NoDecayFallback(t *testing.T) {
	// Zero-valued DecayConfig should behave like SelectStrategy (no decay).
	strategies := []Strategy{
		{ID: "a", Preconditions: []string{"x"}, Successes: 3, Failures: 7},
		{ID: "b", Preconditions: []string{"x"}, Successes: 9, Failures: 1},
	}
	rng := rand.New(rand.NewSource(1))
	pick := SelectStrategyWithDecay("x", strategies, 0, DecayConfig{}, rng)
	if pick.Strategy == nil || pick.Strategy.ID != "b" {
		t.Errorf("expected 'b' (highest rate), got %+v", pick.Strategy)
	}
}

// ---------------------------------------------------------------------------
// S2: Quality-weighted learning
// ---------------------------------------------------------------------------

func TestCompiler_FinishTurn_NoiseSkipped(t *testing.T) {
	c := NewCompiler(Config{ExplorationRate: 0})

	_, sid := c.StartTurn("审查意见")
	st, _ := c.StrategyByID(sid)
	initialSuccess := st.Successes
	initialFailures := st.Failures

	// Aborted trace = NOISE → should not affect stats
	trace := NewTrace("t1", "审查意见", sid, 1)
	trace.Complete(OutcomeAborted, 0, 0)
	c.FinishTurn(trace)

	st2, _ := c.StrategyByID(sid)
	if st2.Successes != initialSuccess || st2.Failures != initialFailures {
		t.Errorf("noise trace should not affect stats: got S=%d F=%d, want S=%d F=%d",
			st2.Successes, st2.Failures, initialSuccess, initialFailures)
	}
}

func TestCompiler_FinishTurn_HighSignalCounts(t *testing.T) {
	c := NewCompiler(Config{ExplorationRate: 0})

	_, sid := c.StartTurn("审查意见")
	st, _ := c.StrategyByID(sid)
	initial := st.Successes

	// Clean success (0 tool errors) = HIGH_SIGNAL
	trace := NewTrace("t1", "审查意见", sid, 1)
	trace.Complete(OutcomeSuccess, 3, 0)
	c.FinishTurn(trace)

	st2, _ := c.StrategyByID(sid)
	if st2.Successes != initial+1 {
		t.Errorf("high-signal success should increment: got %d want %d", st2.Successes, initial+1)
	}
}

// ---------------------------------------------------------------------------
// S3: Persistence (Save/Load)
// ---------------------------------------------------------------------------

func TestCompiler_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compiler.json")

	// Create compiler and accumulate some stats
	c1 := NewCompiler(Config{ExplorationRate: 10, MaxTraces: 500})
	trace := NewTrace("t1", "审查意见", "oa_three_step", 1)
	trace.Complete(OutcomeSuccess, 2, 0)
	c1.FinishTurn(trace)

	// Save
	if err := c1.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file not found: %v", err)
	}

	// Load into a fresh compiler
	c2 := NewCompiler(Config{})
	if err := c2.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify strategy stats survived
	st, ok := c2.StrategyByID("oa_three_step")
	if !ok {
		t.Fatal("oa_three_step not found after load")
	}
	if st.Successes != 1 {
		t.Errorf("loaded successes=%d want 1", st.Successes)
	}
}

func TestCompiler_LoadNonExistent(t *testing.T) {
	c := NewCompiler(Config{})
	// Loading a non-existent file should be a no-op (not an error)
	if err := c.Load("/tmp/does_not_exist_compiler_test.json"); err != nil {
		t.Errorf("Load non-existent should return nil, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// S4: Outcome classification helpers
// ---------------------------------------------------------------------------

func TestContainsFailureSignal(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"任务已完成", false},
		{"无法完成该操作", true},
		{"I cannot help with that", true},
		{"操作失败，请重试", true},
		{"正常回复", false},
		{"An error occurred during processing", true},
	}
	for _, tt := range tests {
		got := containsFailureSignal(tt.text)
		if got != tt.want {
			t.Errorf("containsFailureSignal(%q)=%v want %v", tt.text, got, tt.want)
		}
	}
}

func TestCountToolStats(t *testing.T) {
	// Verify the function handles nil gracefully.
	calls, errors := countToolStats(nil)
	if calls != 0 || errors != 0 {
		t.Error("nil messages should return 0,0")
	}

	// Also verify empty slice returns zero.
	calls, errors = countToolStats([]agentcore.Message{})
	if calls != 0 || errors != 0 {
		t.Error("empty messages should return 0,0")
	}
}
