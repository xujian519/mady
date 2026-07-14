//go:build windows

package mcp

import (
	"fmt"
	"os/exec"
)

// setProcessGroup is a no-op on Windows; process-group handling is done by
// killProcessTree via taskkill.
func setProcessGroup(cmd *exec.Cmd) {}

// killProcessTree uses taskkill /T to terminate the process and its children.
func killProcessTree(pid int) error {
	return exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run()
}
