package component

import (
	"testing"
)

// This file holds table-driven unit tests for the low-level editing
// primitives defined in editor_edit.go: moveCursor, moveWord, the delete
// family (deleteBackward/Forward/WordBackward/WordForward/ToLineStart/
// ToLineEnd), and insertRune.
//
// Each test sets up an Editor with a known buffer + cursor position,
// invokes one primitive directly, and asserts on the resulting buffer
// text (GetValue), cursor position (row/col), and selection state.

// --- test helpers -----------------------------------------------------------

// setupEditorAt builds an Editor whose buffer holds `text` and whose cursor
// is placed at (row, col). Setting cursor to a specific rune column is
// required because SetValue moves the cursor to the end of the buffer.
func setupEditorAt(text string, row, col int64) *Editor {
	e := NewEditor(nil)
	e.SetValue(text)
	e.mu.Lock()
	e.row = row
	e.col = col
	e.mu.Unlock()
	return e
}

// cursorPos reads the current (row, col) cursor position under the read lock.
func cursorPos(e *Editor) (int64, int64) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.row, e.col
}

// allSelected reports the editor-scoped Select-All flag.
func allSelected(e *Editor) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.allSelected
}

// ===========================================================================
// moveCursor
// ===========================================================================

func TestEditorEditMoveCursor(t *testing.T) {
	t.Run("left within line", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 3)
		e.moveCursor(0, -1)
		if row, col := cursorPos(e); row != 0 || col != 2 {
			t.Fatalf("cursor = (%d,%d), want (0,2)", row, col)
		}
		if got := e.GetValue(); got != "hello" {
			t.Fatalf("buffer mutated: got %q", got)
		}
	})

	t.Run("right within line", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 3)
		e.moveCursor(0, 1)
		if row, col := cursorPos(e); row != 0 || col != 4 {
			t.Fatalf("cursor = (%d,%d), want (0,4)", row, col)
		}
	})

	t.Run("left at buffer start clamps", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 0)
		e.moveCursor(0, -1)
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("right at buffer end clamps", func(t *testing.T) {
		e := setupEditorAt("hi", 0, 2)
		e.moveCursor(0, 1)
		if row, col := cursorPos(e); row != 0 || col != 2 {
			t.Fatalf("cursor = (%d,%d), want (0,2)", row, col)
		}
	})

	t.Run("left at col 0 wraps to previous line end", func(t *testing.T) {
		e := setupEditorAt("ab\ncd", 1, 0)
		e.moveCursor(0, -1)
		if row, col := cursorPos(e); row != 0 || col != 2 {
			t.Fatalf("cursor = (%d,%d), want (0,2)", row, col)
		}
	})

	t.Run("right at line end wraps to next line start", func(t *testing.T) {
		e := setupEditorAt("ab\ncd", 0, 2)
		e.moveCursor(0, 1)
		if row, col := cursorPos(e); row != 1 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (1,0)", row, col)
		}
	})

	t.Run("up clamps row", func(t *testing.T) {
		e := setupEditorAt("ab\ncd", 1, 1)
		e.moveCursor(-1, 0)
		if row, col := cursorPos(e); row != 0 || col != 1 {
			t.Fatalf("cursor = (%d,%d), want (0,1)", row, col)
		}
		// Climb past row 0.
		e.moveCursor(-1, 0)
		if row, col := cursorPos(e); row != 0 || col != 1 {
			t.Fatalf("cursor = (%d,%d), want (0,1)", row, col)
		}
	})

	t.Run("down clamps row", func(t *testing.T) {
		e := setupEditorAt("ab\ncd", 0, 1)
		e.moveCursor(1, 0)
		if row, col := cursorPos(e); row != 1 || col != 1 {
			t.Fatalf("cursor = (%d,%d), want (1,1)", row, col)
		}
		// Descend past last row.
		e.moveCursor(1, 0)
		if row, col := cursorPos(e); row != 1 || col != 1 {
			t.Fatalf("cursor = (%d,%d), want (1,1)", row, col)
		}
	})

	t.Run("up clamps col to shorter line", func(t *testing.T) {
		// Cursor at col 5 on a 2-char line: moving up keeps row but the
		// vertical clamp narrows col to len(target line).
		e := setupEditorAt("xy\nshort", 1, 5)
		e.moveCursor(-1, 0)
		if row, col := cursorPos(e); row != 0 || col != 2 {
			t.Fatalf("cursor = (%d,%d), want (0,2)", row, col)
		}
	})

	t.Run("down clamps col to shorter line", func(t *testing.T) {
		e := setupEditorAt("short\nxy", 0, 5)
		e.moveCursor(1, 0)
		if row, col := cursorPos(e); row != 1 || col != 2 {
			t.Fatalf("cursor = (%d,%d), want (1,2)", row, col)
		}
	})

	t.Run("CJK rune counts as one column", func(t *testing.T) {
		e := setupEditorAt("你好世界", 0, 2)
		e.moveCursor(0, -1)
		if row, col := cursorPos(e); row != 0 || col != 1 {
			t.Fatalf("cursor = (%d,%d), want (0,1)", row, col)
		}
	})

	t.Run("clears Select-All flag", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 0)
		e.SelectAll()
		if !allSelected(e) {
			t.Fatal("SelectAll should set flag")
		}
		e.moveCursor(0, 1)
		if allSelected(e) {
			t.Fatal("moveCursor should clear allSelected")
		}
	})

	t.Run("resets lastKill", func(t *testing.T) {
		// After a kill (deleteToLineEnd sets lastKill=true), a cursor move
		// should reset it so a subsequent yankPop is a no-op.
		e := setupEditorAt("hello world", 0, 0)
		e.deleteToLineEnd() // kills "hello world", sets lastKill=true
		e.moveCursor(0, 0)
		// No direct getter for lastKill; assert indirectly via yankPop being
		// a no-op (buffer unchanged, killRing intact).
		before := e.GetValue()
		e.yankPop()
		if got := e.GetValue(); got != before {
			t.Fatalf("yankPop after moveCursor should be no-op: got %q want %q", got, before)
		}
	})
}

