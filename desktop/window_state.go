//go:build darwin

package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// windowState 保存窗口几何信息。
// 由 Go 侧 beforeClose 经 runtime.WindowGetSize/GetPosition 自取完整几何（S-8），
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
		log.Printf("[mady-desktop] mkdir failed: %v", err)
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

// saveWindowState 持久化窗口几何（原子写，纯写入）。
//
// 单一写入者模型（B-3）：只有 beforeClose 写入——Wails v2.13 的 OnBeforeClose
// 覆盖窗口关闭 / Cmd+W / runtime.Quit（⌘Q）等主流退出路径（darwin frontend.go:
// Frontend.Quit 先触发 OnBeforeClose）。已删除前端 beforeunload 的 SaveWindowState
// 绑定：它只携带宽高，会覆盖 X/Y 造成位置丢失，且 WKWebView 的 beforeunload
// 可靠性存疑——删除写入者后不再需要 load-modify-merge 补救。
func saveWindowState(ws windowState) {
	p := windowStatePath()
	if p == "" {
		return
	}
	data, err := json.Marshal(ws)
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Clean(p), data, 0600); err != nil {
		log.Printf("[mady-desktop] save window state failed: %v", err)
	}
}
