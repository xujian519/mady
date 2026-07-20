package component

// review_gate.go — ReviewGate overlay component for structured review.
//
// The ReviewGate renders as a full-panel overlay with:
//   - current judgment and confidence
//   - evidence list (expandable/collapsible)
//   - review checklist (toggleable items)
//   - risk warnings
//   - three action exits: pass, back (supplement evidence), block
//
// Keyboard navigation:
//   j/k — move focus between sections
//   Space — toggle evidence expand / checklist check
//   p — pass review
//   b — go back to supplement evidence
//   f — mark as blocked
//   Esc — close

import (
	"fmt"
	"strings"
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// EvidenceStatus describes the verification status of an evidence item.
type EvidenceStatus int

const (
	EvidencePending   EvidenceStatus = iota // 待核验
	EvidenceConfirmed                       // 已确认
	EvidenceDisputed                        // 有争议
)

// EvidenceStatusLabel returns a human-readable label for the status.
func EvidenceStatusLabel(s EvidenceStatus) string {
	switch s {
	case EvidenceConfirmed:
		return "confirmed"
	case EvidenceDisputed:
		return "disputed"
	default:
		return "pending"
	}
}

// ReviewEvidence represents a single evidence item in the review panel.
type ReviewEvidence struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Role    string         `json:"role"`    // "核心证据" / "辅助证据" / "对照材料"
	Summary string         `json:"summary"` // one-line summary
	Status  EvidenceStatus `json:"status"`
}

// ReviewCheckItem is a single item in the review checklist.
type ReviewCheckItem struct {
	Label   string `json:"label"`
	Checked bool   `json:"checked"`
}

// ReviewGateTheme customizes the review gate panel appearance.
type ReviewGateTheme struct {
	Title     func(string) string
	Border    func(string) string
	Success   func(string) string
	Warning   func(string) string
	Danger    func(string) string
	Dim       func(string) string
	Checked   func(string) string
	Unchecked func(string) string
	Body      func(string) string
}

// DefaultReviewGateTheme returns a theme built from the current palette.
func DefaultReviewGateTheme() ReviewGateTheme {
	p := theme.CurrentPalette()
	return ReviewGateTheme{
		Title:     p.Accent.Bold().Render,
		Border:    p.Border.Render,
		Success:   p.Success.Render,
		Warning:   p.Accent.Render,
		Danger:    p.Error.Render,
		Dim:       p.Dim.Render,
		Checked:   p.Success.Render,
		Unchecked: p.Dim.Render,
		Body:      p.Assistant.Render,
	}
}

// focusSection identifies which section of the review gate has keyboard focus.
type focusSection int

const (
	focusEvidences focusSection = iota
	focusChecklist
	focusActions
)

// ---------------------------------------------------------------------------
// ReviewGate component
// ---------------------------------------------------------------------------

// ReviewGate is a Focusable Component that renders a structured review panel.
// It is designed to be used as the content of a centered overlay.
type ReviewGate struct {
	mu sync.RWMutex

	// Data
	title      string
	judgment   string
	confidence float64 // 0.0–1.0
	evidences  []ReviewEvidence
	checklist  []ReviewCheckItem
	risks      []string

	// UI state
	focused      bool
	focusSec     focusSection    // which section has keyboard focus
	evidenceIdx  int             // selected evidence index
	checklistIdx int             // selected checklist index
	expanded     map[string]bool // evidence ID → expanded

	// Callbacks
	onPass  func()
	onBack  func()
	onBlock func()
	onClose func()

	km *terminal.KeybindingsManager

	// Caching
	cacheWidth int64
	cacheLines []string
	dirty      bool
}

// NewReviewGate creates a ReviewGate with the given data.
func NewReviewGate(judgment string, confidence float64, evidences []ReviewEvidence, checklist []ReviewCheckItem, risks []string) *ReviewGate {
	return &ReviewGate{
		title:      "复核门",
		judgment:   judgment,
		confidence: confidence,
		evidences:  evidences,
		checklist:  checklist,
		risks:      risks,
		expanded:   make(map[string]bool),
		dirty:      true,
	}
}

