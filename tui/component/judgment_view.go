package component

// JudgmentView displays the "current judgment" area at the top of the chat
// layout. It shows the phase, status, judgment summary, confidence, pending
// items, context summary, and action hints — giving the user a one-glance
// overview of where the current task stands.
//
// The view has three rendering modes:
//   - Collapsed (idle / streaming): a single status bar line with mode tag.
//   - Normal (done / ready): status line + judgment summary line.
//   - Expanded (awaiting_review / blocked): full view with judgment,
//     confidence bar, pending items, context items, and action bar.
//
// It implements core.Component and uses a dirty-flag render cache.

import (
	"fmt"
	"strings"
	"sync"

	"github.com/xujian519/mady/tui/theme"
)

// JudgmentAction describes one keyboard-action hint shown at the bottom of
// the expanded judgment view, such as "[r] 复核" or "[e] 证据".
type JudgmentAction struct {
	Key   string // single-key shortcut, e.g. "r", "e", "s"
	Label string // short label, e.g. "复核", "证据", "系统"
}

// JudgmentView renders the current-judgment area.
type JudgmentView struct {
	mu sync.RWMutex

	phase      string   // current phase, e.g. "分析阶段", "复核阶段"
	status     string   // machine status: idle, running, streaming, awaiting_review, done, failed, degraded, blocked
	judgment   string   // one-line judgment summary
	confidence int      // 0-100, or -1 to hide
	pending    []string // pending items (max 3)
	context    []string // context summary items (max 4)
	mode       string   // "normal" (empty) or "degraded"
	actions    []JudgmentAction

	// rendering cache
	dirty  bool
	cache  []string
	cacheW int64

	// labelOverride, when set, overrides the auto-derived status label.
	labelOverride string
}

// NewJudgmentView creates a JudgmentView with default "normal" mode and
// "idle" status. Call SetJudgment / SetPhase / SetStatus / etc. before use.
func NewJudgmentView() *JudgmentView {
	return &JudgmentView{
		status: "idle",
		dirty:  true,
	}
}

// ---------------------------------------------------------------------------
// Setters — each marks the cache dirty.
// ---------------------------------------------------------------------------

// SetPhase sets the task phase label (e.g. "分析阶段", "复核阶段").
func (v *JudgmentView) SetPhase(phase string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.phase != phase {
		v.phase = phase
		v.dirty = true
	}
}

// SetStatus sets the machine status. This controls the rendering mode:
//   - "idle" / "streaming" → collapsed (single line)
//   - "done" / "ready"     → normal (status + judgment line)
//   - "awaiting_review" / "blocked" → expanded (full view)
//   - "degraded"           → adds mode tag
func (v *JudgmentView) SetStatus(status string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.status != status {
		v.status = status
		v.dirty = true
	}
}

// SetJudgment sets the one-line judgment summary text.
func (v *JudgmentView) SetJudgment(judgment string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.judgment != judgment {
		v.judgment = judgment
		v.dirty = true
	}
}

// SetConfidence sets the confidence value (0-100). Pass -1 to hide the
// confidence bar entirely.
func (v *JudgmentView) SetConfidence(confidence int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.confidence != confidence {
		v.confidence = confidence
		v.dirty = true
	}
}

// SetPending sets the pending-items list. Only the first 3 items are shown.
func (v *JudgmentView) SetPending(pending []string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.pending = pending
	v.dirty = true
}

// SetContext sets the context-summary items. Only the first 4 items are shown.
func (v *JudgmentView) SetContext(context []string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.context = context
	v.dirty = true
}

// SetMode sets the mode tag. Pass "" for normal, "degraded" for degraded mode.
func (v *JudgmentView) SetMode(mode string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.mode != mode {
		v.mode = mode
		v.dirty = true
	}
}

// SetStatusLabel overrides the auto-derived status label. Pass "" to restore
// automatic derivation from the status value (e.g. "running" → "运行中").
func (v *JudgmentView) SetStatusLabel(label string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.labelOverride != label {
		v.labelOverride = label
		v.dirty = true
	}
}

// SetActions sets the action hints shown at the bottom of the expanded view.
func (v *JudgmentView) SetActions(actions []JudgmentAction) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.actions = actions
	v.dirty = true
}

// ---------------------------------------------------------------------------
// Accessors
// ---------------------------------------------------------------------------

// Status returns the current machine status.
func (v *JudgmentView) Status() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.status
}

// Phase returns the current phase label.
func (v *JudgmentView) Phase() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.phase
}

// Mode returns the current operating mode ("normal" / "degraded").
func (v *JudgmentView) Mode() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.mode
}

// IsExpanded reports whether the view is in expanded rendering mode
// (awaiting_review or blocked).
func (v *JudgmentView) IsExpanded() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.status == "awaiting_review" || v.status == "blocked"
}

// ---------------------------------------------------------------------------
// core.Component
// ---------------------------------------------------------------------------

