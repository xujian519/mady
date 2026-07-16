// computer_use.go：computer_use 桌面控制工具的核心入口与调度层。
// 职责：输入后端（cuBackend）定义与平台自动检测、cua-driver MCP 客户端单例、
// 工具注册（NewComputerUseTool）与 JSON Schema、各动作到具体平台后端的分发。
// 平台相关实现见 computer_use_{macos,win,lin}.go 与 computer_use_cua_driver.go。

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore"
)

type cuBackend string

const (
	cuBackendCua        cuBackend = "cua-driver"
	cuBackendCliclick   cuBackend = "cliclick"
	cuBackendOSA        cuBackend = "osascript"
	cuBackendPowerShell cuBackend = "powershell"
	cuBackendXDoTool    cuBackend = "xdotool"
	cuBackendYDoTool    cuBackend = "ydotool"
)

var (
	cuBackendCache cuBackend
	cuBackendOnce  sync.Once
)

func isWayland() bool {
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

func detectCUABackend() cuBackend {
	cuBackendOnce.Do(func() {
		switch runtime.GOOS {
		case "darwin":
			if _, err := exec.LookPath("cua-driver"); err == nil {
				cuBackendCache = cuBackendCua
			} else if _, err := exec.LookPath("cliclick"); err == nil {
				cuBackendCache = cuBackendCliclick
			} else {
				cuBackendCache = cuBackendOSA
			}
		case "windows":
			cuBackendCache = cuBackendPowerShell
		case "linux":
			if isWayland() {
				cuBackendCache = cuBackendYDoTool
			} else {
				cuBackendCache = cuBackendXDoTool
			}
		default:
			cuBackendCache = cuBackendOSA
		}
	})
	return cuBackendCache
}

var (
	cuaDriverClient   *mcpClient
	cuaDriverClientMu sync.Mutex
)

func getCuaDriverClient(ctx context.Context) (*mcpClient, error) {
	cuaDriverClientMu.Lock()
	defer cuaDriverClientMu.Unlock()
	if cuaDriverClient != nil {
		return cuaDriverClient, nil
	}
	client, err := newMCPClient(ctx, "cua-driver", "mcp")
	if err != nil {
		return nil, fmt.Errorf("start cua-driver: %w", err)
	}
	cuaDriverClient = client
	return client, nil
}

func (b cuBackend) String() string { return string(b) }

// ComputerUseToolConfig 是 computer_use 工具的可选配置；零值使用默认参数。
type ComputerUseToolConfig struct {
	DefaultClickWait int
}

func computerUseDescription() string {
	return "控制本地桌面。跨平台支持：macOS（cua-driver/cliclick/osascript）、" +
		"Windows（PowerShell）、Linux（xdotool X11 / ydotool+wtype+grim Wayland）。后端根据平台自动检测。" +
		"cua-driver（仅 macOS）在后台运行，不会抢占焦点。" +
		"安装：brew install cua-driver（macOS）或 apt install xdotool（Linux）或 apt install ydotool wtype grim（Linux Wayland）。" +
		"操作：capture（截屏 + 可选 AX 树/SOM）、info、click/double_click/right_click/middle_click、" +
		"drag、type、key（组合键如 cmd+s 或 ctrl+s）、scroll、set_value、" +
		"wait、list_apps、focus_app。" +
		"危险操作（清空废纸篓、注销、rm -rf、ctrl+alt+del 等）按平台阻止。" +
		"破坏性操作通过 COMPUTER_USE_APPROVAL 环境变量提示批准（once/session/none）。"
}

func computerUseSchema() map[string]any {
	actionEnum := []string{"capture", "info", "click", "double_click", "right_click", "middle_click", "drag", "type", "key", "scroll", "set_value", "wait", "list_apps", "focus_app"}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "要执行的操作",
				"enum":        actionEnum,
			},
			"coordinate": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "integer"},
				"description": "像素坐标 [x, y]，用于 click/double_click/right_click",
			},
			"from_coordinate": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "integer"},
				"description": "拖拽操作的起始像素坐标 [x, y]",
			},
			"to_coordinate": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "integer"},
				"description": "拖拽操作的结束像素坐标 [x, y]",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "要输入的文本（用于 action=type）或要设置的值（用于 action=set_value，如下拉框/滑块）",
			},
			"keys": map[string]any{
				"type":        "string",
				"description": "按键组合（用于 action=key），例如 'cmd+s'、'return'、'up'、'pagedown'",
			},
			"direction": map[string]any{
				"type":        "string",
				"description": "滚动方向（用于 action=scroll）。向上/向下/向左/向右",
				"enum":        []string{"up", "down", "left", "right"},
			},
			"amount": map[string]any{
				"type":        "integer",
				"description": "滚动量（刻度数，用于 action=scroll，默认 3，最大 50）",
			},
			"seconds": map[string]any{
				"type":        "number",
				"description": "等待的秒数（用于 action=wait，最大 30）",
			},
			"app": map[string]any{
				"type":        "string",
				"description": "应用名称。focus_app：设置目标。capture：截取应用窗口。",
			},
			"element": map[string]any{
				"type":        "integer",
				"description": "capture(mode=ax) 输出的 AX 元素索引（用于 cua-driver 的 click/set_value）",
			},
			"capture_mode": map[string]any{
				"type":        "string",
				"enum":        []string{"vision", "ax", "som"},
				"description": "'vision'（仅截屏，默认）、'ax'（截屏 + AX 无障碍树及元素 ID，仅 cua-driver）、'som'（截屏 + 编号元素叠加层，仅 cua-driver）",
			},
			"raise_window": map[string]any{
				"type":        "boolean",
				"description": "是否将窗口提升到前台（用于 action=focus_app，默认 false）",
			},
			"capture_after": map[string]any{
				"type":        "boolean",
				"description": "在操作后执行截屏并将结果包含在返回内容中",
			},
			"required": []any{"action"},
		},
	}
}

