//go:build windows

package tools

import "os/exec"

// configureCommand 在 Windows 上没有进程组语义，保持默认。
func configureCommand(_ *exec.Cmd) {}

// killCommandTree 结束直接子进程。
func killCommandTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
