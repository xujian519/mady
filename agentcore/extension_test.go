package agentcore

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

// --- mock extensions ---

type testExtension struct {
	name    string
	initFn  func(ctx context.Context, agent *Agent) error
	dispose func() error
}

func (e *testExtension) Name() string                                 { return e.name }
func (e *testExtension) Init(ctx context.Context, agent *Agent) error { return e.initFn(ctx, agent) }
func (e *testExtension) Dispose() error                               { return e.dispose() }

func TestExtensionRegistry_RegisterAndNames(t *testing.T) {
	t.Parallel()
	reg := NewExtensionRegistry()

	ext1 := &testExtension{
		name: "ext1",
		initFn: func(ctx context.Context, agent *Agent) error {
			return nil
		},
		dispose: func() error { return nil },
	}
	ext2 := &testExtension{
		name: "ext2",
		initFn: func(ctx context.Context, agent *Agent) error {
			return nil
		},
		dispose: func() error { return nil },
	}

	agent := New(stubAgentConfig("ext_test", nil))
	err := reg.Register(context.Background(), agent, ext1, ext2)
	if err != nil {
		t.Fatalf("Register: unexpected error: %v", err)
	}

	names := reg.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d: %v", len(names), names)
	}
	if names[0] != "ext1" || names[1] != "ext2" {
		t.Fatalf("names = %v, want [ext1 ext2]", names)
	}
}

func TestExtensionRegistry_RegisterEmpty(t *testing.T) {
	t.Parallel()
	reg := NewExtensionRegistry()

	agent := New(stubAgentConfig("empty_ext", nil))
	err := reg.Register(context.Background(), agent)
	if err != nil {
		t.Fatalf("Register empty: unexpected error: %v", err)
	}

	names := reg.Names()
	if len(names) != 0 {
		t.Fatalf("expected empty names, got %v", names)
	}
}

func TestExtensionRegistry_Dispose(t *testing.T) {
	t.Parallel()
	var disposeOrder []string

	ext1 := &testExtension{
		name:   "ext1",
		initFn: func(ctx context.Context, agent *Agent) error { return nil },
		dispose: func() error {
			disposeOrder = append(disposeOrder, "ext1")
			return nil
		},
	}
	ext2 := &testExtension{
		name:   "ext2",
		initFn: func(ctx context.Context, agent *Agent) error { return nil },
		dispose: func() error {
			disposeOrder = append(disposeOrder, "ext2")
			return nil
		},
	}

	agent := New(stubAgentConfig("dispose_test", nil))
	reg := NewExtensionRegistry()
	err := reg.Register(context.Background(), agent, ext1, ext2)
	if err != nil {
		t.Fatalf("Register: unexpected error: %v", err)
	}

	err = reg.Dispose()
	if err != nil {
		t.Fatalf("Dispose: unexpected error: %v", err)
	}

	// Dispose should run in reverse order.
	if len(disposeOrder) != 2 {
		t.Fatalf("expected 2 dispose calls, got %d", len(disposeOrder))
	}
	if disposeOrder[0] != "ext2" || disposeOrder[1] != "ext1" {
		t.Fatalf("dispose order = %v, want [ext2 ext1]", disposeOrder)
	}

	// After dispose, Names should be empty.
	if names := reg.Names(); len(names) != 0 {
		t.Fatalf("expected empty names after dispose, got %v", names)
	}
}

func TestExtensionRegistry_DisposeIdempotent(t *testing.T) {
	t.Parallel()
	var disposeCount atomic.Int32

	ext := &testExtension{
		name:   "ext1",
		initFn: func(ctx context.Context, agent *Agent) error { return nil },
		dispose: func() error {
			disposeCount.Add(1)
			return nil
		},
	}

	agent := New(stubAgentConfig("idempotent", nil))
	reg := NewExtensionRegistry()
	_ = reg.Register(context.Background(), agent, ext)

	_ = reg.Dispose()
	_ = reg.Dispose() // second should be a no-op

	if disposeCount.Load() != 1 {
		t.Fatalf("expected 1 dispose call, got %d", disposeCount.Load())
	}
}

func TestExtensionRegistry_DisposeError(t *testing.T) {
	t.Parallel()
	ext1 := &testExtension{
		name:    "ext1",
		initFn:  func(ctx context.Context, agent *Agent) error { return nil },
		dispose: func() error { return nil },
	}
	ext2 := &testExtension{
		name:    "ext2",
		initFn:  func(ctx context.Context, agent *Agent) error { return nil },
		dispose: func() error { return fmt.Errorf("dispose fail") },
	}

	agent := New(stubAgentConfig("dispose_err", nil))
	reg := NewExtensionRegistry()
	_ = reg.Register(context.Background(), agent, ext1, ext2)

	err := reg.Dispose()
	if err == nil {
		t.Fatal("expected error from disposing ext2, got nil")
	}
}

