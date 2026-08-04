//go:build darwin

package main

// app_update.go — 自动更新预留（W4-T12 / M-DSK-PKG-003）。
//
// 当前为占位实现：CheckUpdate 返回「已是最新版本」，为未来接入真实更新通道
// 预留契约。评估结论见 docs/plans/desktop-autoupdate-assessment.md：
//   阶段一：本占位 + 设置面板「检查更新」入口（已实现）+ 版本注入（ldflags）
//   阶段二：公证后接入自建更新通道（整包替换 .app，GitHub Releases 或自托管）
//   注意：公证后必须整包替换 .app（hardened runtime 会破坏内嵌二进制的签名），
//   不能做二进制级热更新。
//
// desktopVersion 默认值与 desktop/wails.json 的 productVersion 保持一致（0.1.0）；
// 发布管线通过 ldflags 注入真实版本：
//   wails build -ldflags "-X main.desktopVersion=$(VERSION)"
// （Makefile desktop-dmg / desktop-build-quick 已注入）。

// desktopVersion 为运行版本号。必须是 var 而非 const，ldflags 才能覆盖；
// 未注入（本地开发 wails dev / go build）时回退到 wails.json 的 productVersion。
var desktopVersion = "0.1.0"

// UpdateInfo 描述一次更新检查的结果。
type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	Message        string `json:"message"`
}

// CheckUpdate 检查是否有可用更新（占位实现，版本号已 ldflags 注入）。
// 真实更新通道（manifest + 校验 + 整包替换）在公证落地后接入，
// 见 docs/plans/desktop-autoupdate-assessment.md 阶段二。
// 中间态文案（M-1）：明确告知「更新通道未接入」，避免误导为「已是最新」。
func (a *App) CheckUpdate() (UpdateInfo, error) {
	if err := a.ready(); err != nil {
		return UpdateInfo{}, err
	}
	return UpdateInfo{
		CurrentVersion: desktopVersion,
		LatestVersion:  desktopVersion,
		HasUpdate:      false,
		Message:        "更新通道未接入（自动更新将在公证落地后启用）；当前版本 " + desktopVersion,
	}, nil
}
