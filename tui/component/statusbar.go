package component

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

type StatusBarSection struct {
	Text string
	Fn   func(string) string
}

type StatusBar struct {
	mu       sync.RWMutex
	sections []StatusBarSection
	elapsed  time.Duration
	running  bool
	start    time.Time
	mode     string
	agent    string

	// Token-usage metrics, surfaced next to the elapsed indicator when running.
	// tokPerSec is computed by the caller (ChatApp) from turn start/end times;
	// 0 means "not set / hide". prompt/completion are cumulative across turns.
	usagePrompt     int64
	usageCompletion int64
	tokPerSec       int64

	// Context-window occupancy: used / total tokens. total==0 means "hide".
	ctxUsed  int64
	ctxTotal int64
}

func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

func (s *StatusBar) SetMode(mode string) {
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
}

func (s *StatusBar) SetAgent(agent string) {
	s.mu.Lock()
	s.agent = agent
	s.mu.Unlock()
}

func (s *StatusBar) SetSections(sections []StatusBarSection) {
	s.mu.Lock()
	s.sections = sections
	s.mu.Unlock()
}

// SetUsage records token-usage metrics for display next to the elapsed
// indicator. tokPerSec==0 hides the rate. prompt/completion are shown
// cumulatively when non-zero.
func (s *StatusBar) SetUsage(prompt, completion, tokPerSec int64) {
	s.mu.Lock()
	s.usagePrompt = prompt
	s.usageCompletion = completion
	s.tokPerSec = tokPerSec
	s.mu.Unlock()
}

// SetContext records the context-window occupancy. total==0 hides the bar.
func (s *StatusBar) SetContext(used, total int64) {
	s.mu.Lock()
	s.ctxUsed = used
	s.ctxTotal = total
	s.mu.Unlock()
}

func (s *StatusBar) Busy() {
	s.mu.Lock()
	s.running = true
	s.start = time.Now()
	s.mu.Unlock()
}

func (s *StatusBar) Idle() {
	s.mu.Lock()
	if s.running {
		s.elapsed = time.Since(s.start)
	}
	s.running = false
	s.mu.Unlock()
}

func (s *StatusBar) Render(width int64) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p := theme.CurrentPalette()
	var left, right strings.Builder

	if s.mode != "" {
		left.WriteString(p.Accent.Render(" " + s.mode + " "))
		left.WriteString(" ")
	}

	if s.running {
		elapsed := time.Since(s.start)
		left.WriteString(p.LoaderSpinner.Render(theme.SymbolThinking + " " + formatDuration(elapsed)))
		// Streaming rate indicator, shown only while running and when a rate
		// has been observed (tokPerSec > 0). Kept compact so narrow terminals
		// are not crowded off the status bar.
		if s.tokPerSec > 0 {
			left.WriteString(" " + p.Accent.Render(fmt.Sprintf("⚡ %s", formatTokenRate(s.tokPerSec))))
		}
	} else if s.agent != "" {
		left.WriteString(p.Dim.Render(theme.SymbolCheck + " " + s.agent))
	}

	// Context-window occupancy bar, prepended to the right cluster. A 10-cell
	// inline bar colored by load (green < 70%, amber < 90%, red otherwise).
	if s.ctxTotal > 0 && s.ctxUsed >= 0 {
		right.WriteString(" " + renderContextBar(s.ctxUsed, s.ctxTotal, p))
	}

	for _, sec := range s.sections {
		text := sec.Text
		if sec.Fn != nil {
			text = sec.Fn(text)
		}
		right.WriteString(" ")
		right.WriteString(text)
	}

	leftStr := left.String()
	rightStr := right.String()

	leftW := core.VisibleWidth(leftStr)
	rightW := core.VisibleWidth(rightStr)
	gap := width - leftW - rightW
	if gap < 1 {
		gap = 1
	}

	line := leftStr + strings.Repeat(" ", int(gap)) + rightStr
	if core.VisibleWidth(line) > width {
		line = core.TruncateToWidth(line, width, "…")
	}
	line = core.PadToWidth(line, width)

	bg := p.BorderMuted
	line = bg.Render(line)

	return []string{line}
}

func (s *StatusBar) Invalidate() {}

func (s *StatusBar) Update(msg core.Msg) core.Cmd {
	if _, ok := msg.(core.WindowSizeMsg); ok {
		s.Invalidate()
	}
	return nil
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// formatTokenRate renders a tok/s value compactly: <1k as-is, >=1k as "1.2k".
func formatTokenRate(tokPerSec int64) string {
	if tokPerSec < 1000 {
		return fmt.Sprintf("%d tok/s", tokPerSec)
	}
	return fmt.Sprintf("%.1fk tok/s", float64(tokPerSec)/1000)
}

// renderContextBar returns a 10-cell inline progress bar showing the
// context-window occupancy, colored by load: green < 70%, amber < 90%,
// red otherwise. Example output at 45%: "█████░░░░░ 45%".
func renderContextBar(used, total int64, p *theme.Palette) string {
	const cells = 10
	if total <= 0 {
		return ""
	}
	pct := int((used * 100) / total)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := (pct * cells) / 100
	if filled > cells {
		filled = cells
	}
	var style func(string) string
	switch {
	case pct >= 90:
		style = p.Error.Render
	case pct >= 70:
		style = p.Accent.Render
	default:
		style = p.Success.Render
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", cells-filled)
	return style(bar) + " " + fmt.Sprintf("%d%%", pct)
}