// NewComputerUseTool 创建 computer_use 桌面控制工具；cfg 为 nil 时使用默认配置。
func NewComputerUseTool(cfg *ComputerUseToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &ComputerUseToolConfig{}
	}
	clickWait := cfg.DefaultClickWait
	if clickWait <= 0 {
		clickWait = 300
	}
	initApprovalMode()

	return &agentcore.Tool{
		Name:        "computer_use",
		Description: computerUseDescription(),
		Parameters:  computerUseSchema(),
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			switch runtime.GOOS {
			case "darwin", "windows", "linux":
			default:
				return nil, fmt.Errorf("computer_use is only supported on macOS, Windows, and Linux")
			}

			var input struct {
				Action         string  `json:"action"`
				Coordinate     []int   `json:"coordinate"`
				FromCoordinate []int   `json:"from_coordinate"`
				ToCoordinate   []int   `json:"to_coordinate"`
				Text           string  `json:"text"`
				Keys           string  `json:"keys"`
				Direction      string  `json:"direction"`
				Amount         int     `json:"amount"`
				Seconds        float64 `json:"seconds"`
				App            string  `json:"app"`
				Element        int     `json:"element"`
				CaptureMode    string  `json:"capture_mode"`
				RaiseWindow    bool    `json:"raise_window"`
				CaptureAfter   bool    `json:"capture_after"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			backend := detectCUABackend()

			// Pre-flight safety checks
			if input.Action == "key" {
				input.Keys = normalizeKeyString(input.Keys)
				if err := checkBlockedKeyCombo(input.Keys); err != nil {
					return nil, err
				}
			}
			if input.Action == "type" || input.Action == "set_value" {
				if err := checkBlockedTypePattern(input.Text); err != nil {
					return nil, err
				}
			}

			// Approval for destructive actions
			if isDestructiveAction(input.Action) {
				approvalMu.Lock()
				if approvalMode == approvalOnce && approvalSeen[input.Action] {
					approvalMu.Unlock()
					return nil, fmt.Errorf("BLOCKED by approval mode (once). Set COMPUTER_USE_APPROVAL=session or none to allow more")
				}
				if approvalMode == approvalOnce || approvalMode == approvalSession {
					if !approvalSeen[input.Action] {
						approvalMu.Unlock()
						fmt.Fprintf(os.Stderr, "\n⚠️  COMPUTER_USE: %s — approve? [y/N/session/always] ", input.Action)
						var resp string
						fmt.Scanln(&resp)
						resp = strings.ToLower(strings.TrimSpace(resp))
						approvalMu.Lock()
						switch resp {
						case "y", "yes":
							// approve once (already tracked by the fact we don't mark session)
						case "session":
							approvalMode = approvalSession
						case "always":
							approvalMode = approvalNone
						default:
							approvalMu.Unlock()
							return nil, fmt.Errorf("DENIED by user")
						}
						approvalSeen[input.Action] = true
						approvalMu.Unlock()
					} else {
						approvalMu.Unlock()
					}
				} else {
					approvalMu.Unlock()
				}
			}

			var actionResult any
			var err error
			switch input.Action {
			case "capture":
				actionResult, err = cuaCapture(ctx, backend, input.App, input.CaptureMode)
			case "info":
				actionResult, err = cuaInfo(ctx, backend)
			case "click", "double_click", "right_click", "middle_click":
				if len(input.Coordinate) < 2 && input.Element <= 0 {
					return nil, fmt.Errorf("coordinate [x, y] or element required for action=%s", input.Action)
				}
				x, y := 0, 0
				if len(input.Coordinate) >= 2 {
					x, y = input.Coordinate[0], input.Coordinate[1]
				}
				msg, e := cuaClick(ctx, backend, input.Action, x, y, input.Element)
				err = e
				if err == nil {
					time.Sleep(time.Duration(clickWait) * time.Millisecond)
					actionResult, _ = result(msg, nil)
				}
			case "drag":
				if len(input.FromCoordinate) < 2 || len(input.ToCoordinate) < 2 {
					return nil, fmt.Errorf("from_coordinate and to_coordinate required for drag")
				}
				msg, e := cuaDrag(ctx, backend, input.FromCoordinate[0], input.FromCoordinate[1], input.ToCoordinate[0], input.ToCoordinate[1])
				err = e
				if err == nil {
					time.Sleep(time.Duration(clickWait) * time.Millisecond)
					actionResult, _ = result(msg, nil)
				}
			case "type":
				if input.Text == "" {
					return nil, fmt.Errorf("text required for action=type")
				}
				msg, e := cuaType(ctx, backend, input.Text)
				err = e
				if err == nil {
					time.Sleep(time.Duration(clickWait) * time.Millisecond)
					actionResult, _ = result(msg, nil)
				}
			case "key":
				if input.Keys == "" {
					return nil, fmt.Errorf("keys required for action=key")
				}
				msg, e := cuaKey(ctx, backend, input.Keys)
				err = e
				if err == nil {
					time.Sleep(time.Duration(clickWait) * time.Millisecond)
					actionResult, _ = result(msg, nil)
				}
			case "scroll":
				if input.Direction == "" {
					return nil, fmt.Errorf("direction required for action=scroll")
				}
				amount := input.Amount
				if amount <= 0 {
					amount = 3
				}
				if amount > 50 {
					amount = 50
				}
				msg, e := cuaScroll(ctx, backend, input.Direction, amount)
				err = e
				if err == nil {
					time.Sleep(time.Duration(clickWait) * time.Millisecond)
					actionResult, _ = result(msg, nil)
				}
			case "set_value":
				msg, e := cuaSetValue(ctx, backend, input.Element, input.Text)
				err = e
				if err == nil {
					time.Sleep(time.Duration(clickWait) * time.Millisecond)
					actionResult, _ = result(msg, nil)
				}
			case "wait":
				seconds := input.Seconds
				if seconds <= 0 {
					seconds = 1
				}
				if seconds > 30 {
					seconds = 30
				}
				time.Sleep(time.Duration(seconds * float64(time.Second)))
				actionResult, _ = result(fmt.Sprintf("Waited %.1f seconds", seconds), nil)
			case "list_apps":
				actionResult, err = cuaListApps(ctx, backend)
			case "focus_app":
				if input.App == "" {
					return nil, fmt.Errorf("app required for action=focus_app")
				}
				msg, e := cuaFocusApp(ctx, backend, input.App, input.RaiseWindow)
				err = e
				if err == nil {
					actionResult, _ = result(msg, nil)
				}
			default:
				return nil, fmt.Errorf("unknown action: %s", input.Action)
			}
			if err != nil {
				return nil, err
			}

			if input.CaptureAfter {
				capResult, capErr := cuaCapture(ctx, backend, input.App, "")
				if capErr == nil {
					if tr, ok := actionResult.(ToolResult); ok {
						if trDet, ok := tr.Details.(map[string]any); ok {
							if capTR, ok := capResult.(ToolResult); ok {
								if capDet, ok := capTR.Details.(map[string]any); ok {
									for k, v := range capDet {
										trDet["capture_after_"+k] = v
									}
								}
							}
						}
					}
				}
			}
			return actionResult, nil
		},
	}
}

func cuaCapture(ctx context.Context, backend cuBackend, appName, mode string) (any, error) {
	switch backend {
	case cuBackendCua:
		return cuaDriverCapture(ctx, appName, mode)
	case cuBackendPowerShell:
		if mode == "som" {
			return winCaptureSOM(ctx, appName)
		}
		return winCapture(ctx, appName)
	case cuBackendXDoTool, cuBackendYDoTool:
		return xdoCapture(ctx, appName)
	default:
		return fallbackCapture(ctx, backend, appName, mode)
	}
}

func cuaInfo(ctx context.Context, backend cuBackend) (any, error) {
	switch backend {
	case cuBackendPowerShell:
		return winInfo()
	case cuBackendXDoTool:
		return xdoInfo()
	default:
		return fallbackInfo()
	}
}

func cuaClick(ctx context.Context, backend cuBackend, action string, x, y, element int) (string, error) {
	switch backend {
	case cuBackendCua:
		return cuaDriverClick(ctx, action, x, y, element)
	case cuBackendPowerShell:
		return winClick(action, x, y)
	case cuBackendXDoTool:
		return xdoClick(action, x, y)
	case cuBackendYDoTool:
		return ydoClick(action, x, y)
	default:
		if action == "middle_click" && backend == cuBackendCliclick {
			return fallbackClick(cuBackendOSA, action, x, y)
		}
		return fallbackClick(backend, action, x, y)
	}
}

func cuaDrag(ctx context.Context, backend cuBackend, x1, y1, x2, y2 int) (string, error) {
	switch backend {
	case cuBackendCua:
		return "", fmt.Errorf("drag is not supported by cua-driver backend. Use click + type + key or fallback backend")
	case cuBackendPowerShell:
		return winDrag(x1, y1, x2, y2)
	case cuBackendXDoTool, cuBackendYDoTool:
		return xdoDrag(x1, y1, x2, y2)
	default:
		return fallbackDrag(backend, x1, y1, x2, y2)
	}
}

func cuaType(ctx context.Context, backend cuBackend, text string) (string, error) {
	switch backend {
	case cuBackendCua:
		return cuaDriverType(ctx, text)
	case cuBackendPowerShell:
		return winType(text)
	case cuBackendXDoTool:
		return xdoType(text)
	case cuBackendYDoTool:
		return ydoType(text)
	default:
		return fallbackType(backend, text)
	}
}

func cuaKey(ctx context.Context, backend cuBackend, keys string) (string, error) {
	switch backend {
	case cuBackendCua:
		return cuaDriverKey(ctx, keys)
	case cuBackendPowerShell:
		return winKey(keys)
	case cuBackendXDoTool:
		return xdoKey(keys)
	case cuBackendYDoTool:
		return ydoKey(keys)
	default:
		return fallbackKey(backend, keys)
	}
}

func cuaScroll(ctx context.Context, backend cuBackend, direction string, amount int) (string, error) {
	switch backend {
	case cuBackendCua:
		return cuaDriverScroll(ctx, direction, amount)
	case cuBackendPowerShell:
		return winScroll(direction, amount)
	case cuBackendXDoTool, cuBackendYDoTool:
		return xdoScroll(direction, amount)
	default:
		return fallbackScroll(backend, direction, amount)
	}
}

func cuaSetValue(ctx context.Context, backend cuBackend, element int, value string) (string, error) {
	switch backend {
	case cuBackendCua:
		return cuaDriverSetValue(ctx, element, value)
	case cuBackendPowerShell:
		return winSetValue(value)
	case cuBackendXDoTool, cuBackendYDoTool:
		return xdoSetValue(value)
	default:
		return fallbackSetValue(backend, value)
	}
}

func cuaListApps(ctx context.Context, backend cuBackend) (string, error) {
	switch backend {
	case cuBackendCua:
		return cuaDriverListApps(ctx)
	case cuBackendPowerShell:
		return winListApps()
	case cuBackendXDoTool, cuBackendYDoTool:
		return xdoListApps()
	default:
		return fallbackListApps()
	}
}

func cuaFocusApp(ctx context.Context, backend cuBackend, app string, raiseWindow bool) (string, error) {
	switch backend {
	case cuBackendCua:
		return cuaDriverFocusApp(ctx, app, raiseWindow)
	case cuBackendPowerShell:
		return winFocusApp(app, raiseWindow)
	case cuBackendXDoTool, cuBackendYDoTool:
		return xdoFocusApp(app, raiseWindow)
	default:
		return fallbackFocusApp(app, raiseWindow)
	}
}
