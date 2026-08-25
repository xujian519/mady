package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// EventLoggerStore is the persistence interface that EventLogger uses to
// write events. The concrete implementation lives in domains/sqlite/ to
// avoid coupling agentcore with SQLite infrastructure.
type EventLoggerStore interface {
	Append(ctx context.Context, eventType, sessionID, agentName string, payload json.RawMessage) (int64, error)
	Close() error
}

// recordedEvents lists the event types that are important enough to persist.
// High-frequency events (EventMessageDelta) are excluded to avoid flooding
// the event store.
var recordedEvents = map[EventType]bool{
	EventAgentStart:     true,
	EventAgentEnd:       true,
	EventAgentError:     true,
	EventAgentInterrupt: true,
	EventApprovalPrompt: true,
	EventToolCallStart:  true,
	EventToolCallEnd:    true,
	EventTurnStart:      true,
	EventTurnEnd:        true,
	EventHandoffStart:   true,
	EventHandoffEnd:     true,
}

// EventLogger subscribes to an EventBus and asynchronously persists
// selected events to a EventLoggerStore. It uses a buffered channel to
// avoid blocking the EventBus dispatch goroutine.
//
// Start must be called after construction to register the EventBus handler.
// The handler is automatically unregistered on Close.
type EventLogger struct {
	store   EventLoggerStore
	ch      chan Event
	cancel  func() // unregisters the EventBus handler
	done    chan struct{}
	started bool
}

// NewEventLogger creates an EventLogger backed by the given store.
// Call Start to begin listening.
func NewEventLogger(store EventLoggerStore) *EventLogger {
	return &EventLogger{
		store: store,
		ch:    make(chan Event, 256),
		done:  make(chan struct{}),
	}
}

// Start registers the EventBus handler and starts the async write loop.
// Must be called exactly once; returns an error on double-start.
func (el *EventLogger) Start(bus *EventBus) error {
	if el.started {
		return fmt.Errorf("event logger already started")
	}
	el.started = true
	el.cancel = bus.OnAll(el.handle)
	go el.loop()
	return nil
}

// handle is the EventBus callback. It runs on the EventBus dispatch
// goroutine and must not block. Events are sent to a channel for
// async writes.
func (el *EventLogger) handle(e Event) {
	if e == nil {
		return
	}
	if !recordedEvents[e.EventKind()] {
		return
	}
	select {
	case el.ch <- e:
	default:
		// Channel full — drop event to avoid blocking EventBus.
		slog.Warn("event_logger: buffer full, dropping event", "type", e.EventKind())
	}
}

// loop runs in a background goroutine, reading events from the channel
// and writing them to the store.
func (el *EventLogger) loop() {
	defer close(el.done)
	ctx := context.Background()
	for e := range el.ch {
		payload, err := json.Marshal(e)
		if err != nil {
			slog.Warn("event_logger: marshal failed", "type", e.EventKind(), "err", err)
			continue
		}
		agentName := extractAgentName(e)
		if _, err := el.store.Append(ctx, string(e.EventKind()), "", agentName, payload); err != nil {
			slog.Warn("event_logger: append failed", "type", e.EventKind(), "err", err)
		}
	}
}

// Close unregisters the EventBus handler, closes the channel to signal
// the loop to finish, and closes the underlying store.
// Safe to call multiple times.
func (el *EventLogger) Close() {
	if el.cancel != nil {
		el.cancel()
		el.cancel = nil
	}
	if !el.started {
		return
	}
	el.started = false
	close(el.ch)
	// Wait for the loop to finish.
	<-el.done
	_ = el.store.Close()
}

// extractAgentName attempts to extract the agent name from common event types
// that carry it. Returns empty string if the event type doesn't carry a name.
func extractAgentName(e Event) string {
	switch v := e.(type) {
	case *AgentStartEvent:
		return v.AgentName
	case *AgentEndEvent:
		return v.AgentName
	case *AgentInterruptEvent:
		return v.AgentName
	case *ApprovalPromptEvent:
		return v.AgentName
	default:
		return ""
	}
}
