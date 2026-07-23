package agentcore

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

const testEventBufSize = 64 // small buffer for fast tests

// newTestEventBus creates an EventBus with a small internal buffer for testing.
func newTestEventBus() *EventBus {
	eb := &EventBus{
		handlers: make(map[EventType]map[uint64]EventHandler),
		global:   make(map[uint64]EventHandler),
		broker:   NewBrokerWithOptions[Event](testEventBufSize),
		done:     make(chan struct{}),
	}
	ready := make(chan struct{})
	go eb.dispatch(ready)
	<-ready
	return eb
}

func TestEventBus_EmitNonBlocking(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	// Fill the buffer to capacity
	for i := 0; i < testEventBufSize; i++ {
		eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	}

	// This emit should not block even though the buffer is full
	done := make(chan struct{})
	go func() {
		eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
		close(done)
	}()

	select {
	case <-done:
		// Success: Emit returned without blocking
	case <-time.After(time.Second):
		t.Fatal("Emit blocked when buffer was full — should have dropped the event instead")
	}
}

func TestEventBus_EmitAfterClose(t *testing.T) {
	eb := newTestEventBus()
	eb.Close()

	// Emit on a closed bus must not panic
	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
}

func TestEventBus_DrainEventuallyCompletes(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	var received atomic.Int32
	eb.On(EventAgentStart, func(e Event) {
		received.Add(1)
	})

	const n = 10
	for i := 0; i < n; i++ {
		eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	}

	eb.Drain()

	if got := received.Load(); got != n {
		t.Fatalf("received %d events, want %d", got, n)
	}
}

func TestEventBus_DrainAfterClose_NoPanic(t *testing.T) {
	eb := newTestEventBus()

	// Fill the buffer so Drain would need to wait
	for i := 0; i < testEventBufSize; i++ {
		eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	}

	eb.Close()

	// Drain on a closed bus must not panic or spin forever
	done := make(chan struct{})
	go func() {
		eb.Drain()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Drain blocked after Close — should have returned immediately")
	}
}

// TestEventBus_HandlerPanicDoesNotKillDispatch verifies #2: a panicking
// handler is recovered, logged to stderr, and does NOT take down the
// dispatch goroutine. Subsequent events must still be delivered to other
// handlers and to the same handler (if it re-registers). Without recover,
// the dispatch goroutine would exit, close(done), and silently drop every
// future event — the event bus would be permanently dead.
func TestEventBus_HandlerPanicDoesNotKillDispatch(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	var got atomic.Int32

	// First global handler panics.
	eb.OnAll(func(e Event) {
		panic("boom from handler")
	})
	// Second global handler must still run despite the first panicking.
	eb.OnAll(func(e Event) {
		got.Add(1)
	})

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()

	// Both events should have been delivered to the second handler.
	if got := got.Load(); got != 2 {
		t.Fatalf("second handler received %d events, want 2 (dispatch died on panic?)", got)
	}
}

func TestEventBus_EmitMustDeliver(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	var received atomic.Int32
	eb.On(EventAgentStart, func(e Event) {
		received.Add(1)
	})

	eb.EmitMustDeliver(t.Context(), &AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()

	if got := received.Load(); got != 1 {
		t.Fatalf("received %d events, want 1", got)
	}
}

func TestEventBus_Subscribe(t *testing.T) {
	eb := NewEventBus()
	defer eb.Close()

	ctx := t.Context()
	ch := eb.Subscribe(ctx)

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})

	select {
	case e := <-ch:
		if e.EventKind() != EventAgentStart {
			t.Fatalf("expected agent_start, got %s", e.EventKind())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event via Subscribe")
	}
}

