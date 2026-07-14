//go:build !windows

package mcp

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child process in its own process group so that
// killProcessTree can terminate the whole tree (including grand-children
// spawned by wrappers such as npx/npm exec) without affecting mady itself.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessTree terminates the direct child and its whole process group.
// It tolerates processes that are already dead.
func killProcessTree(pid int) error {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	return syscall.Kill(pid, syscall.SIGKILL)
}
