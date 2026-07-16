// computer_use_win.go：Windows 后端实现（PowerShell + Win32/UIAutomation）。
// 职责：屏幕/窗口截屏（含 UIA 元素树的 SOM 模式）、点击、拖拽、SendKeys 输入与按键、
// 滚轮滚动、应用列表与窗口聚焦。
// 注意：本文件不能命名为 computer_use_windows.go——以 _windows.go 结尾会被
// Go 工具链视为 GOOS 限定文件，仅在 Windows 上编译，导致其他平台符号缺失。

package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

func winCapture(ctx context.Context, appName string) (any, error) {
	boundsScript := `Add-Type -AssemblyName System.Windows.Forms,System.Drawing
$bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
"$($bounds.X),$($bounds.Y),$($bounds.Width),$($bounds.Height)"`
	if appName != "" {
		boundsScript = fmt.Sprintf(`Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
	[DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT lpRect);
	[StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
}
"@
$proc = Get-Process | Where-Object {$_.MainWindowTitle -ne ""} | Where-Object {$_.ProcessName -like "*%s*"} | Select-Object -First 1
if (-not $proc) { exit 1 }
$rect = New-Object Win32+RECT
[Win32]::GetWindowRect($proc.MainWindowHandle, [ref]$rect)
"$($rect.Left),$($rect.Top),$($rect.Right - $rect.Left),$($rect.Bottom - $rect.Top)"`, appName)
	}

	script := `Add-Type -AssemblyName System.Windows.Forms,System.Drawing
$bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$bmp = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($bounds.X, $bounds.Y, 0, 0, $bounds.Size)
$ms = New-Object System.IO.MemoryStream
$bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
$g.Dispose()
[System.Convert]::ToBase64String($ms.ToArray())`
	out, err := pwshExec(script)
	if err != nil {
		return nil, err
	}
	b64 := strings.TrimSpace(out)

	desc := "powershell"
	raw := make([]byte, base64.StdEncoding.DecodedLen(len(b64)))
	n, _ := base64.StdEncoding.Decode(raw, []byte(b64))
	data := raw[:n]

	if appName != "" {
		boundsOut, err := pwshExec(boundsScript)
		if err == nil {
			parts := strings.Split(strings.TrimSpace(boundsOut), ",")
			if len(parts) == 4 {
				x := parseInt(parts[0])
				y := parseInt(parts[1])
				w := parseInt(parts[2])
				h := parseInt(parts[3])
				if w > 0 && h > 0 {
					if cropped, err := cropImageToBounds(data, x, y, w, h); err == nil {
						data = cropped
						desc = fmt.Sprintf("powershell (cropped to %s)", appName)
					}
				}
			}
		}
	}

	b64 = base64.StdEncoding.EncodeToString(data)
	return result(
		fmt.Sprintf("Screenshot captured (%d bytes) via %s", len(data), desc),
		map[string]any{
			"image_base64": b64,
			"format":       "png",
			"size_bytes":   len(data),
		},
	)
}

