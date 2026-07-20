// computer_use_lin.go：Linux 后端实现（xdotool X11 / ydotool+wtype Wayland）。
// 职责：截屏（grim/import/gnome-screenshot/scrot）、点击、拖拽、输入、按键、滚动、
// 应用列表与窗口聚焦；Wayland 下窗口边界/聚焦走 Hyprland 或 Sway。
// 注意：本文件不能命名为 computer_use_linux.go——以 _linux.go 结尾会被
// Go 工具链视为 GOOS 限定文件，仅在 Linux 上编译，导致其他平台符号缺失。

package desktop

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func xdoCapture(ctx context.Context, appName string) (any, error) {
	screenshotPath := filepath.Join(os.TempDir(), fmt.Sprintf("mady_cu_%d.png", time.Now().UnixNano()))
	if err := scrotExec(screenshotPath); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(screenshotPath)
	os.Remove(screenshotPath)
	if err != nil {
		return nil, fmt.Errorf("read screenshot: %w", err)
	}

	desc := func() string {
		if isWayland() {
			return "wayland"
		}
		return "xdotool"
	}()
	if appName != "" {
		bounds := ""
		if isWayland() {
			bounds, _ = waylandGetWindowBounds(appName)
		} else {
			bounds, _ = linuxGetWindowBounds(appName)
		}
		if bounds != "" {
			parts := strings.Split(bounds, ",")
			if len(parts) == 4 {
				x := parseInt(parts[0])
				y := parseInt(parts[1])
				w := parseInt(parts[2])
				h := parseInt(parts[3])
				if w > 0 && h > 0 {
					if cropped, err := cropImageToBounds(data, x, y, w, h); err == nil {
						data = cropped
						desc = fmt.Sprintf("xdotool (cropped to %s)", appName)
					}
				}
			}
		}
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	return result(
		fmt.Sprintf("Screenshot captured (%d bytes) via %s", len(data), desc),
		map[string]any{
			"image_base64": b64,
			"format":       "png",
			"size_bytes":   len(data),
		},
	)
}

func linuxGetWindowBounds(app string) (string, error) {
	if isWayland() {
		return "", fmt.Errorf("window bounds not available on Wayland")
	}
	ids, err := xdoExec("search", "--onlyvisible", "--name", "--", app)
	if err != nil || strings.TrimSpace(ids) == "" {
		ids2, err2 := xdoExec("search", "--onlyvisible", "--class", app)
		if err2 != nil || strings.TrimSpace(ids2) == "" {
			return "", fmt.Errorf("window not found: %s", app)
		}
		ids = ids2
	}
	winID := strings.Fields(ids)[0]
	geo, err := xdoExec("getwindowgeometry", winID)
	if err != nil {
		return "", err
	}
	var x, y, w, h int
	fmt.Sscanf(geo, "Position: %d,%d\nGeometry: %dx%d", &x, &y, &w, &h)
	if w == 0 || h == 0 {
		return "", fmt.Errorf("could not parse geometry for window %s", winID)
	}
	return fmt.Sprintf("%d,%d,%d,%d", x, y, w, h), nil
}

func xdoInfo() (any, error) {
	geo, err := xdoExec("getdisplaygeometry")
	if err != nil {
		return nil, fmt.Errorf("screen info failed: %w", err)
	}
	geo = strings.TrimSpace(geo)
	parts := strings.Split(geo, " ")
	screen := "unknown"
	if len(parts) >= 2 {
		screen = parts[0] + " x " + parts[1]
	}
	cursor, _ := xdoExec("getmouselocation", "--shell")
	cursor = strings.TrimSpace(cursor)
	active, _ := xdoExec("getactivewindow")
	activeName, _ := xdoExec("getwindowname", strings.TrimSpace(active))
	return result(fmt.Sprintf("Screen: %s | Backend: xdotool\nCursor: %s\nActive: %s", screen, cursor, strings.TrimSpace(activeName)), nil)
}

func xdoClick(action string, x, y int) (string, error) {
	// Move mouse first
	if _, err := xdoExec("mousemove", fmt.Sprintf("%d", x), fmt.Sprintf("%d", y)); err != nil {
		return "", fmt.Errorf("mousemove: %w", err)
	}
	var btn string
	switch action {
	case "double_click":
		btn = "1"
	case "right_click":
		btn = "3"
	case "middle_click":
		btn = "2"
	default:
		btn = "1"
	}
	if action == "double_click" {
		if _, err := xdoExec("click", "--repeat", "2", btn); err != nil {
			return "", fmt.Errorf("%s: %w", action, err)
		}
	} else {
		if _, err := xdoExec("click", btn); err != nil {
			return "", fmt.Errorf("%s: %w", action, err)
		}
	}
	return fmt.Sprintf("%s at (%d, %d) via xdotool", action, x, y), nil
}

func xdoDrag(x1, y1, x2, y2 int) (string, error) {
	if _, err := xdoExec("mousemove", fmt.Sprintf("%d", x1), fmt.Sprintf("%d", y1)); err != nil {
		return "", fmt.Errorf("drag mousemove: %w", err)
	}
	if _, err := xdoExec("mousedown", "1"); err != nil {
		return "", fmt.Errorf("drag mousedown: %w", err)
	}
	if _, err := xdoExec("mousemove", fmt.Sprintf("%d", x2), fmt.Sprintf("%d", y2)); err != nil {
		return "", fmt.Errorf("drag mousemove: %w", err)
	}
	if _, err := xdoExec("mouseup", "1"); err != nil {
		return "", fmt.Errorf("drag mouseup: %w", err)
	}
	return fmt.Sprintf("Dragged from (%d,%d) to (%d,%d) via xdotool", x1, y1, x2, y2), nil
}

func xdoType(text string) (string, error) {
	if _, err := xdoExec("type", "--delay", "12", text); err != nil {
		return "", fmt.Errorf("type: %w", err)
	}
	return fmt.Sprintf("Typed via xdotool: %s", text), nil
}

func xdoKey(keys string) (string, error) {
	parts := strings.Split(keys, "+")
	var mods []string
	var key string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		lower := strings.ToLower(p)
		switch lower {
		case "cmd", "command", "ctrl", "control":
			mods = append(mods, "ctrl")
		case "alt", "option":
			mods = append(mods, "alt")
		case "shift":
			mods = append(mods, "shift")
		default:
			if mapped, ok := xdoKeyMap[lower]; ok {
				key = mapped
			} else {
				key = p
			}
		}
	}
	var xdoArgs []string
	if len(mods) > 0 {
		xdoArgs = append(xdoArgs, "key")
		combo := strings.Join(append(mods, key), "+")
		xdoArgs = append(xdoArgs, combo)
	} else {
		xdoArgs = append(xdoArgs, "key", key)
	}
	if _, err := xdoExec(xdoArgs...); err != nil {
		return "", fmt.Errorf("key: %w", err)
	}
	return fmt.Sprintf("Pressed via xdotool: %s", keys), nil
}

