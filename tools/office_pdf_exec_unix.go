//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
)

// configureCommand 让转换进程独占一个进程组。
//
// LibreOffice 会再拉起 soffice.bin：只杀直接子进程会留下孤儿，占着 profile 锁，
// 让下一次转换失败。
func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killCommandTree 结束整个进程组。
func killCommandTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
