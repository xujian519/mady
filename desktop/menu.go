//go:build darwin

package main

import (
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// createAppMenu 构建 macOS 原生应用菜单栏。
//
// 仅 darwin：在 Windows/Linux 上 Wails 菜单栏会渲染为窗口内一条「文件」条
// （macOS 专属 role 不生效），而本桌面端平台目标为 macOS，故不做平台分支。
// 原生菜单同时让 WebView 内的 Cmd+C/V 等走标准 Edit 角色。
//
// 菜单布局（macOS 惯例，2026-08-04 审阅调整）：
//   - 应用菜单「Mady」：关于（版本对话框）/ 设置（⌘,）/ 退出（⌘Q）
//   - 「编辑」「窗口」：标准 role 菜单（剪贴板快捷键等）
//
// 菜单项通过 Wails 事件与前端通信：「设置」→ app:open-settings，
// 前端 ChatView 监听后打开设置面板。
func (a *App) createAppMenu() *menu.Menu {
	m := menu.NewMenu()

	// 应用菜单（macOS 惯例：关于 / 设置 / 退出置于首菜单）。
	appMenu := m.AddSubmenu("Mady")
	appMenu.AddText("关于 Mady", nil, func(_ *menu.CallbackData) {
		if ctx := a.ctxOrNil(); ctx != nil {
			_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
				Type:    runtime.InfoDialog,
				Title:   "关于 Mady",
				Message: fmt.Sprintf("Mady 桌面端 v%s\n面向专利/法律领域的智能 Agent 工作台", desktopVersion),
			})
		}
	})
	appMenu.AddSeparator()
	appMenu.AddText("设置…", keys.CmdOrCtrl(","), func(_ *menu.CallbackData) {
		if ctx := a.ctxOrNil(); ctx != nil {
			runtime.EventsEmit(ctx, "app:open-settings")
		}
	})
	appMenu.AddSeparator()
	appMenu.AddText("退出 Mady", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
		if ctx := a.ctxOrNil(); ctx != nil {
			runtime.Quit(ctx)
		}
	})

	m.Append(menu.EditMenu())
	m.Append(menu.WindowMenu())
	return m
}
