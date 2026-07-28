package framework

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/agentconfig"
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

// ---------------------------------------------------------------------------
// Pure helper functions
// ---------------------------------------------------------------------------

func TestCwdPartitionName_Deterministic(t *testing.T) {
	a := CwdPartitionName("/some/path")
	b := CwdPartitionName("/some/path")
	if a != b {
		t.Fatalf("not deterministic: %q vs %q", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("expected 16 hex chars, got %d (%q)", len(a), a)
	}
}

func TestCwdPartitionName_Distinct(t *testing.T) {
	if CwdPartitionName("/alpha") == CwdPartitionName("/beta") {
		t.Fatal("different cwds should produce different partition names")
	}
}

func TestTasklistDirForCWD(t *testing.T) {
	// Empty cwd falls back to a flat tasks directory.
	if got, want := TasklistDirForCWD("/data", ""), filepath.Join("/data", "tasks"); got != want {
		t.Fatalf("empty cwd: got %q want %q", got, want)
	}
	// Non-empty cwd is partitioned by hash to avoid collisions.
	cwd := "/home/user/proj"
	want := filepath.Join("/data", "by-cwd", CwdPartitionName(cwd), "tasks")
	if got := TasklistDirForCWD("/data", cwd); got != want {
		t.Fatalf("non-empty cwd: got %q want %q", got, want)
	}
}

// fakeExt is a minimal agentcore.Extension for testing ExtSlice.
type fakeExt struct{ name string }

func (f *fakeExt) Name() string                                 { return f.name }
func (f *fakeExt) Init(context.Context, *agentcore.Agent) error { return nil }
func (f *fakeExt) Dispose() error                               { return nil }

func TestExtSlice(t *testing.T) {
	if got := ExtSlice(nil); got != nil {
		t.Fatalf("nil input should yield nil, got %v", got)
	}
	ext := &fakeExt{name: "x"}
	got := ExtSlice(ext)
	if len(got) != 1 || got[0] != ext {
		t.Fatalf("expected single-element slice containing ext, got %v", got)
	}
}

func TestAgentThinking(t *testing.T) {
	if got := AgentThinking(nil); got != nil {
		t.Fatalf("nil input should yield nil, got %+v", got)
	}
	cfg := &agentconfig.ThinkingConfig{
		IncludeThoughts: true,
		Display:         "summarized",
		Effort:          "high",
		Budget:          4096,
	}
	got := AgentThinking(cfg)
	if got == nil {
		t.Fatal("expected non-nil output for non-nil input")
	}
	if !got.IncludeThoughts {
		t.Error("IncludeThoughts not mapped")
	}
	if got.Budget != 4096 {
		t.Errorf("Budget mismatch: got %d want 4096", got.Budget)
	}
}
