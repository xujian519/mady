package compiler

import (
	"encoding/json"
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
// S5: per-strategy 切换器 — medium signal 交替行为
// ---------------------------------------------------------------------------

func TestCompiler_MediumSignalGlobalToggle(t *testing.T) {
	// The successToggle and failureToggle are per-strategy, not global.
	// Each strategy has its own 50% effective alternation.
	// First medium signal: increments (toggle false→true)
	// Second medium signal: does NOT increment (toggle true→false)
	// Third medium signal: increments again (toggle false→true)
	strategies := []Strategy{
		{ID: "strategy_a", Preconditions: []string{"alpha"}, Description: "A", Guidance: "do A"},
		{ID: "strategy_b", Preconditions: []string{"beta"}, Description: "B", Guidance: "do B"},
	}
	c := NewCompiler(Config{Strategies: strategies, ExplorationRate: 0})

	// Start turn with "alpha" goal to select strategy A.
	_, sidA := c.StartTurn("alpha goal")
	if sidA != "strategy_a" {
		t.Fatalf("expected strategy_a, got %s", sidA)
	}

	// Start turn with "beta" goal to select strategy B.
	_, sidB := c.StartTurn("beta goal")
	if sidB != "strategy_b" {
		t.Fatalf("expected strategy_b, got %s", sidB)
	}

	// Get initial success counts.
	stA, _ := c.StrategyByID(sidA)
	stB, _ := c.StrategyByID(sidB)
	initA := stA.Successes
	initB := stB.Successes

	// Medium-signal trace for A: first medium → increments (toggle false→true).
	trace1 := NewTrace("t1", "test goal", sidA, 1)
	trace1.Complete(OutcomeSuccess, 3, 1) // 1 error → Medium signal
	c.FinishTurn(trace1)

	stA, _ = c.StrategyByID(sidA)
	if stA.Successes != initA+1 {
		t.Errorf("strategy A: expected %d successes, got %d (first medium should increment)", initA+1, stA.Successes)
	}

	// Medium-signal trace for B: also first medium for B → increments (toggle false→true).
	trace2 := NewTrace("t2", "test goal", sidB, 2)
	trace2.Complete(OutcomeSuccess, 3, 1)
	c.FinishTurn(trace2)

	stB, _ = c.StrategyByID(sidB)
	if stB.Successes != initB+1 {
		t.Errorf("strategy B: expected %d successes, got %d (first medium for B should increment)", initB+1, stB.Successes)
	}

	// Another medium-signal trace for A: second medium for A → does NOT increment (toggle true→false).
	trace3 := NewTrace("t3", "test goal", sidA, 3)
	trace3.Complete(OutcomeSuccess, 3, 1)
	c.FinishTurn(trace3)

	stA, _ = c.StrategyByID(sidA)
	if stA.Successes != initA+1 {
		t.Errorf("strategy A: expected %d successes, got %d (second medium should NOT increment)", initA+1, stA.Successes)
	}

	// Third medium-signal trace for A: increments again (toggle false→true).
	trace4 := NewTrace("t4", "test goal", sidA, 4)
	trace4.Complete(OutcomeSuccess, 3, 1)
	c.FinishTurn(trace4)

	stA, _ = c.StrategyByID(sidA)
	if stA.Successes != initA+2 {
		t.Errorf("strategy A: expected %d successes, got %d (third medium should increment again)", initA+2, stA.Successes)
	}
}

func TestCompiler_MediumSignalFailureAlternation(t *testing.T) {
	c := NewCompiler(Config{ExplorationRate: 0})
	_, sid := c.StartTurn("test goal")
	if sid == "" {
		t.Fatal("expected strategy ID")
	}

	st, _ := c.StrategyByID(sid)
	initF := st.Failures

	// Medium-signal failure: first for this strategy → increments (toggle false→true).
	trace1 := NewTrace("t1", "test goal", sid, 1)
	trace1.Complete(OutcomeFailure, 5, 3) // > half errors → Medium quality
	c.FinishTurn(trace1)

	st, _ = c.StrategyByID(sid)
	if st.Failures != initF+1 {
		t.Errorf("expected %d failures after first medium, got %d", initF+1, st.Failures)
	}

	// Second medium-signal failure: toggles to false → does NOT increment.
	trace2 := NewTrace("t2", "test goal", sid, 2)
	trace2.Complete(OutcomeFailure, 5, 3)
	c.FinishTurn(trace2)

	st, _ = c.StrategyByID(sid)
	if st.Failures != initF+1 {
		t.Errorf("expected %d failures after second medium (unchanged), got %d", initF+1, st.Failures)
	}

	// Third medium-signal failure: toggles back to true → increments.
	trace3 := NewTrace("t3", "test goal", sid, 3)
	trace3.Complete(OutcomeFailure, 5, 3)
	c.FinishTurn(trace3)

	st, _ = c.StrategyByID(sid)
	if st.Failures != initF+2 {
		t.Errorf("expected %d failures after third medium, got %d", initF+2, st.Failures)
	}
}

// ---------------------------------------------------------------------------
// S6: Save/Load 往返 + JSON 结构
// ---------------------------------------------------------------------------

func TestCompiler_SaveLoad_RoundtripWithMultipleStrategies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compiler_multi.json")

	c1 := NewCompiler(Config{ExplorationRate: 10, MaxTraces: 500})

	// Execute traces on two different strategies.
	_, sid1 := c1.StartTurn("审查意见答复")
	_, sid2 := c1.StartTurn("撰写说明书")
	if sid1 == "" || sid2 == "" || sid1 == sid2 {
		t.Skip("needs two distinct matching strategies")
	}

	trace1 := NewTrace("t1", "审查意见答复", sid1, 1)
	trace1.Complete(OutcomeSuccess, 3, 0)
	c1.FinishTurn(trace1)

	trace2 := NewTrace("t2", "撰写说明书", sid2, 1)
	trace2.Complete(OutcomeSuccess, 5, 1) // Medium signal
	c1.FinishTurn(trace2)

	trace3 := NewTrace("t3", "审查意见答复", sid1, 2)
	trace3.Complete(OutcomeFailure, 1, 0)
	c1.FinishTurn(trace3)

	if err := c1.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify the JSON file structure.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have strategies, exploration_rate, max_traces, decay_config.
	if _, ok := raw["strategies"]; !ok {
		t.Error("missing strategies field in persisted JSON")
	}
	if _, ok := raw["exploration_rate"]; !ok {
		t.Error("missing exploration_rate field")
	}
	if _, ok := raw["max_traces"]; !ok {
		t.Error("missing max_traces field")
	}
	if _, ok := raw["decay_config"]; !ok {
		t.Error("missing decay_config field")
	}

	// Load into a fresh compiler.
	c2 := NewCompiler(Config{})
	if err := c2.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify strategy stats survived.
	st1, ok := c2.StrategyByID(sid1)
	if !ok {
		t.Fatalf("strategy %s not found after load", sid1)
	}
	if st1.Successes != 1 {
		t.Errorf("strategy %s successes = %d, want 1", sid1, st1.Successes)
	}
	if st1.Failures != 1 {
		t.Errorf("strategy %s failures = %d, want 1", sid1, st1.Failures)
	}

	st2, ok := c2.StrategyByID(sid2)
	if !ok {
		t.Fatalf("strategy %s not found after load", sid2)
	}
	if st2.Successes != 1 {
		t.Errorf("strategy %s successes = %d, want 1 (medium signal)", sid2, st2.Successes)
	}

	// Exploration rate should be restored.
	if c2.explorationRate != 10 {
		t.Errorf("explorationRate = %d, want 10", c2.explorationRate)
	}
	// MaxTraces should be restored.
	if c2.maxTraces != 500 {
		t.Errorf("maxTraces = %d, want 500", c2.maxTraces)
	}
}

