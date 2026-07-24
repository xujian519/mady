package tui

// This file contains the rendering pipeline: RequestRender coalesces burst
// requests, renderFrame composes children rows + overlays into a cell grid,
// emits a differential (or full) frame wrapped in CSI 2026 synchronized
// output, and normalizeLine clamps a component line to the terminal width.

import (
	"bytes"
	"fmt"
	"log/slog"
	"sync/atomic"

	core "github.com/xujian519/mady/tui/core"
)

// RequestRender coalesces repeated calls into a single frame.
func (t *TUI) RequestRender() {
	atomic.StoreInt64(&t.renderRequested, 1)
	select {
	case t.tickCh <- struct{}{}:
	default:
	}
}

func (t *TUI) renderFrame() {
	cols, _ := t.term.Size()
	if cols <= 0 {
		cols = 80
	}

	t.mu.Lock()
	children := make([]core.Component, len(t.children))
	copy(children, t.children)
	prev := t.prevFrame
	prevRaw := t.prevRaw
	prevW := t.prevWidth
	first := t.firstFrame
	t.mu.Unlock()

	// Render children to strings, then parse each line into a cell Row.
	// Parsing happens here (not in components) so component authors keep the
	// simple []string API and the engine owns the cell model.
	//
	// Optimization: store raw output strings alongside parsed Rows. Before
	// calling ParseLine (which walks the string character-by-character to
	// parse ANSI escapes), compare the raw string against the previous
	// frame's raw string. If identical, reuse the already-parsed Row
	// directly. During streaming, only 1-2 lines per frame actually change,
	// so this avoids hundreds of ParseLine calls per frame.
	var rows []core.Row
	var rawLines []string
	for _, c := range children {
		for _, ln := range c.Render(cols) {
			ln = normalizeLine(ln, cols)
			rawLines = append(rawLines, ln)
			// Fast path: if the raw string is byte-identical to the previous
			// frame at the same position, reuse the parsed Row.
			idx := len(rows)
			if idx < len(prevRaw) && idx < len(prev) && ln == prevRaw[idx] {
				rows = append(rows, prev[idx])
				continue
			}
			rows = append(rows, core.ParseLine(ln))
		}
	}

	t.mu.Lock()
	overlays := make([]*Overlay, len(t.overlays))
	copy(overlays, t.overlays)
	t.mu.Unlock()
	_, termRows := t.term.Size()
	if termRows <= 0 {
		termRows = int64(len(rows))
	}
	rows = composeOverlays(rows, overlays, cols, termRows)

	// Safety net: clip the total output to termRows. Even though the Flex
	// layout should always produce exactly termRows lines, any mismatch
	// (terminal resize between two Size reads, a Shrinkable component
	// ignoring OnAllocate, or a Fill child returning more lines than
	// allocated) would overflow the terminal and push the editor off-screen.
	// Clipping from the top keeps the editor and status bar visible at the
	// bottom where the user is typing. A debug log is emitted when clipping
	// fires so layout bugs can be diagnosed even if the user doesn't notice.
	if int64(len(rows)) > termRows {
		slog.Debug("render frame clipped", "got", len(rows), "max", termRows)
		rows = rows[len(rows)-int(termRows):]
	}

	// Locate the IME cursor marker across all rows. ParseLine already strips
	// CURSOR_MARKER and records its column on the Row; here we just find the
	// first row that carries one.
	cursorRow := int64(-1)
	cursorCol := int64(-1)
	for i, r := range rows {
		if r.CursorCol >= 0 {
			cursorRow = int64(i)
			cursorCol = int64(r.CursorCol)
			break
		}
	}

	var buf bytes.Buffer
	if !t.options.DisableSynchronizedOutput {
		buf.WriteString("\x1b[?2026h")
	}

	// Disable auto-wrap (DECAWM) for the duration of the frame render.
	// VisibleWidth (which drives truncation/padding) treats East-Asian
	// Ambiguous chars (—, →, ★, …) as width 1, but CJK-capable terminals
	// on macOS often render them as width 2. When a history line contains
	// such a character, the terminal sees the line as wider than cols and
	// wraps the last column to the next row. If that next row (e.g. the
	// editor top border) is unchanged, the diff engine skips it, and the
	// wrapped character overwrites its first cell — visible as a "gap" at
	// the left edge of the border. With DECAWM off, excess characters are
	// silently dropped at the right margin instead of wrapping.
	buf.WriteString("\x1b[?7l")

	if first || prevW != cols {
		// Full repaint: write every row from top to bottom.
		buf.WriteString("\x1b[?25l")
		buf.WriteString("\x1b[H")
		buf.WriteString("\x1b[0J")
		for i, r := range rows {
			buf.WriteString(core.SerializeRow(r))
			// SerializeRow emits its own reset when needed, but a trailing
			// reset guarantees no style leaks across lines.
			buf.WriteString("\x1b[0m")
			if i < len(rows)-1 {
				buf.WriteString("\r\n")
			}
		}
	} else {
		// Differential repaint: emit only the changed cell segments. This
		// reduces terminal output bandwidth compared to rewriting whole rows.
		buf.WriteString("\x1b[?25l")
		diff := core.DiffFrame(prev, rows)
		for _, d := range diff {
			if d.RawContent != "" {
				// Raw rows lack cell structure — fall back to a full-row
				// rewrite. Reset style first because the SGR state after a
				// cursor move is unknown.
				fmt.Fprintf(&buf, "\x1b[%d;1H\x1b[0m", d.Row+1)
				buf.WriteString(d.RawContent)
				continue
			}
			for _, seg := range d.Segments {
				fmt.Fprintf(&buf, "\x1b[%d;%dH", d.Row+1, seg.StartCol+1)
				buf.WriteString(core.SerializeRowSegment(seg.Cells, seg.AfterStyle))
			}
			if d.ClearTail {
				fmt.Fprintf(&buf, "\x1b[%d;%dH", d.Row+1, d.TailStart+1)
				buf.WriteString("\x1b[0K")
				buf.WriteString("\x1b[0m")
			}
		}
		if len(rows) < len(prev) {
			fmt.Fprintf(&buf, "\x1b[%d;1H", len(rows)+1)
			buf.WriteString("\x1b[0J")
		}
	}

	if cursorRow >= 0 {
		fmt.Fprintf(&buf, "\x1b[%d;%dH", cursorRow+1, cursorCol+1)
		buf.WriteString("\x1b[?25h")
	} else {
		buf.WriteString("\x1b[?25l")
	}

	// Re-enable auto-wrap after the frame render.
	buf.WriteString("\x1b[?7h")

	if !t.options.DisableSynchronizedOutput {
		buf.WriteString("\x1b[?2026l")
	}

	if _, err := t.term.Write(buf.Bytes()); err != nil {
		slog.Default().Debug("terminal write failed", "error", err)
	}

	t.mu.Lock()
	t.prevFrame = rows
	t.prevRaw = rawLines
	t.prevWidth = cols
	t.firstFrame = false
	t.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

// normalizeLine ensures a single component-rendered line fits within `cols`.
// It truncates with ellipsis and preserves ANSI styles across the cut.
func normalizeLine(line string, cols int64) string {
	if core.VisibleWidth(line) <= cols {
		return line
	}
	return core.TruncateToWidth(line, cols, "…")
}
