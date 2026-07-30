package component

import (
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
)

// newTestLoader creates a Loader whose onRequestRender sends on the returned
// channel, so tests can wait for renders instead of sleeping.
func newTestLoader(msg string) (*Loader, <-chan struct{}) {
	ch := make(chan struct{}, 64)
	return NewLoader(func() {
		select {
		case ch <- struct{}{}:
		default:
		}
	}, msg), ch
}

func TestLoaderLifecycle(t *testing.T) {
	l, rendered := newTestLoader("loading")
	if l == nil {
		t.Fatal("NewLoader returned nil")
	}
	if l.IsRunning() {
		t.Fatal("new loader should not be running")
	}

	l.Start()
	if !l.IsRunning() {
		t.Fatal("loader should be running after Start")
	}

	// Wait for at least one animation frame.
	select {
	case <-rendered:
		// OK
	case <-time.After(time.Second):
		t.Fatal("loader did not produce a frame within timeout")
	}

	lines := l.Render(40)
	if len(lines) != 1 {
		t.Fatalf("want 1 rendered line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "loading") {
		t.Fatalf("rendered line should contain message: %q", lines[0])
	}

	l.Stop()
	if l.IsRunning() {
		t.Fatal("loader should not be running after Stop")
	}

	lines = l.Render(40)
	if len(lines) != 1 {
		t.Fatalf("want 1 rendered line after stop, got %d", len(lines))
	}
	if strings.Contains(lines[0], "loading") {
		t.Fatalf("rendered line after stop should be blank, got %q", lines[0])
	}
}

func TestLoaderDoubleStartStop(t *testing.T) {
	l, _ := newTestLoader("x")
	l.Start()
	l.Start() // should be no-op
	if !l.IsRunning() {
		t.Fatal("loader should still be running")
	}
	l.Stop()
	l.Stop() // should be no-op
	if l.IsRunning() {
		t.Fatal("loader should be stopped")
	}
}

func TestLoaderSetMessage(t *testing.T) {
	l, rendered := newTestLoader("old")
	l.Start()

	// Wait for initial render.
	select {
	case <-rendered:
	case <-time.After(time.Second):
		t.Fatal("no initial render")
	}

	l.SetMessage("new")
	// SetMessage triggers onRequestRender; wait for it.
	select {
	case <-rendered:
	case <-time.After(time.Second):
		t.Fatal("no render after SetMessage")
	}

	// Render should now contain the new message.
	lines := l.Render(40)
	if !strings.Contains(lines[0], "new") {
		t.Fatalf("SetMessage did not update rendered output: %q", lines[0])
	}
	l.Stop()
}

func TestLoaderSetStyle(t *testing.T) {
	l, rendered := newTestLoader("x")
	l.SetStyle(core.SpinnerLine)
	l.Start()

	// Wait for at least one animation frame.
	select {
	case <-rendered:
	case <-time.After(time.Second):
		t.Fatal("loader did not produce a frame within timeout")
	}

	lines := l.Render(40)
	l.Stop()
	if len(lines) != 1 {
		t.Fatal("expected one line")
	}
}

func TestCancellableLoaderAbort(t *testing.T) {
	cl := NewCancellableLoader(nil, "loading")
	if cl.Aborted() {
		t.Fatal("new cancellable loader should not be aborted")
	}
	cl.Update(core.KeyMsg{Data: "\x1b"}) // escape
	if !cl.Aborted() {
		t.Fatal("escape should abort the loader")
	}
	if cl.Context().Err() == nil {
		t.Fatal("abort should cancel the context")
	}
}