func TestA2UIEventCreation(t *testing.T) {
	env := map[string]any{
		"version": "v0.9.1",
		"createSurface": map[string]any{
			"surfaceId": "test",
			"catalogId": "https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json",
		},
	}
	ev := NewA2UIEvent(env)
	if ev == nil {
		t.Fatal("NewA2UIEvent returned nil")
	}
	if ev.EventKind() != EventA2UI {
		t.Fatalf("EventKind = %v, want %v", ev.EventKind(), EventA2UI)
	}
	if ev.EventTime().IsZero() {
		t.Fatal("EventTime should not be zero")
	}
	if ev.Envelope == nil {
		t.Fatal("Envelope should not be nil")
	}
	if ev.Envelope["version"] != "v0.9.1" {
		t.Fatalf("version = %v, want v0.9.1", ev.Envelope["version"])
	}
}

func TestA2UIEventEmitAndReceive(t *testing.T) {
	eb := NewEventBus()
	defer eb.Close()

	env := map[string]any{
		"deleteSurface": map[string]any{"surfaceId": "s"},
	}
	ev := NewA2UIEvent(env)
	if ev == nil {
		t.Fatal("NewA2UIEvent returned nil")
	}

	// Register handler
	var got Event
	done := make(chan struct{})
	eb.On(EventA2UI, func(e Event) {
		got = e
		close(done)
	})

	// Emit
	eb.EmitMustDeliver(t.Context(), ev)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for A2UIEvent")
	}

	if got == nil {
		t.Fatal("handler was not called")
	}
	if got.EventKind() != EventA2UI {
		t.Fatalf("EventKind = %v, want %v", got.EventKind(), EventA2UI)
	}
	ae, ok := got.(*A2UIEvent)
	if !ok {
		t.Fatalf("got type = %T, want *A2UIEvent", got)
	}
	if ae.Envelope["deleteSurface"] == nil {
		t.Fatal("Envelope should contain deleteSurface")
	}
}

// --- 新增测试：handler deregistration ---

func TestEventBus_OnDeregistration(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	var count atomic.Int32
	cancel := eb.On(EventAgentStart, func(e Event) {
		count.Add(1)
	})

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()
	if got := count.Load(); got != 1 {
		t.Fatalf("handler received %d events before cancel, want 1", got)
	}

	// Cancel the handler
	cancel()

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()
	if got := count.Load(); got != 1 {
		t.Fatalf("handler received %d events after cancel, want 1 (still)", got)
	}
}

func TestEventBus_OnAllDeregistration(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	var count atomic.Int32
	cancel := eb.OnAll(func(e Event) {
		count.Add(1)
	})

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()
	if got := count.Load(); got != 1 {
		t.Fatalf("OnAll handler received %d events before cancel, want 1", got)
	}

	cancel()

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()
	if got := count.Load(); got != 1 {
		t.Fatalf("OnAll handler received %d events after cancel, want 1 (still)", got)
	}
}

func TestEventBus_Deregistration_OnAndOnAll(t *testing.T) {
	// Verify that typed and global handlers can be independently deregistered.
	eb := newTestEventBus()
	defer eb.Close()

	var typed atomic.Int32
	var global atomic.Int32

	cancelTyped := eb.On(EventAgentStart, func(e Event) {
		typed.Add(1)
	})
	cancelGlobal := eb.OnAll(func(e Event) {
		global.Add(1)
	})

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()
	if typed.Load() != 1 {
		t.Fatalf("typed handler = %d, want 1", typed.Load())
	}
	if global.Load() != 1 {
		t.Fatalf("global handler = %d, want 1", global.Load())
	}

	// Only typed handler cancels
	cancelTyped()

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()
	if typed.Load() != 1 {
		t.Fatalf("typed handler after cancel = %d, want 1", typed.Load())
	}
	if global.Load() != 2 {
		t.Fatalf("global handler after typed cancel = %d, want 2", global.Load())
	}

	// Global also cancels
	cancelGlobal()

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()
	if global.Load() != 2 {
		t.Fatalf("global handler after cancel = %d, want 2", global.Load())
	}
}

// --- 新增测试：JSON 序列化往返 ---

