// Package concurrency provides concurrency control primitives for agentcore.
//
// # Pool
//
// Pool is a semaphore-based concurrency limiter. It restricts the number of
// concurrent goroutines that can execute a critical section without spawning
// dedicated worker goroutines.
//
// Usage:
//
//	pool := concurrency.NewPool(10) // at most 10 concurrent
//	if err := pool.Acquire(ctx); err != nil {
//	    return err
//	}
//	defer pool.Release()
//	// ... critical section ...
//
// # CloseablePool
//
// CloseablePool extends Pool with graceful shutdown. After Close, new Acquire
// calls return ErrPoolClosed, allowing long-running operations to drain.
package concurrency
