package tui

import (
	"time"

	"github.com/xujian519/mady/tui/core"
)

// composeOverlays splices overlay renders onto base rows in stack order.
//
// The composition operates on the cell model: each base row is parsed into
// cells, overlay content is copied cell-by-cell into the target region, and
// the result is left in cell form for the renderer to diff and serialize.
// This eliminates the wide-char truncation and style-loss bugs that the
// previous string-level spliceOverlay had (see cell.go commentary).
//
// IMPORTANT: composeOverlays mutates `base` in place via copy-on-write.
// Rows that are dimmed or spliced get their Cells slice replaced with a
// private copy; rows that are untouched keep their original Cells. Callers
// must not assume base is unmodified after this call.
func composeOverlays(base []core.Row, overlays []*Overlay, cols, rows int64) []core.Row { //nolint:gocognit // 渲染/分发/状态机复杂分支，拆分列入 P3
	if len(overlays) == 0 {
		return base
	}

	// CoW tracker: avoid deep-copying the entire base frame upfront.
	// Rows are deep-copied lazily, just before their first in-place mutation.
	// This cuts per-frame allocs from 61 to ~N+1 (where N = overlay rows).
	modified := make([]bool, len(base))
	cowRow := func(i int) {
		if i >= len(modified) || modified[i] || base[i].Cells == nil {
			return
		}
		cells := make([]core.Cell, len(base[i].Cells))
		copy(cells, base[i].Cells)
		base[i].Cells = cells
		modified[i] = true
	}

	// Ensure we have at least `rows` lines so bottom-anchored overlays land.
	for int64(len(base)) < rows {
		base = append(base, blankRow(cols))
		modified = append(modified, false) // fresh blank rows: never shared
	}

	// If any overlay dims the background, pad every (non-raw) base row out to
	// the full viewport width first. dimBackgroundRows / paintRowRange only
	// touch cells that already exist, so short rows would otherwise leave
	// their un-dimmed tail at the terminal's default background — producing
	// phantom rectangular "boxes" along the right/top of the screen where the
	// dimmed and un-dimmed regions meet. Padding to `cols` makes the frosted
	// glass effect uniform across the whole viewport.
	dims := false
	for _, ov := range overlays {
		if ov != nil && ov.DimBackground {
			dims = true
			break
		}
	}
	if dims {
		for i := range base {
			if base[i].IsRaw() {
				// Empty raw rows (Cells==nil, Raw=="") carry no opaque
				// payload — they're just blank lines. The dim layer skips
				// raw rows, so left as-is they stay at the terminal default
				// background and show up as bright horizontal strips that
				// bracket the dimmed content rows (phantom "box" outlines).
				// Convert them to real blank rows so they dim uniformly.
				// Rows with an actual Raw payload (e.g. Kitty graphics) are
				// left untouched.
				if base[i].Raw == "" {
					base[i] = blankRow(cols)
				}
				continue
			}
			preLen := len(base[i].Cells)
			base[i] = core.PadRow(base[i], cols, core.DefaultStyle)
			if len(base[i].Cells) != preLen {
				// PadRow allocated fresh cells when the row was shorter than
				// cols; mark modified so cowRow skips this row.
				modified[i] = true
			}
		}
	}

	// Apply dimming for overlays that request it.
	if dims {
		// dimBackgroundRows mutates cells in place. Ensure every non-raw row
		// has private cells before mutation. Rows that PadRow padded above
		// (shortened by the PadRow check len(cells) != preLen comparison)
		// already have fresh cells and are skipped by cowRow.
		for i := range base {
			if !base[i].IsRaw() {
				cowRow(i)
			}
		}
	}
	for _, ov := range overlays {
		if ov == nil || !ov.DimBackground {
			continue
		}
		w := ov.Width.resolve(cols)
		h := ov.Height.resolve(rows)
		origin := resolveOverlayOrigin(ov, cols, rows, w, h)
		base = dimBackgroundRows(base, cols, rows, origin.row, origin.col, w, h, ov.dimIntensity(time.Now()))
	}

	for _, ov := range overlays {
		if ov == nil || ov.Content == nil {
			continue
		}
		w := ov.Width.resolve(cols)
		h := ov.Height.resolve(rows)
		rawLines := ov.Content.Render(w)
		// Normalise height: pad short, truncate tall.
		for int64(len(rawLines)) < h {
			rawLines = append(rawLines, core.PadToWidth("", w))
		}
		if int64(len(rawLines)) > h {
			rawLines = rawLines[:h]
		}
		// Parse each rendered line into a Row, then enforce the overlay's
		// width: truncate over-wide rows (a misbehaving component returning
		// more columns than requested would otherwise spill past the overlay
		// region and corrupt the cell grid's column alignment) and pad
		// short rows so the overlay is a solid rectangle.
		content := make([]core.Row, len(rawLines))
		for i, ln := range rawLines {
			r := core.ParseLine(ln)
			// An empty content line parses to a Raw "" row. spliceOverlayRows
			// replaces the WHOLE base row for a raw overlay row, which would
			// wipe out the dimmed side margins on that line — leaving a bright
			// notch in the left/right dim columns (a panel that emits blank
			// lines, e.g. the TODO panel, otherwise shows this). Turn empty
			// lines into a solid w-wide blank so only the overlay columns are
			// overwritten and the margins survive.
			if r.IsRaw() && r.Raw == "" {
				r = blankRow(w)
			}
			r = core.TruncateRow(r, w)
			content[i] = core.PadRow(r, w, core.DefaultStyle)
		}

		origin := resolveOverlayOrigin(ov, cols, rows, w, h)
		// SlideUp 过渡：动画期间 origin 行从视口底部（屏外）向目标位置插值。
		// p=0 时面板完全在屏外（不可见），p=1 时到达目标位置。超出的行由
		// 渲染管线的行数裁剪处理，不会溢出视口。
		if ov.Transition.Kind == OverlayTransitionSlideUp {
			p, _ := ov.transitionProgress(time.Now())
			origin.row = origin.row + int64((1-p)*float64(rows))
		}
		ov.renderedRow = origin.row
		ov.renderedCol = origin.col
		// Record the rendered extent so TranslateMouse can map absolute
		// screen coordinates into the overlay's local space. Without these,
		// renderedWidth/Height stay 0 and TranslateMouse always reports
		// out-of-bounds — overlay content components would receive absolute
		// screen coordinates and their hit-testing would be wrong.
		ov.renderedWidth = w
		ov.renderedHeight = h
		// CoW rows that spliceOverlayRows will mutate in place.
		// If dims=true, all rows were already copied above (cowRow no-op).
		// If dims=false, only overlay rows get copied — the big win.
		for r := origin.row; r < origin.row+h && r < int64(len(base)); r++ {
			cowRow(int(r))
		}
		base = spliceOverlayRows(base, content, origin.row, origin.col, cols)
	}
	return base
}

