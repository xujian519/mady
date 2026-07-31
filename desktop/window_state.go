//go:build darwin

package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// windowState 保存窗口几何信息。
// 仅持久化宽高（S-8：X/Y 从未被 SaveWindowState 保存/恢复，属死字段，
// 若要持久化位置需扩展绑定先保存再恢复）。
type windowState struct {
	Width  int `json:"width"`
	Height int `json:"height"`
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
	if err := os.WriteFile(p, data, 0600); err != nil {
		slog.Warn("mady-desktop: save window state failed", "err", err)
	}
}
