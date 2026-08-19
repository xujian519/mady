package core

import "sync"

// diffCellPool caches short Cell slices used by DiffCells for segment
// copies. The slices are small (typically 1-20 cells) and short-lived;
// pooling reduces GC pressure from per-frame segment allocations.
var diffCellPool = sync.Pool{
	New: func() any {
		cells := make([]Cell, 0, 32)
		return &cells
	},
}

// getDiffCells borrows a Cell slice with the given capacity from the pool.
func getDiffCells(capacity int) []Cell {
	if capacity > 32 {
		return make([]Cell, capacity)
	}
	ptr := diffCellPool.Get().(*[]Cell)
	cells := *ptr
	if capacity > len(cells) {
		// Reset length but keep backing array for capacity up to 32.
		return cells[:capacity]
	}
	return cells[:capacity]
}

// PutDiffCells returns a Cell slice to the pool. The slice must have been
// obtained from getDiffCells and must not be used again after calling put.
func PutDiffCells(cells []Cell) {
	// Only return slices with capacities we can reuse.
	if cap(cells) <= 32 {
		diffCellPool.Put(&cells)
	}
}

// ---------------------------------------------------------------------------
// Cell-level frame diff.
//
// DiffRows compares two frame buffers (slices of Row) and returns the rows
// that differ, ready for the renderer to re-emit. The comparison is
// stricter than the string model's: two rows are equal only if every cell
// matches (style + rune + combining + cursor col). This means rows that
// differ only in SGR encoding (e.g. "\x1b[31m" vs "\x1b[38;5;1m") but
// resolve to the same cell style are now treated as equal — the renderer
// skips them, saving bandwidth that the string diff couldn't.
// ---------------------------------------------------------------------------

// RowDiff is one changed row in a frame diff.
type RowDiff struct {
	Row     int64
	Content Row
}

