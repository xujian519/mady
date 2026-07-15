// Package concurrency provides reusable concurrency control primitives
// for the agentcore runtime. These are extracted from ad-hoc patterns scattered
// across executor.go and graph.go, inspired by pi-go's internal/subagent/pool.go.
package concurrency

import (
	"context"
	"errors"
	"fmt"
)

// Pool is a context-aware concurrency limiter backed by a buffered channel
// semaphore. It supports up to maxConcurrent simultaneous acquisitions.
// When maxConcurrent <= 0, Acquire always succeeds (no limit).
//
// Example:
//
//	pool := concurrency.NewPool(10)
//	for _, item := range items {
//	    go func(item Item) {
//	        if err := pool.Acquire(ctx); err != nil {
//	            return // context canceled
//	        }
//	        defer pool.Release()
//	        process(item)
//	    }(item)
//	}
type Pool struct {
	size int
	sem  chan struct{}
}

// NewPool creates a concurrency pool. Pass 0 for unlimited concurrency.
func NewPool(maxConcurrent int) *Pool {
	if maxConcurrent <= 0 {
		return &Pool{size: 0}
	}
	return &Pool{
		size: maxConcurrent,
		sem:  make(chan struct{}, maxConcurrent),
	}
}

// Acquire blocks until a slot is available or ctx is canceled.
// Returns an error if ctx is done before a slot frees up.
// Always succeeds immediately when maxConcurrent <= 0.
func (p *Pool) Acquire(ctx context.Context) error {
	if p.size <= 0 {
		return nil
	}
	select {
	case p.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("pool acquire canceled: %w", ctx.Err())
	}
}

// Release frees one slot. Must be called exactly once per successful Acquire.
// Panics if called more times than Acquire (programmer error).
func (p *Pool) Release() {
	if p.size <= 0 {
		return
	}
	select {
	case <-p.sem:
	default:
		panic("concurrency.Pool: Release called without matching Acquire")
	}
}

// Size returns the maximum concurrency. 0 means unlimited.
func (p *Pool) Size() int { return p.size }

// Available returns the number of free slots (approximate, for monitoring).
func (p *Pool) Available() int {
	if p.size <= 0 {
		return 0 // unlimited — available isn't meaningful
	}
	return p.size - len(p.sem)
}

// ErrPoolClosed is returned by Acquire when the pool has been closed.
var ErrPoolClosed = errors.New("concurrency pool is closed")

// CloseablePool extends Pool with a shutdown mechanism. After Close() is
// called, subsequent Acquire calls return ErrPoolClosed. Existing holders
// can still Release().
type CloseablePool struct {
	Pool
	closed chan struct{}
}

// NewCloseablePool creates a pool that supports graceful shutdown.
func NewCloseablePool(maxConcurrent int) *CloseablePool {
	return &CloseablePool{
		Pool:   *NewPool(maxConcurrent),
		closed: make(chan struct{}),
	}
}

// Acquire blocks until a slot is available, ctx is canceled, or the pool is closed.
// When the pool has been closed (Close() called), Acquire returns ErrPoolClosed
// immediately or as soon as the currently-blocked goroutine is unblocked.
func (p *CloseablePool) Acquire(ctx context.Context) error {
	if p.size <= 0 {
		// Unlimited pool — just check if closed.
		select {
		case <-p.closed:
			return ErrPoolClosed
		default:
			return nil
		}
	}

	// Select on sem acquisition, ctx cancellation, AND pool closure.
	select {
	case p.sem <- struct{}{}:
		// Acquired a slot. Double-check if pool was closed during our wait.
		select {
		case <-p.closed:
			<-p.sem // release the slot
			return ErrPoolClosed
		default:
			return nil
		}
	case <-p.closed:
		return ErrPoolClosed
	case <-ctx.Done():
		return fmt.Errorf("pool acquire canceled: %w", ctx.Err())
	}
}

// Close prevents new acquisitions. Existing holders can still Release().
func (p *CloseablePool) Close() {
	select {
	case <-p.closed:
	default:
		close(p.closed)
	}
}

// IsClosed reports whether the pool has been closed.
func (p *CloseablePool) IsClosed() bool {
	select {
	case <-p.closed:
		return true
	default:
		return false
	}
}
