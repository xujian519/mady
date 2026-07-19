package agentcore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// MessageBus is a simple fan-out / fan-in message channel for orchestrating
// multiple agents. It is optional; use it when you want explicit routing
// between steps without sharing a single Agent state.
type MessageBus struct {
	mu      sync.Mutex
	subs    map[string][]chan Message
	dropped atomic.Int64
}

// NewMessageBus creates an empty bus.
func NewMessageBus() *MessageBus {
	return &MessageBus{subs: make(map[string][]chan Message)}
}

// Publish delivers a copy of m to every subscriber of topic (non-blocking
// per subscriber: drops if channel buffer full).
func (b *MessageBus) Publish(topic string, m Message) {
	b.mu.Lock()
	chs := append([]chan Message(nil), b.subs[topic]...)
	b.mu.Unlock()
	for _, ch := range chs {
		select {
		case ch <- m:
		default:
			b.dropped.Add(1)
		}
	}
}

// DroppedMessages returns the cumulative count of messages that were dropped
// because a subscriber's channel buffer was full. Useful for monitoring
// backpressure in orchestration pipelines.
func (b *MessageBus) DroppedMessages() int64 {
	return b.dropped.Load()
}

// Subscribe returns a receive-only channel for topic with buffer cap, and
// cancel removes the subscription and closes the channel.
func (b *MessageBus) Subscribe(topic string, cap int) (recv <-chan Message, cancel func()) {
	ch := make(chan Message, cap)
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], ch)
	b.mu.Unlock()
	cancel = func() {
		b.mu.Lock()
		sl := b.subs[topic]
		out := sl[:0]
		for _, c := range sl {
			if c != ch {
				out = append(out, c)
			}
		}
		if len(out) == 0 {
			delete(b.subs, topic)
		} else {
			b.subs[topic] = out
		}
		b.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

// SequentialAgentStep runs one agent after another, passing the previous
// agent's final output as the next agent's user message (unless empty).
func RunSequentialAgents(ctx context.Context, agents []*Agent, user string) (string, error) {
	return RunSequentialAgentsWithDepth(ctx, agents, user, DefaultMaxDelegationDepth)
}

// DefaultMaxDelegationDepth caps how deeply agents may delegate to one
// another (via TaskTool hand-off or sequential chaining) before the
// orchestration is rejected. It guards against unbounded recursion.
const DefaultMaxDelegationDepth = 8

// depthKey is the context key carrying the current delegation depth.
type depthKey struct{}

// DepthFromContext returns the delegation depth stored in ctx, or 0 when
// unset. Depth increases by one each time an agent delegates to another.
func DepthFromContext(ctx context.Context) int {
	if d, ok := ctx.Value(depthKey{}).(int); ok {
		return d
	}
	return 0
}

// WithDepth returns a copy of ctx carrying the given delegation depth.
func WithDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, depthKey{}, depth)
}

// RunSequentialAgentsWithDepth behaves like RunSequentialAgents but rejects
// chains longer than maxDepth. Each step runs at an increasing depth so that
// nested TaskTool delegation inside any step is also bounded.
func RunSequentialAgentsWithDepth(ctx context.Context, agents []*Agent, user string, maxDepth int) (string, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDelegationDepth
	}
	if len(agents) > maxDepth {
		return "", NewNodeError(
			"sequential delegation depth exceeded",
			ErrDepthExceeded,
			"orchestrate",
			fmt.Sprintf("agents:%d max:%d", len(agents), maxDepth),
		)
	}
	var last string
	var err error
	for i, ag := range agents {
		input := user
		if i > 0 && last != "" {
			input = last
		}
		last, err = ag.Run(WithDepth(ctx, i+1), input)
		if err != nil {
			return "", err
		}
	}
	return last, nil
}

// ErrDepthExceeded is returned when delegation depth exceeds the configured
// maximum. Tools and orchestrators wrap it via NewNodeError.
var ErrDepthExceeded = errors.New("agentcore: delegation depth exceeded")
