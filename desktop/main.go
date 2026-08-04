//go:build darwin

package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	// 加载上次窗口状态
	ws := loadWindowState()
	width := 1200
	height := 800
	if ws != nil {
		width = ws.Width
		height = ws.Height
	}

	err := wails.Run(&options.App{
		Title:     "Mady",
		Width:     width,
		Height:    height,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// M-DSK-PKG-004：单实例锁，防多开（会话/数据库/端口资源场景）。
		// UniqueId 复用 Info.plist Bundle ID；第二实例启动时聚焦已运行窗口。
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.mady.desktop",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				app.focusMainWindow()
			},
		},
		// M-DSK-SEC-005：限制 JS↔Go binding 的合法来源（仅本地 dev 服务）。
		BindingsAllowedOrigins: "http://localhost:*,https://localhost:*",
		OnStartup:              app.startup,
		OnShutdown:             app.shutdown,
		OnBeforeClose:          app.beforeClose,
		// 1.3：macOS 原生菜单（设置 ⌘, / 退出 ⌘Q），见 menu.go
		Menu: app.createAppMenu(),
		Bind: []interface{}{app},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.DefaultAppearance,
			WindowIsTranslucent:  false,
			WebviewIsTransparent: false,
		},
	})

	if err != nil {
		log.Fatalf("[mady-desktop] wails.Run failed: %v", err)
	}
}