// blankRow returns a Row of `cols` space cells in the default style.
func blankRow(cols int64) core.Row {
	cells := make([]core.Cell, cols)
	for i := range cells {
		cells[i] = core.Cell{Rune: ' ', Width: 1, Style: core.DefaultStyle}
	}
	return core.Row{Cells: cells, CursorCol: -1}
}

// dimBackgroundRows applies the frosted-glass effect to base rows at the cell
// level: every cell OUTSIDE the overlay region gets a dim attribute and the
// dark glass background, so the backdrop reads as a uniform dimmed layer.
// Cells under the overlay itself are left untouched (the overlay is spliced
// on top afterwards).
//
// intensity ∈ [0,1] 缩放变暗强度（Fade 动画期间从 0 渐变到 1）；1 为全强度
// （既有行为）。0 时跳过变暗（与无 dim 等价）。
//
// No drop shadow is drawn: the shadow ring (a slightly darker bg on the
// overlay's right column and bottom band) rendered as faint rectangular
// "box" edges against the dim backdrop — visually indistinguishable from an
// artifact — so the backdrop is kept perfectly uniform instead.
func dimBackgroundRows(base []core.Row, cols, rows, oRow, oCol, oW, oH int64, intensity float64) []core.Row {
	// Overlay rectangle: rows [ovTop, ovBot), cols [ovLeft, ovRight).
	ovTop, ovBot := oRow, oRow+oH
	ovLeft, ovRight := oCol, oCol+oW

	clamp := func(v int64) int64 {
		if v < 0 {
			return 0
		}
		if v > cols {
			return cols
		}
		return v
	}

	// 限定遍历范围到实际视口高度，跳过多余的 padding 行。
	n := int64(len(base))
	if rows < n {
		n = rows
	}
	for i := int64(0); i < n; i++ {
		r := i
		dim := func(a, b int64) {
			if a, b = clamp(a), clamp(b); b > a {
				base[i] = applyDimToRow(base[i], a, b, true, intensity)
			}
		}

		if r >= ovTop && r < ovBot {
			// Overlay row: dim the side margins, leave the overlay region.
			dim(0, ovLeft)
			dim(ovRight, cols)
		} else {
			// Fully outside the overlay: dim the whole row.
			dim(0, cols)
		}
	}
	return base
}

