// computer_use_cua_driver.go：cua-driver（macOS 后台 MCP 服务）后端实现。
// 职责：窗口枚举与截屏（含 AX 树 / SOM 模式）、点击、输入、按键、滚动、
// set_value、应用列表与目标应用定位；全程后台执行、不抢占焦点。

package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

type mcpContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// CuaWindow 表示 cua-driver 返回的单个窗口信息。
type CuaWindow struct {
	PID      int    `json:"pid"`
	WindowID int    `json:"window_id"`
	Title    string `json:"title"`
	AppName  string `json:"app_name"`
	ZIndex   int    `json:"z_index"`
	OnScreen bool   `json:"on_screen"`
}

func cuaDriverCapture(ctx context.Context, appName, mode string) (any, error) {
	client, err := getCuaDriverClient(ctx)
	if err != nil {
		return nil, err
	}

	raw, err := client.callTool(ctx, "list_windows", map[string]any{
		"on_screen_only": true,
	})
	if err != nil {
		return nil, fmt.Errorf("list_windows: %w", err)
	}

	windows := parseCuaWindows(raw)
	if len(windows) == 0 {
		return nil, fmt.Errorf("no on-screen windows found")
	}

	var target *CuaWindow
	if appName != "" {
		appLower := strings.ToLower(appName)
		for i, w := range windows {
			if w.OnScreen && strings.Contains(strings.ToLower(w.AppName), appLower) {
				target = &windows[i]
				break
			}
		}
		if target == nil {
			return nil, fmt.Errorf("no window found for app: %s", appName)
		}
	} else {
		var best *CuaWindow
		for i, w := range windows {
			if !w.OnScreen {
				continue
			}
			if best == nil || w.ZIndex < best.ZIndex {
				best = &windows[i]
			}
		}
		if best == nil {
			return nil, fmt.Errorf("no on-screen window available")
		}
		target = best
	}

	if mode == "ax" || mode == "som" {
		if mode == "som" && runtime.GOOS != "darwin" {
			return nil, fmt.Errorf("SOM mode is only supported on macOS with cua-driver. Use mode=vision or mode=ax on other platforms")
		}
		raw, err := client.callTool(ctx, "get_window_state", map[string]any{
			"pid":       target.PID,
			"window_id": target.WindowID,
		})
		if err != nil {
			return nil, fmt.Errorf("get_window_state: %w", err)
		}

		var stateResult struct {
			Content []mcpContentPart `json:"content"`
			IsError bool             `json:"isError"`
		}
		if json.Unmarshal(raw, &stateResult) != nil {
			return nil, fmt.Errorf("parse get_window_state result")
		}

		var axTree, imageData string
		for _, part := range stateResult.Content {
			if part.Type == "text" {
				axTree = part.Text
			} else if strings.HasPrefix(part.Type, "image") || strings.HasPrefix(part.Type, "resource") {
				imageData = part.Data
			}
		}

		details := map[string]any{
			"app":       target.AppName,
			"title":     target.Title,
			"window_id": target.WindowID,
		}
		output := fmt.Sprintf("Window: %s | %s\n", target.AppName, target.Title)

		if mode == "som" && imageData != "" && axTree != "" {
			annotated, elements, err := renderSOMOverlay(imageData, axTree)
			if err == nil {
				details["image_base64"] = annotated
				details["format"] = "jpeg"
				details["size_bytes"] = len(annotated) * 3 / 4
				details["som"] = true
				output += "\nSOM Elements:\n"
				for _, el := range elements {
					label := el.Label
					if label != "" {
						label = " " + label
					}
					output += fmt.Sprintf("  [%d] pos=(%d,%d) size=(%dx%d)%s\n", el.ID, el.X, el.Y, el.W, el.H, label)
				}
				output += "\n(Use element IDs with click/set_value action to interact)"
				return result(output, details)
			}
			// fall through to ax mode if SOM rendering fails
		}

		if imageData != "" {
			details["image_base64"] = imageData
			details["format"] = "jpeg"
			details["size_bytes"] = len(imageData) * 3 / 4
		}

		if axTree != "" {
			output += "\nAX Tree:\n" + axTree
		} else {
			output += "\n(AX tree unavailable)"
		}

		return result(output, details)
	}

	raw, err = client.callTool(ctx, "screenshot", map[string]any{
		"window_id": target.WindowID,
		"format":    "jpeg",
		"quality":   85,
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}

	var ssResult struct {
		Content []mcpContentPart `json:"content"`
	}
	if json.Unmarshal(raw, &ssResult) != nil {
		return nil, fmt.Errorf("parse screenshot result")
	}

	for _, part := range ssResult.Content {
		if strings.HasPrefix(part.Type, "image") || strings.HasPrefix(part.Type, "resource") {
			return result(
				fmt.Sprintf("Captured %s: %s | Backend: cua-driver (background, no focus steal)", target.AppName, target.Title),
				map[string]any{
					"image_base64": part.Data,
					"format":       "jpeg",
					"size_bytes":   len(part.Data) * 3 / 4,
					"app":          target.AppName,
					"title":        target.Title,
					"window_id":    target.WindowID,
				},
			)
		}
	}

	return nil, fmt.Errorf("no image in screenshot response")
}

func parseCuaWindows(raw json.RawMessage) []CuaWindow {
	var listResult struct {
		Content []struct {
			Type       string           `json:"type"`
			Text       string           `json:"text,omitempty"`
			Structured *json.RawMessage `json:"structuredContent,omitempty"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &listResult) != nil {
		return nil
	}

	for _, part := range listResult.Content {
		if part.Structured != nil {
			var decoded struct {
				Data []CuaWindow `json:"data"`
			}
			if json.Unmarshal(*part.Structured, &decoded) == nil && len(decoded.Data) > 0 {
				return decoded.Data
			}
		}
	}

	for _, part := range listResult.Content {
		if part.Text != "" {
			var ws []CuaWindow
			if json.Unmarshal([]byte(part.Text), &ws) == nil && len(ws) > 0 {
				return ws
			}
		}
	}
	return nil
}

func cuaDriverClick(ctx context.Context, action string, x, y, element int) (string, error) {
	client, err := getCuaDriverClient(ctx)
	if err != nil {
		return "", err
	}
	var toolName string
	switch action {
	case "double_click":
		toolName = "double_click"
	case "right_click":
		toolName = "right_click"
	case "middle_click":
		toolName = "click"
	default:
		toolName = "click"
	}
	args := map[string]any{}
	if element > 0 {
		args["element"] = element
	} else {
		args["coordinate"] = []int{x, y}
	}
	if action == "middle_click" {
		args["button"] = "middle"
	}
	if _, err := client.callTool(ctx, toolName, args); err != nil {
		return "", fmt.Errorf("%s: %w", toolName, err)
	}
	if element > 0 {
		return fmt.Sprintf("%s on element %d via cua-driver (background)", action, element), nil
	}
	return fmt.Sprintf("%s at (%d, %d) via cua-driver (background)", action, x, y), nil
}

func cuaDriverType(ctx context.Context, text string) (string, error) {
	client, err := getCuaDriverClient(ctx)
	if err != nil {
		return "", err
	}
	if _, err := client.callTool(ctx, "type_text_chars", map[string]any{
		"text": text,
	}); err != nil {
		return "", fmt.Errorf("type_text_chars: %w", err)
	}
	return fmt.Sprintf("Typed via cua-driver (background): %s", text), nil
}

func cuaDriverKey(ctx context.Context, keys string) (string, error) {
	client, err := getCuaDriverClient(ctx)
	if err != nil {
		return "", err
	}

	parts := strings.Split(keys, "+")
	var modifiers []string
	var key string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		lower := strings.ToLower(p)
		if mod, ok := cuaModMap[lower]; ok {
			modifiers = append(modifiers, mod)
		} else {
			key = p
		}
	}

	if len(modifiers) > 0 {
		_, err = client.callTool(ctx, "hotkey", map[string]any{
			"keys":      key,
			"modifiers": modifiers,
		})
	} else {
		_, err = client.callTool(ctx, "press_key", map[string]any{
			"key": key,
		})
	}
	if err != nil {
		return "", fmt.Errorf("key: %w", err)
	}
	return fmt.Sprintf("Pressed: %s via cua-driver (background)", keys), nil
}

func cuaDriverScroll(ctx context.Context, direction string, amount int) (string, error) {
	client, err := getCuaDriverClient(ctx)
	if err != nil {
		return "", err
	}
	if _, err := client.callTool(ctx, "scroll", map[string]any{
		"direction": direction,
		"amount":    amount,
	}); err != nil {
		return "", fmt.Errorf("scroll: %w", err)
	}
	return fmt.Sprintf("Scrolled %s %d via cua-driver (background)", direction, amount), nil
}

func cuaDriverSetValue(ctx context.Context, element int, value string) (string, error) {
	client, err := getCuaDriverClient(ctx)
	if err != nil {
		return "", err
	}
	args := map[string]any{"value": value}
	if element > 0 {
		args["element"] = element
	}
	if _, err := client.callTool(ctx, "set_value", args); err != nil {
		return "", fmt.Errorf("set_value: %w", err)
	}
	return fmt.Sprintf("Set value via cua-driver (AX): %s", value), nil
}

func cuaDriverListApps(ctx context.Context) (string, error) {
	client, err := getCuaDriverClient(ctx)
	if err != nil {
		return "", err
	}
	raw, err := client.callTool(ctx, "list_apps", nil)
	if err != nil {
		return "", fmt.Errorf("list_apps: %w", err)
	}
	var result struct {
		Content []mcpContentPart `json:"content"`
	}
	if json.Unmarshal(raw, &result) == nil {
		for _, part := range result.Content {
			if part.Type == "text" && part.Text != "" {
				return part.Text + "\nBackend: cua-driver", nil
			}
		}
	}
	return "Backend: cua-driver", nil
}

func cuaDriverFocusApp(ctx context.Context, app string, raiseWindow bool) (string, error) {
	client, err := getCuaDriverClient(ctx)
	if err != nil {
		return "", err
	}
	// list_windows is called to confirm cua-driver responsiveness; the window list
	// is not used because cua-driver does not support raising/focusing windows.
	if _, err := client.callTool(ctx, "list_windows", map[string]any{
		"on_screen_only": true,
	}); err != nil {
		return "", fmt.Errorf("list_windows: %w", err)
	}
	if raiseWindow {
		// cua-driver can't raise windows; note this in response
		return fmt.Sprintf("Targeting: %s via cua-driver (background, raise_window requested but cua-driver does not support raising windows)", app), nil
	}
	return fmt.Sprintf("Targeting: %s via cua-driver (background, no focus steal)", app), nil
}
