// computer_use_safety.go：危险操作拦截与人工审批，纯逻辑、平台无关。
// 职责：危险键组合黑名单（按平台过滤）、危险输入文本模式（curl|sh、rm -rf、fork bomb 等）、
// COMPUTER_USE_APPROVAL 环境变量驱动的审批模式（none/once/session）。

package desktop

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

var blockedKeyCombos = []struct {
	keys     string
	reason   string
	platform string // "darwin", "windows", "linux", or "" for all
}{
	// macOS-specific
	{"cmd+shift+backspace", "Empty Trash — blocked for safety", "darwin"},
	{"cmd+option+backspace", "Force Delete — blocked for safety", "darwin"},
	{"cmd+ctrl+q", "Lock Screen — blocked for safety", "darwin"},
	{"cmd+shift+q", "Log Out — blocked for safety", "darwin"},
	{"cmd+option+shift+q", "Force Log Out — blocked for safety", "darwin"},

	// Windows-specific
	{"super+r", "Run dialog — blocked for safety", "windows"},
	{"win+r", "Run dialog — blocked for safety", "windows"},
	{"ctrl+shift+esc", "Task Manager — blocked for safety", "windows"},
	{"ctrl+alt+del", "Security screen — blocked for safety", "windows"},
	{"alt+f4", "Close window (may lose work) — blocked for safety", "windows"},
	{"super+x", "Power menu — blocked for safety", "windows"},
	{"win+x", "Power menu — blocked for safety", "windows"},

	// Linux-specific
	{"ctrl+alt+del", "System reboot dialog — blocked for safety", "linux"},
	{"ctrl+alt+f2", "TTY switch — blocked for safety", "linux"},
	{"alt+f2", "Run dialog — blocked for safety", "linux"},
	{"alt+f4", "Close window (may lose work) — blocked for safety", "linux"},
	{"ctrl+alt+t", "Terminal — blocked for safety", "linux"},

	// Cross-platform shell execution patterns (checked by type pattern matcher)
}

var blockedTypePatterns = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`(?i)curl\s+.*?\|\s*(ba|z|k)?sh\b`), "Piped curl-to-shell execution — blocked for safety"},
	{regexp.MustCompile(`(?i)wget\s+.*?\|\s*(ba|z|k)?sh\b`), "Piped wget-to-shell execution — blocked for safety"},
	{regexp.MustCompile(`(?i)sudo\s+rm\s+-[rfv]+\s+`), "Destructive rm — blocked for safety"},
	{regexp.MustCompile(`(?i)rm\s+-[rfv]+\s+/`), "Root filesystem deletion — blocked for safety"},
	{regexp.MustCompile(`:\(\s*\)?\s*\{[^}]*:[^}]*\}\s*;?\s*:`), "Fork bomb — blocked for safety"},
}

func checkBlockedTypePattern(text string) error {
	for _, bp := range blockedTypePatterns {
		if bp.re.MatchString(text) {
			return fmt.Errorf("BLOCKED: %s", bp.reason)
		}
	}
	return nil
}

func checkBlockedKeyCombo(keys string) error {
	normalized := strings.ToLower(strings.TrimSpace(keys))
	for _, bk := range blockedKeyCombos {
		if bk.platform != "" && bk.platform != runtime.GOOS {
			continue
		}
		if strings.ReplaceAll(normalized, " ", "") == strings.ReplaceAll(bk.keys, " ", "") {
			return fmt.Errorf("BLOCKED: %s", bk.reason)
		}
		parts := strings.Split(bk.keys, "+")
		userParts := strings.Split(normalized, "+")
		if len(parts) == len(userParts) {
			match := true
			for _, up := range userParts {
				up = strings.TrimSpace(up)
				found := false
				for _, bp := range parts {
					bp = strings.TrimSpace(bp)
					if up == bp {
						found = true
						break
					}
				}
				if !found {
					match = false
					break
				}
			}
			if match {
				return fmt.Errorf("BLOCKED: %s", bk.reason)
			}
		}
	}
	return nil
}

type approvalLevel int

const (
	approvalNone approvalLevel = iota
	approvalOnce
	approvalSession
)

var (
	approvalMode approvalLevel
	approvalSeen map[string]bool
	approvalMu   sync.Mutex
)

func initApprovalMode() {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("COMPUTER_USE_APPROVAL")))
	switch mode {
	case "once":
		approvalMode = approvalOnce
	case "session":
		approvalMode = approvalSession
	case "none", "":
		approvalMode = approvalNone
	default:
		approvalMode = approvalNone
	}
	approvalSeen = make(map[string]bool)
}

func isDestructiveAction(action string) bool {
	switch action {
	case "click", "double_click", "right_click", "middle_click", "drag",
		"type", "key", "scroll", "set_value", "focus_app":
		return true
	}
	return false
}