func TestEventBus_JSONRoundtrip_AgentErrorEvent(t *testing.T) {
	orig := NewAgentErrorEvent(ErrExceedMaxSteps)
	data, err := orig.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var restored AgentErrorEvent
	if err := restored.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if restored.EventKind() != orig.EventKind() {
		t.Fatalf("kind = %v, want %v", restored.EventKind(), orig.EventKind())
	}
	if restored.Err == nil || restored.Err.Error() != orig.Err.Error() {
		t.Fatalf("error = %v, want %v", restored.Err, orig.Err)
	}
}

func TestEventBus_JSONRoundtrip_AutoRetryEvent(t *testing.T) {
	orig := NewAutoRetryEvent(2, 5, 100*time.Millisecond, context.DeadlineExceeded)
	data, err := orig.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var restored AutoRetryEvent
	if err := restored.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if restored.EventKind() != orig.EventKind() {
		t.Fatalf("kind = %v, want %v", restored.EventKind(), orig.EventKind())
	}
	if restored.Attempt != orig.Attempt {
		t.Fatalf("attempt = %d, want %d", restored.Attempt, orig.Attempt)
	}
	if restored.MaxRetries != orig.MaxRetries {
		t.Fatalf("maxRetries = %d, want %d", restored.MaxRetries, orig.MaxRetries)
	}
	if !errors.Is(restored.Err, context.DeadlineExceeded) {
		t.Fatalf("error should be DeadlineExceeded, got: %v", restored.Err)
	}
}

func TestEventBus_JSONRoundtrip_ToolCallEndEvent(t *testing.T) {
	orig := NewToolCallEndEvent("tc1", "my_tool", `{"result":"ok"}`, context.Canceled, 50*time.Millisecond)
	data, err := orig.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var restored ToolCallEndEvent
	if err := restored.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if restored.ToolCallID != orig.ToolCallID {
		t.Fatalf("ToolCallID = %q, want %q", restored.ToolCallID, orig.ToolCallID)
	}
	if !errors.Is(restored.Err, context.Canceled) {
		t.Fatalf("error should be Canceled, got: %v", restored.Err)
	}
}

func TestEventBus_JSONRoundtrip_HandoffEndEvent(t *testing.T) {
	testErr := errors.New("handoff rejected")
	orig := NewHandoffEndEvent("patent_agent", "output text", 1*time.Second, testErr, true)
	data, err := orig.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var restored HandoffEndEvent
	if err := restored.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if restored.TargetAgent != orig.TargetAgent {
		t.Fatalf("TargetAgent = %q, want %q", restored.TargetAgent, orig.TargetAgent)
	}
	if restored.Err == nil || restored.Err.Error() != "handoff rejected" {
		t.Fatalf("error = %v, want 'handoff rejected'", restored.Err)
	}
	if restored.Invisible != orig.Invisible {
		t.Fatalf("Invisible = %v, want %v", restored.Invisible, orig.Invisible)
	}
}

// --- 新增测试：SetDrainTimeout / Drain 超时配置 ---

func TestEventBus_DrainTimeout_Short(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	// Set a very short timeout so Drain returns quickly even when busy.
	eb.SetDrainTimeout(1 * time.Millisecond)

	var received atomic.Int32
	eb.On(EventAgentStart, func(e Event) {
		received.Add(1)
		time.Sleep(50 * time.Millisecond) // slow handler
	})

	// Fill buffer with events
	for i := 0; i < testEventBufSize; i++ {
		eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	}

	// Drain with short timeout — should not block for the full default 5s.
	done := make(chan struct{})
	go func() {
		eb.Drain()
		close(done)
	}()

	select {
	case <-done:
		// Success: Drain returned within the short timeout
	case <-time.After(3 * time.Second):
		t.Fatal("Drain did not return within 3s despite 1ms timeout — SetDrainTimeout may not work")
	}
}

