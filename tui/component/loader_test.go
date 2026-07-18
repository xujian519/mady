package component

import (
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
)

func TestLoaderLifecycle(t *testing.T) {
	l := NewLoader(nil, "loading")
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

	// Give animate() a chance to tick at least once.
	time.Sleep(120 * time.Millisecond)

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
	l := NewLoader(nil, "x")
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
	l := NewLoader(nil, "old")
	l.Start()
	l.SetMessage("new")
	// Render should eventually contain the new message.
	var found bool
	for i := 0; i < 20; i++ {
		lines := l.Render(40)
		if strings.Contains(lines[0], "new") {
			found = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	l.Stop()
	if !found {
		t.Fatal("SetMessage did not update rendered output")
	}
}

func TestLoaderSetStyle(t *testing.T) {
	l := NewLoader(nil, "x")
	l.SetStyle(core.SpinnerLine)
	l.Start()
	time.Sleep(120 * time.Millisecond)
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
