package component

// This file holds the Editor rendering and mouse-hit-testing: Render
// (soft-wrap, prompts, CURSOR_MARKER for IME, growth policy, focus
// indicator), handleMouse (click-drag text selection), hitTestLocked
// (screen→buffer coordinate translation using cached lastVisuals), and the
// selection-range helpers.

import (
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// handleMouse implements click-drag text selection, mirroring ChatHistory's
// selection model but scoped to the editor's own buffer. This is purely an
// in-app mechanism layered on top of received MouseMsg events — it never
// needs (or benefits from) disabling terminal mouse reporting, so it has no
// effect on scroll-wheel/click behavior elsewhere in the app.
func (e *Editor) handleMouse(m core.MouseMsg) {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch m.Action {
	case core.MousePress:
		row, col, ok := e.hitTestLocked(m.Row, m.Col)
		if !ok {
			return
		}
		e.allSelected = false
		e.selDragging = true
		e.selActive = true
		e.selStart = editorSelPos{row: row, col: col}
		e.selEnd = e.selStart
	case core.MouseMotion:
		if !e.selDragging {
			return
		}
		if row, col, ok := e.hitTestLocked(m.Row, m.Col); ok {
			e.selEnd = editorSelPos{row: row, col: col}
		}
	case core.MouseRelease:
		if !e.selDragging {
			return
		}
		e.selDragging = false
		if e.selStart == e.selEnd {
			e.selActive = false
		}
	}
}

// hitTestLocked translates a screen (row, col) coordinate — as delivered in
// a MouseMsg, relative to the editor's own rendered area — into a logical
// (hardRow, runeCol) buffer position, using the layout cached by the most
// recent Render call. ok is false when the coordinate falls outside any
// rendered content (e.g. growth-padding rows or stale layout).
func (e *Editor) hitTestLocked(screenRow, screenCol int64) (row, col int64, ok bool) {
	if screenRow < 0 || screenRow >= int64(len(e.lastVisuals)) {
		return 0, 0, false
	}
	v := e.lastVisuals[screenRow]
	if v.hardRow < 0 || v.hardRow >= int64(len(e.lines)) {
		return 0, 0, false
	}
	promptW := e.lastPromptWFirst
	if !v.isFirstSeg {
		promptW = e.lastPromptWCont
	}
	innerCol := screenCol - e.lastPadX - promptW
	if innerCol < 0 {
		innerCol = 0
	}
	line := e.lines[v.hardRow]
	segStart, segEnd := v.colStart, v.colEnd
	if segStart < 0 {
		segStart = 0
	}
	if segEnd > int64(len(line)) {
		segEnd = int64(len(line))
	}
	if segEnd < segStart {
		segEnd = segStart
	}
	seg := line[segStart:segEnd]
	idx := runeIndexAtCell(seg, innerCol)
	return v.hardRow, segStart + idx, true
}

// runeIndexAtCell returns the rune index within runes whose cumulative
// visible cell width first reaches or exceeds targetCell, clamped to
// len(runes). Used to map a mouse click's screen column back to a rune
// offset in CJK-aware (variable cell width) text.
func runeIndexAtCell(runes []rune, targetCell int64) int64 {
	if targetCell <= 0 {
		return 0
	}
	var col int64
	for i, r := range runes {
		w := core.RuneWidth(r)
		if col+w > targetCell {
			return int64(i)
		}
		col += w
	}
	return int64(len(runes))
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// Render soft-wraps the buffer to width, applies prompts, and inserts
// CURSOR_MARKER at the current cursor position (when focused).
func (e *Editor) Render(width int64) []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	pad := repeatSpace(e.padX)
	firstPrompt := e.promptFirst
	contPrompt := e.promptCont
	promptFn := e.promptFn
	textFn := e.textFn
	placeText := e.placeText
	placeFn := e.placeFn

	if promptFn == nil {
		promptFn = func(s string) string { return s }
	}
	if textFn == nil {
		textFn = func(s string) string { return s }
	}
	if placeFn == nil {
		placeFn = theme.CurrentPalette().Dim.Render
	}

	promptWFirst := core.VisibleWidth(firstPrompt)
	promptWCont := core.VisibleWidth(contPrompt)
	innerWidth := width - e.padX - promptWFirst
	if innerWidth < 1 {
		innerWidth = 1
	}
	contInner := width - e.padX - promptWCont
	if contInner < 1 {
		contInner = 1
	}

	// Cache layout metrics so mouse events arriving between renders can be
	// translated back into logical (row, col) buffer positions.
	e.lastPadX = e.padX
	e.lastPromptWFirst = promptWFirst
	e.lastPromptWCont = promptWCont

	// Empty + unfocused = placeholder.
	if len(e.lines) == 1 && len(e.lines[0]) == 0 && !e.focused && placeText != "" {
		e.lastVisuals = nil
		line := pad + promptFn(firstPrompt) + placeFn(core.TruncateToWidth(placeText, innerWidth, "…"))
		out := []string{core.PadToWidth(line, width)}
		for int64(len(out)) < e.minRows {
			out = append(out, core.PadToWidth("", width))
		}
		return out
	}

	type visual struct {
		text       string
		isFirstSeg bool
		hardRow    int64
		colOffset  int64 // rune offset within hard row at which this segment starts
	}
	var visuals []visual
	for i, ln := range e.lines {
		if len(ln) == 0 {
			visuals = append(visuals, visual{text: "", isFirstSeg: true, hardRow: int64(i), colOffset: 0})
			continue
		}
		offset := int64(0)
		first := true
		for offset < int64(len(ln)) {
			w := innerWidth
			if !first {
				w = contInner
			}
			slice := core.SliceRunesByCells(ln, core.CellWidthOfRunes(ln, 0, offset), core.CellWidthOfRunes(ln, 0, offset)+w)
			if slice.EndR == slice.StartR {
				// cannot fit any rune (edge case) — just take 1 rune
				slice.StartR = offset
				if offset < int64(len(ln)) {
					slice.EndR = offset + 1
				}
				slice.Text = string(ln[offset:slice.EndR])
			}
			visuals = append(visuals, visual{
				text:       slice.Text,
				isFirstSeg: first,
				hardRow:    int64(i),
				colOffset:  offset,
			})
			offset = slice.EndR
			first = false
		}
	}

	// Compute cursor visual row/col.
	var cursorV, cursorCol int64 = -1, -1
	if e.focused {
		for vi, v := range visuals {
			if v.hardRow != e.row {
				continue
			}
			segLen := int64(len([]rune(v.text)))
			segEnd := v.colOffset + segLen
			if e.col >= v.colOffset && e.col <= segEnd {
				cursorV = int64(vi)
				cursorCol = core.CellWidthOfRunes(e.lines[e.row], v.colOffset, e.col)
				break
			}
		}
	}

	selStart, selEnd, hasSel := e.normalizedSelectionLocked()

	var out []string
	rowsMeta := make([]editorVisualRow, 0, len(visuals))
	for idx, v := range visuals {
		prompt := firstPrompt
		innerW := innerWidth
		if !v.isFirstSeg {
			prompt = contPrompt
			innerW = contInner
		}
		segRunes := []rune(v.text)
		segLen := int64(len(segRunes))
		rowsMeta = append(rowsMeta, editorVisualRow{
			hardRow:    v.hardRow,
			colStart:   v.colOffset,
			colEnd:     v.colOffset + segLen,
			isFirstSeg: v.isFirstSeg,
		})

		var bodyText string
		switch {
		case e.allSelected && v.text != "":
			bodyText = "\x1b[48;5;33m" + core.StripAnsi(textFn(v.text)) + "\x1b[0m"
		case hasSel:
			if from, to, ok := selRangeInSegment(v.hardRow, v.colOffset, segLen, selStart, selEnd); ok && from < to {
				before := string(segRunes[:from])
				sel := string(segRunes[from:to])
				after := string(segRunes[to:])
				bodyText = before + "\x1b[48;5;33m" + sel + "\x1b[0m" + after
			} else {
				bodyText = textFn(v.text)
			}
		default:
			bodyText = textFn(v.text)
		}

		body := core.PadToWidth(bodyText, innerW)
		if int64(idx) == cursorV {
			body = core.InsertMarker(body, cursorCol)
		}
		line := pad + promptFn(prompt) + body
		out = append(out, core.PadToWidth(line, width))
	}
	// Growth policy.
	for int64(len(out)) < e.minRows {
		emptyPrompt := firstPrompt
		emptyInner := innerWidth
		emptyPad := pad
		body := core.PadToWidth("", emptyInner)
		line := emptyPad + promptFn(emptyPrompt) + body
		out = append(out, core.PadToWidth(line, width))
		rowsMeta = append(rowsMeta, editorVisualRow{hardRow: -1})
	}
	if int64(len(out)) > e.maxRows {
		// Keep the segment containing the cursor visible; drop leading rows.
		if cursorV >= e.maxRows {
			drop := cursorV - e.maxRows + 1
			out = out[drop:]
			rowsMeta = rowsMeta[drop:]
		} else {
			out = out[:e.maxRows]
			rowsMeta = rowsMeta[:e.maxRows]
		}
	}

	if e.focused && e.focusIndicator != "" {
		if len(out) > 0 {
			last := len(out) - 1
			out[last] = e.focusIndicator + out[last]
		}
	}

	e.lastVisuals = rowsMeta
	return out
}

// selRangeInSegment computes the selected rune range [from, to) within a
// single visual segment of hardRow — which spans rune offsets
// [segStart, segStart+segLen) of that hard line — given a normalized
// mouse-drag selection. ok is false when hardRow falls outside the
// selection entirely; from/to may still fall outside [0, segLen) when the
// selection doesn't reach this particular segment, so callers must check
// from < to before using the range (guaranteeing 0 <= from < to <= segLen).
func selRangeInSegment(hardRow, segStart, segLen int64, selStart, selEnd editorSelPos) (from, to int64, ok bool) {
	if hardRow < selStart.row || hardRow > selEnd.row {
		return 0, 0, false
	}
	rowFrom, rowTo := int64(0), segLen+segStart
	if hardRow == selStart.row {
		rowFrom = selStart.col
	}
	if hardRow == selEnd.row {
		rowTo = selEnd.col
	}
	from = rowFrom - segStart
	to = rowTo - segStart
	if from < 0 {
		from = 0
	}
	if to > segLen {
		to = segLen
	}
	return from, to, true
}