func TestEventBus_DrainTimeout_ZeroResetsToDefault(t *testing.T) {
	// Verify the code path: timeout <= 0 falls back to 5s.
	eb := NewEventBus()

	// Setting a value then zero should keep the default working
	eb.SetDrainTimeout(0) // should reset to 5s but not hang

	var received atomic.Int32
	eb.On(EventAgentStart, func(e Event) {
		received.Add(1)
	})

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})

	done := make(chan struct{})
	go func() {
		eb.Drain()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Drain with zero timeout blocked — should have used default 5s and returned quickly")
	}

	eb.Close()
}

// --- 新增测试：DropCount / MustDeliverDropCount ---

func TestEventBus_DropCount_Incremented(t *testing.T) {
	eb := newTestEventBus()

	// Register a slow handler that blocks.
	eb.On(EventAgentStart, func(e Event) {
		time.Sleep(10 * time.Millisecond)
	})

	// The dispatch goroutine reads from the broker; we need to subscribe
	// a slow consumer via Subscribe to slow down the pipeline enough that
	// Publish(es) to the broker channel overflow.
	slowCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_ = eb.Subscribe(slowCtx)

	// Publish enough events to overflow both the broker's dispatch buffer
	// and the subscriber's buffer.
	for i := 0; i < testEventBufSize*4; i++ {
		eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	}

	// Some events were dropped.
	if count := eb.DropCount(); count == 0 {
		t.Fatal("DropCount = 0, expected > 0 after overflow")
	}
	eb.Close()
}

func TestEventBus_PanicCount_Incremented(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	eb.OnAll(func(e Event) {
		panic("expected panic")
	})

	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()

	if count := eb.PanicCount(); count == 0 {
		t.Fatal("PanicCount = 0, expected > 0 after handler panic")
	}
}

// --- 新增测试：On 和 OnAll 关闭后注册返回 no-op ---

func TestEventBus_OnAfterClose_NoOp(t *testing.T) {
	eb := newTestEventBus()
	eb.Close()

	// Should not panic and return a no-op cancel function.
	cancel := eb.On(EventAgentStart, func(e Event) {
		t.Error("handler should never be called on closed bus")
	})
	cancel() // safe to call no-op cancel

	// Emit should not panic either
	eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	eb.Drain()
}

func TestEventBus_OnAllAfterClose_NoOp(t *testing.T) {
	eb := newTestEventBus()
	eb.Close()

	cancel := eb.OnAll(func(e Event) {
		t.Error("handler should never be called on closed bus")
	})
	cancel()
}

// --- 新增测试：MustDeliver 超时行为 ---

func TestEventBus_MustDeliverTimedOut(t *testing.T) {
	// Create a broker with very short timeout to force MustDeliver timeouts.
	b := NewBrokerWithOptions[Event](testEventBufSize)
	b.SetMustDeliverTimeout(1 * time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Subscribe but DON'T read — the channel will fill up.
	_ = b.Subscribe(ctx)

	// Fill the buffer completely via Publish (non-blocking, will drop when full).
	for i := 0; i < testEventBufSize*2; i++ {
		b.Publish(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	}

	// The buffer is now full. PublishMustDeliver should time out.
	b.PublishMustDeliver(ctx, &AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})

	if count := b.MustDeliverDropCount(); count == 0 {
		t.Fatal("MustDeliverDropCount = 0, expected > 0 after buffer full with slow subscriber")
	}

	b.Shutdown()
}

// --- 新增测试：Broker Subscribe 后 Shutdown ---

func TestBroker_SubscribeAfterShutdown_ReturnsClosedChannel(t *testing.T) {
	b := NewBroker[Event]()
	b.Shutdown()

	ch := b.Subscribe(t.Context())

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel from Subscribe after Shutdown")
		}
	default:
		t.Fatal("Subscribe after Shutdown should return an immediately closed channel")
	}
}

// --- 新增测试：Broker MustDeliver 超时行为 ---

