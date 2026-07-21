package component

// evidence_overlay.go — EvidenceOverlay for displaying retrieved knowledge
// sources (law articles, judgments, patent documents) with metadata.
//
// This is a read-only scrollable overlay, accessed via [e] key when the
// judgment area is expanded (awaiting_review / blocked). It shows the
// evidence items used in the current decision context, helping the user
// verify the factual basis of the analysis.
//
// Keyboard navigation:
//   ↑↓ — scroll through items
//   PgUp/PgDn — page scroll
//   Esc — close

import (
	"fmt"
	"strings"
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// EvidenceItem represents one evidence record shown in the EvidenceOverlay.
// It maps to knowledge retrieval results (ScoredChunk, CitationMeta, etc.)
// with display-friendly fields.
type EvidenceItem struct {
	// Title is a brief heading (e.g., "专利法第22条第3款 · 创造性").
	Title string
	// Source describes the origin (e.g., "审查指南第二部分第四章").
	Source string
	// Score is the relevance score (0.0-1.0). -1 hides the score.
	Score float64
	// Excerpt is a short text snippet from the evidence.
	Excerpt string
}

// EvidenceOverlay renders a list of evidence items in a scrollable panel.
type EvidenceOverlay struct {
	mu sync.RWMutex

	title  string         // overlay title, e.g. "引用证据详情"
	items  []EvidenceItem // evidence items
	scroll int            // current scroll offset
	cursor int            // currently selected item index (-1 = none)

	onClose func()

	// rendering cache
	dirty  bool
	cache  []string
	cacheW int64
}

// NewEvidenceOverlay creates an empty evidence overlay.
// Call SetItems before mounting.
func NewEvidenceOverlay() *EvidenceOverlay {
	return &EvidenceOverlay{
		title:  "引用证据详情",
		scroll: 0,
		cursor: 0,
		dirty:  true,
	}
}

// SetTitle sets the overlay title.
func (e *EvidenceOverlay) SetTitle(title string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.title != title {
		e.title = title
		e.dirty = true
	}
}

// SetItems replaces the evidence list. Passing nil clears the list.
func (e *EvidenceOverlay) SetItems(items []EvidenceItem) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.items = items
	e.scroll = 0
	e.cursor = 0
	e.dirty = true
}

// SetOnClose registers the close callback.
func (e *EvidenceOverlay) SetOnClose(fn func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onClose = fn
}

// SetKeybindings is a no-op for the evidence overlay (it uses fixed
// navigation keys), but callers may set it for compatibility with
// the overlay management pattern.
func (e *EvidenceOverlay) SetKeybindings(_ *terminal.KeybindingsManager) {}

// ---------------------------------------------------------------------------
// core.Component
// ---------------------------------------------------------------------------

// Invalidate marks the render cache dirty.
func (e *EvidenceOverlay) Invalidate() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dirty = true
}