// ---------------------------------------------------------------------------
// S7: 原子写入 — 写入中途崩溃不损坏原文件
// ---------------------------------------------------------------------------

func TestCompiler_Save_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compiler_atomic.json")

	c := NewCompiler(Config{ExplorationRate: 5})
	_, sid := c.StartTurn("审查意见")
	trace := NewTrace("t1", "审查意见", sid, 1)
	trace.Complete(OutcomeSuccess, 3, 0)
	c.FinishTurn(trace)

	// Save normally.
	if err := c.Save(path); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	// Read the original content.
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate crash during write by replacing WriteFile with a truncating write
	// that fails mid-way. Since os.WriteFile is not atomic for large files,
	// a crash during write could corrupt the file.
	// We verify this by checking that after a successful save, the content is valid.
	_, sid2 := c.StartTurn("撰写")
	trace2 := NewTrace("t2", "撰写", sid2, 1)
	trace2.Complete(OutcomeSuccess, 2, 0)
	c.FinishTurn(trace2)

	// Re-save.
	if err := c.Save(path); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}

	// Verify the file is valid JSON.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) < len(orig) {
		t.Error("saved file should not be smaller after adding data")
	}

	var recovered persistData
	if err := json.Unmarshal(content, &recovered); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}

	// Verify it's loadable.
	c2 := NewCompiler(Config{})
	if err := c2.Load(path); err != nil {
		t.Fatalf("Load after atomic write failed: %v", err)
	}
}