// applyDimToRow adds the dim attribute and a dark glass background to cells
// in the column range [start, end) of row. If row is Raw it is left as-is
// (dim doesn't compose cleanly with opaque content).
//
// intensity ∈ [0,1] 缩放变暗强度：≥1 为全强度（dim 属性 + 玻璃背景）；
// (0,1) 只加 dim 属性（两级近似，避免颜色混合的解析开销）；0 不修改。
func applyDimToRow(row core.Row, start, end int64, withBg bool, intensity float64) core.Row {
	if row.IsRaw() || intensity <= 0 {
		return row
	}
	// 部分强度时只加 dim 属性（终端属性无法连续混合，取两级近似）。
	full := intensity >= 1
	for c := start; c < end && c < int64(len(row.Cells)); c++ {
		cell := &row.Cells[c]
		if cell.IsContinuation() {
			// Continuation cell (right half of a wide char). Dim the primary
			// cell to the left so the glyph itself dims, but ALSO dim this
			// cell's own background. Otherwise the continuation half keeps
			// its original (often default) background and shows up as a
			// bright half-cell speckle across the dimmed region — most
			// visible with CJK text, where every wide char leaves a gap.
			if full && withBg {
				cell.Style.Bg = dimBgColor()
			}
			cell.Style.Attrs |= dimTextAttr
			if c > 0 {
				cell = &row.Cells[c-1]
			} else {
				continue
			}
		}
		cell.Style.Attrs |= dimTextAttr
		if full && withBg {
			cell.Style.Bg = dimBgColor()
		}
	}
	return row
}

