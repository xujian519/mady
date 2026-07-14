//go:build !windows

package mcp

import (
	"os"
	"syscall"
)

// isOwnedByCurrentUser returns true if the file at path is owned by the
// current process's UID. Used to reject untrusted .mcp.json files in
// shared directories.
func isOwnedByCurrentUser(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true // file doesn't exist — not a security risk
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return stat.Uid == uint32(os.Getuid())
}
