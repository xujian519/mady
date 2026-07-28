package framework

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// waitForDone polls IsDone until true or the timeout elapses.
func waitForDone(d *DeferredInit, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if d.IsDone() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return d.IsDone()
}

// ---------------------------------------------------------------------------
// DeferredInit — concurrent lazy initialization primitive
// ---------------------------------------------------------------------------

func TestDeferredInit_AllSuccess(t *testing.T) {
	d := NewDeferredInit()
	var count atomic.Int32
	d.Add("a", func() error { count.Add(1); return nil })
	d.Add("b", func() error { count.Add(1); return nil })
	d.StartAll(context.Background())

	if !waitForDone(d, time.Second) {
		t.Fatal("timed out waiting for deferred init")
	}
	if d.HasErrors() {
		t.Fatalf("unexpected errors: %v", d.Errors())
	}
	if got := count.Load(); got != 2 {
		t.Fatalf("expected 2 tasks run, got %d", got)
	}
}

func TestDeferredInit_Empty(t *testing.T) {
	d := NewDeferredInit()
	d.StartAll(context.Background())
	if !waitForDone(d, time.Second) {
		t.Fatal("timed out waiting for empty init")
	}
	if d.HasErrors() {
		t.Fatalf("empty init should have no errors: %v", d.Errors())
	}
	if summary := d.ErrorSummary(); summary != "" {
		t.Fatalf("empty init summary should be empty, got %q", summary)
	}
}

func TestDeferredInit_PartialFailure(t *testing.T) {
	d := NewDeferredInit()
	d.Add("ok", func() error { return nil })
	d.Add("boom", func() error { return errors.New("disk full") })
	d.StartAll(context.Background())
	waitForDone(d, time.Second)

	if !d.HasErrors() {
		t.Fatal("expected errors after a failed task")
	}
	errs := d.Errors()
	if errs["boom"] != "disk full" {
		t.Fatalf("expected boom=disk full, got %v", errs)
	}
	if _, ok := errs["ok"]; ok {
		t.Fatal("ok task should not appear in error map")
	}
	summary := d.ErrorSummary()
	if !strings.Contains(summary, "boom") || !strings.Contains(summary, "disk full") {
		t.Fatalf("unexpected summary: %q", summary)
	}
}

func TestDeferredInit_IdempotentStart(t *testing.T) {
	d := NewDeferredInit()
	var count atomic.Int32
	d.Add("t", func() error { count.Add(1); return nil })
	d.StartAll(context.Background())
	d.StartAll(context.Background()) // second call must be a no-op
	waitForDone(d, time.Second)

	if got := count.Load(); got != 1 {
		t.Fatalf("task should run exactly once even with double StartAll, got %d", got)
	}
	if !d.HasStarted() {
		t.Fatal("HasStarted should be true after StartAll")
	}
}

func TestDeferredInit_AddAfterStartRunsImmediately(t *testing.T) {
	d := NewDeferredInit()
	var ran atomic.Bool
	d.Add("blocker", func() error {
		time.Sleep(50 * time.Millisecond) // ensure StartAll has begun but not finished
		return nil
	})
	d.StartAll(context.Background())

	// Wait until the runner has started, then add a late task.
	for !d.HasStarted() {
		time.Sleep(time.Millisecond)
	}
	d.Add("late", func() error { ran.Store(true); return nil })
	waitForDone(d, time.Second)

	if !ran.Load() {
		t.Fatal("task added after StartAll should execute immediately")
	}
	if _, ok := d.Errors()["late"]; ok {
		t.Fatal("late task should not be in error map")
	}
}

func TestDeferredInit_CancelSkipsRemaining(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	d := NewDeferredInit()
	// "slow" cancels the context from within, so "after" observes ctx.Done.
	d.Add("slow", func() error {
		cancel()
		time.Sleep(20 * time.Millisecond)
		return nil
	})
	d.Add("after", func() error { return errors.New("should not run") })
	d.StartAll(ctx)
	waitForDone(d, time.Second)

	errs := d.Errors()
	if errs["after"] == "should not run" {
		t.Fatal("'after' should have been skipped (canceled), not executed")
	}
	if !strings.Contains(errs["after"], "canceled") {
		t.Fatalf("expected 'after' to be marked canceled, got %q", errs["after"])
	}
}
