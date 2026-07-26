package core

// ---------------------------------------------------------------------------
// TUI error types
//
// These replace ad-hoc fmt.Errorf("tui: ...") with typed errors that
// callers can switch on for conditional handling (retry, degrade, or
// report). Every type here has at least one verified consumer.
//
// Sprint 1 deleted an earlier errors.go (TermError/NetError/LogicError)
// because it had zero consumers. These types are introduced with consumers
// identified upfront.
// ---------------------------------------------------------------------------

import "fmt"

// TerminalError is returned when terminal I/O operations fail (termios,
// stdin, stdout). The caller (lifecycle, event loop) can inspect this type
// to decide whether to attempt recovery or abort.
type TerminalError struct {
	Op  string // e.g. "start", "stop", "restore termios"
	Err error  // wrapped underlying error (may be nil for sentinel use)
}

func (e *TerminalError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("tui: terminal %s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("tui: terminal %s", e.Op)
}

func (e *TerminalError) Unwrap() error { return e.Err }

// ClipboardError wraps clipboard operation failures. The TUI's PrintError
// handler can format these differently (e.g. a suffix suggesting the user
// check their clipboard manager) vs unexpected internal errors.
type ClipboardError struct {
	Op  string // "copy" or "paste"
	Err error
}

func (e *ClipboardError) Error() string {
	return fmt.Sprintf("tui: clipboard %s: %v", e.Op, e.Err)
}

func (e *ClipboardError) Unwrap() error { return e.Err }
