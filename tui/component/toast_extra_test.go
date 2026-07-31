package component

import (
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
)

func TestToastRenderLevels(t *testing.T) {
	for _, lvl := range []ToastLevel{ToastInfo, ToastSuccess, ToastWarning, ToastError} {
		toast := NewToast(time.Hour)
		toast.Show("消息", lvl)
		lines := toast.Render(40)
		if len(lines) != 1 {
			t.Fatalf("level %d: expected 1 line, got %d", lvl, len(lines))
		}
		if w := core.VisibleWidth(lines[0]); w != 40 {
			t.Fatalf("level %d: line width %d != 40 (line=%q)", lvl, w, lines[0])
		}
		if !strings.Contains(lines[0], "消息") {
			t.Fatalf("level %d: expected message in line %q", lvl, lines[0])
		}
		toast.Dismiss()
		if lines := toast.Render(40); lines != nil {
			t.Fatalf("level %d: expected nil render after dismiss", lvl)
		}
	}
}

func TestToastRenderInactiveOrEmpty(t *testing.T) {
	toast := NewToast(time.Hour)
	if lines := toast.Render(40); lines != nil {
		t.Fatalf("expected nil render for inactive toast, got %v", lines)
	}
	toast.mu.Lock()
	toast.active = true
	toast.message = ""
	toast.mu.Unlock()
	if lines := toast.Render(40); lines != nil {
		t.Fatalf("expected nil render for empty message, got %v", lines)
	}
}

func TestToastTruncatesLongMessage(t *testing.T) {
	toast := NewToast(time.Hour)
	toast.Show(strings.Repeat("长", 100), ToastInfo)
	lines := toast.Render(20)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if w := core.VisibleWidth(lines[0]); w != 20 {
		t.Fatalf("line width %d != 20 (line=%q)", w, lines[0])
	}
}

func TestToastRequestRenderCallback(t *testing.T) {
	toast := NewToast(time.Hour)
	rendered := make(chan struct{}, 10)
	toast.SetOnRequestRender(func() { rendered <- struct{}{} })
	toast.Show("x", ToastInfo)
	select {
	case <-rendered:
	case <-time.After(2 * time.Second):
		t.Fatal("expected requestRender on Show")
	}
	toast.Dismiss()
	select {
	case <-rendered:
	case <-time.After(2 * time.Second):
		t.Fatal("expected requestRender on Dismiss")
	}
}

func TestToastStaleShowSeq(t *testing.T) {
	toast := NewToast(60 * time.Millisecond)
	toast.Show("first", ToastInfo)
	time.Sleep(30 * time.Millisecond)
	toast.Show("second", ToastInfo) // bumps seq; first goroutine must be stale

	// t=70ms: first toast's 60ms timer fired but was stale; second toast's
	// timer (started t=30ms) is not due until t=90ms.
	time.Sleep(40 * time.Millisecond)
	toast.mu.RLock()
	active := toast.active
	msg := toast.message
	toast.mu.RUnlock()
	if !active {
		t.Fatal("expected most recent toast still active (stale expiry must be ignored)")
	}
	if msg != "second" {
		t.Fatalf("expected message 'second', got %q", msg)
	}
	// By t=120ms the second toast expires normally.
	time.Sleep(50 * time.Millisecond)
	if toast.IsActive() {
		t.Fatal("expected toast expired after its own duration")
	}
}

func TestToastAutoDismissFiresOnExpire(t *testing.T) {
	toast := NewToast(30 * time.Millisecond)
	expired := make(chan struct{}, 1)
	toast.SetOnExpire(func() { expired <- struct{}{} })
	toast.Show("auto", ToastInfo)
	select {
	case <-expired:
	case <-time.After(2 * time.Second):
		t.Fatal("expected onExpire after duration")
	}
	if toast.IsActive() {
		t.Fatal("expected toast inactive after expiry")
	}
}

func TestToastUpdateAndInvalidate(t *testing.T) {
	toast := NewToast(time.Hour)
	toast.Invalidate() // no-op
	if cmd := toast.Update(core.KeyMsg{Data: "x"}); cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
}

func TestToastNewToastZeroDuration(t *testing.T) {
	toast := NewToast(0)
	if toast.duration != 2*time.Second {
		t.Fatalf("expected default 2s duration, got %v", toast.duration)
	}
}

func TestToastHelperMethods(t *testing.T) {
	toast := NewToast(time.Hour)
	h := NewToastHelper(toast)
	h.CopyOK()
	if !toast.IsActive() {
		t.Fatal("expected active after CopyOK")
	}
	toast.mu.RLock()
	msg := toast.message
	toast.mu.RUnlock()
	if !strings.Contains(msg, "已复制") {
		t.Fatalf("expected copy message, got %q", msg)
	}
	h.ThemeSwitched("brand")
	toast.mu.RLock()
	msg = toast.message
	toast.mu.RUnlock()
	if !strings.Contains(msg, "主题") {
		t.Fatalf("expected theme message, got %q", msg)
	}
	h.StatusMessage("working")
	toast.mu.RLock()
	msg = toast.message
	toast.mu.RUnlock()
	if msg != "working" {
		t.Fatalf("expected status message, got %q", msg)
	}
}
