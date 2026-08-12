// computer_use_macos.go：macOS 回退后端实现（cliclick / osascript）。
// 职责：screencapture 截屏、点击/双击/右键/中键、拖拽、文本输入、按键组合、
// 滚动、set_value（输入+回车回退）、应用列表、窗口聚焦与窗口边界查询。

package desktop

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// macOS 虚拟键码常量（用于 AppleScript key code 命令）
const (
	osaPageUp   = 116
	osaPageDown = 121
)

func fallbackInfo() (any, error) {
	info, err := osaExec(`tell application "System Events"
		set screenRes to bounds of window of desktop
		set cursorPos to position of mouse
		set frontApp to name of first process whose frontmost is true
		set allApps to name of every process whose background only is false
		set appCount to count of allApps
		return "Screen: " & (item 1 of screenRes as text) & "x" & (item 2 of screenRes as text) & ¬
			" | Cursor: (" & (item 1 of cursorPos as text) & ", " & (item 2 of cursorPos as text) & ")" & ¬
			" | Frontmost: " & frontApp & ¬
			" | Running apps: " & (appCount as text)
	end tell`)
	if err != nil {
		return nil, fmt.Errorf("screen info failed: %w", err)
	}
	return result(info+"\nBackend: osascript", nil)
}

func fallbackCapture(backend cuBackend, appName, mode string) (any, error) {
	screenshotPath := filepath.Join(os.TempDir(), fmt.Sprintf("mady_cu_%d.png", time.Now().UnixNano()))
	var args []string
	if appName != "" {
		bounds, err := getWindowBounds(appName)
		if err == nil {
			args = append(args, "-R", bounds)
		}
	}
	args = append(args, "-x", screenshotPath)
	if err := exec.Command("screencapture", args...).Run(); err != nil { //nolint:gosec,noctx // G204: screencapture by design for desktop control
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	data, err := os.ReadFile(screenshotPath) //nolint:gosec // G304: path from temp dir managed by tool
	os.Remove(screenshotPath)                //nolint:gosec // G104: cleanup-only; best-effort remove temp file
	if err != nil {
		return nil, fmt.Errorf("read screenshot: %w", err)
	}

	if mode == "ax" {
		info, _ := osaExec(`tell app "System Events" to get name of first process whose frontmost is true`)
		return result(
			fmt.Sprintf("Screenshot captured (%d bytes). AX tree not available without cua-driver.", len(data)),
			map[string]any{
				"size_bytes":           len(data),
				"screenshot_available": true,
				"frontmost_app":        info,
			},
		)
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	return result(
		fmt.Sprintf("Screenshot captured (%d bytes) via %s", len(data), backend),
		map[string]any{
			"image_base64": b64,
			"format":       "png",
			"size_bytes":   len(data),
		},
	)
}

func fallbackClick(backend cuBackend, action string, x, y int) (string, error) {
	if backend == cuBackendCliclick {
		// cliclick doesn't support middle_click; fall through to osascript
		if action == "middle_click" {
			return osaClick(action, x, y)
		}
		var cliAction string
		switch action {
		case "double_click":
			cliAction = "dc"
		case "right_click":
			cliAction = "rc"
		default:
			cliAction = "c"
		}
		if err := cliclickExec(fmt.Sprintf("%s:%d,%d", cliAction, x, y)); err != nil {
			return "", fmt.Errorf("%s: %w", action, err)
		}
		return fmt.Sprintf("%s at (%d, %d) via cliclick", action, x, y), nil
	}

	return osaClick(action, x, y)
}

func osaClick(action string, x, y int) (string, error) {
	var clickType string
	switch action {
	case "double_click":
		clickType = "double click"
	case "right_click":
		clickType = "click button 2"
	case "middle_click":
		clickType = "click button 3"
	default:
		clickType = "click"
	}
	if _, err := osaExec(fmt.Sprintf(`tell app "System Events" to %s at {%d, %d}`, clickType, x, y)); err != nil {
		return "", fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Sprintf("%s at (%d, %d) via osascript", action, x, y), nil
}

func fallbackDrag(backend cuBackend, x1, y1, x2, y2 int) (string, error) {
	if backend == cuBackendCliclick {
		if err := cliclickExec(
			fmt.Sprintf("dd:%d,%d", x1, y1),
			fmt.Sprintf("du:%d,%d", x2, y2),
		); err != nil {
			return "", fmt.Errorf("drag: %w", err)
		}
		return fmt.Sprintf("Dragged from (%d,%d) to (%d,%d) via cliclick", x1, y1, x2, y2), nil
	}

	script := fmt.Sprintf(`tell application "System Events"
		set mouseDown at {%d, %d}
		delay 0.1
		set mouseUp at {%d, %d}
	end tell`, x1, y1, x2, y2)
	if _, err := osaExec(script); err != nil {
		return "", fmt.Errorf("drag: %w", err)
	}
	return fmt.Sprintf("Dragged from (%d,%d) to (%d,%d) via osascript", x1, y1, x2, y2), nil
}

func fallbackType(backend cuBackend, text string) (string, error) {
	escaped := strings.ReplaceAll(text, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	if backend == cuBackendCliclick {
		if err := cliclickExec(fmt.Sprintf(`t:"%s"`, escaped)); err != nil {
			return "", fmt.Errorf("type: %w", err)
		}
		return fmt.Sprintf("Typed via cliclick: %s", text), nil
	}
	if _, err := osaExec(fmt.Sprintf(`tell app "System Events" to keystroke "%s"`, escaped)); err != nil {
		return "", fmt.Errorf("type: %w", err)
	}
	return fmt.Sprintf("Typed via osascript: %s", text), nil
}

func fallbackKey(backend cuBackend, keys string) (string, error) {
	if backend == cuBackendCliclick {
		return cliclickKeyImpl(keys)
	}
	return osaKeyImpl(keys)
}

func cliclickKeyImpl(keys string) (string, error) {
	parts := strings.Split(keys, "+")
	var modifiers []string
	var key string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		lower := strings.ToLower(p)
		if mod, ok := cliclickModMap[lower]; ok {
			modifiers = append(modifiers, mod)
		} else if named, ok := cliclickKeyNames[lower]; ok {
			key = named
		} else {
			key = p
		}
	}
	if len(modifiers) > 0 {
		var args []string
		for _, m := range modifiers {
			args = append(args, fmt.Sprintf("kd:%s", m))
		}
		args = append(args, fmt.Sprintf("kp:%s", key))
		for i := len(modifiers) - 1; i >= 0; i-- {
			args = append(args, fmt.Sprintf("ku:%s", modifiers[i]))
		}
		if err := cliclickExec(args...); err != nil {
			return "", fmt.Errorf("key combo: %w", err)
		}
	} else {
		if err := cliclickExec(fmt.Sprintf("kp:%s", key)); err != nil {
			return "", fmt.Errorf("key press: %w", err)
		}
	}
	return fmt.Sprintf("Pressed via cliclick: %s", keys), nil
}

var osaKeyCodeMap = map[string]int{
	"page up":   osaPageUp,
	"page down": osaPageDown,
}

// osaSendKeyScript 生成 AppleScript key code 或 keystroke 命令。
// 单字常量用 keystroke（如 return/delete/up），双字及以上用 key code
// （如 page up — keystroke page up 在 AppleScript 中是语法错误）。
func osaSendKeyScript(key string, modifiers []string) string {
	if code, ok := osaKeyCodeMap[key]; ok {
		if len(modifiers) > 0 {
			return fmt.Sprintf(
				`tell app "System Events" to key code %d using {%s}`,
				code, strings.Join(modifiers, ", "),
			)
		}
		return fmt.Sprintf(`tell app "System Events" to key code %d`, code)
	}
	if len(modifiers) > 0 {
		return fmt.Sprintf(`tell app "System Events" to keystroke %s using {%s}`, key, strings.Join(modifiers, ", "))
	}
	return fmt.Sprintf(`tell app "System Events" to keystroke %s`, key)
}

func osaKeyImpl(keys string) (string, error) {
	parts := strings.Split(keys, "+")
	var modifiers []string
	var key string
	isNamedKey := false
	for _, p := range parts {
		p = strings.TrimSpace(p)
		lower := strings.ToLower(p)
		if mod, ok := osaModMap[lower]; ok {
			modifiers = append(modifiers, mod)
		} else if named, ok := osaKeyNames[lower]; ok {
			key = named
			isNamedKey = true
		} else {
			key = p
		}
	}

	var script string
	switch {
	case len(modifiers) > 0 && isNamedKey:
		script = osaSendKeyScript(key, modifiers)
	case len(modifiers) > 0:
		script = fmt.Sprintf(`tell app "System Events" to keystroke "%s" using {%s}`, key, strings.Join(modifiers, ", "))
	case isNamedKey:
		script = osaSendKeyScript(key, nil)
	default:
		script = fmt.Sprintf(`tell app "System Events" to keystroke "%s"`, key)
	}
	if _, err := osaExec(script); err != nil {
		return "", fmt.Errorf("key: %w", err)
	}
	return fmt.Sprintf("Pressed via osascript: %s", keys), nil
}

func fallbackScroll(backend cuBackend, direction string, amount int) (string, error) {
	if backend == cuBackendCliclick {
		var flag string
		switch direction {
		case "up":
			flag = "wu"
		case "down":
			flag = "wd"
		case "left":
			flag = "wl"
		case "right":
			flag = "wr"
		}
		if err := cliclickExec(fmt.Sprintf("%s:%d", flag, amount)); err != nil {
			return "", fmt.Errorf("scroll: %w", err)
		}
		return fmt.Sprintf("Scrolled %s %d via cliclick", direction, amount), nil
	}

	repeat := max(1, amount/3)
	// AppleScript keystroke 支持单字常量（up/down/left/right），
	// 但 page up / page down 是双 token 无法解析，必须用 key code。
	var scrollCmd string
	switch direction {
	case "up":
		scrollCmd = fmt.Sprintf("key code %d", osaPageUp)
		for i := 1; i < repeat; i++ {
			scrollCmd += fmt.Sprintf("\nkey code %d", osaPageUp)
		}
	case "down":
		scrollCmd = fmt.Sprintf("key code %d", osaPageDown)
		for i := 1; i < repeat; i++ {
			scrollCmd += fmt.Sprintf("\nkey code %d", osaPageDown)
		}
	case "left":
		scrollCmd = fmt.Sprintf("repeat %d times\nkeystroke left\nend repeat", repeat)
	case "right":
		scrollCmd = fmt.Sprintf("repeat %d times\nkeystroke right\nend repeat", repeat)
	}
	if _, err := osaExec(fmt.Sprintf(`tell app "System Events" to %s`, scrollCmd)); err != nil {
		return "", fmt.Errorf("scroll: %w", err)
	}
	return fmt.Sprintf("Scrolled %s via osascript", direction), nil
}

func fallbackSetValue(backend cuBackend, value string) (string, error) {
	// Fallback: type the value and press enter (works for most input fields)
	if _, err := fallbackType(backend, value); err != nil {
		return "", fmt.Errorf("set_value: %w", err)
	}
	osaKeyImpl("return") //nolint:gosec // G104: fire-and-forget key press after type
	return fmt.Sprintf("Set value via fallback (type+enter): %s", value), nil
}

func fallbackListApps() (string, error) {
	apps, err := osaExec(`tell app "System Events"
		set appList to name of every process whose background only is false
		set appStr to ""
		repeat with appName in appList
			set appStr to appStr & (appName as text) & return
		end repeat
		return appStr
	end tell`)
	if err != nil {
		return "", fmt.Errorf("list apps: %w", err)
	}
	lines := strings.Split(apps, "\n")
	var visible []string
	for _, a := range lines {
		a = strings.TrimSpace(a)
		if a != "" {
			visible = append(visible, a)
		}
	}
	return strings.Join(visible, "\n") + "\nBackend: osascript", nil
}

func fallbackFocusApp(app string, raiseWindow bool) (string, error) {
	if raiseWindow {
		if _, err := osaExec(fmt.Sprintf(`tell app "%s" to activate`, app)); err != nil {
			return "", fmt.Errorf("focus: %w", err)
		}
		return fmt.Sprintf("Focused via osascript: %s (raised)", app), nil
	}
	// Without raise, just verify app is running without bringing it to front
	if _, err := osaExec(fmt.Sprintf(`tell app "System Events" to exists process "%s"`, app)); err != nil {
		return "", fmt.Errorf("focus: %w", err)
	}
	return fmt.Sprintf("Targeting: %s via osascript (not raised)", app), nil
}

func getWindowBounds(app string) (string, error) {
	return osaExec(fmt.Sprintf(`tell app "System Events"
		set appProc to first process whose name contains "%s"
		set appWin to window 1 of appProc
		set {x, y, w, h} to position and size of appWin
		return (x as text) & "," & (y as text) & "," & (w as text) & "," & (h as text)
	end tell`, app))
}
