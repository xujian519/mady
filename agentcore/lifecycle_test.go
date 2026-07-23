package agentcore

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// --- ObserversToHook tests ---

func TestObserversToHook_SingleObserver(t *testing.T) {
	t.Parallel()
	observer := &testAgentRunObserver{}

	hook := ObserversToHook(observer)
	if hook == nil {
		t.Fatal("expected non-nil LifecycleHook")
	}

	// A single observer should NOT be wrapped in a LifecycleChain.
	_, isChain := hook.(LifecycleChain)
	if isChain {
		t.Fatal("single observer should not be wrapped in LifecycleChain")
	}

	// Verify the hook actually calls through to the observer.
	var called atomic.Int32
	observer.beforeFn = func(ctx context.Context, arc *AgentRunContext) error {
		called.Add(1)
		return nil
	}

	err := hook.BeforeAgentRun(context.Background(), &AgentRunContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called.Load() != 1 {
		t.Fatalf("BeforeAgentRun called %d times, want 1", called.Load())
	}
}

func TestObserversToHook_MultiObserver(t *testing.T) {
	t.Parallel()
	o1 := &testTurnObserver{}
	o2 := &testTurnObserver{}

	hook := ObserversToHook(o1, o2)
	if hook == nil {
		t.Fatal("expected non-nil LifecycleHook")
	}

	// Multiple observers should be composed via LifecycleChain.
	chain, ok := hook.(LifecycleChain)
	if !ok {
		t.Fatalf("expected LifecycleChain for multiple observers, got %T", hook)
	}
	if len(chain) != 2 {
		t.Fatalf("expected chain of length 2, got %d", len(chain))
	}

	// Verify BeforeTurn calls both in order.
	var order []string
	o1.beforeFn = func(ctx context.Context, arc *AgentRunContext) error {
		order = append(order, "o1")
		return nil
	}
	o2.beforeFn = func(ctx context.Context, arc *AgentRunContext) error {
		order = append(order, "o2")
		return nil
	}

	err := hook.BeforeTurn(context.Background(), &AgentRunContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 2 || order[0] != "o1" || order[1] != "o2" {
		t.Fatalf("BeforeTurn order = %v, want [o1 o2]", order)
	}
}

func TestObserversToHook_MultiInterfaceObserver(t *testing.T) {
	t.Parallel()
	// One type implementing both AgentRunObserver and TurnObserver.
	multi := &testMultiObserver{}

	hook := ObserversToHook(multi)
	if hook == nil {
		t.Fatal("expected non-nil LifecycleHook")
	}

	// Should be wrapped in a LifecycleChain because wrapObserver returns a chain
	// for multi-interface observers.
	_, isChain := hook.(LifecycleChain)
	if !isChain {
		t.Fatalf("expected LifecycleChain for multi-interface observer, got %T", hook)
	}

	// Verify both interfaces work through the hook.
	var runCalled, turnCalled atomic.Int32
	multi.runBeforeFn = func(ctx context.Context, arc *AgentRunContext) error {
		runCalled.Add(1)
		return nil
	}
	multi.turnBeforeFn = func(ctx context.Context, arc *AgentRunContext) error {
		turnCalled.Add(1)
		return nil
	}

	err := hook.BeforeAgentRun(context.Background(), &AgentRunContext{})
	if err != nil {
		t.Fatalf("BeforeAgentRun unexpected error: %v", err)
	}
	err = hook.BeforeTurn(context.Background(), &AgentRunContext{})
	if err != nil {
		t.Fatalf("BeforeTurn unexpected error: %v", err)
	}

	if runCalled.Load() != 1 {
		t.Fatalf("BeforeAgentRun called %d times, want 1", runCalled.Load())
	}
	if turnCalled.Load() != 1 {
		t.Fatalf("BeforeTurn called %d times, want 1", turnCalled.Load())
	}
}

func TestObserversToHook_NilInput(t *testing.T) {
	t.Parallel()
	hook := ObserversToHook(nil)
	if hook != nil {
		t.Fatalf("expected nil for nil input, got %T", hook)
	}
}

func TestObserversToHook_UnsupportedType(t *testing.T) {
	t.Parallel()
	// A plain struct that doesn't implement any observer interface.
	type unsupported struct{}
	hook := ObserversToHook(unsupported{})
	if hook != nil {
		t.Fatalf("expected nil for unsupported type, got %T", hook)
	}
}

func TestObserversToHook_MixedTypes(t *testing.T) {
	t.Parallel()
	runObs := &testAgentRunObserver{}
	turnObs := &testTurnObserver{}

	hook := ObserversToHook(runObs, turnObs)
	if hook == nil {
		t.Fatal("expected non-nil LifecycleHook")
	}

	chain, ok := hook.(LifecycleChain)
	if !ok {
		t.Fatalf("expected LifecycleChain, got %T", hook)
	}
	if len(chain) != 2 {
		t.Fatalf("expected chain length 2, got %d", len(chain))
	}
}

// --- LifecycleChain tests ---

func TestLifecycleChain_BeforeForward_AfterReverse(t *testing.T) {
	t.Parallel()
	var order []string

	h1 := &testLifecycleOrderHook{
		name: "h1",
		beforeFn: func() {
			order = append(order, "h1")
		},
		afterFn: func() {
			order = append(order, "h1-after")
		},
	}
	h2 := &testLifecycleOrderHook{
		name: "h2",
		beforeFn: func() {
			order = append(order, "h2")
		},
		afterFn: func() {
			order = append(order, "h2-after")
		},
	}
	h3 := &testLifecycleOrderHook{
		name: "h3",
		beforeFn: func() {
			order = append(order, "h3")
		},
		afterFn: func() {
			order = append(order, "h3-after")
		},
	}

	chain := LifecycleChain{h1, h2, h3}

	// BeforeAgentRun should run forward: h1, h2, h3
	err := chain.BeforeAgentRun(context.Background(), &AgentRunContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedBefore := []string{"h1", "h2", "h3"}
	for i, v := range expectedBefore {
		if order[i] != v {
			t.Fatalf("BeforeAgentRun order[%d] = %q, want %q", i, order[i], v)
		}
	}

	// AfterAgentRun should run reverse: h3-after, h2-after, h1-after
	chain.AfterAgentRun(context.Background(), &AgentRunContext{}, "", nil)
	expectedAfter := []string{"h1", "h2", "h3", "h3-after", "h2-after", "h1-after"}
	if len(order) != len(expectedAfter) {
		t.Fatalf("total order length = %d, want %d", len(order), len(expectedAfter))
	}
	for i, v := range expectedAfter {
		if order[i] != v {
			t.Fatalf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}

func TestLifecycleChain_BeforeTurnForward_AfterTurnReverse(t *testing.T) {
	t.Parallel()
	var order []string

	h1 := &testLifecycleOrderHook{name: "h1",
		beforeTurnFn: func() { order = append(order, "h1") },
		afterTurnFn:  func() { order = append(order, "h1-after") },
	}
	h2 := &testLifecycleOrderHook{name: "h2",
		beforeTurnFn: func() { order = append(order, "h2") },
		afterTurnFn:  func() { order = append(order, "h2-after") },
	}

	chain := LifecycleChain{h1, h2}

	_ = chain.BeforeTurn(context.Background(), &AgentRunContext{})
	expectedBefore := []string{"h1", "h2"}
	for i, v := range expectedBefore {
		if order[i] != v {
			t.Fatalf("BeforeTurn order[%d] = %q, want %q", i, order[i], v)
		}
	}

	chain.AfterTurn(context.Background(), &AgentRunContext{}, TurnInfo{})
	expectedAfter := []string{"h1", "h2", "h2-after", "h1-after"}
	if len(order) != len(expectedAfter) {
		t.Fatalf("total order length = %d, want %d", len(order), len(expectedAfter))
	}
	for i, v := range expectedAfter {
		if order[i] != v {
			t.Fatalf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}

func TestLifecycleChain_BeforeError_StopsChain(t *testing.T) {
	t.Parallel()
	var called atomic.Int32

	h1 := &testLifecycleOrderHook{
		name: "h1",
		beforeFn: func() {
			called.Add(1)
		},
	}
	h3 := &testLifecycleOrderHook{
		name: "h3",
		beforeFn: func() {
			called.Add(1)
		},
	}

	// errorBeforeHook returns an error on BeforeAgentRun.
	h2Err := &errorBeforeHook{}
	errorChain := LifecycleChain{h1, h2Err, h3}

	err := errorChain.BeforeAgentRun(context.Background(), &AgentRunContext{})
	if err == nil {
		t.Fatal("expected error from h2, got nil")
	}

	// h1 should have been called, h2Err should have errored, h3 should NOT.
	if called.Load() != 1 {
		t.Fatalf("before functions called %d times, want 1 (only h1)", called.Load())
	}

	// AfterAgentRun should still run all in reverse (h3, h2Err, h1) despite the error.
	var afterCalled atomic.Int32
	h1.afterFn = func() { afterCalled.Add(1) }
	h3.afterFn = func() { afterCalled.Add(1) }

	errorChain.AfterAgentRun(context.Background(), &AgentRunContext{}, "", err)
	if afterCalled.Load() != 2 {
		t.Fatalf("after functions called %d times, want 2 (h3 and h1 in reverse)", afterCalled.Load())
	}
}

// --- observer / hook test helpers ---

type testAgentRunObserver struct {
	AgentRunObserver
	beforeFn func(ctx context.Context, arc *AgentRunContext) error
	afterFn  func(ctx context.Context, arc *AgentRunContext, output string, err error)
}

func (o *testAgentRunObserver) BeforeAgentRun(ctx context.Context, arc *AgentRunContext) error {
	if o.beforeFn != nil {
		return o.beforeFn(ctx, arc)
	}
	return nil
}
func (o *testAgentRunObserver) AfterAgentRun(ctx context.Context, arc *AgentRunContext, output string, err error) {
	if o.afterFn != nil {
		o.afterFn(ctx, arc, output, err)
	}
}

type testTurnObserver struct {
	beforeFn func(ctx context.Context, arc *AgentRunContext) error
	afterFn  func(ctx context.Context, arc *AgentRunContext, info TurnInfo)
}

func (o *testTurnObserver) BeforeTurn(ctx context.Context, arc *AgentRunContext) error {
	if o.beforeFn != nil {
		return o.beforeFn(ctx, arc)
	}
	return nil
}
func (o *testTurnObserver) AfterTurn(ctx context.Context, arc *AgentRunContext, info TurnInfo) {
	if o.afterFn != nil {
		o.afterFn(ctx, arc, info)
	}
}

type testMultiObserver struct {
	runBeforeFn  func(ctx context.Context, arc *AgentRunContext) error
	runAfterFn   func(ctx context.Context, arc *AgentRunContext, output string, err error)
	turnBeforeFn func(ctx context.Context, arc *AgentRunContext) error
	turnAfterFn  func(ctx context.Context, arc *AgentRunContext, info TurnInfo)
}

func (o *testMultiObserver) BeforeAgentRun(ctx context.Context, arc *AgentRunContext) error {
	if o.runBeforeFn != nil {
		return o.runBeforeFn(ctx, arc)
	}
	return nil
}
func (o *testMultiObserver) AfterAgentRun(ctx context.Context, arc *AgentRunContext, output string, err error) {
	if o.runAfterFn != nil {
		o.runAfterFn(ctx, arc, output, err)
	}
}
func (o *testMultiObserver) BeforeTurn(ctx context.Context, arc *AgentRunContext) error {
	if o.turnBeforeFn != nil {
		return o.turnBeforeFn(ctx, arc)
	}
	return nil
}
func (o *testMultiObserver) AfterTurn(ctx context.Context, arc *AgentRunContext, info TurnInfo) {
	if o.turnAfterFn != nil {
		o.turnAfterFn(ctx, arc, info)
	}
}

// testLifecycleOrderHook records call order for Before/After assertions.
type testLifecycleOrderHook struct {
	BaseLifecycleHook
	name         string
	beforeFn     func()
	afterFn      func()
	beforeTurnFn func()
	afterTurnFn  func()
}

func (h *testLifecycleOrderHook) BeforeAgentRun(ctx context.Context, arc *AgentRunContext) error {
	if h.beforeFn != nil {
		h.beforeFn()
	}
	return nil
}
func (h *testLifecycleOrderHook) AfterAgentRun(ctx context.Context, arc *AgentRunContext, output string, err error) {
	if h.afterFn != nil {
		h.afterFn()
	}
}
func (h *testLifecycleOrderHook) BeforeTurn(ctx context.Context, arc *AgentRunContext) error {
	if h.beforeTurnFn != nil {
		h.beforeTurnFn()
	}
	return nil
}
func (h *testLifecycleOrderHook) AfterTurn(ctx context.Context, arc *AgentRunContext, info TurnInfo) {
	if h.afterTurnFn != nil {
		h.afterTurnFn()
	}
}

// errorBeforeHook returns an error on BeforeAgentRun.
type errorBeforeHook struct {
	BaseLifecycleHook
}

func (h *errorBeforeHook) BeforeAgentRun(ctx context.Context, arc *AgentRunContext) error {
	return errors.New("boom")
}