// ===========================================================================
// moveWord
// ===========================================================================

func TestEditorEditMoveWord(t *testing.T) {
	t.Run("word left in middle of word", func(t *testing.T) {
		// "hello world", cursor at col 8 (inside "world") -> start of "world" (col 6)
		e := setupEditorAt("hello world", 0, 8)
		e.moveWord(-1)
		if row, col := cursorPos(e); row != 0 || col != 6 {
			t.Fatalf("cursor = (%d,%d), want (0,6)", row, col)
		}
	})

	t.Run("word left across spaces lands on previous word start", func(t *testing.T) {
		// Cursor at col 6 (start of "world") -> skips the space, lands at start of "hello" (col 0)
		e := setupEditorAt("hello world", 0, 6)
		e.moveWord(-1)
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("word left at buffer start stays", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 0)
		e.moveWord(-1)
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("word left at col 0 of non-first row jumps to previous line end", func(t *testing.T) {
		e := setupEditorAt("foo\nbar", 1, 0)
		e.moveWord(-1)
		if row, col := cursorPos(e); row != 0 || col != 3 {
			t.Fatalf("cursor = (%d,%d), want (0,3)", row, col)
		}
	})

	t.Run("word right in middle of word jumps to word end", func(t *testing.T) {
		// "hello world", cursor at col 2 (inside "hello") -> end of "hello" (col 5, at space)
		e := setupEditorAt("hello world", 0, 2)
		e.moveWord(1)
		if row, col := cursorPos(e); row != 0 || col != 5 {
			t.Fatalf("cursor = (%d,%d), want (0,5)", row, col)
		}
	})

	t.Run("word right from space skips to next word end", func(t *testing.T) {
		// Cursor at col 5 (space) -> end of "world" (col 11)
		e := setupEditorAt("hello world", 0, 5)
		e.moveWord(1)
		if row, col := cursorPos(e); row != 0 || col != 11 {
			t.Fatalf("cursor = (%d,%d), want (0,11)", row, col)
		}
	})

	t.Run("word right at buffer end stays", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 5)
		e.moveWord(1)
		if row, col := cursorPos(e); row != 0 || col != 5 {
			t.Fatalf("cursor = (%d,%d), want (0,5)", row, col)
		}
	})

	t.Run("word right at line end jumps to next line start", func(t *testing.T) {
		e := setupEditorAt("foo\nbar", 0, 3)
		e.moveWord(1)
		if row, col := cursorPos(e); row != 1 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (1,0)", row, col)
		}
	})

	t.Run("word right across punctuation", func(t *testing.T) {
		// "foo.bar" — punctuation is not a word rune, so cursor at 0 jumps to col 3 (the '.')
		e := setupEditorAt("foo.bar", 0, 0)
		e.moveWord(1)
		if row, col := cursorPos(e); row != 0 || col != 3 {
			t.Fatalf("cursor = (%d,%d), want (0,3)", row, col)
		}
	})

	t.Run("CJK is treated as word runes", func(t *testing.T) {
		// "你好世界" all letters -> one word. moveWord(1) from col 1 lands at end (col 4).
		e := setupEditorAt("你好世界", 0, 1)
		e.moveWord(1)
		if row, col := cursorPos(e); row != 0 || col != 4 {
			t.Fatalf("cursor = (%d,%d), want (0,4)", row, col)
		}
	})

	t.Run("clears Select-All flag", func(t *testing.T) {
		e := setupEditorAt("hello world", 0, 0)
		e.SelectAll()
		e.moveWord(1)
		if allSelected(e) {
			t.Fatal("moveWord should clear allSelected")
		}
	})
}

