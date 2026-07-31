//go:build darwin

package main

// tray.go — 系统托盘与长任务完成通知（W4-T11）。
//
// 托盘：使用 getlantern/systray（Wails v2 无内置托盘 API），
// 提供「显示主窗口 / 退出」菜单，图标复用 build/appicon.png。
// 通知：macOS 通过 osascript 发送系统通知（无额外依赖），
// 在 Chat 完成（agui:done）时提示用户长任务已结束。
//
// 平台说明：Windows/Linux 托盘留待 W3-T4 适配（build tag 隔离，
// 非 darwin 平台不编译本文件）。

import (
	_ "embed"
	"log"
	"os/exec"
	"time"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// chatNotifyThreshold 是「长任务」时长阈值（G-I4）：
// 成功完成的对话若运行时长低于此值（交互式短对话），不弹系统通知，
// 避免逐条消息通知轰炸；失败通知不受阈值限制。
const chatNotifyThreshold = 30 * time.Second

//go:embed build/appicon.png
var trayIconBytes []byte

// startTray 启动系统托盘（独立 goroutine，非阻塞）。
// 在 startup 阶段调用（a.ctx 已就绪，供菜单回调操作窗口）。
func (a *App) startTray() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[mady-desktop] tray panic recovered: %v", r)
			}
		}()
		systray.Run(
			func() { a.trayReady() },
			func() { log.Println("[mady-desktop] tray exited") },
		)
	}()
}

// trayReady 初始化托盘图标与菜单（在 systray goroutine 中执行）。
func (a *App) trayReady() {
	systray.SetIcon(trayIconBytes)
	systray.SetTitle("Mady")
	systray.SetTooltip("Mady — 专利/法律 AI 助手")

	mShow := systray.AddMenuItem("显示 Mady", "将主窗口显示到前台")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出 Mady 应用")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				if ctx := a.ctxOrNil(); ctx != nil {
					runtime.WindowShow(ctx)
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				if ctx := a.ctxOrNil(); ctx != nil {
					runtime.Quit(ctx) // 走 Wails OnShutdown 干净退出
				}
				return
			}
		}
	}()
}

// notifyMacOS 通过 osascript 发送系统通知。
// title 为标题（如「Mady 分析完成」），message 为摘要文本。
func notifyMacOS(title, message string) {
	// osascript 参数经 -e 传字符串；含引号/换行的消息需转义，此处截断换行防注入
	safeMsg := sanitizeNotification(message)
	safeTitle := sanitizeNotification(title)
	script := `display notification "` + safeMsg + `" with title "` + safeTitle + `"`
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		log.Printf("[mady-desktop] notification failed: %v", err)
	}
}

// sanitizeNotification 移除通知文本中的引号与换行，避免 osascript 语法错误。
func sanitizeNotification(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '"', '\n', '\r', '\\':
			continue
		}
		out = append(out, r)
		if len(out) >= 120 { // 截断长摘要
			break
		}
	}
	return string(out)
}

// notifyChatDone 在 Chat 完成时发送系统通知（长任务完成提示）。
// errMsg 非空表示失败通知。
func (a *App) notifyChatDone(errMsg string) {
	// 延迟执行，避免与 agui:done 推送竞争窗口焦点
	time.AfterFunc(300*time.Millisecond, func() {
		if errMsg != "" {
			notifyMacOS("Mady 分析失败", "任务未完成："+errMsg)
			return
		}
		notifyMacOS("Mady 分析完成", "长任务已结束，可返回查看结果。")
	})
}