// SetTitle overrides the panel title (default "复核门").
func (g *ReviewGate) SetTitle(t string) {
	g.mu.Lock()
	g.title = t
	g.dirty = true
	g.mu.Unlock()
}

// SetOnPass registers the callback for the "pass review" action.
func (g *ReviewGate) SetOnPass(fn func()) {
	g.mu.Lock()
	g.onPass = fn
	g.mu.Unlock()
}

// SetOnBack registers the callback for the "back to supplement evidence" action.
func (g *ReviewGate) SetOnBack(fn func()) {
	g.mu.Lock()
	g.onBack = fn
	g.mu.Unlock()
}

// SetOnBlock registers the callback for the "mark as blocked" action.
func (g *ReviewGate) SetOnBlock(fn func()) {
	g.mu.Lock()
	g.onBlock = fn
	g.mu.Unlock()
}

// SetOnClose registers the callback for closing (Esc).
func (g *ReviewGate) SetOnClose(fn func()) {
	g.mu.Lock()
	g.onClose = fn
	g.mu.Unlock()
}

// SetKeybindings sets the keybinding manager for input matching.
func (g *ReviewGate) SetKeybindings(km *terminal.KeybindingsManager) {
	g.mu.Lock()
	g.km = km
	g.mu.Unlock()
}

// Focusable interface

func (g *ReviewGate) SetFocused(v bool) {
	g.mu.Lock()
	g.focused = v
	g.mu.Unlock()
}

func (g *ReviewGate) Focused() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.focused
}

// Component interface

func (g *ReviewGate) Render(width int64) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.dirty && g.cacheWidth == width && g.cacheLines != nil {
		return g.cacheLines
	}
	g.cacheLines = g.renderLocked(width)
	g.cacheWidth = width
	g.dirty = false
	return g.cacheLines
}

func (g *ReviewGate) Invalidate() {
	g.mu.Lock()
	g.dirty = true
	g.mu.Unlock()
}

func (g *ReviewGate) Update(msg core.Msg) core.Cmd {
	switch m := msg.(type) {
	case core.KeyMsg:
		data := m.Data
		g.mu.Lock()
		defer g.mu.Unlock()

		switch {
		case data == "\x1b" || g.matches(data, "escape"):
			if g.onClose != nil {
				g.mu.Unlock()
				g.onClose()
				g.mu.Lock()
			}
		case g.matches(data, "down") || g.matches(data, "j"):
			g.focusNextLocked()
			g.dirty = true
		case g.matches(data, "up") || g.matches(data, "k"):
			g.focusPrevLocked()
			g.dirty = true
		case g.matches(data, "space") || data == " ":
			g.toggleCurrentLocked()
			g.dirty = true
		case g.matches(data, "p"):
			g.mu.Unlock()
			if g.onPass != nil {
				g.onPass()
			}
			g.mu.Lock()
		case g.matches(data, "b"):
			g.mu.Unlock()
			if g.onBack != nil {
				g.onBack()
			}
			g.mu.Lock()
		case g.matches(data, "f"):
			g.mu.Unlock()
			if g.onBlock != nil {
				g.onBlock()
			}
			g.mu.Lock()
		}
	case core.WindowSizeMsg:
		g.Invalidate()
	}
	return nil
}

// matches is a helper that checks key data against a binding ID.
// It uses the keybindings manager if set (runtime path, via the km wired in
// ChatApp.OpenReviewGate); when km is nil (e.g. in unit tests) it falls back
// to direct comparison. The fallback exists solely so tests can call Update
// without setting up a full KeybindingsManager.
func (g *ReviewGate) matches(data string, id string) bool {
	if g.km != nil {
		return terminal.MatchesKey(data, id)
	}
	// Test fallback: direct key matching for common keys.
	switch id {
	case "escape":
		return data == "\x1b"
	case "up", "k":
		return data == "\x1b[A" || data == "\x1bOA" || data == "k"
	case "down", "j":
		return data == "\x1b[B" || data == "\x1bOB" || data == "j"
	case "space":
		return data == " "
	case "p":
		return data == "p"
	case "b":
		return data == "b"
	case "f":
		return data == "f"
	default:
		return false
	}
}

