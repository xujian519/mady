//go:build windows

package mcp

// isOwnedByCurrentUser returns true on Windows since Unix-style ownership
// checks are not available. The $PWD/.mcp.json security check relies on
// content-hash trust validation instead (see config_trust.go).
func isOwnedByCurrentUser(_ string) bool {
	return true // Windows: allow by default; trust is verified via content hash
}