func TestExtensionRegistry_RegisterFailureRollback(t *testing.T) {
	t.Parallel()
	var disposeOrder []string

	ext1 := &testExtension{
		name: "ext1",
		initFn: func(ctx context.Context, agent *Agent) error {
			return nil
		},
		dispose: func() error {
			disposeOrder = append(disposeOrder, "ext1")
			return nil
		},
	}
	ext2 := &testExtension{
		name: "ext2",
		initFn: func(ctx context.Context, agent *Agent) error {
			return fmt.Errorf("init fail")
		},
		dispose: func() error {
			disposeOrder = append(disposeOrder, "ext2")
			return nil
		},
	}
	ext3 := &testExtension{
		name: "ext3",
		initFn: func(ctx context.Context, agent *Agent) error {
			return nil
		},
		dispose: func() error {
			disposeOrder = append(disposeOrder, "ext3")
			return nil
		},
	}

	agent := New(stubAgentConfig("rollback", nil))
	reg := NewExtensionRegistry()

	// ext2 Init fails, so ext1 should be rolled back (disposed).
	err := reg.Register(context.Background(), agent, ext1, ext2, ext3)
	if err == nil {
		t.Fatal("expected error from ext2 init failure")
	}

	// Only ext1 should be disposed (reverse of those that succeeded).
	if len(disposeOrder) != 1 {
		t.Fatalf("expected 1 dispose (ext1 rollback), got %d: %v", len(disposeOrder), disposeOrder)
	}
	if disposeOrder[0] != "ext1" {
		t.Fatalf("expected dispose of ext1, got: %v", disposeOrder)
	}

	// Registry should be empty after rollback.
	if names := reg.Names(); len(names) != 0 {
		t.Fatalf("expected empty names after rollback, got %v", names)
	}
}

// TestExtensionRegistry_Visit verifies the Visit method.
func TestExtensionRegistry_Visit(t *testing.T) {
	t.Parallel()
	var visited bool

	ext := &testExtension{
		name:    "found_me",
		initFn:  func(ctx context.Context, agent *Agent) error { return nil },
		dispose: func() error { return nil },
	}

	agent := New(stubAgentConfig("visit_test", nil))
	reg := NewExtensionRegistry()
	_ = reg.Register(context.Background(), agent, ext)

	reg.Visit("found_me", func(e Extension) {
		visited = true
		if e.Name() != "found_me" {
			t.Errorf("expected name 'found_me', got %q", e.Name())
		}
	})
	if !visited {
		t.Fatal("Visit did not call the function")
	}

	// Visit a non-existent extension should not call the function.
	reg.Visit("not_found", func(e Extension) {
		t.Fatal("should not be called for unknown extension")
	})
}

// TestExtensionRegistry_SnapshotEvents verifies SnapshotEvents.
func TestExtensionRegistry_SnapshotEvents(t *testing.T) {
	t.Parallel()
	// Extension that implements EventSnapshotProvider.
	snapshotExt := &testSnapshotExtension{
		name:    "snapshot",
		initFn:  func(ctx context.Context, agent *Agent) error { return nil },
		dispose: func() error { return nil },
		events: []Event{
			&AgentStartEvent{baseEvent: newBase(EventAgentStart)},
		},
	}

	agent := New(stubAgentConfig("snapshot_test", nil))
	reg := NewExtensionRegistry()
	_ = reg.Register(context.Background(), agent, snapshotExt)

	events := reg.SnapshotEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 snapshot event, got %d", len(events))
	}
	if events[0].EventKind() != EventAgentStart {
		t.Fatalf("event kind = %s, want %s", events[0].EventKind(), EventAgentStart)
	}
}

// testSnapshotExtension implements EventSnapshotProvider.
type testSnapshotExtension struct {
	name    string
	initFn  func(ctx context.Context, agent *Agent) error
	dispose func() error
	events  []Event
}

func (e *testSnapshotExtension) Name() string { return e.name }
func (e *testSnapshotExtension) Init(ctx context.Context, agent *Agent) error {
	return e.initFn(ctx, agent)
}
func (e *testSnapshotExtension) Dispose() error          { return e.dispose() }
func (e *testSnapshotExtension) SnapshotEvents() []Event { return e.events }
