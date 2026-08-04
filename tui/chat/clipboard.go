package chat

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

var clipboardCtx = context.Background()

// isSSHSession reports whether the current session is a remote SSH connection.
// Delegates to the terminal package's cached detection, which checks
// SSH_TTY, SSH_CONNECTION, and SSH_CLIENT exactly once at startup.
// 在 SSH 会话中，原生剪贴板工具（pbcopy/xclip/clip）操作的是服务端
// 剪贴板（无用），必须使用 OSC 52 转义序列写入客户端剪贴板。
func isSSHSession() bool {
	return terminal.CurrentTerminalContext().IsSSH
}

// CopyToClipboard writes text to the system clipboard.
// It tries native tools first (pbcopy/xclip/clip), then falls back to OSC 52
// for terminals that support it (iTerm2, Terminal.app, Kitty, WezTerm, Ghostty, VS Code, etc).
// In SSH sessions, native tools are skipped and OSC 52 is used directly.
func CopyToClipboard(text string) error {
	// In SSH sessions, skip native clipboard (which would write to the remote
	// machine) and use OSC 52 to reach the client's clipboard directly.
	if isSSHSession() {
		return copyOSC52(text)
	}

	// Try native platform tools first.
	err := copyNative(text)
	if err == nil {
		return nil
	}

	// Fallback to OSC 52 terminal escape sequence.
	return copyOSC52(text)
}

func copyNative(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(clipboardCtx, "pbcopy")
	case "linux":
		if p, _ := exec.LookPath("xclip"); p != "" {
			cmd = exec.CommandContext(clipboardCtx, "xclip", "-selection", "clipboard")
		} else if p, _ := exec.LookPath("xsel"); p != "" {
			cmd = exec.CommandContext(clipboardCtx, "xsel", "--clipboard", "--input")
		} else {
			return &core.ClipboardError{Op: "copy", Err: fmt.Errorf("no clipboard command found")}
		}
	case "windows":
		cmd = exec.CommandContext(clipboardCtx, "clip.exe")
	default:
		return &core.ClipboardError{Op: "copy", Err: fmt.Errorf("unsupported platform: %s", runtime.GOOS)}
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// copyOSC52 writes text to clipboard using the OSC 52 terminal escape sequence.
// Supported terminals: iTerm2, Terminal.app (macOS 13.4+), Kitty, WezTerm, Ghostty,
// Alacritty, VS Code, Cursor, foot, tmux (with allow-passthrough on), and many others.
func copyOSC52(text string) error {
	// OSC 52 format: ESC ] 52 ; c ; <base64> BEL (or ST)
	// 'c' = clipboard selection
	encoded := base64.StdEncoding.EncodeToString([]byte(text))

	// Truncate if too long (OSC 52 has practical limits ~100KB)
	const maxLen = 100_000
	if len(encoded) > maxLen {
		encoded = encoded[:maxLen]
	}

	// Write the OSC 52 sequence to stdout
	osc := fmt.Sprintf("\x1b]52;c;%s\x07", encoded)
	_, err := fmt.Print(osc)
	return err
}

// ReadFromClipboard reads text from the system clipboard.
// It tries native tools first (pbpaste/xclip/clip), then falls back to an
// empty string. OSC 52 readback is not implemented (terminal support is rare).
func ReadFromClipboard() (string, error) {
	return readNative()
}

func readNative() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(clipboardCtx, "pbpaste")
	case "linux":
		if p, _ := exec.LookPath("xclip"); p != "" {
			cmd = exec.CommandContext(clipboardCtx, "xclip", "-selection", "clipboard", "-o")
		} else if p, _ := exec.LookPath("xsel"); p != "" {
			cmd = exec.CommandContext(clipboardCtx, "xsel", "--clipboard", "--output")
		} else {
			return "", &core.ClipboardError{Op: "paste", Err: fmt.Errorf("no clipboard command found")}
		}
	case "windows":
		cmd = exec.CommandContext(clipboardCtx, "powershell", "-command", "Get-Clipboard")
	default:
		return "", &core.ClipboardError{Op: "paste", Err: fmt.Errorf("unsupported platform: %s", runtime.GOOS)}
	}
	out, err := cmd.Output()
	if err != nil {
		return "", &core.ClipboardError{Op: "paste", Err: fmt.Errorf("read clipboard: %w", err)}
	}
	return strings.TrimRight(string(out), "\n\r"), nil
}
