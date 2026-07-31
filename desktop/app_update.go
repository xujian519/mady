//go:build darwin

package main

// app_update.go — 自动更新预留（W4-T12 / M-DSK-PKG-003）。
//
// 当前为占位实现：CheckUpdate 返回「已是最新版本」，为未来接入真实更新通道
// 预留契约。评估结论见 docs/plans/desktop-autoupdate-assessment.md：
//   阶段一：本占位 + 设置面板「检查更新」入口（已实现）
//   阶段二：公证后接入自建更新通道（整包替换 .app，GitHub Releases 或自托管）
//   注意：公证后必须整包替换 .app（hardened runtime 会破坏内嵌二进制的签名），
//   不能做二进制级热更新。
//
// desktopVersion 与 desktop/wails.json 的 productVersion 保持一致；
// 发布管线接入后改为 ldflags 注入（-X main.desktopVersion=$(VERSION)）。

const desktopVersion = "0.1.0"

// UpdateInfo 描述一次更新检查的结果。
type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	Message        string `json:"message"`
}

// CheckUpdate 检查是否有可用更新（占位实现）。
// 返回「当前已是最新版本」；真实通道接入后在此实现版本比对与下载引导。
func (a *App) CheckUpdate() (UpdateInfo, error) {
	if err := a.ready(); err != nil {
		return UpdateInfo{}, err
	}
	return UpdateInfo{
		CurrentVersion: desktopVersion,
		LatestVersion:  desktopVersion,
		HasUpdate:      false,
		Message:        "当前已是最新版本",
	}, nil
}
