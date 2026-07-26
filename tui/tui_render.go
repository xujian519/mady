package tui

// This file contains the rendering pipeline: RequestRender coalesces burst
// requests, renderFrame composes children rows + overlays into a cell grid,
// emits a differential (or full) frame wrapped in CSI 2026 synchronized
// output, and normalizeLine clamps a component line to the terminal width.

import (
	"bytes"
	"fmt"
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"

	core "github.com/xujian519/mady/tui/core"
	terminal "github.com/xujian519/mady/tui/terminal"
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
	renderStart := time.Now() // for budget monitoring
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
			// Assert: after normalization, no line should exceed cols. A
			// component returning over-wide content is a layout bug that
			// would corrupt subsequent rows (wide chars/phrases spill into
			// the next line under DECAWM-off). Log once per offending line
			// so the buggy component author can diagnose without crashing.
			if w := core.VisibleWidth(ln); w > cols {
				slog.Default().Debug("component returned over-width line",
					"component", fmt.Sprintf("%T", c),
					"width", cols,
					"got", w,
				)
			}
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
	buf.WriteString("\x1b[?7l")

	if first || prevW != cols {
		// Full repaint: write every row from top to bottom.
		// Always hide cursor during full repaint to avoid flicker while
		// rows are being redrawn. Stateful cursor visibility is restored below.
		buf.WriteString(terminal.HideCursor())
		buf.WriteString(terminal.CursorHome())
		buf.WriteString(terminal.ClearFromCursorDown())
		for i, r := range rows {
			buf.WriteString(core.SerializeRow(r))
			// SerializeRow emits its own reset when needed, but a trailing
			// reset guarantees no style leaks across lines.
			buf.WriteString(terminal.Reset)
			if i < len(rows)-1 {
				buf.WriteString("\r\n")
			}
		}
	} else {
		// Differential repaint: emit only the changed cell segments. This
		// reduces terminal output bandwidth compared to rewriting whole rows.
		// Hide cursor during the repaint only when it was previously visible,
		// since the diff writes move the cursor via MoveTo. If already hidden
		// from a prior frame, skip the hide to preserve the blink timer.
		if t.lastCursor.visible || t.lastCursor.first {
			buf.WriteString(terminal.HideCursor())
		}
		diff := core.DiffFrame(prev, rows)
		for _, d := range diff {
			if d.RawContent != "" {
				// Raw rows lack cell structure — fall back to a full-row
				// rewrite. Reset style first because the SGR state after a
				// cursor move is unknown.
				// Delegate to SerializeRow (which sanitizes dangerous escape
				// sequences) rather than sanitizing here, so that SerializeRow
				// is the single chokepoint for all row-to-ANSI-string conversion.
				buf.WriteString(terminal.CursorPosition(d.Row+1, 1) + terminal.Reset)
				buf.WriteString(core.SerializeRow(rows[d.Row]))
				continue
			}
			for _, seg := range d.Segments {
				buf.WriteString(terminal.CursorPosition(d.Row+1, seg.StartCol+1))
				buf.WriteString(core.SerializeRowSegment(seg.Cells, seg.AfterStyle))
				core.PutDiffCells(seg.Cells)
			}
			if d.ClearTail {
				buf.WriteString(terminal.CursorPosition(d.Row+1, d.TailStart+1))
				buf.WriteString(terminal.ClearToEndOfLine())
				buf.WriteString(terminal.Reset)
			}
		}
		if len(rows) < len(prev) {
			buf.WriteString(terminal.CursorPosition(int64(len(rows)+1), 1))
			buf.WriteString(terminal.ClearFromCursorDown())
		}
	}

	// Stateful cursor placement: only emit Show/Hide and MoveTo when the
	// desired state differs from the previous frame. This preserves the
	// terminal's cursor blink timer — constant re-hide/reset every frame
	// prevents the blink cycle from completing.
	wantVisible := cursorRow >= 0
	wantRow := cursorRow
	wantCol := cursorCol

	if wantVisible != t.lastCursor.visible {
		// Visibility transition: emit Show/Hide.
		if wantVisible {
			buf.WriteString(terminal.CursorPosition(wantRow+1, wantCol+1) + terminal.ShowCursor())
		} else {
			buf.WriteString(terminal.HideCursor())
		}
	} else if wantVisible && (wantRow != t.lastCursor.row || wantCol != t.lastCursor.col) {
		// Already visible but position changed: reposition without Show/Hide.
		buf.WriteString(terminal.CursorPosition(wantRow+1, wantCol+1))
	}
	// No change: zero cursor commands emitted — blink timer undisturbed.

	t.lastCursor.visible = wantVisible
	t.lastCursor.row = wantRow
	t.lastCursor.col = wantCol
	t.lastCursor.first = false

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

	// Record frame timestamp in a circular buffer for FPS computation.
	// O(1) insert — no shift-copy. frameHead points at the oldest entry.
	now := time.Now()
	c := debugFrameCap
	if c < 2 {
		c = 2
	}
	idx := (t.frameHead + t.frameRingCount) % c
	t.frameStamps[idx] = now
	if t.frameRingCount >= c {
		t.frameHead = (t.frameHead + 1) % c
	} else {
		t.frameRingCount++
	}

	// Compute FPS from the oldest to newest timestamp in the ring.
	n := t.frameRingCount
	if n >= 2 {
		oldest := t.frameStamps[t.frameHead]
		newest := t.frameStamps[(t.frameHead+n-1)%c]
		window := newest.Sub(oldest)
		if window > 0 {
			t.lastFPS = float64(n-1) / window.Seconds()
		}
	}

	// Measure render duration and count budget violations (>16ms).
	t.lastRenderDur = time.Since(renderStart)
	if t.lastRenderDur > 16*time.Millisecond {
		t.slowFrameCount++
	}

	// Sample memory stats every ~100 frames (approx 1-2s at 60fps).
	t.frameTotal++
	if t.frameTotal%100 == 0 {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		t.lastAlloc = m.Alloc
	}
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
