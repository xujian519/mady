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
}

func (s *memoryEventStore) Append(_ context.Context, eventType, _, _ string, payload json.RawMessage) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, struct {
		EventType string
		Payload   json.RawMessage
	}{eventType, payload})
	return int64(len(s.events)), nil
}
func (s *memoryEventStore) Close() error { return nil }

func (s *memoryEventStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func TestEventLogger_RecordsSelectedEvents(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	store := &memoryEventStore{}
	logger := NewEventLogger(store)
	if err := logger.Start(bus); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer logger.Close()

	// Emit events that should be recorded.
	bus.Emit(NewAgentStartEvent("test-agent", "hello"))
	bus.Emit(NewApprovalPromptEvent("test-agent", "needs review", nil))
	bus.Emit(NewToolCallStartEvent(ToolCall{Name: "test_tool"}))

	// Give async goroutine time to flush.
	time.Sleep(100 * time.Millisecond)

	if n := store.count(); n != 3 {
		t.Fatalf("expected 3 recorded events, got %d", n)
	}
}

func TestEventLogger_SkipsHighFrequencyEvents(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	store := &memoryEventStore{}
	logger := NewEventLogger(store)
	if err := logger.Start(bus); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer logger.Close()

	// Emit a high-frequency event that should be skipped.
	bus.Emit(NewMessageDeltaEvent("hello", BlockKindText))

	time.Sleep(50 * time.Millisecond)

	if n := store.count(); n != 0 {
		t.Fatalf("expected 0 events for MessageDelta, got %d", n)
	}
}

func TestEventLogger_CloseUnregistersHandler(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	store := &memoryEventStore{}
	logger := NewEventLogger(store)
	if err := logger.Start(bus); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer logger.Close()

	// Emit an event and verify it's recorded.
	bus.Emit(NewAgentStartEvent("test", "input"))
	time.Sleep(100 * time.Millisecond)
	if n := store.count(); n != 1 {
		t.Fatalf("expected 1 event before close, got %d", n)
	}

	// Close the logger — handler should be unregistered.
	logger.Close()

	// Emit another event — should not be recorded.
	bus.Emit(NewAgentStartEvent("test", "after close"))
	time.Sleep(50 * time.Millisecond)

	if n := store.count(); n != 1 {
		t.Fatalf("expected 1 event (no more after close), got %d", n)
	}
}
