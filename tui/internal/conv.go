// Package internal shared utilities for the TUI module.
//
// This package is internal to the TUI module and must not be imported by
// external consumers.
package internal

import "strconv"

// ITOA converts an int64 to its string representation.
// It exists as a shared utility to eliminate duplicate implementations
// across multiple tui sub-packages. Performance-sensitive callers in
// hot paths (render loop) should prefer strconv.FormatInt or this
// function; both allocate minimally.
func ITOA(n int64) string {
	return strconv.FormatInt(n, 10)
}