func xdoScroll(direction string, amount int) (string, error) {
	var btn string
	switch direction {
	case "up":
		btn = "4"
	case "down":
		btn = "5"
	case "left":
		btn = "6"
	case "right":
		btn = "7"
	}
	for i := 0; i < amount; i++ {
		if _, err := xdoExec("click", btn); err != nil {
			return "", fmt.Errorf("scroll: %w", err)
		}
	}
	return fmt.Sprintf("Scrolled %s %d via xdotool", direction, amount), nil
}

func xdoSetValue(value string) (string, error) {
	if _, err := xdoType(value); err != nil {
		return "", fmt.Errorf("set_value: %w", err)
	}
	xdoKey("return")
	return fmt.Sprintf("Set value via xdotool (type+enter): %s", value), nil
}

func xdoListApps() (string, error) {
	out, err := xdoExec("search", "--onlyvisible", "--name", ".")
	if err != nil {
		return "", fmt.Errorf("list apps: %w", err)
	}
	ids := strings.Fields(out)
	var visible []string
	for _, id := range ids {
		name, err := xdoExec("getwindowname", id)
		if err != nil {
			continue
		}
		name = strings.TrimSpace(name)
		if name != "" {
			visible = append(visible, name)
		}
	}
	return strings.Join(visible, "\n") + "\nBackend: xdotool", nil
}