// ===========================================================================
// deleteBackward
// ===========================================================================

func TestEditorEditDeleteBackward(t *testing.T) {
	t.Run("delete char before cursor", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 3)
		e.deleteBackward()
		if got := e.GetValue(); got != "helo" {
			t.Fatalf("value = %q, want %q", got, "helo")
		}
		if row, col := cursorPos(e); row != 0 || col != 2 {
			t.Fatalf("cursor = (%d,%d), want (0,2)", row, col)
		}
	})

	t.Run("delete at buffer start is no-op", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 0)
		e.deleteBackward()
		if got := e.GetValue(); got != "hello" {
			t.Fatalf("value = %q, want %q", got, "hello")
		}
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("delete at col 0 merges with previous line", func(t *testing.T) {
		e := setupEditorAt("foo\nbar", 1, 0)
		e.deleteBackward()
		if got := e.GetValue(); got != "foobar" {
			t.Fatalf("value = %q, want %q", got, "foobar")
		}
		if row, col := cursorPos(e); row != 0 || col != 3 {
			t.Fatalf("cursor = (%d,%d), want (0,3)", row, col)
		}
	})

	t.Run("delete on empty buffer is no-op", func(t *testing.T) {
		e := NewEditor(nil) // lines = [[]]
		e.deleteBackward()
		if got := e.GetValue(); got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
	})

	t.Run("deletes one CJK rune", func(t *testing.T) {
		e := setupEditorAt("你好", 0, 2)
		e.deleteBackward()
		if got := e.GetValue(); got != "你" {
			t.Fatalf("value = %q, want %q", got, "你")
		}
		if row, col := cursorPos(e); row != 0 || col != 1 {
			t.Fatalf("cursor = (%d,%d), want (0,1)", row, col)
		}
	})

	t.Run("with allSelected clears buffer", func(t *testing.T) {
		e := setupEditorAt("hello\nworld", 1, 5)
		e.SelectAll()
		e.deleteBackward()
		if got := e.GetValue(); got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
		if allSelected(e) {
			t.Fatal("allSelected should be cleared after delete")
		}
	})

	t.Run("fires onChange with new value", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 3)
		var changed string
		e.OnChange(func(v string) { changed = v })
		e.deleteBackward()
		if changed != "helo" {
			t.Fatalf("onChange received %q, want %q", changed, "helo")
		}
	})
}