func winCaptureSOM(ctx context.Context, appName string) (any, error) {
	// Step 1: capture screenshot
	screenshotResult, err := winCapture(ctx, appName)
	if err != nil {
		return nil, err
	}
	tr, ok := screenshotResult.(ToolResult)
	if !ok {
		return nil, fmt.Errorf("unexpected screenshot result type")
	}
	details, ok := tr.Details.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected screenshot details type")
	}
	b64, ok := details["image_base64"].(string)
	if !ok || b64 == "" {
		return nil, fmt.Errorf("no image in screenshot")
	}

	// Step 2: get UIA accessibility tree with element positions
	uiaScript := `Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
	[DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
}
"@
$hwnd = [Win32]::GetForegroundWindow()
$rootEl = [System.Windows.Automation.AutomationElement]::FromHandle($hwnd)
$walker = [System.Windows.Automation.TreeWalker]::ContentViewWalker
$result = New-Object System.Collections.ArrayList
$nextId = 1
function Walk($el, $depth) {
	if ($depth -gt 6) { return }
	$rect = $el.Current.BoundingRectangle
	if ($rect.Width -gt 0 -and $rect.Height -gt 0) {
		[void]$result.Add(@{
			id = $nextId
			name = $el.Current.Name
			type = $el.Current.ControlType.ProgrammaticName
			x = [int]$rect.X
			y = [int]$rect.Y
			w = [int]$rect.Width
			h = [int]$rect.Height
		})
		$script:nextId++
	}
	$child = $walker.GetFirstChild($el)
	while ($child -ne $null) {
		Walk $child ($depth + 1)
		$child = $walker.GetNextChild($child)
	}
}
$script:nextId = 1
Walk $rootEl 0
$result | ConvertTo-Json -Compress`
	uiaOut, err := pwshExec(uiaScript)
	if err != nil {
		// fallback to regular capture
		return screenshotResult, nil
	}
	uiaOut = strings.TrimSpace(uiaOut)

	var uiaElements []somElement
	if err := json.Unmarshal([]byte(uiaOut), &uiaElements); err != nil {
		return screenshotResult, nil
	}
	if len(uiaElements) == 0 {
		return screenshotResult, nil
	}

	// Step 3: render SOM overlay
	annotated, err := renderSOMOverlayFromB64(b64, uiaElements)
	if err != nil {
		return screenshotResult, nil
	}

	details["image_base64"] = annotated
	details["format"] = "png"
	details["size_bytes"] = len(annotated) * 3 / 4
	details["som"] = true

	output := "SOM Elements:\n"
	for _, el := range uiaElements {
		label := el.Label
		if label != "" {
			label = " " + label
		}
		output += fmt.Sprintf("  [%d] pos=(%d,%d) size=(%dx%d)%s\n", el.ID, el.X, el.Y, el.W, el.H, label)
	}
	output += "\n(Use element IDs with click or set_value)"
	return result(output, details)
}

func winInfo() (any, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms
$screen = [System.Windows.Forms.Screen]::PrimaryScreen
$cursor = [System.Windows.Forms.Cursor]::Position
$w = $screen.Bounds.Width
$h = $screen.Bounds.Height
"Screen: $w x $h | Cursor: ($($cursor.X), $($cursor.Y)) | Backend: powershell"`
	out, err := pwshExec(script)
	if err != nil {
		return nil, fmt.Errorf("screen info failed: %w", err)
	}
	return result(strings.TrimSpace(out), nil)
}

func winClick(action string, x, y int) (string, error) {
	var downFlag, upFlag string
	switch action {
	case "right_click":
		downFlag, upFlag = "0x0008", "0x0010"
	case "middle_click":
		downFlag, upFlag = "0x0020", "0x0040"
	default:
		downFlag, upFlag = "0x0002", "0x0004"
	}
	script := fmt.Sprintf(`Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
	[DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
	[DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
}
"@
[Win32]::SetCursorPos(%d, %d)
[Win32]::mouse_event(%s, 0, 0, 0, [UIntPtr]::Zero)
[Win32]::mouse_event(%s, 0, 0, 0, [UIntPtr]::Zero)`, x, y, downFlag, upFlag)
	if action == "double_click" {
		script += fmt.Sprintf(`
Start-Sleep -Milliseconds 50
[Win32]::mouse_event(%s, 0, 0, 0, [UIntPtr]::Zero)
[Win32]::mouse_event(%s, 0, 0, 0, [UIntPtr]::Zero)`, downFlag, upFlag)
	}
	if _, err := pwshExec(script); err != nil {
		return "", fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Sprintf("%s at (%d, %d) via powershell", action, x, y), nil
}

func winDrag(x1, y1, x2, y2 int) (string, error) {
	script := fmt.Sprintf(`Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
	[DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
	[DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
}
"@
[Win32]::SetCursorPos(%d, %d)
[Win32]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)
[Win32]::SetCursorPos(%d, %d)
[Win32]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)`, x1, y1, x2, y2)
	if _, err := pwshExec(script); err != nil {
		return "", fmt.Errorf("drag: %w", err)
	}
	return fmt.Sprintf("Dragged from (%d,%d) to (%d,%d) via powershell", x1, y1, x2, y2), nil
}

func winType(text string) (string, error) {
	escaped := strings.ReplaceAll(text, `"`, `""`)
	script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait("%s")`, escaped)
	if _, err := pwshExec(script); err != nil {
		return "", fmt.Errorf("type: %w", err)
	}
	return fmt.Sprintf("Typed via powershell: %s", text), nil
}

func winKey(keys string) (string, error) {
	parts := strings.Split(keys, "+")
	var mods []string
	var key string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		lower := strings.ToLower(p)
		switch lower {
		case "cmd", "command", "ctrl", "control":
			mods = append(mods, "^")
		case "alt", "option":
			mods = append(mods, "%")
		case "shift":
			mods = append(mods, "+")
		default:
			if vk, ok := winVK[lower]; ok {
				key = string(rune(vk))
			} else {
				key = p
			}
		}
	}
	sendKey := key
	if len(mods) > 0 {
		sendKey = strings.Join(mods, "") + "(" + key + ")"
	}
	script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait("%s")`, sendKey)
	if _, err := pwshExec(script); err != nil {
		return "", fmt.Errorf("key: %w", err)
	}
	return fmt.Sprintf("Pressed via powershell: %s", keys), nil
}

