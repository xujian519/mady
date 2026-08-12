// computer_use_cua.go：cua-driver / 平台后端各动作实现。
// 平台分发入口见 computer_use.go（computerUseHandler / newComputerUseActions）。

package desktop

import (
	"context"
	"fmt"
)

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