// ===========================================================================
// deleteForward
// ===========================================================================

func TestEditorEditDeleteForward(t *testing.T) {
	t.Run("delete char after cursor", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 2)
		e.deleteForward()
		if got := e.GetValue(); got != "helo" {
			t.Fatalf("value = %q, want %q", got, "helo")
		}
		// Cursor position is unchanged when deleting forward.
		if row, col := cursorPos(e); row != 0 || col != 2 {
			t.Fatalf("cursor = (%d,%d), want (0,2)", row, col)
		}
	})

	t.Run("delete at end of last line is no-op", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 5)
		e.deleteForward()
		if got := e.GetValue(); got != "hello" {
			t.Fatalf("value = %q, want %q", got, "hello")
		}
		if row, col := cursorPos(e); row != 0 || col != 5 {
			t.Fatalf("cursor = (%d,%d), want (0,5)", row, col)
		}
	})

	t.Run("delete at line end merges with next line", func(t *testing.T) {
		e := setupEditorAt("foo\nbar", 0, 3)
		e.deleteForward()
		if got := e.GetValue(); got != "foobar" {
			t.Fatalf("value = %q, want %q", got, "foobar")
		}
		if row, col := cursorPos(e); row != 0 || col != 3 {
			t.Fatalf("cursor = (%d,%d), want (0,3)", row, col)
		}
	})

	t.Run("delete on empty buffer is no-op", func(t *testing.T) {
		e := NewEditor(nil)
		e.deleteForward()
		if got := e.GetValue(); got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
	})

	t.Run("deletes one CJK rune", func(t *testing.T) {
		e := setupEditorAt("你好", 0, 0)
		e.deleteForward()
		if got := e.GetValue(); got != "好" {
			t.Fatalf("value = %q, want %q", got, "好")
		}
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("with allSelected clears buffer", func(t *testing.T) {
		e := setupEditorAt("hello\nworld", 1, 5)
		e.SelectAll()
		e.deleteForward()
		if got := e.GetValue(); got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("fires onChange with new value", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 2)
		var changed string
		e.OnChange(func(v string) { changed = v })
		e.deleteForward()
		if changed != "helo" {
			t.Fatalf("onChange received %q, want %q", changed, "helo")
		}
	})
}

// ===========================================================================
// deleteWordBackward
// ===========================================================================

func TestEditorEditDeleteWordBackward(t *testing.T) {
	t.Run("delete word before cursor", func(t *testing.T) {
		// "hello world", cursor at end (col 11) -> removes "world", col -> 6
		e := setupEditorAt("hello world", 0, 11)
		e.deleteWordBackward()
		if got := e.GetValue(); got != "hello " {
			t.Fatalf("value = %q, want %q", got, "hello ")
		}
		if row, col := cursorPos(e); row != 0 || col != 6 {
			t.Fatalf("cursor = (%d,%d), want (0,6)", row, col)
		}
	})

	t.Run("delete word spanning preceding space", func(t *testing.T) {
		// Cursor at col 6 (start of "world") -> FindWordBoundaryLeft skips
		// the space then "hello", removing "hello " and leaving "world".
		e := setupEditorAt("hello world", 0, 6)
		e.deleteWordBackward()
		if got := e.GetValue(); got != "world" {
			t.Fatalf("value = %q, want %q", got, "world")
		}
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("delete word at buffer start is no-op", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 0)
		e.deleteWordBackward()
		if got := e.GetValue(); got != "hello" {
			t.Fatalf("value = %q, want %q", got, "hello")
		}
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("delete word at col 0 merges with previous line", func(t *testing.T) {
		e := setupEditorAt("foo\nbar", 1, 0)
		e.deleteWordBackward()
		if got := e.GetValue(); got != "foobar" {
			t.Fatalf("value = %q, want %q", got, "foobar")
		}
		if row, col := cursorPos(e); row != 0 || col != 3 {
			t.Fatalf("cursor = (%d,%d), want (0,3)", row, col)
		}
	})

	t.Run("deletes CJK word", func(t *testing.T) {
		// "你好世界" is one word (all letters). At end (col 4), removes all.
		e := setupEditorAt("你好世界", 0, 4)
		e.deleteWordBackward()
		if got := e.GetValue(); got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("with allSelected clears buffer", func(t *testing.T) {
		e := setupEditorAt("hello\nworld", 1, 5)
		e.SelectAll()
		e.deleteWordBackward()
		if got := e.GetValue(); got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
	})

	t.Run("pushes killed word onto kill ring", func(t *testing.T) {
		e := setupEditorAt("hello world", 0, 11)
		e.deleteWordBackward()
		// yank should paste back the most recently killed word ("world").
		e.yank()
		if got := e.GetValue(); got != "hello world" {
			t.Fatalf("after yank value = %q, want %q", got, "hello world")
		}
	})
}

// ===========================================================================
// deleteWordForward
// ===========================================================================

func TestEditorEditDeleteWordForward(t *testing.T) {
	t.Run("delete word after cursor", func(t *testing.T) {
		// "hello world", cursor at col 0 -> removes "hello", leaves " world"
		e := setupEditorAt("hello world", 0, 0)
		e.deleteWordForward()
		if got := e.GetValue(); got != " world" {
			t.Fatalf("value = %q, want %q", got, " world")
		}
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("delete word from space position skips to next word", func(t *testing.T) {
		// Cursor at col 5 (space) -> FindWordBoundaryRight skips space then "world", removes " world".
		e := setupEditorAt("hello world", 0, 5)
		e.deleteWordForward()
		if got := e.GetValue(); got != "hello" {
			t.Fatalf("value = %q, want %q", got, "hello")
		}
		if row, col := cursorPos(e); row != 0 || col != 5 {
			t.Fatalf("cursor = (%d,%d), want (0,5)", row, col)
		}
	})

	t.Run("delete word at end of last line is no-op", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 5)
		e.deleteWordForward()
		if got := e.GetValue(); got != "hello" {
			t.Fatalf("value = %q, want %q", got, "hello")
		}
	})

	t.Run("delete word at line end merges with next line", func(t *testing.T) {
		e := setupEditorAt("foo\nbar", 0, 3)
		e.deleteWordForward()
		if got := e.GetValue(); got != "foobar" {
			t.Fatalf("value = %q, want %q", got, "foobar")
		}
		if row, col := cursorPos(e); row != 0 || col != 3 {
			t.Fatalf("cursor = (%d,%d), want (0,3)", row, col)
		}
	})

	t.Run("deletes CJK word", func(t *testing.T) {
		e := setupEditorAt("你好世界", 0, 0)
		e.deleteWordForward()
		if got := e.GetValue(); got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
	})

	t.Run("with allSelected clears buffer", func(t *testing.T) {
		e := setupEditorAt("hello\nworld", 1, 5)
		e.SelectAll()
		e.deleteWordForward()
		if got := e.GetValue(); got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
	})

	t.Run("pushes killed word onto kill ring", func(t *testing.T) {
		e := setupEditorAt("hello world", 0, 0)
		e.deleteWordForward()
		// yank pastes the killed word ("hello") back at cursor (col 0).
		e.yank()
		if got := e.GetValue(); got != "hello world" {
			t.Fatalf("after yank value = %q, want %q", got, "hello world")
		}
	})
}

// ===========================================================================
// deleteToLineStart
// ===========================================================================

func TestEditorEditDeleteToLineStart(t *testing.T) {
	t.Run("delete from cursor to line start", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 3)
		e.deleteToLineStart()
		if got := e.GetValue(); got != "lo" {
			t.Fatalf("value = %q, want %q", got, "lo")
		}
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("delete at col 0 is no-op", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 0)
		e.deleteToLineStart()
		if got := e.GetValue(); got != "hello" {
			t.Fatalf("value = %q, want %q", got, "hello")
		}
		if row, col := cursorPos(e); row != 0 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (0,0)", row, col)
		}
	})

	t.Run("delete to start keeps rest of multi-line buffer", func(t *testing.T) {
		// Only the cursor's line prefix is removed; the following line is intact.
		e := setupEditorAt("foo\nbar", 1, 2)
		e.deleteToLineStart()
		if got := e.GetValue(); got != "foo\nr" {
			t.Fatalf("value = %q, want %q", got, "foo\nr")
		}
		if row, col := cursorPos(e); row != 1 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (1,0)", row, col)
		}
	})

	t.Run("delete full line prefix of CJK", func(t *testing.T) {
		e := setupEditorAt("你好世界", 0, 3)
		e.deleteToLineStart()
		if got := e.GetValue(); got != "界" {
			t.Fatalf("value = %q, want %q", got, "界")
		}
	})

	t.Run("with allSelected clears buffer", func(t *testing.T) {
		e := setupEditorAt("hello\nworld", 1, 5)
		e.SelectAll()
		e.deleteToLineStart()
		if got := e.GetValue(); got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
	})

	t.Run("pushes killed prefix onto kill ring", func(t *testing.T) {
		// "hello" at col 3 -> deleteToLineStart removes "hel" (cursor now at 0,
		// buffer "lo"). yank pastes "hel" back at cursor 0 -> "hello".
		e := setupEditorAt("hello", 0, 3)
		e.deleteToLineStart()
		e.yank()
		if got := e.GetValue(); got != "hello" {
			t.Fatalf("after yank value = %q, want %q", got, "hello")
		}
	})
}

// ===========================================================================
// deleteToLineEnd
// ===========================================================================

func TestEditorEditDeleteToLineEnd(t *testing.T) {
	t.Run("delete from cursor to line end", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 2)
		e.deleteToLineEnd()
		if got := e.GetValue(); got != "he" {
			t.Fatalf("value = %q, want %q", got, "he")
		}
		// Cursor stays at its column.
		if row, col := cursorPos(e); row != 0 || col != 2 {
			t.Fatalf("cursor = (%d,%d), want (0,2)", row, col)
		}
	})

	t.Run("delete at end of last line is no-op", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 5)
		e.deleteToLineEnd()
		if got := e.GetValue(); got != "hello" {
			t.Fatalf("value = %q, want %q", got, "hello")
		}
	})

	t.Run("delete at line end merges with next line", func(t *testing.T) {
		e := setupEditorAt("foo\nbar", 0, 3)
		e.deleteToLineEnd()
		if got := e.GetValue(); got != "foobar" {
			t.Fatalf("value = %q, want %q", got, "foobar")
		}
		if row, col := cursorPos(e); row != 0 || col != 3 {
			t.Fatalf("cursor = (%d,%d), want (0,3)", row, col)
		}
	})

	t.Run("delete suffix of CJK line", func(t *testing.T) {
		e := setupEditorAt("你好世界", 0, 2)
		e.deleteToLineEnd()
		if got := e.GetValue(); got != "你好" {
			t.Fatalf("value = %q, want %q", got, "你好")
		}
	})

	t.Run("with allSelected clears buffer", func(t *testing.T) {
		e := setupEditorAt("hello\nworld", 1, 5)
		e.SelectAll()
		e.deleteToLineEnd()
		if got := e.GetValue(); got != "" {
			t.Fatalf("value = %q, want empty", got)
		}
	})

	t.Run("pushes killed suffix onto kill ring", func(t *testing.T) {
		e := setupEditorAt("hello", 0, 2)
		e.deleteToLineEnd()
		e.yank()
		if got := e.GetValue(); got != "hello" {
			t.Fatalf("after yank value = %q, want %q", got, "hello")
		}
	})
}

