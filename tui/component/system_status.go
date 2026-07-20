package component

// system_status.go — SystemStatus overlay component for system-mode display.
//
// The SystemStatus renders as a centered overlay showing:
//   - current operating mode (normal/degraded) and reason
//   - recent system events (max 3)
//   - impact summary on current task
//   - action bar: [l] detailed log, [Esc] close
//
// Keyboard navigation:
//   l — open detailed log (if onLogDetail set)
//   Esc — close overlay

import (
	"strings"
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// SysEvent describes one system event in the timeline.
type SysEvent struct {
	Time    string `json:"time"`    // e.g. "18:41"
	Message string `json:"message"` // event description
	Level   string `json:"level"`   // "info", "warn", "error"
}

// SystemStatusTheme customizes the system status panel appearance.
type SystemStatusTheme struct {
	Title    func(string) string
	Border   func(string) string
	Mode     func(string) string // normal mode label
	Degraded func(string) string // degraded/warning label
	Error    func(string) string // error-level event
	Info     func(string) string // info-level event
	Dim      func(string) string
	Body     func(string) string
	Accent   func(string) string
}

// DefaultSystemStatusTheme returns a theme built from the current palette.
func DefaultSystemStatusTheme() SystemStatusTheme {
	p := theme.CurrentPalette()
	return SystemStatusTheme{
		Title:    p.Accent.Bold().Render,
		Border:   p.Border.Render,
		Mode:     p.Success.Render,
		Degraded: p.Accent.Render,
		Error:    p.Error.Render,
		Info:     p.Assistant.Render,
		Dim:      p.Dim.Render,
		Body:     p.Assistant.Render,
		Accent:   p.Accent.Render,
	}
}

// ---------------------------------------------------------------------------
// SystemStatus component
// ---------------------------------------------------------------------------

// SystemStatus is a Focusable Component that renders system operating
// conditions — mode, recent events, and current impact.
type SystemStatus struct {
	mu sync.RWMutex

	// Data
	mode       string     // "normal" / "degraded"
	modeReason string     // human-readable reason for degraded mode
	events     []SysEvent // recent events (max 3 displayed)
	impacts    []string   // impact statements

	// Callbacks
	onLogDetail func()
	onClose     func()

	km *terminal.KeybindingsManager

	// Focus state
	focused bool

	// Caching
	cacheWidth int64
	cacheLines []string
	dirty      bool

	theme SystemStatusTheme
}

// NewSystemStatus creates a SystemStatus with default theme.
func NewSystemStatus() *SystemStatus {
	return &SystemStatus{
		theme: DefaultSystemStatusTheme(),
		dirty: true,
	}
}

// ---------------------------------------------------------------------------
// Setters
// ---------------------------------------------------------------------------

// SetMode sets the operating mode and (optionally) the reason.
func (s *SystemStatus) SetMode(mode string, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode != mode || s.modeReason != reason {
		s.mode = mode
		s.modeReason = reason
		s.dirty = true
	}
}

// SetEvents sets the recent system events list.
func (s *SystemStatus) SetEvents(events []SysEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = events
	s.dirty = true
}

// SetImpacts sets the impact summary items.
func (s *SystemStatus) SetImpacts(impacts []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.impacts = impacts
	s.dirty = true
}

// SetOnLogDetail sets the callback for the [l] detailed-log action.
func (s *SystemStatus) SetOnLogDetail(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLogDetail = fn
}

// SetOnClose sets the callback for closing the overlay.
func (s *SystemStatus) SetOnClose(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onClose = fn
}

// SetKeybindings wires a keybindings manager for runtime key resolution.
func (s *SystemStatus) SetKeybindings(km *terminal.KeybindingsManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.km = km
}

// ---------------------------------------------------------------------------
// core.Focusable
// ---------------------------------------------------------------------------

// SetFocused marks the component as focused or unfocused.
func (s *SystemStatus) SetFocused(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.focused = v
	s.dirty = true
}

// Focused reports whether the component currently has focus.
func (s *SystemStatus) Focused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.focused
}

