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

	children, overlays, prev, prevRaw, prevW, first := t.snapshotFrame()

	// 行级缓存只能在“基础帧”之间复用。composeOverlays 会把 overlay 像素
	// 写进返回的行，合成后的行与基础行 raw 字符串不再处于同一坐标系；
	// 若把合成行当成下一帧的基础行缓存，overlay 关闭后旧像素会永久残留。
	hasOverlays := len(overlays) > 0
	if hasOverlays {
		prev, prevRaw = nil, nil
	}

	rows, rawLines := t.renderChildrenRows(children, cols, prev, prevRaw)

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

	cursorRow, cursorCol := locateCursor(rows)

	var buf bytes.Buffer
	if !t.options.DisableSynchronizedOutput {
		buf.WriteString("\x1b[?2026h")
	}
	// Disable auto-wrap (DECAWM) for the duration of the frame render.
	buf.WriteString("\x1b[?7l")

	if first || prevW != cols {
		t.writeFullRepaint(&buf, rows)
	} else {
		t.writeDiffRepaint(&buf, prev, rows)
	}

	t.applyCursorState(&buf, cursorRow, cursorCol)

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
	if hasOverlays {
		// 本帧被 overlay 合成污染，不能作为下一帧的行缓存来源。
		t.prevRaw = nil
	} else {
		t.prevRaw = rawLines
	}
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

// snapshotFrame 在锁内拷贝渲染所需的帧快照（children/overlays/prev/raw/width/first）。
func (t *TUI) snapshotFrame() ([]core.Component, []*Overlay, []core.Row, []string, int64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	children := make([]core.Component, len(t.children))
	copy(children, t.children)
	overlays := make([]*Overlay, len(t.overlays))
	copy(overlays, t.overlays)
	prev := t.prevFrame
	prevRaw := t.prevRaw
	prevW := t.prevWidth
	first := t.firstFrame
	return children, overlays, prev, prevRaw, prevW, first
}

// invalidateFrameCache 丢弃行级渲染缓存并强制下一帧全量重绘。
//
// overlay 栈变化（打开/关闭/弹出）时必须调用：合成帧与基础帧的坐标系
// 不同，继续复用会把 overlay 像素当作基础内容。全量重绘是兜底，确保
// 终端上的残留立即被清掉。
func (t *TUI) invalidateFrameCache() {
	t.mu.Lock()
	t.prevRaw = nil
	t.firstFrame = true
	t.mu.Unlock()
}

// renderChildrenRows 渲染所有子组件为行，并应用缓存优化：原始字符串与
// 上一帧相同的位置直接复用已解析的 Row。
func (t *TUI) renderChildrenRows(children []core.Component, cols int64, prev []core.Row, prevRaw []string) ([]core.Row, []string) {
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
	return rows, rawLines
}

// locateCursor 在所有行中查找 IME 光标标记（ParseLine 已剥离标记并记录
// 列号），返回首个携带标记的行列。
func locateCursor(rows []core.Row) (int64, int64) {
	for i, r := range rows {
		if r.CursorCol >= 0 {
			return int64(i), int64(r.CursorCol)
		}
	}
	return -1, -1
}

// writeFullRepaint 全量重绘：从顶部逐行写入，先隐藏光标避免闪烁。
func (t *TUI) writeFullRepaint(buf *bytes.Buffer, rows []core.Row) {
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
}