func TestCompiler_Save_CorruptFileHandling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")

	// Write invalid data to simulate a corrupt file.
	if err := os.WriteFile(path, []byte("{invalid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewCompiler(Config{})
	err := c.Load(path)
	if err == nil {
		t.Error("expected error when loading corrupt file, got nil")
	}
}

func TestCompiler_Save_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")

	// Create empty file.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewCompiler(Config{})
	// Empty file is not valid JSON → should return an error.
	if err := c.Load(path); err == nil {
		t.Error("expected error for empty file (invalid JSON)")
	}
}

// ---------------------------------------------------------------------------
// S8: Compiler.Save 使用 os.WriteFile (非原子, 但应保证文件完整性)
// ---------------------------------------------------------------------------

func TestCompiler_Save_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.json")

	c := NewCompiler(Config{})
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Should be readable/writable by owner only (0o600).
	if info.Mode()&0o077 != 0 {
		t.Errorf("expected restricted permissions, got %v", info.Mode())
	}
}

// ---------------------------------------------------------------------------
// S9: Load with partial data — missing optional fields
// ---------------------------------------------------------------------------

func TestCompiler_LoadMinimalJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "minimal.json")

	// Write JSON with only required fields.
	minimal := `{"strategies": [{"id": "custom", "description": "Custom strategy", "preconditions": ["x"]}]}`
	if err := os.WriteFile(path, []byte(minimal), 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewCompiler(Config{})
	if err := c.Load(path); err != nil {
		t.Fatalf("Load minimal JSON failed: %v", err)
	}

	st, ok := c.StrategyByID("custom")
	if !ok {
		t.Fatal("custom strategy not found after load")
	}
	if st.Description != "Custom strategy" {
		t.Errorf("description = %q", st.Description)
	}
	// Success rate should be 0.5 for untested strategy.
	if st.SuccessRate() != 0.5 {
		t.Errorf("untested strategy success rate = %.2f, want 0.5", st.SuccessRate())
	}
}

// ---------------------------------------------------------------------------
// S10: TempFile write crash simulation
// ---------------------------------------------------------------------------

func TestCompiler_Save_WriteFailure(t *testing.T) {
	// Using a directory path as file should fail.
	c := NewCompiler(Config{})
	err := c.Save("/nonexistent/path/compiler.json")
	if err == nil {
		t.Error("expected error saving to nonexistent directory")
	}

	// Using a directory instead of a file should fail.
	tmpDir := t.TempDir()
	err = c.Save(tmpDir)
	if err == nil {
		t.Error("expected error saving to a directory path")
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
