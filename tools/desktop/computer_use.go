// computer_use.go：computer_use 桌面控制工具的核心入口与调度层。
// 职责：输入后端（cuBackend）定义与平台自动检测、cua-driver MCP 客户端单例、
// 工具注册（NewComputerUseTool）与 JSON Schema、各动作到具体平台后端的分发。
// 平台相关实现见 computer_use_{macos,win,lin}.go 与 computer_use_cua_driver.go。

package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
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
	// cua-driver 是全局单例子进程，生命周期远长于任何单次请求。
	// 绝不能把请求级 ctx 绑到子进程上——newMCPClient 内部使用
	// exec.CommandContext(ctx,...)，一旦该 ctx 取消就会 SIGKILL 子进程，
	// 导致首次请求结束后所有后续 computer_use 调用全部失败。
	// 故此处用 context.Background() 创建子进程，仅在 Close() 时 kill。
	_ = ctx
	client, err := newMCPClient(context.Background(), "cua-driver", "mcp")
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
		},
		"required": []any{"action"},
	}
}

// computerUseInput 是 computer_use 工具的参数结构体，与 JSON schema 字段对应。
type computerUseInput struct {
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

// actionHandler 是 computer_use 单次 action 的处理函数签名。
type actionHandler func(ctx context.Context, backend cuBackend, input computerUseInput) (any, error)

// newComputerUseActions 构建 action → handler 查表。clickWait 由 map 闭包捕获，
// 使 handlers 保持纯函数签名，便于独立测试。
func newComputerUseActions(clickWait int) map[string]actionHandler {
	return map[string]actionHandler{
		"capture":      handleCapture,
		"info":         handleInfo,
		"click":        handleClickWithWait(clickWait),
		"double_click": handleClickWithWait(clickWait),
		"right_click":  handleClickWithWait(clickWait),
		"middle_click": handleClickWithWait(clickWait),
		"drag":         handleDragWithWait(clickWait),
		"type":         handleTypeWithWait(clickWait),
		"key":          handleKeyWithWait(clickWait),
		"scroll":       handleScrollWithWait(clickWait),
		"set_value":    handleSetValueWithWait(clickWait),
		"wait":         handleWait,
		"list_apps":    handleListApps,
		"focus_app":    handleFocusApp,
	}
}

// computerUseHandler 是 NewComputerUseTool 闭包 handler 的命名替代。
// 职责链：平台检查 → 参数解析 → 安全检查 → 破坏性操作审批 → action dispatch → capture_after。
func computerUseHandler(clickWait int) func(ctx context.Context, args json.RawMessage) (any, error) {
	actions := newComputerUseActions(clickWait)
	return func(ctx context.Context, args json.RawMessage) (any, error) {
		switch runtime.GOOS {
		case "darwin", "windows", "linux":
		default:
			return nil, fmt.Errorf("computer_use is only supported on macOS, Windows, and Linux")
		}

		var input computerUseInput
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
			approved, err := awaitApproval(input.Action)
			if err != nil {
				return nil, err
			}
			if !approved {
				return nil, fmt.Errorf("DENIED by user")
			}
		}

		handler, ok := actions[input.Action]
		if !ok {
			return nil, fmt.Errorf("unknown action: %s", input.Action)
		}
		actionResult, err := handler(ctx, backend, input)
		if err != nil {
			return nil, err
		}

		if input.CaptureAfter {
			actionResult = addCaptureAfter(ctx, backend, input, actionResult)
		}
		return actionResult, nil
	}
}

// --- 独立 action handler 函数集 ---
// 每个 handler 封装其所属 case 的全部逻辑（含校验、调用、clickWait 休眠、结果格式）。
// 基准的函数签名便于独立测试。

func handleCapture(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
	return cuaCapture(ctx, backend, input.App, input.CaptureMode)
}

func handleInfo(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
	return cuaInfo(backend)
}

// handleClickWithWait 返回绑定了 clickWait 的 click handler。
// 覆盖 click / double_click / right_click / middle_click，由 input.Action 区分。
func handleClickWithWait(clickWait int) actionHandler {
	return func(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
		if len(input.Coordinate) < 2 && input.Element <= 0 {
			return nil, fmt.Errorf("coordinate [x, y] or element required for action=%s", input.Action)
		}
		x, y := 0, 0
		if len(input.Coordinate) >= 2 {
			x, y = input.Coordinate[0], input.Coordinate[1]
		}
		msg, err := cuaClick(ctx, backend, input.Action, x, y, input.Element)
		if err != nil {
			return nil, err
		}
		time.Sleep(time.Duration(clickWait) * time.Millisecond)
		return result(msg, nil)
	}
}

