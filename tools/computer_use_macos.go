// computer_use_macos.go：macOS 回退后端实现（cliclick / osascript）。
// 职责：screencapture 截屏、点击/双击/右键/中键、拖拽、文本输入、按键组合、
// 滚动、set_value（输入+回车回退）、应用列表、窗口聚焦与窗口边界查询。

package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

func fallbackCapture(ctx context.Context, backend cuBackend, appName, mode string) (any, error) {
	screenshotPath := filepath.Join(os.TempDir(), fmt.Sprintf("mady_cu_%d.png", time.Now().UnixNano()))
	var args []string
	if appName != "" {
		bounds, err := getWindowBounds(appName)
		if err == nil {
			args = append(args, "-R", bounds)
		}
	}
	args = append(args, "-x", screenshotPath)
	if err := exec.Command("screencapture", args...).Run(); err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	data, err := os.ReadFile(screenshotPath)
	os.Remove(screenshotPath)
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
		if _, err := cliclickExec(fmt.Sprintf("%s:%d,%d", cliAction, x, y)); err != nil {
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
		if _, err := cliclickExec(
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
		if _, err := cliclickExec(fmt.Sprintf(`t:"%s"`, escaped)); err != nil {
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
		if _, err := cliclickExec(args...); err != nil {
			return "", fmt.Errorf("key combo: %w", err)
		}
	} else {
		if _, err := cliclickExec(fmt.Sprintf("kp:%s", key)); err != nil {
			return "", fmt.Errorf("key press: %w", err)
		}
	}
	return fmt.Sprintf("Pressed via cliclick: %s", keys), nil
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
		script = fmt.Sprintf(`tell app "System Events" to keystroke %s using {%s}`, key, strings.Join(modifiers, ", "))
	case len(modifiers) > 0:
		script = fmt.Sprintf(`tell app "System Events" to keystroke "%s" using {%s}`, key, strings.Join(modifiers, ", "))
	case isNamedKey:
		script = fmt.Sprintf(`tell app "System Events" to keystroke %s`, key)
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
		if _, err := cliclickExec(fmt.Sprintf("%s:%d", flag, amount)); err != nil {
			return "", fmt.Errorf("scroll: %w", err)
		}
		return fmt.Sprintf("Scrolled %s %d via cliclick", direction, amount), nil
	}

	repeat := max(1, amount/3)
	var keyName string
	switch direction {
	case "up":
		keyName = "page up"
	case "down":
		keyName = "page down"
	case "left":
		keyName = "left"
	case "right":
		keyName = "right"
	}
	if _, err := osaExec(fmt.Sprintf(`tell app "System Events" to repeat %d times
		keystroke %s
	end repeat`, repeat, keyName)); err != nil {
		return "", fmt.Errorf("scroll: %w", err)
	}
	return fmt.Sprintf("Scrolled %s via osascript", direction), nil
}

func fallbackSetValue(backend cuBackend, value string) (string, error) {
	// Fallback: type the value and press enter (works for most input fields)
	if _, err := fallbackType(backend, value); err != nil {
		return "", fmt.Errorf("set_value: %w", err)
	}
	osaKeyImpl("return")
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