func xdoFocusApp(app string, raiseWindow bool) (string, error) {
	if isWayland() {
		return waylandFocusApp(app, raiseWindow)
	}
	ids, err := xdoExec("search", "--onlyvisible", "--name", "--", app)
	if err != nil || strings.TrimSpace(ids) == "" {
		ids2, err2 := xdoExec("search", "--onlyvisible", "--class", app)
		if err2 != nil || strings.TrimSpace(ids2) == "" {
			return "", fmt.Errorf("no window found for app: %s", app)
		}
		ids = ids2
	}
	winID := strings.Fields(ids)[0]
	if raiseWindow {
		if _, err := xdoExec("windowactivate", winID); err != nil {
			return "", fmt.Errorf("focus: %w", err)
		}
		return fmt.Sprintf("Focused via xdotool: %s (raised)", app), nil
	}
	return fmt.Sprintf("Targeting: %s via xdotool (not raised)", app), nil
}

func waylandGetWindowBounds(app string) (string, error) {
	// Try Hyprland
	if _, err := exec.LookPath("hyprctl"); err == nil {
		out, err := exec.Command("hyprctl", "clients", "-j").Output()
		if err == nil {
			var clients []struct {
				Title        string `json:"title"`
				InitialTitle string `json:"initialTitle"`
				Class        string `json:"class"`
				At           []int  `json:"at"`
				Size         []int  `json:"size"`
			}
			if json.Unmarshal(out, &clients) == nil {
				appLower := strings.ToLower(app)
				for _, c := range clients {
					if strings.Contains(strings.ToLower(c.Class), appLower) ||
						strings.Contains(strings.ToLower(c.Title), appLower) ||
						strings.Contains(strings.ToLower(c.InitialTitle), appLower) {
						if len(c.At) >= 2 && len(c.Size) >= 2 && c.Size[0] > 0 && c.Size[1] > 0 {
							return fmt.Sprintf("%d,%d,%d,%d", c.At[0], c.At[1], c.Size[0], c.Size[1]), nil
						}
					}
				}
			}
		}
	}

	// Try Sway
	if _, err := exec.LookPath("swaymsg"); err == nil {
		out, err := exec.Command("swaymsg", "-t", "get_tree").Output()
		if err == nil {
			var root struct {
				Nodes []struct {
					Nodes []struct {
						Nodes []struct {
							AppID string `json:"app_id"`
							Name  string `json:"name"`
							Rect  struct {
								X      int `json:"x"`
								Y      int `json:"y"`
								Width  int `json:"width"`
								Height int `json:"height"`
							} `json:"rect"`
						} `json:"nodes"`
					} `json:"nodes"`
				} `json:"nodes"`
			}
			if json.Unmarshal(out, &root) == nil {
				appLower := strings.ToLower(app)
				for _, o1 := range root.Nodes {
					for _, o2 := range o1.Nodes {
						for _, leaf := range o2.Nodes {
							if strings.Contains(strings.ToLower(leaf.AppID), appLower) ||
								strings.Contains(strings.ToLower(leaf.Name), appLower) {
								if leaf.Rect.Width > 0 && leaf.Rect.Height > 0 {
									return fmt.Sprintf("%d,%d,%d,%d", leaf.Rect.X, leaf.Rect.Y, leaf.Rect.Width, leaf.Rect.Height), nil
								}
							}
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("no Wayland compositor recognized (supported: Hyprland, Sway)")
}

func waylandFocusApp(app string, raiseWindow bool) (string, error) {
	lower := strings.ToLower(app)

	// Try Hyprland
	if _, err := exec.LookPath("hyprctl"); err == nil {
		out, err := exec.Command("hyprctl", "clients", "-j").Output()
		if err == nil {
			var clients []struct {
				Title        string `json:"title"`
				InitialTitle string `json:"initialTitle"`
				Class        string `json:"class"`
				Address      string `json:"address"`
			}
			if json.Unmarshal(out, &clients) == nil {
				for _, c := range clients {
					if strings.Contains(strings.ToLower(c.Class), lower) ||
						strings.Contains(strings.ToLower(c.Title), lower) ||
						strings.Contains(strings.ToLower(c.InitialTitle), lower) {
						if raiseWindow {
							exec.Command("hyprctl", "dispatch", "focuswindow", "address:"+c.Address).Run()
							return fmt.Sprintf("Focused via Hyprland: %s (raised)", app), nil
						}
						return fmt.Sprintf("Targeting: %s via Hyprland (not raised)", app), nil
					}
				}
			}
		}
	}

	// Try Sway
	if _, err := exec.LookPath("swaymsg"); err == nil {
		if raiseWindow {
			if err := exec.Command("swaymsg", fmt.Sprintf(`[title="(?i)%s"]`, app), "focus").Run(); err == nil {
				return fmt.Sprintf("Focused via Sway: %s (raised)", app), nil
			}
			if err := exec.Command("swaymsg", fmt.Sprintf(`[app_id="(?i)%s"]`, app), "focus").Run(); err == nil {
				return fmt.Sprintf("Focused via Sway: %s (raised)", app), nil
			}
		} else {
			return fmt.Sprintf("Targeting: %s via Sway (not raised)", app), nil
		}
	}

	return "", fmt.Errorf("no Wayland compositor found for focus (supported: Hyprland, Sway)")
}

func ydoClick(action string, x, y int) (string, error) {
	if _, err := ydoExec("mousemove", "--absolute", "-x", fmt.Sprintf("%d", x), "-y", fmt.Sprintf("%d", y)); err != nil {
		return "", fmt.Errorf("mousemove: %w", err)
	}
	var btnCode string
	switch action {
	case "right_click":
		btnCode = "0x2"
	case "middle_click":
		btnCode = "0x3"
	default:
		btnCode = "0x1"
	}
	if action == "double_click" {
		for i := 0; i < 2; i++ {
			if _, err := ydoExec("click", btnCode); err != nil {
				return "", fmt.Errorf("%s: %w", action, err)
			}
		}
	} else {
		if _, err := ydoExec("click", btnCode); err != nil {
			return "", fmt.Errorf("%s: %w", action, err)
		}
	}
	return fmt.Sprintf("%s at (%d, %d) via ydotool", action, x, y), nil
}

func ydoType(text string) (string, error) {
	if _, err := wtypeExec(text); err != nil {
		return "", fmt.Errorf("type: %w", err)
	}
	return fmt.Sprintf("Typed via wtype: %s", text), nil
}

func ydoKey(keys string) (string, error) {
	parts := strings.Split(keys, "+")
	var mods []string
	var key string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		lower := strings.ToLower(p)
		switch lower {
		case "cmd", "command", "ctrl", "control":
			mods = append(mods, "KEY_LEFTCTRL")
		case "alt", "option":
			mods = append(mods, "KEY_LEFTALT")
		case "shift":
			mods = append(mods, "KEY_LEFTSHIFT")
		case "super", "win", "meta":
			mods = append(mods, "KEY_LEFTMETA")
		default:
			if code, ok := ydoKeyCodes[lower]; ok {
				key = code
			} else if len(p) == 1 {
				key = fmt.Sprintf("KEY_%s", strings.ToUpper(p))
			} else {
				key = p
			}
		}
	}
	for _, m := range mods {
		if _, err := ydoExec("key", m); err != nil {
			return "", fmt.Errorf("key: %w", err)
		}
	}
	if _, err := ydoExec("key", key); err != nil {
		return "", fmt.Errorf("key: %w", err)
	}
	for i := len(mods) - 1; i >= 0; i-- {
		if _, err := ydoExec("key", mods[i]); err != nil {
			return "", fmt.Errorf("key release: %w", err)
		}
	}
	return fmt.Sprintf("Pressed via ydotool: %s", keys), nil
}
