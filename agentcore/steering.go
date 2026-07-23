package agentcore

import (
	"errors"
	"log/slog"
	"sync"
)

// ErrQueueFull is returned by messageQueue.Push when the queue has reached
// its configured MaxSize capacity.
var ErrQueueFull = errors.New("message queue is full")

// SteeringMode controls how multiple queued messages are drained.
type SteeringMode string

const (
	// SteeringAll drains all pending messages at once.
	SteeringAll SteeringMode = "all"
	// SteeringOneAtATime drains one message per LLM turn.
	SteeringOneAtATime SteeringMode = "one_at_a_time"
)

// messageQueue is a thread-safe queue of messages used for
// steering (interrupt) and follow-up (queue after current turn) injection.
//
// maxSize limits the number of buffered messages. 0 means unlimited.
type messageQueue struct {
	mu      sync.Mutex
	msgs    []Message
	mode    SteeringMode
	maxSize int // 0 = unlimited
}

func newMessageQueue(mode SteeringMode, maxSize int) *messageQueue {
	if mode == "" {
		mode = SteeringAll
	}
	return &messageQueue{mode: mode, maxSize: maxSize}
}

// Push adds a message to the queue. Returns ErrQueueFull when maxSize is set
// and the queue is at capacity. The message is NOT added on error.
func (q *messageQueue) Push(msg Message) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.maxSize > 0 && len(q.msgs) >= q.maxSize {
		slog.Warn("steering: message queue full, dropping message",
			"max_size", q.maxSize, "mode", q.mode)
		return ErrQueueFull
	}
	q.msgs = append(q.msgs, msg)
	return nil
}

// Drain returns and removes pending messages according to the configured mode.
func (q *messageQueue) Drain() []Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.msgs) == 0 {
		return nil
	}
	switch q.mode {
	case SteeringOneAtATime:
		msg := q.msgs[0]
		q.msgs = q.msgs[1:]
		return []Message{msg}
	default:
		msgs := q.msgs
		q.msgs = nil
		return msgs
	}
}

// Len returns the number of pending messages.
func (q *messageQueue) Len() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return int64(len(q.msgs))
}