// writeDiffRepaint 差分重绘：仅输出发生变化的单元段，减少终端输出带宽。
func (t *TUI) writeDiffRepaint(buf *bytes.Buffer, prev, rows []core.Row) {
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

// applyCursorState 按状态机规则输出光标命令：仅在可见性/位置变化时发出，
// 保持终端的光标闪烁计时器不被每帧的 hide/show 干扰。
func (t *TUI) applyCursorState(buf *bytes.Buffer, cursorRow, cursorCol int64) {
	wantVisible := cursorRow >= 0

	if wantVisible != t.lastCursor.visible {
		// Visibility transition: emit Show/Hide.
		if wantVisible {
			buf.WriteString(terminal.CursorPosition(cursorRow+1, cursorCol+1) + terminal.ShowCursor())
		} else {
			buf.WriteString(terminal.HideCursor())
		}
	} else if wantVisible && (cursorRow != t.lastCursor.row || cursorCol != t.lastCursor.col) {
		// Already visible but position changed: reposition without Show/Hide.
		buf.WriteString(terminal.CursorPosition(cursorRow+1, cursorCol+1))
	}
	// No change: zero cursor commands emitted — blink timer undisturbed.

	t.lastCursor.visible = wantVisible
	t.lastCursor.row = cursorRow
	t.lastCursor.col = cursorCol
	t.lastCursor.first = false
}

// minTermCols defines the minimum terminal width for the TUI.
// Below this value, a resize hint is shown instead of the normal UI.
// Row height is not checked — terminals naturally scroll content
// vertically, and many valid use cases (embedded terminals, panes)
// may have fewer than 24 rows.
const minTermCols int64 = 80

// resizeHintMinCols 是画完整提示框所需的最小宽度。更窄或行数不足时
// 退化为单行提示，避免提示框自身折行、错位——那正是它要避免的情况。
const resizeHintMinCols int64 = 50

// resizeHintLines 返回“终端过窄”提示的各行，保证每行可见宽度 ≤ cols。
// 宽度按显示单元格计算（CJK 为 2 列），而不是字节长度。
func resizeHintLines(cols, rows int64) []string {
	if cols <= 0 {
		cols = minTermCols
	}
	hintCN := "终端窗口太小，请放大以继续使用"
	hintEN := fmt.Sprintf("Terminal too narrow · min %d · current %d", minTermCols, cols)
	if cols < resizeHintMinCols || rows < 4 {
		return []string{core.TruncateToWidth(hintCN, cols, "…")}
	}

	inner := core.VisibleWidth(hintCN)
	if w := core.VisibleWidth(hintEN); w > inner {
		inner = w
	}
	boxWidth := inner + 6 // ║ + 2 空格 + inner + 2 空格 + ║
	if boxWidth < resizeHintMinCols {
		boxWidth = resizeHintMinCols
	}
	if boxWidth > cols {
		boxWidth = cols
	}
	inner = boxWidth - 6

	pad := func(s string) string {
		if n := inner - core.VisibleWidth(s); n > 0 {
			return s + strings.Repeat(" ", int(n))
		}
		return core.TruncateToWidth(s, inner, "…")
	}
	border := strings.Repeat("═", int(boxWidth-2))
	return []string{
		"╔" + border + "╗",
		"║  " + pad(hintCN) + "  ║",
		"║  " + pad(hintEN) + "  ║",
		"╚" + border + "╝",
	}
}

func (t *TUI) renderResizeHint(cols, rows int64) {
	lines := resizeHintLines(cols, rows)

	var buf bytes.Buffer
	buf.WriteString("\x1b[?25l") // hide cursor
	buf.WriteString("\x1b[H")    // cursor home
	buf.WriteString("\x1b[J")    // clear below
	// 关闭自动换行：提示框宽度可能正好等于终端宽度，最后一列若触发
	// 自动换行会把右边框折到下一行（40/60 列下曾出现）。
	buf.WriteString("\x1b[?7l")

	lineW := core.VisibleWidth(lines[0])
	leftX := (cols - lineW) / 2
	if leftX < 0 {
		leftX = 0
	}
	topY := rows/2 - int64(len(lines))/2
	if topY+int64(len(lines)) > rows {
		topY = rows - int64(len(lines))
	}
	if topY < 0 {
		topY = 0
	}
	for i, ln := range lines {
		fmt.Fprintf(&buf, "\x1b[%d;%dH%s", topY+int64(i)+1, leftX+1, ln)
	}

	buf.WriteString("\x1b[?7h") // 恢复自动换行
	// Move cursor out of the way and show it.
	fmt.Fprintf(&buf, "\x1b[%d;1H", topY+int64(len(lines))+1)
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
