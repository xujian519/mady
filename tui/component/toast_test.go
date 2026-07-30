package component

import (
	"strings"
	"testing"
	"time"
)

func TestToastShowDismiss(t *testing.T) {
	t.Parallel()
	toast := NewToast(100 * time.Millisecond)
	if toast.IsActive() {
		t.Fatal("new toast should not be active")
	}
	toast.Show("hello", ToastInfo)
	if !toast.IsActive() {
		t.Fatal("toast should be active after Show")
	}
	toast.Dismiss()
	if toast.IsActive() {
		t.Fatal("toast should be inactive after Dismiss")
	}
}

func TestToastAutoDismiss(t *testing.T) {
	t.Parallel()
	toast := NewToast(50 * time.Millisecond)
	toast.Show("auto", ToastInfo)
	time.Sleep(100 * time.Millisecond)
	if toast.IsActive() {
		t.Fatal("toast should auto-dismiss after duration")
	}
}

func TestToastShowTwiceSeq(t *testing.T) {
	t.Parallel()
	// Rapid successive Show() calls: the first goroutine should not
	// dismiss the second toast (seq counter protection).
	toast := NewToast(100 * time.Millisecond)
	toast.Show("first", ToastInfo)
	toast.Show("second", ToastSuccess)
	// The toast should show "second" (the latest message)
	lines := toast.Render(40)
	if len(lines) == 0 {
		t.Fatal("expected rendered lines for active toast")
	}
	if !strings.Contains(lines[0], "second") {
		t.Fatalf("expected 'second' in toast, got %q", lines[0])
	}
	// Wait and verify the second toast auto-dismisses
	time.Sleep(150 * time.Millisecond)
	if toast.IsActive() {
		t.Fatal("toast should auto-dismiss after second Show")
	}
}

func TestToastDismissAfterExpiry(t *testing.T) {
	t.Parallel()
	toast := NewToast(20 * time.Millisecond)
	toast.Show("quick", ToastInfo)
	// Expired goroutine calling Dismiss should be a no-op
	time.Sleep(50 * time.Millisecond)
	toast.Dismiss() // no-op, already inactive
	if toast.IsActive() {
		t.Fatal("toast should be inactive after auto-dismiss")
	}
}

func TestToastRenderStyles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		level ToastLevel
		msg   string
	}{
		{"info", ToastInfo, "info message"},
		{"success", ToastSuccess, "✓ done"},
		{"warning", ToastWarning, "⚠ caution"},
		{"error", ToastError, "✗ failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			toast := NewToast(time.Second)
			toast.Show(tc.msg, tc.level)
			lines := toast.Render(60)
			if len(lines) == 0 {
				t.Fatal("expected at least one line")
			}
			if !strings.Contains(lines[0], tc.msg) {
				t.Fatalf("expected %q in rendered line, got %q", tc.msg, lines[0])
			}
			toast.Dismiss()
		})
	}
}

func TestToastRenderInactive(t *testing.T) {
	t.Parallel()
	toast := NewToast(time.Second)
	lines := toast.Render(40)
	if lines != nil {
		t.Fatalf("inactive toast should return nil, got %d lines", len(lines))
	}
}

func TestToastNewWithZeroDuration(t *testing.T) {
	t.Parallel()
	toast := NewToast(0)
	if toast.duration != 2*time.Second {
		t.Fatalf("zero duration should default to 2s, got %v", toast.duration)
	}
}

func TestToastHelper(t *testing.T) {
	t.Parallel()
	toast := NewToast(time.Second)
	helper := NewToastHelper(toast)
	helper.CopyOK()
	if !toast.IsActive() {
		t.Fatal("toast should be active after CopyOK")
	}
	if !strings.Contains(toast.message, "已复制") {
		t.Fatalf("expected copy message, got %q", toast.message)
	}
	toast.Dismiss()

	helper.ThemeSwitched("Dark")
	if !strings.Contains(toast.message, "Dark") {
		t.Fatalf("expected theme name in message, got %q", toast.message)
	}
	toast.Dismiss()

	helper.StatusMessage("processing...")
	if !strings.Contains(toast.message, "processing") {
		t.Fatalf("expected status message, got %q", toast.message)
	}
	toast.Dismiss()
}

func TestToastOnExpire(t *testing.T) {
	t.Parallel()
	expired := make(chan struct{}, 1)
	toast := NewToast(20 * time.Millisecond)
	toast.SetOnExpire(func() { expired <- struct{}{} })
	toast.Show("expire test", ToastInfo)
	select {
	case <-expired:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Fatal("onExpire callback was not called")
	}
}
