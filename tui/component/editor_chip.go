package component

// editor_chip.go — Chip (inline token) data model for the editor.
//
// Chips are structured inline elements that render as styled capsules within
// the editor text. They represent file references (@file:foo.go), folder
// references (@folder:src/), or other structured tokens. The chip model lives
// alongside the editor's rune buffer: we store chips as a parallel array
// with byte positions into the hard line, and rendering interleaves chip
// visuals with plain text.
//
// Cursor movement skips across chips atomically, and backspace/delete
// removes the entire chip at once.

import "github.com/xujian519/mady/tui/theme"

// ChipKind identifies the type of a chip.
type ChipKind int

const (
	ChipFile    ChipKind = iota // @file: reference
	ChipFolder                  // @folder: reference
	ChipSession                 // @session: reference
)

// Chip represents an inline token rendered in the editor.
type Chip struct {
	Kind    ChipKind
	Value   string // the raw value (e.g., "main.go")
	Display string // rendered label (e.g., "@file:main.go")
	Style   theme.Style
}

// ChipPosition records where a chip appears in the buffer.
type ChipPosition struct {
	HardRow   int64 // index into editor.lines
	RuneStart int64 // rune offset within the hard row where the chip begins
	RuneEnd   int64 // rune offset after the chip (exclusive)
	Chip      *Chip
}

// chipState manages the chip lifecycle for the editor.
type chipState struct {
	chips []ChipPosition // sorted by (HardRow, RuneStart)
}

// InsertChip adds a chip at the given (row, col) rune position in the buffer.
// Returns the number of runes the chip occupies in the buffer (always 0 for
// chips — they are rendered inline but don't occupy the text buffer).
func (cs *chipState) InsertChip(row, col int64, chip *Chip) ChipPosition {
	cp := ChipPosition{
		HardRow:   row,
		RuneStart: col,
		RuneEnd:   col, // chips don't occupy rune space
		Chip:      chip,
	}
	cs.chips = append(cs.chips, cp)
	cs.sort()
	return cp
}

// RemoveChipAt removes a chip at the given buffer position. Returns true if found.
func (cs *chipState) RemoveChipAt(row, col int64) bool {
	for i, cp := range cs.chips {
		if cp.HardRow == row && col >= cp.RuneStart && col <= cp.RuneEnd {
			cs.chips = append(cs.chips[:i], cs.chips[i+1:]...)
			return true
		}
	}
	return false
}

// ChipAt returns the chip whose start position matches the given position.
// Uses exact RuneStart match (not range match). For zero-width chips this is
// equivalent; for non-zero-width chips use RemoveChipAt for range-based lookup.
func (cs *chipState) ChipAt(row, col int64) (*ChipPosition, bool) {
	for i := range cs.chips {
		cp := &cs.chips[i]
		if cp.HardRow == row && cp.RuneStart == col {
			return cp, true
		}
	}
	return nil, false
}

// Clear removes all chips.
func (cs *chipState) Clear() {
	cs.chips = nil
}

// sort ensures chips are ordered by (HardRow, RuneStart).
func (cs *chipState) sort() {
	for i := 1; i < len(cs.chips); i++ {
		for j := i; j > 0; j-- {
			a, b := cs.chips[j-1], cs.chips[j]
			if a.HardRow > b.HardRow || (a.HardRow == b.HardRow && a.RuneStart > b.RuneStart) {
				cs.chips[j-1], cs.chips[j] = b, a
			}
		}
	}
}

// ShiftAfter shifts all chip positions on or after (row, col) by delta runes.
// positive delta = insertion, negative delta = deletion. Used to keep chip
// positions in sync with buffer edits.
func (cs *chipState) ShiftAfter(row, col, delta int64) {
	for i := range cs.chips {
		cp := &cs.chips[i]
		if cp.HardRow > row {
			// Chips on later rows shift with line insertion/deletion.
			cp.HardRow += delta
			if cp.HardRow < 0 {
				cp.HardRow = 0
			}
			continue
		}
		if cp.HardRow != row {
			continue
		}
		if cp.RuneStart >= col {
			cp.RuneStart += delta
			if cp.RuneStart < col && delta < 0 {
				cp.RuneStart = col
			}
		}
		// cp.RuneEnd stays equal to RuneStart for zero-width chips.
		cp.RuneEnd = cp.RuneStart
	}
}

// adjustChipsLineMerge adjusts chip positions when the content of hardRow is
// appended to the previous line (a line-merge or line-join operation). It
// offsets chips on hardRow by offsetRunes and shifts all chips on subsequent
// rows up by one. This keeps chip positions in sync when deleteBackward,
// deleteForward, deleteWordBackward, deleteWordForward, or deleteToLineEnd
// merge two lines into one.
func adjustChipsLineMerge(cs chipState, hardRow, offsetRunes int64) {
	for i := range cs.chips {
		cp := &cs.chips[i]
		if cp.HardRow == hardRow {
			// Chip on the merged row: advance by offsetRunes (the length of
			// the preceding content that was prepended by the merge).
			cp.RuneStart += offsetRunes
			cp.RuneEnd = cp.RuneStart
		} else if cp.HardRow > hardRow {
			// Chip on a subsequent row: shift up by one (the merged row was
			// removed from the line array).
			cp.HardRow--
		}
	}
}

// cloneChips returns a deep copy of a ChipPosition slice.
func cloneChips(src []ChipPosition) []ChipPosition {
	if src == nil {
		return nil
	}
	dst := make([]ChipPosition, len(src))
	copy(dst, src)
	// Chip pointers are not deep-copied (chips are read-only after creation).
	return dst
}

// chipKindPrefix returns the display prefix for a chip kind.
func chipKindPrefix(kind ChipKind) string {
	switch kind {
	case ChipFile:
		return "@file:"
	case ChipFolder:
		return "@folder:"
	case ChipSession:
		return "@session:"
	default:
		return "@"
	}
}