// focusNextLocked moves focus to the next section/item.
// Caller must hold g.mu.
func (g *ReviewGate) focusNextLocked() {
	switch g.focusSec {
	case focusEvidences:
		if len(g.evidences) > 0 {
			if g.evidenceIdx < len(g.evidences)-1 {
				g.evidenceIdx++
			} else {
				g.focusSec = focusChecklist
				g.evidenceIdx = 0
				g.checklistIdx = 0
			}
		} else {
			g.focusSec = focusChecklist
		}
	case focusChecklist:
		if len(g.checklist) > 0 {
			if g.checklistIdx < len(g.checklist)-1 {
				g.checklistIdx++
			} else {
				g.focusSec = focusActions
				g.checklistIdx = 0
			}
		} else {
			g.focusSec = focusActions
		}
	case focusActions:
		g.focusSec = focusEvidences
		g.evidenceIdx = 0
	}
}

// focusPrevLocked moves focus to the previous section/item.
// Caller must hold g.mu.
func (g *ReviewGate) focusPrevLocked() {
	switch g.focusSec {
	case focusEvidences:
		if len(g.evidences) > 0 && g.evidenceIdx > 0 {
			g.evidenceIdx--
		} else {
			g.focusSec = focusActions
			if len(g.evidences) > 0 {
				g.evidenceIdx = len(g.evidences) - 1
			}
		}
	case focusChecklist:
		if g.checklistIdx > 0 {
			g.checklistIdx--
		} else {
			g.focusSec = focusEvidences
			g.checklistIdx = 0
			if len(g.evidences) > 0 {
				g.evidenceIdx = len(g.evidences) - 1
			}
		}
	case focusActions:
		g.focusSec = focusChecklist
		if len(g.checklist) > 0 {
			g.checklistIdx = len(g.checklist) - 1
		}
	}
}

