//go:build darwin

package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// windowState 保存窗口几何信息。
// Width/Height 由前端 beforeunload 时 SaveWindowState 保存（见 app.go）；
// X/Y 由 Go 侧 beforeClose 经 runtime.WindowGetPosition 自取（S-8 修复），
// nil 表示未保存过位置（Wails 默认居中）。
type windowState struct {
	Width  int  `json:"width"`
	Height int  `json:"height"`
	X      *int `json:"x,omitempty"`
	Y      *int `json:"y,omitempty"`
}

func windowStatePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(cacheDir, "mady")
	if err := os.MkdirAll(dir, 0750); err != nil {
		slog.Warn("mady-desktop: mkdir failed", "err", err)
	}
	return filepath.Join(dir, "window_state.json")
}

func loadWindowState() *windowState {
	p := windowStatePath()
	if p == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Clean(p))
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
	// 位置合法性：绝对值上限 10000，防坏数据把窗口丢出屏幕（S-8）。
	if ws.X != nil && (absInt(*ws.X) > 10000) {
		ws.X = nil
	}
	if ws.Y != nil && (absInt(*ws.Y) > 10000) {
		ws.Y = nil
	}
	return &ws
}

// absInt 返回 int 绝对值（Go 标准库 math 包的 Abs 只支持 float）。
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
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
	if err := os.WriteFile(p, data, 0600); err != nil {
		slog.Warn("mady-desktop: save window state failed", "err", err)
	}
}
