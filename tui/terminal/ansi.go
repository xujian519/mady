package terminal

import "strconv"

// ---------------------------------------------------------------------------
// ANSI escape sequence builders
//
// These are pure functions that return ANSI control strings. They do not
// perform I/O themselves — callers write the result to a terminal or writer.
// Moved from the theme package; these are terminal I/O primitives, not
// styling concerns.
// ---------------------------------------------------------------------------

// ANSI escape sequence constants.
const (
	Esc   = "\033[" // CSI introducer
	Reset = Esc + "0m"
)

// HideCursor hides the cursor.
func HideCursor() string { return Esc + "?25l" }
func ShowCursor() string { return Esc + "?25h" }

// ClearFromCursorDown clears from the cursor to the end of the screen.
func ClearFromCursorDown() string { return Esc + "0J" }

// CursorHome returns "ESC[H" — move cursor to row 1, column 1.
func CursorHome() string { return Esc + "H" }

// CursorPosition returns "ESC[row;colH" — absolute cursor positioning (CUP).
func CursorPosition(row, col int64) string {
	return Esc + strconv.FormatInt(row, 10) + ";" + strconv.FormatInt(col, 10) + "H"
}

// ClearToEndOfLine clears from cursor to end of line.
func ClearToEndOfLine() string { return Esc + "0K" }

// SetWindowTitle returns an OSC escape sequence to set the terminal window
// title (icon name + window title). Uses OSC 0 sequence: ESC ] 0 ; title BEL.
// This is widely supported by xterm, iTerm2, Kitty, WezTerm, and Terminal.app.
func SetWindowTitle(title string) string {
	return "\x1b]0;" + title + "\x07"
}
