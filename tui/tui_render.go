package tui

// This file contains the rendering pipeline: RequestRender coalesces burst
// requests, renderFrame composes children rows + overlays into a cell grid,
// emits a differential (or full) frame wrapped in CSI 2026 synchronized
// output, and normalizeLine clamps a component line to the terminal width.
//
// Minimum terminal size:
//   - Below 80 columns or 24 rows the terminal is too small for the TUI
//     layout. renderFrame detects this and displays a resize hint instead
//     of the normal UI.
//   - When the terminal is resized back to ≥80×24, normal rendering resumes
//     automatically on the next frame.

import (
	"bytes"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
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

	// Minimum terminal size gate: below 80×24 the layout does not fit.
	// Show a resize hint instead of garbled content. Normal rendering
	// resumes automatically on the next frame after resize.
	termCols, termRows := t.term.Size()
	if termCols < minTermCols {
		// Use the actual terminal height for the hint layout; if it's
		// too small to display the full hint box, the hint text will
		// still be readable at the top of the terminal.
		hintRows := termRows
		if hintRows < 3 {
			hintRows = 3
		}
		t.renderResizeHint(termCols, hintRows)
		return
	}
	cols := termCols
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
		// 受信任链接：组件显式提供与渲染行一一对应的 LinkSpan 元数据
		// （见 core.LinkProvider）。LLM 原始输出不经过此通道。
		var links [][]core.LinkSpan
		if lp, ok := c.(core.LinkProvider); ok {
			links = lp.RenderLinks(cols)
		}
		for j, ln := range c.Render(cols) {
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
			r := core.ParseLine(ln)
			if links != nil && j < len(links) {
				r.Links = links[j]
			}
			rows = append(rows, r)
		}
	}

	t.mu.Lock()
	overlays := make([]*Overlay, len(t.overlays))
	copy(overlays, t.overlays)
	t.mu.Unlock()
	_, termRows2 := t.term.Size()
	if termRows2 <= 0 {
		termRows2 = int64(len(rows))
	}
	rows = composeOverlays(rows, overlays, cols, termRows2)

	// Safety net: clip the total output to termRows. Even though the Flex
	// layout should always produce exactly termRows lines, any mismatch
	// (terminal resize between two Size reads, a Shrinkable component
	// ignoring OnAllocate, or a Fill child returning more lines than
	// allocated) would overflow the terminal and push the editor off-screen.
	// Clipping from the top keeps the editor and status bar visible at the
	// bottom where the user is typing. A debug log is emitted when clipping
	// fires so layout bugs can be diagnosed even if the user doesn't notice.
	if int64(len(rows)) > termRows2 {
		slog.Debug("render frame clipped", "got", len(rows), "max", termRows2)
		rows = rows[len(rows)-int(termRows2):]
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
		// Record that the cursor is hidden so the stateful cursor block
		// below re-emits ShowCursor when the cursor should be visible
		// again. Without this, a full repaint (e.g. after a resize) left
		// the cursor permanently hidden (P1-6).
		t.lastCursor.visible = false
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
				// The sanitized Raw content may itself carry an unterminated
				// SGR (e.g. a truncated streaming chunk); close it so the
				// style cannot leak into the unchanged rows below (P2-11).
				buf.WriteString(terminal.Reset)
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
// ---------------------------------------------------------------------------
// Minimum terminal size
// ---------------------------------------------------------------------------

// minTermCols defines the minimum terminal width for the TUI.
// Below this value, a resize hint is shown instead of the normal UI.
// Row height is not checked — terminals naturally scroll content
// vertically, and many valid use cases (embedded terminals, panes)
// may have fewer than 24 rows.
const minTermCols int64 = 80

// renderResizeHint displays a centered, boxed resize hint when the terminal
// is too small for the TUI layout. Pure text-only rendering (no components,
// no cell grid) — guaranteed to work at any terminal size.
func (t *TUI) renderResizeHint(cols, rows int64) {
	var buf bytes.Buffer
	buf.WriteString("\x1b[?25l") // hide cursor
	buf.WriteString("\x1b[H")    // cursor home
	buf.WriteString("\x1b[J")    // clear below

	// Build the hint message.
	hint := fmt.Sprintf("Terminal too narrow  |  Min: %d cols  |  Current: %d cols",
		minTermCols, cols)
	hintCN := "终端窗口太小，请放大以继续使用"

	// Use a simple box. Box width is derived from the visible (display-cell)
	// width, not byte length — CJK text is multi-byte and would misalign the
	// right border if padded with %-*s. The +6 accounts for the two box
	// borders plus the 2-space padding on each side (║ + 2 + inner + 2 + ║),
	// so padLine fills the content to exactly inner and all rows line up
	// with the border rows.
	boxWidth := core.VisibleWidth(hint) + 6
	if boxWidth < 50 {
		boxWidth = 50
	}
	inner := boxWidth - 6 // 扣除 "║" + 2 空格 + 2 空格 + "║" 共 6 列固定开销
	padLine := func(s string) string {
		if pad := inner - core.VisibleWidth(s); pad > 0 {
			return s + strings.Repeat(" ", int(pad))
		}
		return s
	}
	topY := rows / 2
	textY := topY + 1
	hintCNY := textY + 1
	botY := hintCNY + 1
	leftX := (cols - boxWidth) / 2
	if leftX < 0 {
		leftX = 0
	}

	// Top border
	fmt.Fprintf(&buf, "\x1b[%d;%dH╔", topY+1, leftX+1)
	for i := int64(2); i < boxWidth; i++ {
		buf.WriteString("═")
	}
	buf.WriteString("╗")

	// Chinese hint
	fmt.Fprintf(&buf, "\x1b[%d;%dH║  %s  ║", hintCNY+1, leftX+1, padLine(hintCN))
	// English hint
	fmt.Fprintf(&buf, "\x1b[%d;%dH║  %s  ║", textY+1, leftX+1, padLine(hint))
	// Bottom border
	fmt.Fprintf(&buf, "\x1b[%d;%dH╚", botY+1, leftX+1)
	for i := int64(2); i < boxWidth; i++ {
		buf.WriteString("═")
	}
	buf.WriteString("╝")

	// Move cursor out of the way and show it.
	fmt.Fprintf(&buf, "\x1b[%d;1H", botY+2)
	buf.WriteString("\x1b[?25h") // show cursor

	if _, err := t.term.Write(buf.Bytes()); err != nil {
		slog.Default().Debug("resize hint write failed", "error", err)
	}

	// Mark as first frame so normal rendering does a full clear on return.
	t.mu.Lock()
	t.firstFrame = true
	t.mu.Unlock()
}

func normalizeLine(line string, cols int64) string {
	if core.VisibleWidth(line) <= cols {
		return line
	}
	return core.TruncateToWidth(line, cols, "…")
}
