package component

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// DebugOverlay — a diagnostic panel triggered by ctrl+shift+d.
//
// Displays FPS, msgCh queue depth, ChatApp FSM state, heap allocation, and
// a scrollable log of recent event types. Auto-refreshes every 500ms via
// core.Tick. Escape closes the overlay.
//
// Typical usage:
//
//	d := component.NewDebugOverlay(tuiSource, appSource)
//	d.SetOnClose(func() { tuiApp.RemoveOverlay(ov) })
//	ov := tui.NewCenteredOverlay(d, 60, 60)
//	tuiApp.PushOverlay(ov)
// ---------------------------------------------------------------------------

// DebugTUISource is the interface the DebugOverlay needs from the TUI engine.
type DebugTUISource interface {
	MsgQueueDepth() int
	FrameStats() float64
	RecentEvents() []string
	DebugAlloc() uint64
	TotalMsgCount() uint64
	RenderDuration() time.Duration
	SlowFrameCount() uint64
}

// DebugAppSource is the interface the DebugOverlay needs from the ChatApp.
type DebugAppSource interface {
	State() interface{ String() string }
}

// debugTick is an internal Msg that triggers a data refresh.
type debugTick struct {
	core.MsgBase
}

// DebugOverlay renders a floating diagnostics panel.
type DebugOverlay struct {
	mu sync.RWMutex

	tuiSource DebugTUISource
	appSource DebugAppSource
	onClose   func()
	offset    int64

	// Cached data (refreshed on debugTick).
	fps        float64
	queueDepth int
	alloc      uint64
	msgCount   uint64
	fsmState   string
	events     []string
	renderDur  time.Duration
	slowFrames uint64

	// Scrollable event viewport.
	maxVisible int64
	started    bool
}

// NewDebugOverlay constructs a DebugOverlay bound to the given data sources.
func NewDebugOverlay(tuiSrc DebugTUISource, appSrc DebugAppSource) *DebugOverlay {
	return &DebugOverlay{
		tuiSource:  tuiSrc,
		appSource:  appSrc,
		maxVisible: 20,
	}
}

// SetOnClose registers a callback for when Escape is pressed.
func (d *DebugOverlay) SetOnClose(fn func()) {
	d.mu.Lock()
	d.onClose = fn
	d.mu.Unlock()
}

// SetMaxVisible clamps the number of event lines shown at once.
func (d *DebugOverlay) SetMaxVisible(n int64) {
	d.mu.Lock()
	d.maxVisible = n
	d.mu.Unlock()
}

// ScrollBy adjusts the event-log viewport offset.
func (d *DebugOverlay) ScrollBy(delta int64) {
	d.mu.Lock()
	d.offset += delta
	if d.offset < 0 {
		d.offset = 0
	}
	d.mu.Unlock()
}

// refreshDataLocked reads fresh data from the sources. Caller must hold d.mu.
func (d *DebugOverlay) refreshDataLocked() {
	if d.tuiSource != nil {
		d.fps = d.tuiSource.FrameStats()
		d.queueDepth = d.tuiSource.MsgQueueDepth()
		d.alloc = d.tuiSource.DebugAlloc()
		d.msgCount = d.tuiSource.TotalMsgCount()
		d.renderDur = d.tuiSource.RenderDuration()
		d.slowFrames = d.tuiSource.SlowFrameCount()
		d.events = d.tuiSource.RecentEvents()
	}
	if d.appSource != nil {
		s := d.appSource.State()
		d.fsmState = s.String()
	}
}

func (d *DebugOverlay) Update(msg core.Msg) core.Cmd {
	switch m := msg.(type) {
	case core.KeyMsg:
		data := m.Data
		switch {
		case terminal.MatchesKey(data, "escape") || data == "\x1b":
			d.mu.RLock()
			fn := d.onClose
			d.mu.RUnlock()
			if fn != nil {
				fn()
			}
		case terminal.MatchesKey(data, "up"):
			d.ScrollBy(-1)
		case terminal.MatchesKey(data, "down"):
			d.ScrollBy(1)
		case terminal.MatchesKey(data, "pageUp"):
			d.ScrollBy(-5)
		case terminal.MatchesKey(data, "pageDown"):
			d.ScrollBy(5)
		}
		return nil

	case debugTick:
		d.mu.Lock()
		d.refreshDataLocked()
		d.mu.Unlock()
		// Reschedule the next tick.
		return core.Tick(500*time.Millisecond, func(_ time.Time) core.Msg {
			return debugTick{}
		})

	case core.WindowSizeMsg:
		d.mu.Lock()
		d.mu.Unlock()
		return nil
	}

	// Kick off the refresh cycle on the very first Update.
	if !d.started {
		d.started = true
		return core.Tick(100*time.Millisecond, func(_ time.Time) core.Msg {
			return debugTick{}
		})
	}
	return nil
}

