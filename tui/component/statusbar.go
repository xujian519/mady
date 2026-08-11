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

	// Phase 4: enhanced status fields
	caseName     string // current case/thread name
	pendingCount int    // pending review items, 0 = hide
	persisted    bool   // session save state indicator

	// Phase 5: turn + retry density (Reasonix-style).
	// turn counts completed agent turns (0 hides); retry shows buffered retry
	// state (retryMax > 0 = show).
	turn         int64
	retryAttempt int64
	retryMax     int64
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

// SetCaseInfo displays the current case/thread name on the status bar.
func (s *StatusBar) SetCaseInfo(name string) {
	s.mu.Lock()
	s.caseName = name
	s.mu.Unlock()
}

// SetPendingReview shows the count of items awaiting human review.
// count <= 0 hides the indicator.
func (s *StatusBar) SetPendingReview(count int) {
	s.mu.Lock()
	s.pendingCount = count
	s.mu.Unlock()
}

// SetPersisted shows/hides a save-state indicator.
func (s *StatusBar) SetPersisted(saved bool) {
	s.mu.Lock()
	s.persisted = saved
	s.mu.Unlock()
}

// SetTurn records the completed agent-turn counter (0 hides).
// Corresponds to Reasonix's turnIndex surfaced next to the mode chip.
func (s *StatusBar) SetTurn(n int64) {
	s.mu.Lock()
	s.turn = n
	s.mu.Unlock()
}

// SetRetry shows the buffered-retry chip "⏳ attempt/maxRetries" when maxRetries > 0.
// Matches Reasonix's "retrying (n/m)" running indicator.
func (s *StatusBar) SetRetry(attempt, maxRetries int64) {
	s.mu.Lock()
	s.retryAttempt = attempt
	s.retryMax = maxRetries
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
		// Cumulative prompt/completion tokens (Reasonix turn-usage receipt style).
		// Shown when running on medium-wide terminals; diagnostic, not primary.
		if (s.usagePrompt > 0 || s.usageCompletion > 0) && width >= 80 {
			left.WriteString(" " + p.Dim.Render(
				fmt.Sprintf("P%s C%s",
					formatCompactTokens(s.usagePrompt),
					formatCompactTokens(s.usageCompletion)),
			))
		}
		// Streaming rate indicator — compact, only on wide terminals.
		if s.tokPerSec > 0 && width >= 100 {
			left.WriteString(" " + p.Dim.Render(fmt.Sprintf("%s/s", formatTokenRate(s.tokPerSec))))
		}
	} else if s.agent != "" {
		left.WriteString(p.Dim.Render(theme.SymbolCheck + " " + s.agent))
	}

	// Right cluster: turn counter, retry chip, case name, pending count.
	// Turn counter: shown from any width that fits the case info cluster.
	if s.turn > 0 {
		right.WriteString(" " + p.Accent.Render(fmt.Sprintf("T%d", s.turn)))
	}
	// Retry chip: surfaced when the caller reports buffered retry state.
	if s.retryMax > 0 {
		right.WriteString(" " + p.Warning.Render(
			fmt.Sprintf("⏳%d/%d", clampRange(s.retryAttempt, 1, s.retryMax), s.retryMax),
		))
	}
	if width >= 80 {
		if s.caseName != "" {
			right.WriteString(" " + p.Dim.Render(s.caseName))
		}
		if s.pendingCount > 0 {
			right.WriteString(" " + p.Accent.Render(fmt.Sprintf("⚖%d", s.pendingCount)))
		}
	}
	// Context bar — only on wide terminals; it's diagnostic, not primary.
	if s.ctxTotal > 0 && s.ctxUsed >= 0 && width >= 100 {
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
	d = d % time.Minute
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

// formatCompactTokens renders an absolute token count compactly: <1k as "N",
// <1M as "1.2K", otherwise "3.4M". Mirrors Reasonix's turn-usage receipt.
func formatCompactTokens(n int64) string {
	switch {
	case n < 0:
		return "0"
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}

// clampRange returns v clamped to [lo, hi]. Used so the retry chip never
// renders "0/3" when attempt falls back to zero.
func clampRange(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
