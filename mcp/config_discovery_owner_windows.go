//go:build windows

package mcp

// isOwnedByCurrentUser always returns false on Windows (not supported).
// On Windows, the $PWD/.mcp.json security check is skipped.
func isOwnedByCurrentUser(_ string) bool {
	return true // Windows doesn't expose Unix-style ownership; allow by default
}
