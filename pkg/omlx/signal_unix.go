//go:build !windows

package omlx

import "syscall"

// osSignal returns the signal used for graceful process termination on Unix.
func osSignal() syscall.Signal {
	return syscall.SIGTERM
}
