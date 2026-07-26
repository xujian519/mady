package core

import "runtime"

// CaptureStack returns the current goroutine's stack trace as a string.
// It is used by PanicMsg to attach diagnostic information when a Cmd panics.
// The trace is truncated to 4 KB to avoid excessive memory allocation in
// the (rare) panic path.
func CaptureStack() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}
