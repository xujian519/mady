// Package agentcore provides a generic pub/sub broker for fan-out event delivery.
//
// Broker is a lightweight in-process event broker that supports two delivery
// semantics:
//
//   - Publish: best-effort, non-blocking, lossy under contention. If a
//     subscriber's channel is full the event is dropped and a counter is
//     incremented. This is the right choice for high-frequency intermediate
//     updates (e.g. streaming token deltas).
//
//   - PublishMustDeliver: bounded-blocking. For each subscriber it first
//     tries a non-blocking send, then falls back to a blocking send with a
//     hard timeout (default 50ms). The publisher never blocks indefinitely.
//     This is the right choice for terminal events (finish, tool result,
//     error, cancel) that must not be silently dropped.
//
// Drop counters are exposed so callers can surface saturation in telemetry.
package agentcore

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// brokerDefaultBufSize is the per-subscriber channel capacity.
	// Sized to cover a long streaming assistant turn even under UI stalls.
	brokerDefaultBufSize = 4096

	// brokerDefaultMustDeliverTimeout is the per-subscriber upper bound on
	// how long PublishMustDeliver will block waiting for buffer space.
	brokerDefaultMustDeliverTimeout = 50 * time.Millisecond
)

// Broker is a generic fan-out event broker. Subscribers receive events on
// individual buffered channels; publishes fan out to all active subscribers.
//
// The zero value is not usable; create one with NewBroker or NewBrokerWithOptions.
type Broker[T any] struct {
	subs                 map[chan T]struct{}
	mu                   sync.RWMutex
	done                 chan struct{}
	channelBufferSize    int
	mustDeliverTimeout   time.Duration
	dropCount            atomic.Uint64
	mustDeliverDropCount atomic.Uint64
}

// NewBroker creates a Broker with the default buffer size (4096).
func NewBroker[T any]() *Broker[T] {
	return NewBrokerWithOptions[T](brokerDefaultBufSize)
}

// NewBrokerWithOptions creates a Broker with a custom per-subscriber buffer size.
func NewBrokerWithOptions[T any](channelBufferSize int) *Broker[T] {
	return &Broker[T]{
		subs:               make(map[chan T]struct{}),
		done:               make(chan struct{}),
		channelBufferSize:  channelBufferSize,
		mustDeliverTimeout: brokerDefaultMustDeliverTimeout,
	}
}

// SetMustDeliverTimeout overrides the per-subscriber timeout used by
// PublishMustDeliver. A zero or negative value resets to the default.
func (b *Broker[T]) SetMustDeliverTimeout(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if d <= 0 {
		b.mustDeliverTimeout = brokerDefaultMustDeliverTimeout
		return
	}
	b.mustDeliverTimeout = d
}

// Shutdown closes all subscriber channels and marks the broker as done.
// Subsequent publishes are no-ops. Idempotent.
func (b *Broker[T]) Shutdown() {
	select {
	case <-b.done:
		return
	default:
		close(b.done)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}

// Subscribe returns a channel that receives every published event.
// The channel is closed when the context is canceled or the broker is shut down.
func (b *Broker[T]) Subscribe(ctx context.Context) <-chan T {
	b.mu.Lock()
	defer b.mu.Unlock()

	select {
	case <-b.done:
		ch := make(chan T)
		close(ch)
		return ch
	default:
	}

	sub := make(chan T, b.channelBufferSize)
	b.subs[sub] = struct{}{}

	go func() {
		// Wait for either context cancellation or broker shutdown.
		select {
		case <-ctx.Done():
		case <-b.done:
		}

		b.mu.Lock()
		defer b.mu.Unlock()

		// If the broker was already shut down, Shutdown has already
		// closed the channel and removed it from subs — nothing to do.
		select {
		case <-b.done:
			return
		default:
		}

		// Double-check the sub is still registered (Shutdown may have
		// beaten us to it).
		if _, ok := b.subs[sub]; !ok {
			return
		}
		delete(b.subs, sub)
		close(sub)
	}()

	return sub
}

// SubscriberCount returns the number of active subscribers.
func (b *Broker[T]) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}

// DropCount returns the cumulative number of events dropped by Publish
// because a subscriber's channel was full.
func (b *Broker[T]) DropCount() uint64 {
	return b.dropCount.Load()
}

// MustDeliverDropCount returns the cumulative number of events dropped by
// PublishMustDeliver after the per-subscriber timeout expired.
func (b *Broker[T]) MustDeliverDropCount() uint64 {
	return b.mustDeliverDropCount.Load()
}

// Publish delivers an event to every active subscriber.
//
// Delivery is non-blocking and lossy: if a subscriber's channel is full the
// event is dropped for that subscriber, a warning is logged, and DropCount
// is incremented. Use PublishMustDeliver for events that must not be
// silently dropped.
func (b *Broker[T]) Publish(event T) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	select {
	case <-b.done:
		return
	default:
	}

	for sub := range b.subs {
		select {
		case sub <- event:
		default:
			b.dropCount.Add(1)
			slog.Warn("agentcore: pubsub buffer full, dropping event")
		}
	}
}

// PublishMustDeliver delivers an event with bounded-blocking semantics.
//
// For each subscriber it first attempts a non-blocking send, then falls back
// to a blocking send bounded by a per-subscriber timeout (default 50ms). On
// timeout the event is dropped for that subscriber and MustDeliverDropCount
// is incremented. The publisher never blocks indefinitely.
//
// Use this for terminal events (finish, tool result, error, cancel).
func (b *Broker[T]) PublishMustDeliver(ctx context.Context, event T) {
	// Snapshot subscribers under a brief read lock so per-subscriber timeout
	// waits do not block Subscribe/Unsubscribe.
	b.mu.RLock()

	select {
	case <-b.done:
		b.mu.RUnlock()
		return
	default:
	}

	timeout := b.mustDeliverTimeout
	subs := make([]chan T, 0, len(b.subs))
	for sub := range b.subs {
		subs = append(subs, sub)
	}
	b.mu.RUnlock()

	for _, sub := range subs {
		// Fast path: non-blocking send.
		select {
		case sub <- event:
			continue
		default:
		}

		// Slow path: bounded blocking send.
		timer := time.NewTimer(timeout)
		select {
		case sub <- event:
			timer.Stop()
		case <-timer.C:
			b.mustDeliverDropCount.Add(1)
			slog.Error("agentcore: PublishMustDeliver timed out",
				"timeout", timeout)
		case <-ctx.Done():
			timer.Stop()
			return
		case <-b.done:
			timer.Stop()
			return
		}
	}
}