func (d *DebugOverlay) Render(width int64) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Render is always called on each frame; cache disabled.

	pal := theme.CurrentPalette()
	var out []string

	// ── Header ──────────────────────────────────────────────────────────
	title := "⏣ Debug Overlay"
	out = append(out, pal.User.Render(title), pal.Dim.Render(strings.Repeat("─", int(width))))

	// ── FPS + Queue + Messages ─────────────────────────────────────────
	fpsStr := fmt.Sprintf("%.1f", d.fps)
	fpsLabel := pal.Accent
	if d.fps < 30 {
		fpsLabel = pal.Error
	} else if d.fps < 55 {
		fpsLabel = pal.User
	}
	qLabel := pal.Success
	if d.queueDepth > 200 {
		qLabel = pal.Error
	} else if d.queueDepth > 100 {
		qLabel = pal.User
	}
	out = append(out, fmt.Sprintf(" %s %s", pal.Dim.Render("FPS:"), fpsLabel.Render(fpsStr)),
		fmt.Sprintf(" %s %s", pal.Dim.Render("Queue:"), qLabel.Render(fmt.Sprintf("%d", d.queueDepth))),
		fmt.Sprintf(" %s %s", pal.Dim.Render("Msgs:"), pal.User.Render(fmt.Sprintf("%d", d.msgCount))))

	// ── FSM State + Memory ──────────────────────────────────────────────────
	stateStr := d.fsmState
	if stateStr == "" {
		stateStr = "N/A"
	}
	out = append(out, fmt.Sprintf(" %s %s", pal.Dim.Render("State:"), pal.Accent.Render(stateStr)),
		fmt.Sprintf(" %s %s", pal.Dim.Render("Heap:"), pal.User.Render(formatBytes(d.alloc))),
		fmt.Sprintf(" %s %s", pal.Dim.Render("Frame:"), renderDurDisplay(d.renderDur, d.slowFrames)),
		pal.Dim.Render(strings.Repeat("─", int(width))))

	// ── Event Log ───────────────────────────────────────────────────────
	events := d.events
	totalEvents := len(events)

	if totalEvents > 0 {
		start := int(d.offset)
		if start >= totalEvents {
			start = totalEvents - 1
			if start < 0 {
				start = 0
			}
		}
		end := start + int(d.maxVisible)
		if end > totalEvents {
			end = totalEvents
		}
		displayEvents := events[start:end]

		out = append(out, fmt.Sprintf(" %s (%d/%d)",
			pal.Dim.Render("Events:"),
			end-start,
			totalEvents,
		))

		for _, ev := range displayEvents {
			out = append(out, fmt.Sprintf("  %s",
				pal.Dim.Render("▸")+" "+pal.User.Render(ev),
			))
		}
	} else {
		out = append(out, fmt.Sprintf(" %s %s",
			pal.Dim.Render("Events:"),
			pal.Muted.Render("(none)"),
		))
	}

	// ── Footer ──────────────────────────────────────────────────────────
	out = append(out, "")
	contentW := width - 4
	if contentW < 30 {
		contentW = 30
	}
	out = append(out, pal.Dim.Italic().Render(
		" "+core.TruncateToWidth("↑↓ scroll · Esc close · ctrl+shift+d toggle", contentW, "…"),
	))

	return out
}

// Invalidate forces a re-render on the next frame. Rendering is always
// full-frame (cache disabled), so this is a no-op.
func (d *DebugOverlay) Invalidate() {}

// formatBytes renders a byte count as a human-readable string.
func formatBytes(b uint64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// renderDurDisplay formats a frame render duration with a warning marker if
// the frame exceeded the 16ms (60fps) budget.
func renderDurDisplay(dur time.Duration, slowFrames uint64) string {
	label := dur.Round(100 * time.Microsecond).String()
	if dur > 16*time.Millisecond {
		return fmt.Sprintf("%s ⚠ %dx16ms+", label, slowFrames)
	}
	if dur > 8*time.Millisecond {
		return label + " ⚡"
	}
	return label
}
