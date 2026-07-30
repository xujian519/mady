package component

// toast.go — Toast notification component.
//
// Displays a brief, auto-dismissing notification message at the top of the
// chat area. Used for transient feedback: "✓ 已复制", "主题已切换", etc.
//
// The Toast is rendered as a single colored line and automatically removed
// after a configurable duration. It is not an overlay — it embeds directly
// into the layout and requests removal when the timer expires.
//
// 输出示例：
//
//	✓ 已复制                                    (Success 色)
//	⚙ 压缩中... 45%                             (Accent 色)
//	◷ 发送中                                    (Dim 色)

import (
	"fmt"
	"sync"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// ToastLevel controls the visual style of a toast notification.
type ToastLevel int

const (
	ToastInfo    ToastLevel = iota // Dim style, informational
	ToastSuccess                   // Success style, green
	ToastWarning                   // Warning style, amber
	ToastError                     // Error style, red
)

// Toast is a transient notification bar that auto-dismisses.
type Toast struct {
	mu        sync.RWMutex
	message   string
	level     ToastLevel
	duration  time.Duration
	startedAt time.Time
	active    bool
	seq       uint64 // incremented on each Show(); goroutines check to avoid stale expiry
	onExpire  func()
	onRender  func()
}

// NewToast creates a Toast that auto-dismisses after the given duration.
func NewToast(duration time.Duration) *Toast {
	if duration <= 0 {
		duration = 2 * time.Second
	}
	return &Toast{
		duration: duration,
	}
}

// Show displays a message with the given level and starts the auto-dismiss timer.
// Each call increments an internal sequence counter; only the most recent
// Show's expiry takes effect — stale goroutines from earlier calls are no-ops.
func (t *Toast) Show(msg string, level ToastLevel) {
	t.mu.Lock()
	t.message = msg
	t.level = level
	t.active = true
	t.startedAt = time.Now()
	t.seq++ // invalidate any previous goroutine's expiry
	mySeq := t.seq
	t.mu.Unlock()
	t.requestRender()

	// Schedule auto-dismiss. Checks seq to avoid stale goroutines from
	// rapid successive Show() calls hiding the most recent toast early.
	go func() {
		time.Sleep(t.duration)
		t.mu.Lock()
		if t.active && t.seq == mySeq {
			t.active = false
			fn := t.onExpire
			t.mu.Unlock()
			t.requestRender()
			if fn != nil {
				fn()
			}
		} else {
			t.mu.Unlock()
		}
	}()
}

// Dismiss hides the toast immediately.
func (t *Toast) Dismiss() {
	t.mu.Lock()
	t.active = false
	t.mu.Unlock()
	t.requestRender()
}

// IsActive reports whether the toast is currently displayed.
func (t *Toast) IsActive() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.active
}

// SetOnExpire registers a callback fired when the toast auto-dismisses.
func (t *Toast) SetOnExpire(fn func()) {
	t.mu.Lock()
	t.onExpire = fn
	t.mu.Unlock()
}

// SetOnRequestRender wires a render request callback.
func (t *Toast) SetOnRequestRender(fn func()) {
	t.mu.Lock()
	t.onRender = fn
	t.mu.Unlock()
}

func (t *Toast) requestRender() {
	t.mu.RLock()
	fn := t.onRender
	t.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (t *Toast) Invalidate() {}

func (t *Toast) Render(width int64) []string {
	t.mu.RLock()
	msg := t.message
	lvl := t.level
	active := t.active
	t.mu.RUnlock()

	if !active || msg == "" {
		return nil // zero lines when hidden
	}

	pal := theme.CurrentPalette()
	var styleFn func(string) string
	switch lvl {
	case ToastSuccess:
		styleFn = pal.Success.Render
	case ToastWarning:
		styleFn = pal.Warning.Render
	case ToastError:
		styleFn = pal.Error.Render
	default:
		styleFn = pal.Dim.Render
	}

	line := styleFn(" " + msg + " ")
	return []string{core.PadToWidth(core.TruncateToWidth(line, width, "…"), width)}
}

func (t *Toast) Update(msg core.Msg) core.Cmd { return nil }

// Ensure Toast implements core.Component.
var _ core.Component = (*Toast)(nil)

// ToastHelper provides convenience methods for common toast messages.
type ToastHelper struct {
	toast *Toast
}

// NewToastHelper wraps a Toast with convenience methods.
func NewToastHelper(t *Toast) *ToastHelper {
	return &ToastHelper{toast: t}
}

// CopyOK shows "✓ 已复制" success toast.
func (h *ToastHelper) CopyOK() {
	h.toast.Show("✓ 已复制", ToastSuccess)
}

// ThemeSwitched shows the theme toggle notification.
func (h *ToastHelper) ThemeSwitched(name string) {
	h.toast.Show(fmt.Sprintf("◈ 主题: %s", name), ToastInfo)
}

// StatusMessage shows a brief informational status.
func (h *ToastHelper) StatusMessage(msg string) {
	h.toast.Show(msg, ToastInfo)
}
