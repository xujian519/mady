//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/doomloop"
	"github.com/xujian519/mady/domains"
)

// ============================================================================
// doomLoopProvider — mock LLM provider that produces controlled response
// patterns to trigger specific doomloop conditions.
// ============================================================================

type doomLoopProvider struct {
	mu    sync.Mutex
	calls int

	contentFn   func(call int) string
	toolCallsFn func(call int) []agentcore.ToolCall
}

func (p *doomLoopProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	p.mu.Lock()
	call := p.calls
	p.calls++
	p.mu.Unlock()
	return &agentcore.ProviderResponse{
		Content:   p.contentFn(call),
		ToolCalls: p.toolCallsFn(call),
	}, nil
}

func (p *doomLoopProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

// ============================================================================
// signalCapture — collects doomloop signals during a test run.
// ============================================================================

// signalCapture collects doomloop.Signal values emitted during test execution.
type signalCapture struct {
	mu      sync.Mutex
	signals []doomloop.Signal
}

func (sc *signalCapture) add(s doomloop.Signal) {
	sc.mu.Lock()
	sc.signals = append(sc.signals, s)
	sc.mu.Unlock()
}

// requireDetected fails the test unless the given detector fired at least once.
func (sc *signalCapture) requireDetected(t *testing.T, detector doomloop.DetectorID) {
	t.Helper()
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if len(sc.signals) == 0 {
		t.Fatalf("expected at least 1 doomloop signal, got 0")
	}
	for _, s := range sc.signals {
		if s.Detector == detector {
			t.Logf("%s signal: %s (fatal=%v, turn=%d)", detector, s.Reason, s.Fatal, s.Turn)
			return
		}
	}
	t.Errorf("expected %s detector signal. All signals:", detector)
	for _, s := range sc.signals {
		t.Errorf("  detector=%s reason=%q fatal=%v", s.Detector, s.Reason, s.Fatal)
	}
}

// requireNone fails the test if any signal was emitted.
func (sc *signalCapture) requireNone(t *testing.T) {
	t.Helper()
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if len(sc.signals) > 0 {
		t.Errorf("expected 0 doomloop signals, got %d:", len(sc.signals))
		for _, s := range sc.signals {
			t.Errorf("  detector=%s reason=%q", s.Detector, s.Reason)
		}
	}
}

// newDoomloopWithCapture creates a DoomLoop with signal capture attached.
func newDoomloopWithCapture(opts ...doomloop.Option) (*doomloop.DoomLoop, *signalCapture) {
	sc := &signalCapture{}
	allOpts := make([]doomloop.Option, 0, len(opts)+1)
	allOpts = append(allOpts, opts...)
	allOpts = append(allOpts, doomloop.WithOnSignal(sc.add))
	return doomloop.New(allOpts...), sc
}

// ============================================================================
// Helpers
// ============================================================================

// dummyTool returns a simple no-op tool so the executor doesn't fail
// when the mock provider returns tool calls.
func dummyTool(name string) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        name,
		Description: "A dummy tool for testing",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Func: func(_ context.Context, _ json.RawMessage) (any, error) {
			return "ok", nil
		},
	}
}

// makeToolCall is a convenience wrapper that builds a ToolCall for dummyTool.
func makeToolCall(name string) agentcore.ToolCall {
	return agentcore.ToolCall{
		ID:        "call_test_" + name,
		Name:      name,
		Arguments: "{}",
	}
}

// newStubAgent is a shorthand for creating an agent with a stub provider.
func runStubAgent(t *testing.T, cfg agentcore.Config, input string) (string, error) {
	t.Helper()
	agent := agentcore.New(cfg)
	defer agent.Close()
	return agent.Run(context.Background(), input)
}

// ============================================================================
// Test 1: ToolCallLoop — repeated identical tool calls
// ============================================================================

