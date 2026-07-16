// computer_use_exec.go：外部命令执行助手。
// 职责：统一封装 osascript / cliclick / pwsh / xdotool / ydotool / wtype 的调用，
// 捕获 stdout/stderr 并包装错误；scrotExec 在 Linux 下依次尝试 grim/import/gnome-screenshot/scrot。

package tools

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

func osaExec(script string) (string, error) {
	cmd := exec.Command("osascript", "-e", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("osascript: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func cliclickExec(args ...string) (string, error) {
	cmd := exec.Command("cliclick", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cliclick: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

var (
	pwshBinary string
	pwshOnce   sync.Once
)

func getPwshBinary() string {
	pwshOnce.Do(func() {
		if _, err := exec.LookPath("pwsh"); err == nil {
			pwshBinary = "pwsh"
		} else {
			pwshBinary = "powershell"
		}
	})
	return pwshBinary
}

func pwshExec(script string) (string, error) {
	binary := getPwshBinary()
	cmd := exec.Command(binary, "-NoProfile", "-NonInteractive", "-Command", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w\nstderr: %s", binary, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func xdoExec(args ...string) (string, error) {
	cmd := exec.Command("xdotool", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("xdotool: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func ydoExec(args ...string) (string, error) {
	cmd := exec.Command("ydotool", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ydotool: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func wtypeExec(text string) (string, error) {
	cmd := exec.Command("wtype", text)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("wtype: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func scrotExec(path string) error {
	if isWayland() {
		if err := exec.Command("grim", path).Run(); err == nil {
			return nil
		}
		return fmt.Errorf("no Wayland screenshot tool found (try: apt install grim)")
	}
	if err := exec.Command("import", "-window", "root", path).Run(); err == nil {
		return nil
	}
	if err := exec.Command("gnome-screenshot", "-f", path).Run(); err == nil {
		return nil
	}
	if err := exec.Command("scrot", path).Run(); err == nil {
		return nil
	}
	return fmt.Errorf("no screenshot tool found (try: apt install imagemagick, gnome-screenshot, grim, or scrot)")
}
