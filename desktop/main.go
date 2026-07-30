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
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind:       []interface{}{app},
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
