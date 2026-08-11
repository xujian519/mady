package tui

// This file manages the focus stack (which component receives input) and the
// overlay stack (floating panels mounted above the root view). Both stacks
// are guarded by t.mu; mutations trigger a re-render.

import (
	"time"

	core "github.com/xujian519/mady/tui/core"
)

// Focus pushes c onto the focus stack and makes it the active input target.
func (t *TUI) Focus(c core.Component) {
	if c == nil {
		return
	}
	t.mu.Lock()
	for i, f := range t.focus {
		if f == c {
			t.focus = append(t.focus[:i], t.focus[i+1:]...)
			break
		}
	}
	t.focus = append(t.focus, c)
	if fc, ok := c.(core.Focusable); ok {
		fc.SetFocused(true)
	}
	for i := 0; i < len(t.focus)-1; i++ {
		if fc, ok := t.focus[i].(core.Focusable); ok {
			fc.SetFocused(false)
		}
	}
	t.mu.Unlock()
	t.RequestRender()
}

// Unfocus pops c from the focus stack (if present) and returns focus to the
// previous target.
func (t *TUI) Unfocus(c core.Component) {
	t.mu.Lock()
	for i, f := range t.focus {
		if f == c {
			t.focus = append(t.focus[:i], t.focus[i+1:]...)
			if fc, ok := c.(core.Focusable); ok {
				fc.SetFocused(false)
			}
			break
		}
	}
	if len(t.focus) > 0 {
		if fc, ok := t.focus[len(t.focus)-1].(core.Focusable); ok {
			fc.SetFocused(true)
		}
	}
	t.mu.Unlock()
	t.RequestRender()
}

// Focused returns the current top of the focus stack (may be nil).
func (t *TUI) Focused() core.Component {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.focus) == 0 {
		return nil
	}
	return t.focus[len(t.focus)-1]
}

// ---------------------------------------------------------------------------
// Overlay helpers (public API — the Overlay type itself lives in overlay.go).
// ---------------------------------------------------------------------------

// PushOverlay mounts an overlay on top of the root view.
func (t *TUI) PushOverlay(o *Overlay) {
	if o == nil {
		return
	}
	t.mu.Lock()
	t.overlays = append(t.overlays, o)
	if o.Transition.Kind != OverlayTransitionNone {
		// 动画起始时刻在锁内写入；动画 goroutine 由 startOverlayAnimation
		// 创建（goroutine 创建 happens-after 本写入），渲染线程经
		// RequestRender 的原子标记同步，均无数据竞争。
		o.openAt = time.Now()
	}
	t.mu.Unlock()
	if o.Focus {
		t.Focus(o.Content)
	}
	t.startOverlayAnimation(o)
	t.RequestRender()
}

// animationFrameInterval 是 overlay 打开动画的帧间隔（≈60fps）。
const animationFrameInterval = 16 * time.Millisecond

// startOverlayAnimation 驱动 overlay 打开动画：每帧请求一次渲染，直到动画
// 结束。使用链式 Tick 自终止——动画完成后不残留周期 goroutine。
func (t *TUI) startOverlayAnimation(o *Overlay) {
	if o == nil || o.Transition.Kind == OverlayTransitionNone {
		return
	}
	var step func(now time.Time) core.Msg
	step = func(now time.Time) core.Msg {
		_, done := o.transitionProgress(now)
		if !done {
			t.Tick(animationFrameInterval, step)
		}
		t.RequestRender()
		return nil
	}
	t.Tick(animationFrameInterval, step)
}

// PopOverlay removes the top overlay; returns it or nil if the stack is empty.
func (t *TUI) PopOverlay() *Overlay {
	t.mu.Lock()
	if len(t.overlays) == 0 {
		t.mu.Unlock()
		return nil
	}
	top := t.overlays[len(t.overlays)-1]
	t.overlays = t.overlays[:len(t.overlays)-1]
	t.mu.Unlock()
	if top.Focus {
		t.Unfocus(top.Content)
	}
	t.RequestRender()
	return top
}

// RemoveOverlay pops the given overlay (no-op if not on the stack).
func (t *TUI) RemoveOverlay(o *Overlay) bool {
	if o == nil {
		return false
	}
	t.mu.Lock()
	for i, cur := range t.overlays {
		if cur == o {
			t.overlays = append(t.overlays[:i], t.overlays[i+1:]...)
			t.mu.Unlock()
			if o.Focus {
				t.Unfocus(o.Content)
			}
			t.RequestRender()
			return true
		}
	}
	t.mu.Unlock()
	return false
}

// Overlays returns a snapshot of the current overlay stack.
func (t *TUI) Overlays() []*Overlay {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make([]*Overlay, len(t.overlays))
	copy(cp, t.overlays)
	return cp
}
