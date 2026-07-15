package concurrency

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPool_AcquireRelease(t *testing.T) {
	p := NewPool(2)

	// Acquire 2 slots.
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatalf("second acquire: %v", err)
	}

	// Third acquire should block; use context with short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := p.Acquire(ctx)
	if err == nil {
		t.Fatal("expected timeout error for third acquire")
	}

	// Release one slot.
	p.Release()

	// Now third acquire should succeed.
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	p.Release()
	p.Release()
}

func TestPool_Unlimited(t *testing.T) {
	p := NewPool(0) // unlimited

	const n = 100
	var wg sync.WaitGroup
	var count atomic.Int32

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.Acquire(context.Background()); err != nil {
				t.Errorf("unlimited pool Acquire failed: %v", err)
				return
			}
			count.Add(1)
			p.Release()
		}()
	}
	wg.Wait()

	if int(count.Load()) != n {
		t.Fatalf("expected %d acquires, got %d", n, count.Load())
	}
}

func TestPool_ConcurrencyLimit(t *testing.T) {
	const maxConcurrent = 3
	const total = 20

	p := NewPool(maxConcurrent)
	var maxRunning atomic.Int32
	var running atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.Acquire(context.Background()); err != nil {
				t.Errorf("Acquire failed: %v", err)
				return
			}
			cur := running.Add(1)
			// Track maximum concurrent.
			for {
				prev := maxRunning.Load()
				if cur <= prev || maxRunning.CompareAndSwap(prev, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond) // simulate work
			running.Add(-1)
			p.Release()
		}()
	}
	wg.Wait()

	if int(maxRunning.Load()) > maxConcurrent {
		t.Fatalf("max concurrency %d exceeded: %d", maxConcurrent, maxRunning.Load())
	}
	if int(maxRunning.Load()) < 1 {
		t.Fatal("expected at least 1 concurrent execution")
	}
}

func TestPool_PanicOnDoubleRelease(t *testing.T) {
	p := NewPool(1)
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	p.Release()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on double Release")
		}
	}()
	p.Release() // should panic
}

func TestCloseablePool_Close(t *testing.T) {
	p := NewCloseablePool(2)

	// Acquire one slot.
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Close the pool.
	p.Close()

	if !p.IsClosed() {
		t.Fatal("expected pool to be closed")
	}

	// New acquisitions should fail immediately.
	err := p.Acquire(context.Background())
	if err != ErrPoolClosed {
		t.Fatalf("expected ErrPoolClosed, got %v", err)
	}

	// Release should still work.
	p.Release()
}

func TestCloseablePool_CloseIsIdempotent(t *testing.T) {
	p := NewCloseablePool(1)
	p.Close()
	p.Close() // should not panic
	if !p.IsClosed() {
		t.Fatal("expected pool to be closed")
	}
}

func TestPool_SizeAndAvailable(t *testing.T) {
	p := NewPool(5)
	if p.Size() != 5 {
		t.Fatalf("expected size 5, got %d", p.Size())
	}
	if p.Available() != 5 {
		t.Fatalf("expected 5 available, got %d", p.Available())
	}

	p.Acquire(context.Background())
	p.Acquire(context.Background())
	if p.Available() != 3 {
		t.Fatalf("expected 3 available, got %d", p.Available())
	}

	p.Release()
	p.Release()
	if p.Available() != 5 {
		t.Fatalf("expected 5 available after release, got %d", p.Available())
	}
}
