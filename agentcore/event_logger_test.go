package agentcore

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// memoryEventStore implements EventLoggerStore in-memory for testing.
type memoryEventStore struct {
	mu     sync.Mutex
	events []struct {
		EventType string
		Payload   json.RawMessage
	}
	signal func() // 每次 Append 后调用，测试用同步点
}

func (s *memoryEventStore) Append(_ context.Context, eventType, _, _ string, payload json.RawMessage) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, struct {
		EventType string
		Payload   json.RawMessage
	}{eventType, payload})
	if s.signal != nil {
		s.signal()
	}
	return int64(len(s.events)), nil
}
func (s *memoryEventStore) Close() error { return nil }

func (s *memoryEventStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

// waitForEvents 阻塞直到满足条件的 goroutine 完成（通过 signal channel）。
func waitForEvent(t *testing.T, store *memoryEventStore, ch <-chan struct{}, expected int, timeout time.Duration) {
	t.Helper()
	received := 0
	deadline := time.After(timeout)
	for received < expected {
		select {
		case <-ch:
			received++
		case <-deadline:
			store.mu.Lock()
			n := len(store.events)
			store.mu.Unlock()
			t.Fatalf("timed out waiting for %d events, got %d", expected, n)
		}
	}
}

func TestEventLogger_RecordsSelectedEvents(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	store := &memoryEventStore{}
	done := make(chan struct{}, 3)
	store.signal = func() { done <- struct{}{} }

	logger := NewEventLogger(store)
	if err := logger.Start(bus); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer logger.Close()

	// Emit events that should be recorded.
	bus.Emit(NewAgentStartEvent("test-agent", "hello"))
	bus.Emit(NewApprovalPromptEvent("test-agent", "needs review", nil))
	bus.Emit(NewToolCallStartEvent(ToolCall{Name: "test_tool"}))

	waitForEvent(t, store, done, 3, time.Second)
	if n := store.count(); n != 3 {
		t.Fatalf("expected 3 recorded events, got %d", n)
	}
}

func TestEventLogger_SkipsHighFrequencyEvents(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	store := &memoryEventStore{}
	done := make(chan struct{}, 1)
	store.signal = func() { done <- struct{}{} }

	logger := NewEventLogger(store)
	if err := logger.Start(bus); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer logger.Close()

	// Emit a high-frequency event that should be skipped.
	bus.Emit(NewMessageDeltaEvent("hello", BlockKindText))

	// Wait briefly to ensure async processing had a chance (no event should be recorded).
	select {
	case <-done:
		if n := store.count(); n != 0 {
			t.Fatalf("expected 0 events for MessageDelta, got %d", n)
		}
	case <-time.After(50 * time.Millisecond):
	}

	if n := store.count(); n != 0 {
		t.Fatalf("expected 0 events for MessageDelta, got %d", n)
	}
}

func TestEventLogger_CloseUnregistersHandler(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	store := &memoryEventStore{}
	done := make(chan struct{}, 2)
	store.signal = func() { done <- struct{}{} }

	logger := NewEventLogger(store)
	if err := logger.Start(bus); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer logger.Close()

	// Emit an event and verify it's recorded.
	bus.Emit(NewAgentStartEvent("test", "input"))
	waitForEvent(t, store, done, 1, time.Second)
	if n := store.count(); n != 1 {
		t.Fatalf("expected 1 event before close, got %d", n)
	}

	// Close the logger — handler should be unregistered.
	logger.Close()

	// Emit another event — should not be recorded.
	bus.Emit(NewAgentStartEvent("test", "after close"))

	// Give async processing a brief chance to NOT deliver (closed handler drops events).
	select {
	case <-done:
		t.Fatal("received event after logger close — handler was not unregistered")
	case <-time.After(50 * time.Millisecond):
	}

	if n := store.count(); n != 1 {
		t.Fatalf("expected 1 event (no more after close), got %d", n)
	}
}