// Render produces the rendered lines.
func (e *EvidenceOverlay) Render(width int64) []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.dirty && e.cacheW == width && e.cache != nil {
		return e.cache
	}

	p := theme.CurrentPalette()
	var lines []string

	// --- Header ---
	lines = append(lines,
		p.Accent.Render(e.title),
		p.BorderMuted.Render(strings.Repeat("─", int(width))),
	)

	if len(e.items) == 0 {
		lines = append(lines, p.Dim.Render("  （暂无引用证据）"))
		e.cache = lines
		e.cacheW = width
		e.dirty = false
		return lines
	}

	// Calculate visible area: 1 header line + 1 separator + footer area (2)
	availableHeight := 20 // default if we can't calculate
	itemsPerPage := availableHeight - 4
	if itemsPerPage < 3 {
		itemsPerPage = 3
	}

	// Clamp scroll and cursor.
	e.clampScroll(itemsPerPage)

	// --- Items ---
	visibleEnd := e.scroll + itemsPerPage
	if visibleEnd > len(e.items) {
		visibleEnd = len(e.items)
	}

	for i := e.scroll; i < visibleEnd; i++ {
		item := e.items[i]
		cursorMark := "  "
		if i == e.cursor {
			cursorMark = p.Accent.Render("▶ ")
		}

		// Title line.
		titleText := item.Title
		if item.Score >= 0 {
			titleText = fmt.Sprintf("%s (相关度: %.2f)", titleText, item.Score)
		}
		lines = append(lines, fmt.Sprintf("%s%s", cursorMark, titleText))

		// Source line.
		if item.Source != "" {
			lines = append(lines, fmt.Sprintf("    %s: %s",
				p.Dim.Render("来源"), p.Dim.Render(item.Source)))
		}

		// Excerpt line (single line, truncated).
		if item.Excerpt != "" {
			excerpt := truncateToWidth(item.Excerpt, int(width)-6)
			lines = append(lines, fmt.Sprintf("    %s", p.Dim.Render(excerpt)))
		}

		// Separator between items.
		if i < visibleEnd-1 {
			lines = append(lines, p.Dim.Render("    · · ·"))
		}
	}

	// --- Footer ---
	lines = append(lines, p.BorderMuted.Render(strings.Repeat("─", int(width))))
	footerParts := []string{
		fmt.Sprintf("%d/%d", e.cursor+1, len(e.items)),
		"↑↓ 浏览",
		"Esc 关闭",
	}
	lines = append(lines, p.Dim.Render(strings.Join(footerParts, "  ")))

	e.cache = lines
	e.cacheW = width
	e.dirty = false
	return lines
}

// ---------------------------------------------------------------------------
// core.Updatable
// ---------------------------------------------------------------------------

// Update handles keyboard input for scrolling and closing.
func (e *EvidenceOverlay) Update(msg core.Msg) core.Cmd {
	if m, ok := msg.(core.KeyMsg); ok {
		for _, k := range terminal.ParseKeys(m.Data) {
			switch strings.ToLower(k.Name) {
			case "escape":
				if e.onClose != nil {
					e.onClose()
				}
				return nil
			case "up", "k":
				e.moveCursor(-1)
			case "down", "j":
				e.moveCursor(1)
			case "pageup":
				e.pageScroll(-1)
			case "pagedown":
				e.pageScroll(1)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (e *EvidenceOverlay) moveCursor(delta int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.items) == 0 {
		return
	}
	e.cursor += delta
	if e.cursor < 0 {
		e.cursor = 0
	}
	if e.cursor >= len(e.items) {
		e.cursor = len(e.items) - 1
	}
	e.dirty = true
}

func (e *EvidenceOverlay) pageScroll(delta int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.items) == 0 {
		return
	}
	itemsPerPage := 16 // approximate
	if delta > 0 {
		e.cursor += itemsPerPage
		e.scroll += itemsPerPage
	} else {
		e.cursor -= itemsPerPage
		e.scroll -= itemsPerPage
	}
	e.clampScroll(itemsPerPage)
	e.dirty = true
}

func (e *EvidenceOverlay) clampScroll(itemsPerPage int) {
	if e.cursor < 0 {
		e.cursor = 0
	}
	if e.cursor >= len(e.items) {
		e.cursor = len(e.items) - 1
	}
	if e.scroll > e.cursor {
		e.scroll = e.cursor
	}
	if e.scroll+itemsPerPage <= e.cursor {
		e.scroll = e.cursor - itemsPerPage + 1
	}
	if e.scroll < 0 {
		e.scroll = 0
	}
	maxScroll := len(e.items) - itemsPerPage
	if maxScroll < 0 {
		maxScroll = 0
	}
	if e.scroll > maxScroll {
		e.scroll = maxScroll
	}
}

// truncateToWidth truncates a string if it exceeds the given pixel width.
// Simple approximation: each CJK character ≈ 2 chars wide, ASCII ≈ 1.
func truncateToWidth(s string, maxWidth int) string {
	width := 0
	runes := []rune(s)
	for i, r := range runes {
		cw := 1
		if r >= 0x4e00 && r <= 0x9fff {
			cw = 2
		} else if r >= 0x3000 && r <= 0x303f {
			cw = 2
		}
		if width+cw > maxWidth {
			return string(runes[:i]) + "…"
		}
		width += cw
	}
	return s
}