func TestDoomLoopE2E_ToolCallLoop(t *testing.T) {
	dl, sc := newDoomloopWithCapture(doomloop.WithToolCallLoop(3))

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "e2e-test-doomloop-toolcall",
			Model: "stub",
			Provider: &doomLoopProvider{
				contentFn: func(_ int) string { return "" },
				toolCallsFn: func(_ int) []agentcore.ToolCall {
					return []agentcore.ToolCall{makeToolCall("dummy_tool")}
				},
			},
		},
		ExecutionConfig: agentcore.ExecutionConfig{MaxTurns: 10},
		Lifecycle:       dl.AsHook(),
		Tools:           []*agentcore.Tool{dummyTool("dummy_tool")},
	}

	_, err := runStubAgent(t, cfg, "test loop")
	if err != nil {
		t.Logf("agent.Run returned error (expected for doomloop): %v", err)
	}
	sc.requireDetected(t, doomloop.DetectorToolCallLoop)
}

// ============================================================================
// Test 2: TextRepetition — repeated text content in model output
// ============================================================================

func TestDoomLoopE2E_TextRepetition(t *testing.T) {
	// Set a very high tool call loop threshold so it doesn't preempt
	// the text repetition detector.
	dl, sc := newDoomloopWithCapture(
		doomloop.WithToolCallLoop(20),
		doomloop.WithTextRepetition(3),
	)

	// The provider returns repeated text AND a tool call on each turn.
	// This keeps the agent in the inner loop so AfterModelCall fires
	// multiple times, allowing the text repetition detector to accumulate
	// history. Without tool calls, the agent exits on the first text-only
	// response and the detector only sees 1 call (< minRepeat of 3).
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "e2e-test-doomloop-textrep",
			Model: "stub",
			Provider: &doomLoopProvider{
				contentFn: func(_ int) string { return "这是一个重复的文本内容" },
				toolCallsFn: func(_ int) []agentcore.ToolCall {
					return []agentcore.ToolCall{makeToolCall("dummy_tool")}
				},
			},
		},
		ExecutionConfig: agentcore.ExecutionConfig{MaxTurns: 10},
		Lifecycle:       dl.AsHook(),
		Tools:           []*agentcore.Tool{dummyTool("dummy_tool")},
	}

	_, err := runStubAgent(t, cfg, "test repetition")
	if err != nil {
		t.Logf("agent.Run returned error (expected for doomloop): %v", err)
	}
	sc.requireDetected(t, doomloop.DetectorTextRepetition)
}

// ============================================================================
// Test 3: CircuitBreaker — total tool calls exceed the global limit
// ============================================================================

func TestDoomLoopE2E_CircuitBreaker(t *testing.T) {
	dl, sc := newDoomloopWithCapture(doomloop.WithCircuitBreaker(3))

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "e2e-test-doomloop-circuit",
			Model: "stub",
			Provider: &doomLoopProvider{
				contentFn: func(_ int) string { return "" },
				toolCallsFn: func(call int) []agentcore.ToolCall {
					if call < 3 {
						return []agentcore.ToolCall{
							makeToolCall("dummy_tool"),
							makeToolCall("dummy_tool"),
						}
					}
					return nil
				},
			},
		},
		ExecutionConfig: agentcore.ExecutionConfig{MaxTurns: 10},
		Lifecycle:       dl.AsHook(),
		Tools:           []*agentcore.Tool{dummyTool("dummy_tool")},
	}

	_, err := runStubAgent(t, cfg, "test circuit breaker")
	if err != nil {
		t.Logf("agent.Run returned error (expected for doomloop): %v", err)
	}
	sc.requireDetected(t, doomloop.DetectorCircuitBreaker)
}

// ============================================================================
// Test 4: NormalOperation — varied responses should NOT trigger doomloop
// ============================================================================