// spliceOverlayRows copies overlay cells onto base at (row, col), handling
// wide-char boundaries correctly. When an overlay cell lands on the left
// half of a wide base cell, the right-half continuation is also cleared
// (replaced with a narrow space). When it lands on a continuation cell,
// the left-half primary is cleared too — preventing orphaned half-glyphs.
//
// Raw overlay rows (e.g. Kitty graphics APC) replace the corresponding base
// row entirely — an overlay is supposed to cover the base, not be hidden by
// it. Raw rows can't be cell-spliced because they carry opaque escape
// payloads with no column structure, so whole-row replacement is the only
// sound semantics.
func spliceOverlayRows(base []core.Row, content []core.Row, row, col, cols int64) []core.Row { //nolint:gocognit // 渲染/分发/状态机复杂分支，拆分列入 P3
	// Grow base vertically if needed.
	for int64(len(base)) < row+int64(len(content)) {
		base = append(base, blankRow(cols))
	}
	for i, srcRow := range content {
		targetIdx := row + int64(i)
		if targetIdx >= int64(len(base)) {
			break
		}
		target := &base[targetIdx]

		// Raw overlay row: replace the base row wholesale. Overlay semantics
		// say the overlay covers the base, so base-wins was wrong here.
		if srcRow.IsRaw() {
			base[targetIdx] = srcRow
			continue
		}

		if target.IsRaw() {
			// Base is raw (opaque) but overlay is cells. The overlay can't
			// be cell-spliced onto an opaque row, so the overlay wins by
			// replacing the row with a blank cell row first, then splicing.
			base[targetIdx] = blankRow(cols)
			target = &base[targetIdx]
		}

		// Ensure the target row has at least `cols` cells. Base rows
		// produced by component Render may be shorter than the viewport
		// width (short content lines, trailing blanks, recently-truncated
		// rows). Splicing overlay cells past the end of target.Cells would
		// panic with index-out-of-range; pad with default-style spaces so
		// the overlay can be placed anywhere within the viewport.
		if n := int64(len(target.Cells)); n < cols {
			padded := make([]core.Cell, cols)
			copy(padded, target.Cells)
			for i := n; i < cols; i++ {
				padded[i] = core.Cell{Rune: ' ', Width: 1, Style: core.DefaultStyle}
			}
			target.Cells = padded
		}

		colIdx := col
		for _, src := range srcRow.Cells {
			if src.IsContinuation() {
				// Continuation cells are emitted by the primary; skip.
				continue
			}
			if colIdx >= cols {
				break
			}
			// Make room: if we're about to overwrite a wide primary or its
			// continuation, normalise the affected base cells first.
			clearWideBoundary(target, colIdx, src.Width, cols)
			// Place the source cell.
			target.Cells[colIdx] = src
			if src.Width == 2 {
				// Place a continuation placeholder.
				if colIdx+1 < int64(len(target.Cells)) {
					target.Cells[colIdx+1] = core.Cell{Width: 0, Style: src.Style}
				}
				colIdx += 2
			} else {
				colIdx++
			}
		}
		// Preserve cursor marker from the source row if present.
		if srcRow.CursorCol >= 0 && target.CursorCol < 0 {
			target.CursorCol = srcRow.CursorCol + int(col)
		}
	}
	return base
}

// clearWideBoundary normalises base cells at colIdx so that placing a cell
// of width `incoming` there doesn't leave an orphaned half of a wide char.
// Specifically:
//   - If base[colIdx] is a wide primary (Width=2), its continuation at
//     colIdx+1 is replaced with a narrow space (the overlay cell overwrites
//     the primary; the continuation is no longer valid).
//   - If base[colIdx] is a continuation (Width=0), the primary at colIdx-1
//     is replaced with a narrow space (the overlay cell overwrites the
//     right half; the left half must yield).
//   - If incoming width is 2 and base[colIdx+1] is a wide primary, that
//     primary's continuation at colIdx+2 is replaced with a narrow space.
func clearWideBoundary(row *core.Row, colIdx int64, incoming int8, _ int64) {
	if row.IsRaw() || len(row.Cells) == 0 {
		return
	}
	n := int64(len(row.Cells))
	// Overwrite target: handle wide primary at colIdx.
	if colIdx < n && row.Cells[colIdx].Width == 2 {
		// Right-half continuation at colIdx+1 will be orphaned — replace it
		// with a narrow space so the row stays well-formed.
		if colIdx+1 < n {
			row.Cells[colIdx+1] = core.Cell{Rune: ' ', Width: 1, Style: row.Cells[colIdx].Style}
		}
	}
	// Overwrite target: handle continuation at colIdx (orphaning left half).
	if colIdx < n && row.Cells[colIdx].IsContinuation() && colIdx > 0 {
		// Left-half primary at colIdx-1 is now orphaned — collapse it to a
		// narrow space so we don't render a half-wide glyph.
		row.Cells[colIdx-1] = core.Cell{Rune: ' ', Width: 1, Style: row.Cells[colIdx].Style}
	}
	// Incoming wide char: if its continuation (colIdx+1) lands on a wide
	// primary, that primary's own continuation at colIdx+2 is orphaned.
	if incoming == 2 && colIdx+1 < n && row.Cells[colIdx+1].Width == 2 {
		if colIdx+2 < n {
			row.Cells[colIdx+2] = core.Cell{Rune: ' ', Width: 1, Style: row.Cells[colIdx+1].Style}
		}
	}
}