// toggleCurrentLocked toggles the focused item (expand evidence or check item).
// Caller must hold g.mu.
func (g *ReviewGate) toggleCurrentLocked() {
	switch g.focusSec {
	case focusEvidences:
		if g.evidenceIdx >= 0 && g.evidenceIdx < len(g.evidences) {
			id := g.evidences[g.evidenceIdx].ID
			g.expanded[id] = !g.expanded[id]
		}
	case focusChecklist:
		if g.checklistIdx >= 0 && g.checklistIdx < len(g.checklist) {
			g.checklist[g.checklistIdx].Checked = !g.checklist[g.checklistIdx].Checked
		}
	case focusActions:
		// no toggle action for actions section
	}
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

func (g *ReviewGate) renderLocked(width int64) []string {
	pal := theme.CurrentPalette()
	t := DefaultReviewGateTheme()

	var out []string

	// ── Title bar ──
	titleLine := t.Title("  " + g.title + "  ")
	borderLine := t.Border(strings.Repeat("═", int(width)))
	out = append(out, borderLine, core.PadToWidth(titleLine, width), borderLine, "")

	// ── Current judgment ──
	judgmentHeader := t.Dim("当前判断")
	out = append(out, core.PadToWidth(judgmentHeader, width), core.PadToWidth(t.Body(g.judgment), width))

	// Confidence bar
	if g.confidence >= 0 {
		confBar := renderGateConfidenceBar(g.confidence, t, width)
		out = append(out, confBar)
	}

	// ── Evidences section ──
	if len(g.evidences) > 0 {
		out = append(out, "")
		evHeader := t.Dim("主要依据")
		if g.focusSec == focusEvidences && g.focused {
			evHeader = pal.SelectHighlight.Render("▸ 主要依据")
		}
		out = append(out, core.PadToWidth(evHeader, width))

		for i, ev := range g.evidences {
			expanded := g.expanded[ev.ID]
			marker := "▶"
			if expanded {
				marker = "▼"
			}

			prefix := "  "
			if g.focusSec == focusEvidences && g.evidenceIdx == i && g.focused {
				prefix = "▸ "
			}

			statusLabel := EvidenceStatusLabel(ev.Status)
			var statusStyle string
			switch ev.Status {
			case EvidenceConfirmed:
				statusStyle = t.Success("[" + statusLabel + "]")
			case EvidenceDisputed:
				statusStyle = t.Danger("[" + statusLabel + "]")
			default:
				statusStyle = t.Dim("[" + statusLabel + "]")
			}

			evLine := fmt.Sprintf("%s%s %s  %s", prefix, t.Body(marker), t.Body(ev.Title), statusStyle)
			if ev.Role != "" {
				evLine += "  " + t.Dim("("+ev.Role+")")
			}
			out = append(out, core.PadToWidth(evLine, width))

			if expanded && ev.Summary != "" {
				summaryLine := "    " + t.Dim(ev.Summary)
				out = append(out, core.PadToWidth(summaryLine, width))
			}
		}
	}

	// ── Checklist section ──
	if len(g.checklist) > 0 {
		out = append(out, "")
		clHeader := t.Dim("复核清单")
		if g.focusSec == focusChecklist && g.focused {
			clHeader = pal.SelectHighlight.Render("▸ 复核清单")
		}
		out = append(out, core.PadToWidth(clHeader, width))

		for i, item := range g.checklist {
			prefix := "  "
			if g.focusSec == focusChecklist && g.checklistIdx == i && g.focused {
				prefix = "▸ "
			}

			var checkMark string
			if item.Checked {
				checkMark = t.Checked("[x]")
			} else {
				checkMark = t.Unchecked("[ ]")
			}

			line := prefix + checkMark + " " + t.Body(item.Label)
			out = append(out, core.PadToWidth(line, width))
		}
	}

	// ── Risks section ──
	if len(g.risks) > 0 {
		out = append(out, "")
		riskHeader := t.Warning("⚠ 风险提示")
		out = append(out, core.PadToWidth(riskHeader, width))
		for _, r := range g.risks {
			riskLine := "  - " + t.Dim(r)
			out = append(out, core.PadToWidth(riskLine, width))
		}
	}

	// ── Action bar ──
	out = append(out, "")
	bottomBorder := t.Border(strings.Repeat("─", int(width)))
	out = append(out, bottomBorder)

	highlighted := g.focusSec == focusActions && g.focused

	passLabel := "[p] 通过复核" //nolint:gosec // UI label, not a credential
	backLabel := "[b] 返回补证据"
	blockLabel := "[f] 标记阻塞"
	escLabel := "[Esc] 返回"

	if highlighted {
		passLabel = pal.SelectHighlight.Render("▸ [p] 通过复核")
	} else {
		passLabel = t.Success(passLabel)
	}
	backLabel = t.Warning(backLabel)
	blockLabel = t.Danger(blockLabel)
	escLabel = t.Dim(escLabel)

	actionLine := passLabel + "  " + backLabel + "  " + blockLabel
	paddedActions := core.PadToWidth(actionLine, width)

	// Right-align Esc
	escWidth := core.VisibleWidth(escLabel)
	actionWidth := core.VisibleWidth(paddedActions)
	spaces := width - actionWidth - escWidth
	if spaces < 1 {
		spaces = 1
	}
	finalLine := paddedActions + strings.Repeat(" ", int(spaces)) + escLabel
	out = append(out, core.PadToWidth(finalLine, width))

	return out
}

// renderGateConfidenceBar draws a 10-cell confidence bar for the review gate.
func renderGateConfidenceBar(conf float64, t ReviewGateTheme, width int64) string {
	const cells = 10
	pct := int(conf * 100)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := (pct * cells) / 100

	var barColor func(string) string
	switch {
	case pct >= 67:
		barColor = t.Success
	case pct >= 34:
		barColor = t.Warning
	default:
		barColor = t.Danger
	}

	var level string
	switch {
	case pct >= 67:
		level = "高"
	case pct >= 34:
		level = "中"
	default:
		level = "低"
	}

	bar := "  置信度: " + barColor(strings.Repeat("█", filled)+strings.Repeat("░", cells-filled))
	bar += fmt.Sprintf(" %d%% (%s)", pct, level)
	return core.PadToWidth(bar, width)
}
