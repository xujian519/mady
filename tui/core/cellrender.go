package core

import "strings"

// ---------------------------------------------------------------------------
// Row → ANSI string serializer.
//
// SerializeRow converts a Row back to an ANSI string suitable for writing
// to the terminal. For Raw rows it emits the verbatim string. For cell rows
// it walks the cells, emitting an SGR escape only when the active style
// changes from one cell to the next (the "SGR optimiser").
//
// Wide chars: a Width=2 cell emits its rune; the following continuation
// cell (Width=0, Rune=0) is skipped. Combining marks are emitted after
// their base rune.
//
// The output always ends in a reset if any non-default style is active at
// end-of-line, so consecutive SerializeRow outputs never leak style into
// the next line.
// ---------------------------------------------------------------------------

// SerializeRow converts a Row to an ANSI string.
// SerializeRow converts a Row into an ANSI string suitable for terminal
// output. This is the single chokepoint for all row-to-ANSI-string conversion:
// every output path (full repaint, differential repaint) goes through here.
// For Raw rows (non-cell-structured content, e.g. LLM output with
// unrepresentable escape sequences), the content is sanitized to strip
// dangerous escape sequences (OSC/DCS/APC/non-SGR CSI) before emission.
func SerializeRow(row Row) string {
	if row.IsRaw() {
		return SanitizeRawContent(row.Raw)
	}
	if len(row.Cells) == 0 {
		return ""
	}
	var b strings.Builder
	prev := DefaultStyle
	activeNonDefault := false
	for i := 0; i < len(row.Cells); i++ {
		c := row.Cells[i]
		if c.IsContinuation() {
			// Right half of a wide char — already emitted with the primary.
			continue
		}
		if !c.Style.Equal(prev) {
			sgr := RenderSGR(prev, c.Style)
			if sgr != "" {
				b.WriteString(sgr)
			}
			prev = c.Style
			activeNonDefault = !c.Style.Equal(DefaultStyle)
		}
		if c.Rune != 0 {
			b.WriteRune(c.Rune)
		}
		if c.Combining != nil {
			for _, m := range *c.Combining {
				b.WriteRune(m)
			}
		}
	}
	if activeNonDefault {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// SerializeRowSegment serializes a slice of cells starting at a specific
// column. It resets the terminal style at the beginning (because the SGR
// state after a cursor move is unknown), emits the cells, and then leaves
// the terminal in afterStyle so that the unchanged suffix of the row is
// rendered correctly.
func SerializeRowSegment(cells []Cell, afterStyle Style) string {
	if len(cells) == 0 {
		if afterStyle.Equal(DefaultStyle) {
			return ""
		}
		return RenderSGR(DefaultStyle, afterStyle)
	}
	var b strings.Builder
	// Always emit a leading reset — the terminal's SGR state at the cursor
	// position is indeterminate (prior segments on this row may have left it
	// in any style). See tui/tui_render.go: the diff renderer emits segments
	// with cursor-positioning between them, but cursor moves do not reset SGR.
	b.WriteString("\x1b[0m")
	prev := DefaultStyle
	for i := 0; i < len(cells); i++ {
		c := cells[i]
		if c.IsContinuation() {
			continue
		}
		if !c.Style.Equal(prev) {
			if sgr := RenderSGR(prev, c.Style); sgr != "" {
				b.WriteString(sgr)
			}
			prev = c.Style
		}
		if c.Rune != 0 {
			b.WriteRune(c.Rune)
		}
		if c.Combining != nil {
			for _, m := range *c.Combining {
				b.WriteRune(m)
			}
		}
	}
	if !afterStyle.Equal(prev) {
		if sgr := RenderSGR(prev, afterStyle); sgr != "" {
			b.WriteString(sgr)
		}
	}
	return b.String()
}

// SerializeRows converts a slice of Rows to a single ANSI string with
// "\r\n" between lines. Used for full-frame repaints.
func SerializeRows(rows []Row) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString(SerializeRow(r))
	}
	return b.String()
}
