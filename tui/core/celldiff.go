package core

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
func DiffRows(old, new []Row) []RowDiff {
	var out []RowDiff
	for i, n := range new {
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

// DiffFrame returns cell-level diffs for every row that changed between
// old and new. Rows only present in old are omitted; the caller is
// expected to clear trailing lines when new is shorter than old.
//
// Optimization: scans from both ends to find the common prefix and suffix
// of unchanged rows, then only diffs the middle section. In streaming
// scenarios where only 1-2 lines change per frame (out of 500+), this
// avoids ~499 RowsEqual calls per frame.
func DiffFrame(old, new []Row) []RowCellDiff {
	// Find common prefix — rows that are identical from the top.
	prefix := 0
	for prefix < len(old) && prefix < len(new) && RowsEqual(old[prefix], new[prefix]) {
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
	if len(old) == len(new) {
		for suffix < len(old)-prefix && suffix < len(new)-prefix {
			if !RowsEqual(old[len(old)-1-suffix], new[len(new)-1-suffix]) {
				break
			}
			suffix++
		}
	}

	oldEnd := len(old) - suffix
	newStart := prefix
	newEnd := len(new) - suffix

	var out []RowCellDiff
	for i := newStart; i < newEnd; i++ {
		n := new[i]
		if i >= oldEnd {
			// Row exists only in new (new rows appended).
			if n.IsRaw() {
				out = append(out, RowCellDiff{Row: int64(i), RawContent: n.Raw})
			} else {
				out = append(out, RowCellDiff{
					Row:      int64(i),
					Segments: []Segment{{StartCol: 0, Cells: n.Cells, AfterStyle: DefaultStyle}},
				})
			}
			continue
		}
		if !RowsEqual(old[i], n) {
			if old[i].IsRaw() || n.IsRaw() {
				// Raw rows lack Cells — fall back to full-row write.
				out = append(out, RowCellDiff{Row: int64(i), RawContent: n.Raw})
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
// If either row is a raw row, the whole new row is returned as a single
// segment. The returned diff may be empty when the rows are identical.
func DiffCells(old, new Row) RowCellDiff {
	if old.IsRaw() || new.IsRaw() {
		return RowCellDiff{
			Segments: []Segment{{StartCol: 0, Cells: new.Cells, AfterStyle: DefaultStyle}},
		}
	}

	maxLen := len(old.Cells)
	if len(new.Cells) > maxLen {
		maxLen = len(new.Cells)
	}

	// Find the first column where the rows differ.
	l := 0
	for l < maxLen {
		if l >= len(old.Cells) || l >= len(new.Cells) || !EqualCell(old.Cells[l], new.Cells[l]) {
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
		if r >= len(old.Cells) || r >= len(new.Cells) || !EqualCell(old.Cells[r], new.Cells[r]) {
			break
		}
		r--
	}
	if r < l {
		r = l
	}

	// Never split a wide character: expand the segment to the primary cell.
	l = adjustStart(old, new, l)
	r = adjustEnd(old, new, r)

	end := r + 1
	if end > len(new.Cells) {
		end = len(new.Cells)
	}

	after := DefaultStyle
	if end < len(new.Cells) {
		after = new.Cells[end].Style
	}

	cells := make([]Cell, end-l)
	copy(cells, new.Cells[l:end])

	var diff RowCellDiff
	diff.Segments = []Segment{{StartCol: int64(l), Cells: cells, AfterStyle: after}}
	if len(new.Cells) < len(old.Cells) {
		diff.ClearTail = true
		diff.TailStart = int64(len(new.Cells))
	}
	return diff
}

// adjustStart moves the left boundary to the primary cell of a wide
// character so we never emit only the continuation half.
func adjustStart(old, new Row, l int) int {
	if l <= 0 {
		return l
	}
	if l < len(new.Cells) && new.Cells[l].IsContinuation() {
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
// Note: this function exists for API symmetry with adjustStart and for
// future-proofing against alternative segment-boundary strategies.
func adjustEnd(old, new Row, r int) int {
	return r
}