func winScroll(direction string, amount int) (string, error) {
	delta := amount * 120
	if direction == "down" || direction == "right" {
		delta = -delta
	}
	var hScrollFlag string
	if direction == "left" || direction == "right" {
		hScrollFlag = ", 0x0001"
	}
	script := fmt.Sprintf(`Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
	[DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
}
"@
[Win32]::mouse_event(0x0800, 0, 0, %d%s, [UIntPtr]::Zero)`, delta, hScrollFlag)
	if _, err := pwshExec(script); err != nil {
		return "", fmt.Errorf("scroll: %w", err)
	}
	return fmt.Sprintf("Scrolled %s %d via powershell", direction, amount), nil
}

func winSetValue(value string) (string, error) {
	if _, err := winType(value); err != nil {
		return "", fmt.Errorf("set_value: %w", err)
	}
	winKey("return")
	return fmt.Sprintf("Set value via powershell (type+enter): %s", value), nil
}

func winListApps() (string, error) {
	script := `Get-Process | Where-Object {$_.MainWindowHandle -ne 0} | Sort-Object ProcessName | Select-Object -ExpandProperty ProcessName`
	out, err := pwshExec(script)
	if err != nil {
		return "", fmt.Errorf("list apps: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var visible []string
	for _, a := range lines {
		a = strings.TrimSpace(a)
		if a != "" {
			visible = append(visible, a)
		}
	}
	return strings.Join(visible, "\n") + "\nBackend: powershell", nil
}

func winFocusApp(app string, raiseWindow bool) (string, error) {
	script := fmt.Sprintf(`Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
	[DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
	[DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
	[DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
	[DllImport("user32.dll")] public static extern int GetWindowText(IntPtr hWnd, System.Text.StringBuilder text, int count);
}
"@
$proc = Get-Process | Where-Object {$_.MainWindowTitle -ne ""} | Where-Object {$_.ProcessName -like "*%s*"}
if (-not $proc) { exit 1 }
if (%t) {
	[Win32]::ShowWindow($proc.MainWindowHandle, 9)
	[Win32]::SetForegroundWindow($proc.MainWindowHandle)
}`, app, raiseWindow)
	if _, err := pwshExec(script); err != nil {
		return "", fmt.Errorf("focus: %w", err)
	}
	if raiseWindow {
		return fmt.Sprintf("Focused via powershell: %s (raised)", app), nil
	}
	return fmt.Sprintf("Targeting: %s via powershell (not raised)", app), nil
}
