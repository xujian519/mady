package component

import (
	"fmt"
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// Scrollbar configuration
// ---------------------------------------------------------------------------

// ScrollbarMode controls when the scrollbar is rendered.
type ScrollbarMode int

const (
	// ScrollbarAuto shows the scrollbar only when content exceeds the viewport.
	ScrollbarAuto ScrollbarMode = iota
	// ScrollbarAlways always reserves space and renders the scrollbar.
	ScrollbarAlways
	// ScrollbarNever disables the scrollbar entirely.
	ScrollbarNever
)

// ScrollbarConfig configures the visual scrollbar drawn alongside viewport content.
type ScrollbarConfig struct {
	// Mode controls visibility. Default: ScrollbarAuto.
	Mode ScrollbarMode
	// TrackSymbol is the character used for the scrollbar track.
	// Default: ' ' (space — the background color provides the track).
	TrackSymbol rune
	// ThumbSymbol is the character used for the scrollbar thumb.
	// Default: '▐' (RIGHT HALF BLOCK, U+2590).
	ThumbSymbol rune
	// Width is the number of columns reserved for the scrollbar (including gap).
	// Default: 1 (single column with no gap). Use 2 for a gap between content and scrollbar.
	Width int64
	// FollowDim when true dims the scrollbar when following the tail.
	// Default: true.
	FollowDim bool
}

// defaultScrollbarConfig returns sensible defaults.
func defaultScrollbarConfig() ScrollbarConfig {
	return ScrollbarConfig{
		Mode:        ScrollbarAuto,
		TrackSymbol: ' ',
		ThumbSymbol: '▐', // RIGHT HALF BLOCK
		Width:       1,
		FollowDim:   true,
	}
}

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

	// scrollbar configuration.
	sb ScrollbarConfig
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

// SetScrollbarConfig configures the visual scrollbar. Passing the zero value
// (ScrollbarConfig{}) disables the scrollbar.
func (v *Viewport) SetScrollbarConfig(cfg ScrollbarConfig) {
	v.mu.Lock()
	if cfg.Width < 1 {
		cfg.Width = 1
	}
	if cfg.ThumbSymbol == 0 {
		cfg.ThumbSymbol = '▐'
	}
	v.sb = cfg
	v.mu.Unlock()
}

// ScrollbarConfig returns the current scrollbar configuration.
func (v *Viewport) ScrollbarConfig() ScrollbarConfig {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.sb
}

// SetScrollbarEnabled is a convenience method to enable/disable the scrollbar
// with default settings.
func (v *Viewport) SetScrollbarEnabled(enabled bool) {
	if enabled {
		v.SetScrollbarConfig(defaultScrollbarConfig())
	} else {
		v.SetScrollbarConfig(ScrollbarConfig{Mode: ScrollbarNever})
	}
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

// Render returns the visible slice of content, optionally with a scrollbar
// drawn on the right edge.
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
	sb := v.sb // copy
	v.mu.RUnlock()

	total := int64(len(content))

	// Determine scrollbar reservation.
	var sbWidth int64
	if sb.Mode != ScrollbarNever && total > maxRows {
		sbWidth = sb.Width
	}
	contentWidth := width - sbWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	if maxRows <= 0 || total <= maxRows {
		visible := make([]string, len(content))
		copy(visible, content)
		return appendScrollbar(visible, width, contentWidth, sb, sbWidth, false, 0, 0, 0, 0)
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

	// Compute scrollbar thumb position.
	var thumbStart, thumbEnd int64
	if sbWidth > 0 && total > maxRows {
		thumbLen := maxRows * maxRows / total
		if thumbLen < 1 {
			thumbLen = 1
		}
		start := total - maxRows - offset
		if start < 0 {
			start = 0
		}
		thumbStart = start * (maxRows - thumbLen) / (total - maxRows)
		thumbEnd = thumbStart + thumbLen
		if thumbEnd > maxRows {
			thumbEnd = maxRows
		}
	}

	result := appendScrollbar(visible, width, contentWidth, sb, sbWidth, v.follow, total, maxRows, thumbStart, thumbEnd)
	return padLines(result, width)
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

// appendScrollbar appends a scrollbar column to each visible line when
// sbWidth > 0. The thumb is rendered using the palette's current theme:
// track = Dim, thumb = Muted (following) or Border (not following).
func appendScrollbar(visible []string, width, contentWidth int64, sb ScrollbarConfig, sbWidth int64, following bool, _, _ int64, thumbStart, thumbEnd int64) []string {
	if sbWidth <= 0 || len(visible) == 0 {
		// No scrollbar — just pad to the full width.
		out := make([]string, len(visible))
		for i, ln := range visible {
			if core.VisibleWidth(ln) < width {
				out[i] = core.PadToWidth(ln, width)
			} else {
				out[i] = ln
			}
		}
		return out
	}

	pal := theme.CurrentPalette()
	// Background-only scrollbar: track uses surface background, thumb uses
	// raised surface. No visible character — the color difference alone
	// indicates position. This is visually quieter than ▐ / ░ symbols.
	trackStyle := pal.SurfaceBg
	thumbStyle := pal.SurfaceRaisedBg
	if !following && sb.FollowDim {
		thumbStyle = pal.SurfaceBg
	}

	out := make([]string, len(visible))
	trackChar := " "
	thumbChar := " "

	for i := int64(0); i < int64(len(visible)); i++ {
		ln := visible[i]
		if core.VisibleWidth(ln) > contentWidth {
			ln = core.TruncateToWidth(ln, contentWidth, "…")
		} else {
			ln = core.PadToWidth(ln, contentWidth)
		}

		// Draw the scrollbar cell.
		if i >= thumbStart && i < thumbEnd {
			ln += thumbStyle.Render(thumbChar)
		} else {
			ln += trackStyle.Render(trackChar)
		}

		out[i] = ln
	}
	return out
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
