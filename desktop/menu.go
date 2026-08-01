//go:build darwin

package main

import (
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
// 菜单项通过 Wails 事件与前端通信：
//   - 「设置」（⌘,）→ app:open-settings，前端 ChatView 监听后打开设置面板
func (a *App) createAppMenu() *menu.Menu {
	m := menu.NewMenu()
	m.Append(menu.AppMenu())

	fileMenu := m.AddSubmenu("文件")
	fileMenu.AddText("设置", keys.CmdOrCtrl(","), func(_ *menu.CallbackData) {
		if ctx := a.ctxOrNil(); ctx != nil {
			runtime.EventsEmit(ctx, "app:open-settings")
		}
	})
	fileMenu.AddText("退出 Mady", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
		if ctx := a.ctxOrNil(); ctx != nil {
			runtime.Quit(ctx)
		}
	})
	m.Append(menu.EditMenu())
	m.Append(menu.WindowMenu())
	return m
}
