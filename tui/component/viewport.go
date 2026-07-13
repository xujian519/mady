package component

import (
	"fmt"
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// Viewport — a scrollable window into a vertical content buffer.
//
// Viewport is useful for any content that is taller than the available
// screen rows: logs, lists, help text, transcript panels, etc. It keeps
// track of a scroll offset and renders only the visible slice, optionally
// drawing a "more lines" indicator when the top of the content is scrolled
// off-screen.
//
// Scroll direction:
//   - ScrollBy(n) with n > 0 reveals earlier rows (scrolls up).
//   - ScrollBy(n) with n < 0 reveals later rows (scrolls down).
//   - FollowTail() jumps to the bottom and re-enables auto-follow; new
//     content set via SetContent keeps the tail visible again.
//
// The internal offset is the number of lines hidden above the visible
// window, measured from the tail. offset == 0 means the last maxRows rows
// are visible.
// ---------------------------------------------------------------------------

// Viewport renders a scrollable window into a []string content buffer.
type Viewport struct {
	mu sync.RWMutex

	content     []string
	offset      int64
	maxRows     int64
	follow      bool
	indicator   bool
	indicatorFn func(string) string
}

// NewViewport returns a viewport with the given visible height.
// Auto-follow is enabled by default so the tail is visible initially.
func NewViewport(maxRows int64) *Viewport {
	return &Viewport{
		maxRows: maxRows,
		follow:  true,
		indicatorFn: func(s string) string {
			return theme.CurrentPalette().Dim.Render(s)
		},
	}
}

// SetContent replaces the full content buffer. The caller is expected to
// call Invalidate on the parent container so the TUI re-renders.
func (v *Viewport) SetContent(content []string) {
	v.mu.Lock()
	v.content = content
	v.clampLocked()
	v.mu.Unlock()
}

// Content returns a snapshot of the current buffer.
func (v *Viewport) Content() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]string, len(v.content))
	copy(out, v.content)
	return out
}

// SetMaxRows changes the visible height and re-clamps the offset.
func (v *Viewport) SetMaxRows(n int64) {
	v.mu.Lock()
	v.maxRows = n
	v.clampLocked()
	v.mu.Unlock()
}

// MaxRows returns the configured visible height.
func (v *Viewport) MaxRows() int64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.maxRows
}

// SetIndicator enables or disables the "^ N more lines" row shown when the
// content is scrolled up.
func (v *Viewport) SetIndicator(enabled bool) {
	v.mu.Lock()
	v.indicator = enabled
	v.mu.Unlock()
}

// SetIndicatorFn installs a custom renderer for the indicator text. Pass
// nil to restore the default dim style.
func (v *Viewport) SetIndicatorFn(fn func(string) string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if fn == nil {
		v.indicatorFn = func(s string) string {
			return theme.CurrentPalette().Dim.Render(s)
		}
		return
	}
	v.indicatorFn = fn
}

// ScrollBy moves the viewport by n rows. Positive n reveals earlier rows
// (scrolls up); negative n reveals later rows (scrolls down). Scrolling up
// disables follow-tail.
func (v *Viewport) ScrollBy(n int64) {
	v.mu.Lock()
	v.offset += n
	if n != 0 {
		v.follow = false
	}
	v.clampLocked()
	v.mu.Unlock()
}

// ScrollTo sets the absolute offset from the bottom of the content. Passing 0
// shows the tail; larger values show rows further from the tail. The value is
// clamped so it never exceeds the available overflow.
func (v *Viewport) ScrollTo(offset int64) {
	v.mu.Lock()
	v.offset = offset
	v.follow = false
	v.clampLocked()
	v.mu.Unlock()
}

// Offset returns the current number of lines scrolled up from the tail.
func (v *Viewport) Offset() int64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.offset
}

// FollowTail jumps to the bottom of the content and re-enables auto-follow.
func (v *Viewport) FollowTail() {
	v.mu.Lock()
	v.offset = 0
	v.follow = true
	v.clampLocked()
	v.mu.Unlock()
}

// Following reports whether the viewport is currently following the tail.
func (v *Viewport) Following() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.follow
}

// Total returns the total number of rows in the content buffer.
func (v *Viewport) Total() int64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return int64(len(v.content))
}

// Render returns the visible slice of content, padded to the requested width.
func (v *Viewport) Render(width int64) []string {
	if width < 1 {
		width = 1
	}
	v.mu.RLock()
	content := v.content
	offset := v.offset
	maxRows := v.maxRows
	indicator := v.indicator
	indicatorFn := v.indicatorFn
	v.mu.RUnlock()

	total := int64(len(content))
	if maxRows <= 0 || total <= maxRows {
		return padLines(content, width)
	}

	// offset is lines scrolled up from the tail. 0 shows the last maxRows
	// rows; larger values show rows further above the tail.
	start := total - maxRows - offset
	if start < 0 {
		start = 0
	}
	end := start + maxRows
	if end > total {
		end = total
		start = end - maxRows
		if start < 0 {
			start = 0
		}
	}

	visible := make([]string, end-start)
	copy(visible, content[start:end])

	if indicator && offset > 0 {
		ind := indicatorFn(fmt.Sprintf("^ %d more lines", offset))
		if int64(len(visible)) >= maxRows && len(visible) > 0 {
			visible = visible[:len(visible)-1]
		}
		visible = append([]string{ind}, visible...)
	}

	return padLines(visible, width)
}

// Invalidate is a no-op for Viewport because it holds no derived cache.
func (v *Viewport) Invalidate() {}

// clampLocked keeps the offset within valid bounds. Caller must hold v.mu.
func (v *Viewport) clampLocked() {
	total := int64(len(v.content))
	if v.maxRows <= 0 || total <= v.maxRows {
		v.offset = 0
		v.follow = true
		return
	}
	if v.offset < 0 {
		v.offset = 0
		v.follow = true
		return
	}
	maxOffset := total - v.maxRows
	if v.offset > maxOffset {
		v.offset = maxOffset
	}
}

func padLines(lines []string, width int64) []string {
	out := make([]string, len(lines))
	for i, ln := range lines {
		if core.VisibleWidth(ln) < width {
			out[i] = core.PadToWidth(ln, width)
		} else {
			out[i] = ln
		}
	}
	return out
}