// ---------------------------------------------------------------------------
// core.Component
// ---------------------------------------------------------------------------

// Render produces the rendered lines at the given width.
func (s *SystemStatus) Render(width int64) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.dirty && s.cacheWidth == width && s.cacheLines != nil {
		return s.cacheLines
	}

	t := s.theme
	pal := theme.CurrentPalette()

	var out []string

	// ── Title bar ──
	titleLine := t.Title("  系统态  ")
	borderLine := t.Border(strings.Repeat("═", int(width)))
	out = append(out, borderLine, core.PadToWidth(titleLine, width), borderLine, "")

	// ── Current operating mode ──
	modeHeader := t.Dim("当前运行")
	out = append(out, core.PadToWidth(modeHeader, width))

	modeLabel := s.mode
	if modeLabel == "" {
		modeLabel = "normal"
	}
	modeStyle := t.Mode
	if modeLabel == "degraded" {
		modeStyle = t.Degraded
	}
	out = append(out, core.PadToWidth(" - 模式: "+modeStyle(modeLabel), width))

	if s.modeReason != "" {
		out = append(out, core.PadToWidth(" - 原因: "+t.Body(s.modeReason), width))
	}

	// ── Recent events ──
	if len(s.events) > 0 {
		out = append(out, "")
		evHeader := t.Dim("最近事件")
		out = append(out, core.PadToWidth(evHeader, width))

		display := s.events
		if len(display) > 3 {
			display = display[:3]
		}
		for _, ev := range display {
			var line string
			timeStr := ev.Time
			if timeStr == "" {
				timeStr = "--:--"
			}
			switch ev.Level {
			case "error":
				line = " - " + t.Dim(timeStr) + " " + t.Error(ev.Message)
			case "warn":
				line = " - " + t.Dim(timeStr) + " " + t.Degraded(ev.Message)
			default:
				line = " - " + t.Dim(timeStr) + " " + t.Info(ev.Message)
			}
			out = append(out, core.PadToWidth(line, width))
		}
	}

	// ── Current impact ──
	if len(s.impacts) > 0 {
		out = append(out, "")
		impactHeader := t.Dim("当前影响")
		out = append(out, core.PadToWidth(impactHeader, width))

		for _, imp := range s.impacts {
			out = append(out, core.PadToWidth(" - "+t.Body(imp), width))
		}
	}

	// ── Action bar ──
	out = append(out, "", pal.BorderMuted.Render(strings.Repeat("─", int(width))))
	actionParts := []string{}
	if s.onLogDetail != nil {
		actionParts = append(actionParts, "[l] 详细日志")
	}
	actionParts = append(actionParts, "[Esc] 返回")
	out = append(out, core.PadToWidth(t.Dim(strings.Join(actionParts, "   ")), width))

	// Cache and return.
	s.cacheLines = out
	s.cacheWidth = width
	s.dirty = false
	return out
}

// Invalidate marks the render cache dirty.
func (s *SystemStatus) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = true
}

// Update processes keyboard events.
func (s *SystemStatus) Update(msg core.Msg) core.Cmd {
	keyMsg, ok := msg.(core.KeyMsg)
	if !ok {
		return nil
	}
	data := keyMsg.Data

	// Check Esc first (always works).
	if s.matches(data, "escape") {
		s.mu.Lock()
		onClose := s.onClose
		s.mu.Unlock()
		if onClose != nil {
			onClose()
		}
		return nil
	}

	// Check l → detailed log.
	if s.matches(data, "l") {
		s.mu.Lock()
		onLogDetail := s.onLogDetail
		s.mu.Unlock()
		if onLogDetail != nil {
			onLogDetail()
		}
		return nil
	}

	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// matches checks key data against a binding ID, with a km → direct fallback.
func (s *SystemStatus) matches(data string, id string) bool {
	if s.km != nil {
		return terminal.MatchesKey(data, id)
	}
	// Test/fallback path — direct key matching for common keys.
	switch id {
	case "escape":
		return data == "\x1b"
	case "l":
		return data == "l"
	default:
		return false
	}
}