// Render produces the rendered lines at the given width.
func (v *JudgmentView) Render(width int64) []string {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.dirty && v.cacheW == width && v.cache != nil {
		return v.cache
	}

	p := theme.CurrentPalette()
	var lines []string

	// --- Status bar line ---
	var statusLine strings.Builder
	if v.phase != "" {
		statusLine.WriteString(p.Accent.Render(v.phase))
		statusLine.WriteString(" · ")
	}
	statusLine.WriteString(v.statusLabel())
	if v.mode == "degraded" {
		statusLine.WriteString(" · ")
		statusLine.WriteString(p.Accent.Render("degraded"))
	}
	lines = append(lines,
		p.BorderMuted.Render(strings.Repeat("─", int(width))),
		statusLine.String(),
		p.BorderMuted.Render(strings.Repeat("─", int(width))),
	)

	// --- Judgment section (expanded or normal only) ---
	if v.isExpanded() || v.isNormal() {
		if v.judgment != "" {
			lines = append(lines, v.judgment)
		}

		// Confidence bar (expanded only)
		if v.isExpanded() && v.confidence >= 0 {
			barLen := v.confidence / 10
			if barLen > 10 {
				barLen = 10
			}
			if barLen < 0 {
				barLen = 0
			}
			filled := strings.Repeat("█", barLen)
			empty := strings.Repeat("░", 10-barLen)
			confLabel := confidenceLabel(v.confidence)
			confStyle := confidenceStyle(v.confidence, p)
			lines = append(lines, fmt.Sprintf("置信度: %s%s %s",
				confStyle.Render(filled),
				p.Dim.Render(empty),
				confStyle.Render(confLabel),
			))
		}

		// Pending items (expanded only, non-empty)
		if v.isExpanded() && len(v.pending) > 0 {
			items := v.pending
			if len(items) > 3 {
				items = items[:3]
			}
			lines = append(lines, p.Dim.Render("仍待确认:")+" "+strings.Join(items, "、"))
		}
	}

	// --- Context section (expanded only) ---
	if v.isExpanded() && len(v.context) > 0 {
		items := v.context
		if len(items) > 4 {
			items = items[:4]
		}
		lines = append(lines,
			p.BorderMuted.Render(strings.Repeat("─", int(width))),
			p.Dim.Render("当前上下文"),
		)
		for _, item := range items {
			lines = append(lines, "  · "+item)
		}
	}

	// --- Action bar (expanded only) ---
	if v.isExpanded() && len(v.actions) > 0 {
		lines = append(lines, p.BorderMuted.Render(strings.Repeat("─", int(width))))
		var actionParts []string
		for _, a := range v.actions {
			actionParts = append(actionParts, fmt.Sprintf("[%s] %s", a.Key, a.Label))
		}
		lines = append(lines, p.Dim.Render(strings.Join(actionParts, "   ")))
	}

	// Cache and return.
	v.cache = lines
	v.cacheW = width
	v.dirty = false
	return lines
}

// Invalidate marks the render cache dirty.
func (v *JudgmentView) Invalidate() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.dirty = true
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// statusLabel returns the human-readable label for the current status.
func (v *JudgmentView) statusLabel() string {
	if v.labelOverride != "" {
		return v.labelOverride
	}
	return statusLabelFromStatus(v.status)
}

// statusLabelFromStatus maps machine status to a Chinese label.
func statusLabelFromStatus(s string) string {
	switch s {
	case "idle":
		return "空闲"
	case "running":
		return "运行中"
	case "streaming":
		return "输出中"
	case "awaiting_review":
		return "等待复核"
	case "blocked":
		return "已阻塞"
	case "done":
		return "已完成"
	case "failed":
		return "失败"
	case "degraded":
		return "降级运行"
	default:
		return s
	}
}

// isExpanded returns true when the view should render in full expanded mode.
func (v *JudgmentView) isExpanded() bool {
	return v.status == "awaiting_review" || v.status == "blocked"
}

// isNormal returns true when the view should render at normal (compact) size.
func (v *JudgmentView) isNormal() bool {
	return v.status == "done" || v.status == "ready" || v.status == "degraded"
}

// confidenceLabel maps a 0-100 confidence value to a label string.
func confidenceLabel(c int) string {
	switch {
	case c < 0:
		return ""
	case c <= 30:
		return "低"
	case c <= 60:
		return "中"
	default:
		return "高"
	}
}

// confidenceStyle returns the palette style for a given confidence level.
func confidenceStyle(c int, p *theme.Palette) theme.Style {
	switch {
	case c < 0:
		return p.Dim
	case c <= 30:
		return p.ConfidenceLow
	case c <= 60:
		return p.ConfidenceMedium
	default:
		return p.ConfidenceHigh
	}
}

// IsEmpty reports whether the view has no meaningful data to display.
// Used by the layout to decide whether to include this component.
func (v *JudgmentView) IsEmpty() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.judgment == "" && v.status == "idle" && len(v.pending) == 0 && len(v.context) == 0
}

// Height returns the rendered height. Useful for layout calculations without
// producing full render output.
func (v *JudgmentView) Height(width int64) int64 {
	return int64(len(v.Render(width)))
}
