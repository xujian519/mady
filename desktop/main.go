//go:build darwin

package main

import (
	"embed"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

// windowState 保存窗口几何信息。
type windowState struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	X      int `json:"x,omitempty"`
	Y      int `json:"y,omitempty"`
}

func windowStatePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(cacheDir, "mady")
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Warn("mady-desktop: mkdir failed", "err", err)
	}
	return filepath.Join(dir, "window_state.json")
}

func loadWindowState() *windowState {
	p := windowStatePath()
	if p == "" {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var ws windowState
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil
	}
	if ws.Width < 400 || ws.Height < 300 {
		return nil
	}
	return &ws
}

func saveWindowState(ws windowState) {
	p := windowStatePath()
	if p == "" {
		return
	}
	data, err := json.Marshal(ws)
	if err != nil {
		return
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		slog.Warn("mady-desktop: save window state failed", "err", err)
	}
}

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