func TestDoomLoopE2E_NormalOperation(t *testing.T) {
	dl, sc := newDoomloopWithCapture(
		doomloop.WithToolCallLoop(3),
		doomloop.WithTextRepetition(3),
		doomloop.WithCycleLength(2),
		doomloop.WithEmptyResultMax(3),
		doomloop.WithCircuitBreaker(20),
		doomloop.WithCompactionMax(3),
	)

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "e2e-test-doomloop-normal",
			Model: "stub",
			Provider: &doomLoopProvider{
				contentFn: func(call int) string {
					return []string{
						"第一轮：讨论问题背景",
						"第二轮：分析技术方案",
						"第三轮：总结结论",
					}[call%3]
				},
				toolCallsFn: func(call int) []agentcore.ToolCall {
					if call == 2 {
						return []agentcore.ToolCall{makeToolCall("search_tool")}
					}
					return nil
				},
			},
		},
		ExecutionConfig: agentcore.ExecutionConfig{MaxTurns: 5},
		Lifecycle:       dl.AsHook(),
		Tools:           []*agentcore.Tool{dummyTool("search_tool")},
	}

	output, err := runStubAgent(t, cfg, "正常操作测试")
	if err != nil {
		t.Fatalf("agent.Run failed unexpectedly: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	sc.requireNone(t)
}

// ============================================================================
// Test 5: EmptyResult — consecutive empty tool results
// ============================================================================

func TestDoomLoopE2E_EmptyResult(t *testing.T) {
	dl, sc := newDoomloopWithCapture(doomloop.WithEmptyResultMax(3))

	emptyTool := &agentcore.Tool{
		Name:        "empty_tool",
		Description: "A tool that returns empty result",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Func: func(_ context.Context, _ json.RawMessage) (any, error) {
			return "", nil
		},
	}

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "e2e-test-doomloop-empty",
			Model: "stub",
			Provider: &doomLoopProvider{
				contentFn: func(_ int) string { return "" },
				toolCallsFn: func(call int) []agentcore.ToolCall {
					if call < 5 {
						return []agentcore.ToolCall{makeToolCall("empty_tool")}
					}
					return nil
				},
			},
		},
		ExecutionConfig: agentcore.ExecutionConfig{MaxTurns: 10},
		Lifecycle:       dl.AsHook(),
		Tools:           []*agentcore.Tool{emptyTool},
	}

	_, err := runStubAgent(t, cfg, "test empty results")
	if err != nil {
		t.Logf("agent.Run returned error (expected for doomloop): %v", err)
	}
	sc.requireDetected(t, doomloop.DetectorEmptyResult)
}

// ============================================================================
// Test 6: DoomLoop lifecycle chain integration with domain-level patterns
// ============================================================================

func TestDoomLoopE2E_DomainLifecycleChain(t *testing.T) {
	dl := doomloop.New()

	resetHook := &resetTrackerHook{}
	lifecycleChain := agentcore.AppendLifecycle(dl.AsHook(), resetHook)

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "e2e-test-domain-chain",
			Model: "stub",
			Provider: &doomLoopProvider{
				contentFn:   func(_ int) string { return "normal operation" },
				toolCallsFn: func(_ int) []agentcore.ToolCall { return nil },
			},
		},
		ExecutionConfig: agentcore.ExecutionConfig{MaxTurns: 3},
		Lifecycle:       lifecycleChain,
	}

	output, err := runStubAgent(t, cfg, "test domain wiring")
	if err != nil {
		t.Fatalf("agent.Run failed: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if !resetHook.called {
		t.Error("BeforeAgentRun hook was not called (lifecycle chain may be broken)")
	}
	if sigs := dl.Signals(); sigs == nil {
		t.Error("Signals() returned nil (DoomLoop may not be properly initialized)")
	}
}

// resetTrackerHook records whether BeforeAgentRun was called.
type resetTrackerHook struct {
	agentcore.BaseLifecycleHook
	called bool
}

func (h *resetTrackerHook) BeforeAgentRun(_ context.Context, _ *agentcore.AgentRunContext) error {
	h.called = true
	return nil
}

// ============================================================================
// Test 7: Domain config wiring — verify doomloop creates a valid agent
// ============================================================================

func TestDoomLoopE2E_DomainConfigAssistant(t *testing.T) {
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "e2e-test-assistant-doomloop",
			Model: "stub",
			Provider: &doomLoopProvider{
				contentFn: func(_ int) string {
					return `{"action":"test","result":"done","success":true}`
				},
				toolCallsFn: func(_ int) []agentcore.ToolCall { return nil },
			},
		},
		ExecutionConfig: agentcore.ExecutionConfig{MaxTurns: 3},
	}
	cfg := domains.AssistantAgentConfig(base)

	output, err := runStubAgent(t, cfg, "验证 Assistant 领域配置包含 DoomLoop")
	if err != nil {
		t.Fatalf("assistant agent.Run failed: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output from assistant agent")
	}
}

// ============================================================================
// Utility
// ============================================================================

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