func TestBroker_MustDeliverTimeout(t *testing.T) {
	// Create a broker with a tiny buffer (capacity 1) and short timeout (10ms).
	b := NewBrokerWithOptions[Event](1)
	b.SetMustDeliverTimeout(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Subscribe a single slow subscriber.
	_ = b.Subscribe(ctx)

	// Fill the subscriber buffer.
	b.Publish(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})

	// The buffer is now full. PublishMustDeliver should time out on the
	// slow (non-reading) subscriber, incrementing MustDeliverDropCount.
	b.PublishMustDeliver(ctx, &AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})

	if count := b.MustDeliverDropCount(); count == 0 {
		t.Fatal("MustDeliverDropCount = 0, expected > 0 when subscriber buffer is full")
	}

	b.Shutdown()
}

func TestBroker_MustDeliverTimeout_MultipleSubscribers(t *testing.T) {
	b := NewBrokerWithOptions[Event](1)
	b.SetMustDeliverTimeout(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Fast subscriber: reads events immediately.
	fastCh := b.Subscribe(ctx)

	// Slow subscriber: buffer fills up.
	_ = b.Subscribe(ctx)

	// Fill both subscriber buffers: one event for each.
	b.Publish(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	b.Publish(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})

	// Drain the fast subscriber so it can receive the next event.
	<-fastCh
	<-fastCh

	// PublishMustDeliver should succeed for the fast subscriber (buffer empty)
	// but time out for the slow one (buffer full).
	b.PublishMustDeliver(ctx, &AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})

	if count := b.MustDeliverDropCount(); count == 0 {
		t.Fatal("MustDeliverDropCount = 0, expected > 0 when slow subscriber buffer is full")
	}

	// Fast subscriber should still have received the event.
	select {
	case <-fastCh:
		// Success: fast subscriber got the event.
	case <-time.After(time.Second):
		t.Fatal("fast subscriber did not receive event within timeout")
	}

	b.Shutdown()
}

// --- 新增测试：Drain 超时行为 ---

func TestEventBus_DrainTimeout_WithCustomTimeout(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	// Set a short drain timeout to verify Drain returns within it.
	eb.SetDrainTimeout(5 * time.Millisecond)

	var received atomic.Int32
	eb.On(EventAgentStart, func(e Event) {
		received.Add(1)
		time.Sleep(100 * time.Millisecond) // slow handler
	})

	// Emit events that will be processed slowly.
	for i := 0; i < 3; i++ {
		eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	}

	// Drain with short timeout should not block for the handler's full duration.
	start := time.Now()
	eb.Drain()
	elapsed := time.Since(start)

	// Should return well before 5 seconds (default).
	if elapsed > 2*time.Second {
		t.Fatalf("Drain took %v, expected to return quickly with 5ms timeout", elapsed)
	}
}

func TestEventBus_DrainTimeout_FastHandlerSucceeds(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	eb.SetDrainTimeout(5 * time.Millisecond)

	var received atomic.Int32
	eb.On(EventAgentStart, func(e Event) {
		received.Add(1)
	})

	n := 5
	for i := 0; i < n; i++ {
		eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	}

	eb.Drain()

	// All events should have been processed.
	if got := received.Load(); int(got) != n {
		t.Fatalf("received %d events, want %d", got, n)
	}
}

func TestEventBus_DrainTimeout_AfterCloseReturnsQuickly(t *testing.T) {
	eb := newTestEventBus()

	eb.SetDrainTimeout(5 * time.Millisecond)

	var received atomic.Int32
	eb.On(EventAgentStart, func(e Event) {
		received.Add(1)
		time.Sleep(50 * time.Millisecond)
	})

	// Emit events and close immediately.
	for i := 0; i < 5; i++ {
		eb.Emit(&AgentStartEvent{baseEvent: baseEvent{Kind: EventAgentStart}})
	}

	eb.Close()

	// Drain after close should return quickly.
	start := time.Now()
	eb.Drain()
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Fatalf("Drain after Close took %v, expected quick return", elapsed)
	}
}
