package agentcore

import (
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