// DiffRows returns every row index at which old and new differ, or where
// new is longer than old. Rows present in old but missing in new are not
// returned (the caller clears trailing lines separately).
func DiffRows(old, newRows []Row) []RowDiff {
	var out []RowDiff
	for i, n := range newRows {
		if i >= len(old) {
			out = append(out, RowDiff{Row: int64(i), Content: n})
			continue
		}
		if !RowsEqual(old[i], n) {
			out = append(out, RowDiff{Row: int64(i), Content: n})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Cell-level diff within a row.
//
// DiffFrame builds on DiffRows: for every row that changed, it computes the
// exact columns that changed instead of treating the whole row as dirty.
// This is the next step down from row-level diff and reduces terminal
// output bandwidth for small updates (e.g. a single cursor character, a
// spinner, or a streaming token).
// ---------------------------------------------------------------------------

// Segment describes one changed run of cells inside a row. AfterStyle is
// the style of the first unchanged cell that follows the segment (or the
// default style when the segment reaches the end of the row). It lets the
// renderer leave the terminal in the correct SGR state for the unchanged
// suffix.
type Segment struct {
	StartCol   int64
	Cells      []Cell
	AfterStyle Style
}

// RowCellDiff is the cell-level diff for a single row.
type RowCellDiff struct {
	Row        int64
	Segments   []Segment
	RawContent string // non-empty when old/new is a Raw row (fallback to full-row write)
	ClearTail  bool   // true when new row is shorter than old; caller must erase tail
	TailStart  int64  // first column to clear when ClearTail is true
}

// linkRowSentinel 是触发"全行重写"的 RawContent 哨兵：含 Links 的行由
// SerializeRow 全行序列化（OSC 8 注入依赖行级列游标，segment diff 无法
// 补全链接边界），渲染端见 tui/tui_render.go 的 RawContent 分支。
const linkRowSentinel = " "

// DiffFrame returns cell-level diffs for every row that changed between
// old and new. Rows only present in old are omitted; the caller is
// expected to clear trailing lines when new is shorter than old.
//
// Optimization: scans from both ends to find the common prefix and suffix
// of unchanged rows, then only diffs the middle section. In streaming
// scenarios where only 1-2 lines change per frame (out of 500+), this
// avoids ~499 RowsEqual calls per frame.
func DiffFrame(old, newFrame []Row) []RowCellDiff { //nolint:gocognit // 渲染/分发/状态机复杂分支，拆分列入 P3
	// Find common prefix — rows that are identical from the top.
	prefix := 0
	for prefix < len(old) && prefix < len(newFrame) && RowsEqual(old[prefix], newFrame[prefix]) {
		prefix++
	}

	// Find common suffix — rows that are identical from the bottom,
	// but only after the prefix to avoid double-counting.
	//
	// The suffix optimization is only valid when old and new have the same
	// length. When lengths differ (e.g., autocomplete appears/disappears,
	// editor resizes), matched suffix rows sit at different absolute screen
	// positions in old vs new. Skipping their re-emission would leave stale
	// content at the old positions, causing visual corruption.
	suffix := 0
	if len(old) == len(newFrame) {
		for suffix < len(old)-prefix && suffix < len(newFrame)-prefix {
			if !RowsEqual(old[len(old)-1-suffix], newFrame[len(newFrame)-1-suffix]) {
				break
			}
			suffix++
		}
	}

	oldEnd := len(old) - suffix
	newStart := prefix
	newEnd := len(newFrame) - suffix

	var out []RowCellDiff
	for i := newStart; i < newEnd; i++ {
		n := newFrame[i]
		if i >= oldEnd {
			// Row exists only in new (new rows appended).
			if n.IsRaw() {
				out = append(out, RowCellDiff{Row: int64(i), RawContent: n.Raw})
			} else if len(n.Links) > 0 {
				// 含链接的新行：全行重写（经 SerializeRow 注入 OSC 8）。
				out = append(out, RowCellDiff{Row: int64(i), RawContent: linkRowSentinel})
			} else {
				out = append(out, RowCellDiff{
					Row:      int64(i),
					Segments: []Segment{{StartCol: 0, Cells: n.Cells, AfterStyle: DefaultStyle}},
				})
			}
			continue
		}
		if !RowsEqual(old[i], n) {
			if old[i].IsRaw() || n.IsRaw() || len(old[i].Links) > 0 || len(n.Links) > 0 {
				switch {
				case !n.IsRaw():
					// n 是 cell 行。若含链接，用 RawContent 哨兵触发全行
					// SerializeRow（OSC 8 注入依赖行级列游标，segment diff
					// 无法补全边界）；否则全行 segment（原有行为）。
					if len(n.Links) > 0 {
						out = append(out, RowCellDiff{Row: int64(i), RawContent: linkRowSentinel})
					} else {
						// Old row was raw but the new row is cell-structured.
						// Emit the new cells as a full-row segment so the stale
						// raw content is actually replaced — previously this
						// produced RawContent=="" with no segments, leaving a
						// ghost of the old raw line on screen (P1-5).
						out = append(out, RowCellDiff{
							Row:      int64(i),
							Segments: []Segment{{StartCol: 0, Cells: n.Cells, AfterStyle: DefaultStyle}},
						})
					}
				case n.Raw == "":
					// New row is an empty raw row: clear the whole line.
					out = append(out, RowCellDiff{Row: int64(i), ClearTail: true, TailStart: 0})
				default:
					// Raw rows lack Cells — fall back to full-row write.
					out = append(out, RowCellDiff{Row: int64(i), RawContent: n.Raw})
				}
			} else {
				d := DiffCells(old[i], n)
				d.Row = int64(i)
				out = append(out, d)
			}
		}
	}
	return out
}

// DiffCells computes the smallest changed cell segment between old and new.
// The returned diff may be empty when the rows are identical. Raw rows are
// handled by DiffFrame before reaching this function (raw rows lack Cells),
// so a raw argument here is a caller error — return an empty diff rather
// than an empty segment that would render nothing (P2-15).
func DiffCells(old, newRow Row) RowCellDiff {
	if old.IsRaw() || newRow.IsRaw() {
		return RowCellDiff{}
	}

	maxLen := len(old.Cells)
	if len(newRow.Cells) > maxLen {
		maxLen = len(newRow.Cells)
	}

	// Find the first column where the rows differ.
	l := 0
	for l < maxLen {
		if l >= len(old.Cells) || l >= len(newRow.Cells) || !EqualCell(old.Cells[l], newRow.Cells[l]) {
			break
		}
		l++
	}
	if l == maxLen {
		return RowCellDiff{}
	}

	// Find the last column where the rows differ.
	r := maxLen - 1
	for r >= 0 {
		if r >= len(old.Cells) || r >= len(newRow.Cells) || !EqualCell(old.Cells[r], newRow.Cells[r]) {
			break
		}
		r--
	}
	if r < l {
		r = l
	}

	// Never split a wide character: expand the segment to the primary cell.
	l = adjustStart(old, newRow, l)
	r = adjustEnd(old, newRow, r)

	end := r + 1
	if end > len(newRow.Cells) {
		end = len(newRow.Cells)
	}

	after := DefaultStyle
	if end < len(newRow.Cells) {
		after = newRow.Cells[end].Style
	}

	cells := getDiffCells(end - l)
	copy(cells, newRow.Cells[l:end])

	var diff RowCellDiff
	diff.Segments = []Segment{{StartCol: int64(l), Cells: cells, AfterStyle: after}}
	if len(newRow.Cells) < len(old.Cells) {
		diff.ClearTail = true
		diff.TailStart = int64(len(newRow.Cells))
	}
	return diff
}

// adjustStart moves the left boundary to the primary cell of a wide
// character so we never emit only the continuation half.
func adjustStart(old, newRow Row, l int) int {
	if l <= 0 {
		return l
	}
	if l < len(newRow.Cells) && newRow.Cells[l].IsContinuation() {
		return l - 1
	}
	if l < len(old.Cells) && old.Cells[l].IsContinuation() {
		return l - 1
	}
	return l
}

// adjustEnd returns the right boundary unchanged.
// Reasoning: when r points to a continuation cell, the primary cell (at r-1)
// is guaranteed to be within [l, r] because continuation cells never differ
// independently of their primary. Expanding r further right would include
// unchanged cells, wasting terminal output.

func adjustEnd(old, newRow Row, r int) int {
	return r
}