func handleDragWithWait(clickWait int) actionHandler {
	return func(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
		if len(input.FromCoordinate) < 2 || len(input.ToCoordinate) < 2 {
			return nil, fmt.Errorf("from_coordinate and to_coordinate required for drag")
		}
		msg, err := cuaDrag(backend, input.FromCoordinate[0], input.FromCoordinate[1], input.ToCoordinate[0], input.ToCoordinate[1])
		if err != nil {
			return nil, err
		}
		time.Sleep(time.Duration(clickWait) * time.Millisecond)
		return result(msg, nil)
	}
}

func handleTypeWithWait(clickWait int) actionHandler {
	return func(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
		if input.Text == "" {
			return nil, fmt.Errorf("text required for action=type")
		}
		msg, err := cuaType(ctx, backend, input.Text)
		if err != nil {
			return nil, err
		}
		time.Sleep(time.Duration(clickWait) * time.Millisecond)
		return result(msg, nil)
	}
}

func handleKeyWithWait(clickWait int) actionHandler {
	return func(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
		if input.Keys == "" {
			return nil, fmt.Errorf("keys required for action=key")
		}
		msg, err := cuaKey(ctx, backend, input.Keys)
		if err != nil {
			return nil, err
		}
		time.Sleep(time.Duration(clickWait) * time.Millisecond)
		return result(msg, nil)
	}
}

func handleScrollWithWait(clickWait int) actionHandler {
	return func(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
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
		msg, err := cuaScroll(ctx, backend, input.Direction, amount)
		if err != nil {
			return nil, err
		}
		time.Sleep(time.Duration(clickWait) * time.Millisecond)
		return result(msg, nil)
	}
}

func handleSetValueWithWait(clickWait int) actionHandler {
	return func(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
		msg, err := cuaSetValue(ctx, backend, input.Element, input.Text)
		if err != nil {
			return nil, err
		}
		time.Sleep(time.Duration(clickWait) * time.Millisecond)
		return result(msg, nil)
	}
}

func handleWait(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
	seconds := input.Seconds
	if seconds <= 0 {
		seconds = 1
	}
	if seconds > 30 {
		seconds = 30
	}
	time.Sleep(time.Duration(seconds * float64(time.Second)))
	return result(fmt.Sprintf("Waited %.1f seconds", seconds), nil)
}

func handleListApps(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
	return cuaListApps(ctx, backend)
}

func handleFocusApp(ctx context.Context, backend cuBackend, input computerUseInput) (any, error) {
	if input.App == "" {
		return nil, fmt.Errorf("app required for action=focus_app")
	}
	msg, err := cuaFocusApp(ctx, backend, input.App, input.RaiseWindow)
	if err != nil {
		return nil, err
	}
	return result(msg, nil)
}

// addCaptureAfter 将 capture_after 截屏结果合并进 action 的返回结果中。
func addCaptureAfter(ctx context.Context, backend cuBackend, input computerUseInput, actionResult any) any {
	capResult, capErr := cuaCapture(ctx, backend, input.App, "")
	if capErr != nil {
		return actionResult
	}
	tr, ok := actionResult.(toolResult)
	if !ok {
		return actionResult
	}
	trDet, ok := tr.Details.(map[string]any)
	if !ok {
		return actionResult
	}
	capTR, ok := capResult.(toolResult)
	if !ok {
		return actionResult
	}
	capDet, ok := capTR.Details.(map[string]any)
	if !ok {
		return actionResult
	}
	for k, v := range capDet {
		trDet["capture_after_"+k] = v
	}
	return actionResult
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
		Func:        computerUseHandler(clickWait),
	}
}

func cuaCapture(ctx context.Context, backend cuBackend, appName, mode string) (any, error) {
	switch backend {
	case cuBackendCua:
		return cuaDriverCapture(ctx, appName, mode)
	case cuBackendPowerShell:
		if mode == "som" {
			return winCaptureSOM(appName)
		}
		return winCapture(appName)
	case cuBackendXDoTool, cuBackendYDoTool:
		return xdoCapture(appName)
	default:
		return fallbackCapture(backend, appName, mode)
	}
}

func cuaInfo(backend cuBackend) (any, error) {
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

func cuaDrag(backend cuBackend, x1, y1, x2, y2 int) (string, error) {
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