// ===========================================================================
// insertRune
// ===========================================================================

func TestEditorEditInsertRune(t *testing.T) {
	t.Run("insert printable rune at cursor", func(t *testing.T) {
		e := setupEditorAt("ac", 0, 1)
		e.insertRune('b')
		if got := e.GetValue(); got != "abc" {
			t.Fatalf("value = %q, want %q", got, "abc")
		}
		if row, col := cursorPos(e); row != 0 || col != 2 {
			t.Fatalf("cursor = (%d,%d), want (0,2)", row, col)
		}
	})

	t.Run("insert newline splits the line", func(t *testing.T) {
		e := setupEditorAt("abc", 0, 1)
		e.insertRune('\n')
		if got := e.GetValue(); got != "a\nbc" {
			t.Fatalf("value = %q, want %q", got, "a\nbc")
		}
		if row, col := cursorPos(e); row != 1 || col != 0 {
			t.Fatalf("cursor = (%d,%d), want (1,0)", row, col)
		}
	})

	t.Run("insert at end of line", func(t *testing.T) {
		e := setupEditorAt("ab", 0, 2)
		e.insertRune('c')
		if got := e.GetValue(); got != "abc" {
			t.Fatalf("value = %q, want %q", got, "abc")
		}
		if row, col := cursorPos(e); row != 0 || col != 3 {
			t.Fatalf("cursor = (%d,%d), want (0,3)", row, col)
		}
	})

	t.Run("insert into empty buffer", func(t *testing.T) {
		e := NewEditor(nil)
		e.insertRune('x')
		if got := e.GetValue(); got != "x" {
			t.Fatalf("value = %q, want %q", got, "x")
		}
		if row, col := cursorPos(e); row != 0 || col != 1 {
			t.Fatalf("cursor = (%d,%d), want (0,1)", row, col)
		}
	})

	t.Run("insert CJK rune", func(t *testing.T) {
		e := setupEditorAt("好", 0, 0)
		e.insertRune('你')
		if got := e.GetValue(); got != "你好" {
			t.Fatalf("value = %q, want %q", got, "你好")
		}
		if row, col := cursorPos(e); row != 0 || col != 1 {
			t.Fatalf("cursor = (%d,%d), want (0,1)", row, col)
		}
	})

	t.Run("insert replaces allSelected content", func(t *testing.T) {
		e := setupEditorAt("hello\nworld", 1, 5)
		e.SelectAll()
		e.insertRune('x')
		if got := e.GetValue(); got != "x" {
			t.Fatalf("value = %q, want %q", got, "x")
		}
		if row, col := cursorPos(e); row != 0 || col != 1 {
			t.Fatalf("cursor = (%d,%d), want (0,1)", row, col)
		}
		if allSelected(e) {
			t.Fatal("allSelected should be cleared after insert")
		}
	})

	t.Run("fires onChange with new value", func(t *testing.T) {
		e := setupEditorAt("ac", 0, 1)
		var changed string
		e.OnChange(func(v string) { changed = v })
		e.insertRune('b')
		if changed != "abc" {
			t.Fatalf("onChange received %q, want %q", changed, "abc")
		}
	})

	t.Run("clears mouse-drag selection", func(t *testing.T) {
		// A non-empty mouse selection should be cleared by insertRune.
		// We simulate by setting selection state directly (the public path
		// is via MouseMsg, but the primitive must still reset the flags).
		e := setupEditorAt("hello", 0, 0)
		e.mu.Lock()
		e.selActive = true
		e.selDragging = true
		e.selStart = editorSelPos{row: 0, col: 0}
		e.selEnd = editorSelPos{row: 0, col: 3}
		e.mu.Unlock()
		e.insertRune('!')
		e.mu.RLock()
		selActive := e.selActive
		selDragging := e.selDragging
		e.mu.RUnlock()
		if selActive || selDragging {
			t.Fatalf("mouse selection should be cleared: selActive=%v selDragging=%v", selActive, selDragging)
		}
	})
}
