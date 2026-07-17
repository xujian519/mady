//go:build !darwin && !linux

package terminal

import "errors"

// Fallback stubs for platforms where termios ioctl details are not implemented.
//
// Design decision (2026-07-18): Mady's primary target platforms are macOS and
// Linux. Adding golang.org/x/term as a dependency solely for fringe platform
// (Windows, BSD, etc.) raw-mode support is not justified. Current fallback:
//   - ProcessTerminal returns an error from Start()
//   - Callers can use VirtualTerminal for tests
//   - getWinsize returns a reasonable default (80×24)
//
// If Windows TUI support becomes a requirement, migrate to golang.org/x/term.

type termios struct{}

func getTermios(fd uintptr) (termios, error) {
	return termios{}, errors.New("tui: raw mode not implemented on this platform")
}

func setTermios(fd uintptr, t *termios) error {
	return errors.New("tui: raw mode not implemented on this platform")
}

func makeRaw(orig termios) termios { return orig }

func getWinsize(fd uintptr) (cols, rows int64, err error) {
	return 80, 24, nil
}
