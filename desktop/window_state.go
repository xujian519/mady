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

// saveWindowState 持久化窗口几何。
//
// X/Y 为 nil 时保留已存值（load-modify-merge）：前端 beforeunload 的 SaveWindowState
// 只携带宽高，不能覆盖 beforeClose 已保存的位置——否则正常退出路径下位置被清空，
// 下次启动回到默认居中（2026-08-04 审阅修复：双写入者竞态）。
func saveWindowState(ws windowState) {
	p := windowStatePath()
	if p == "" {
		return
	}
	// 合并已存位置：新值未携带 X/Y（nil）时保留旧值，避免覆盖丢失。
	if ws.X == nil || ws.Y == nil {
		if prev := loadWindowState(); prev != nil {
			if ws.X == nil {
				ws.X = prev.X
			}
			if ws.Y == nil {
				ws.Y = prev.Y
			}
		}
	}
	data, err := json.Marshal(ws)
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Clean(p), data, 0600); err != nil {
		slog.Warn("mady-desktop: save window state failed", "err", err)
	}
}
