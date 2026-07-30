package main

// system_status_panel.go wires the SystemStatus component into the TUI as a
// centered overlay for viewing system operating conditions — mode (normal/
// degraded), recent events, and current impact on the user's task.
//
// Data source: fc state (MemoryManager, EvidenceExt, FileCheckpointExt,
// ProjectRegistry, CaseIndex) combined with session runtime stats.
//
// Follows the same pattern as settings_panel.go and skill_panel.go.

import (
	"fmt"
	"runtime"
	"time"

	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
)

// openSystemStatus builds a SystemStatus overlay from the current session
// and framework state, then opens it as a dimmed overlay.
func (s *tuiSession) openSystemStatus() {
	ss := component.NewSystemStatus()

	// Determine operating mode.
	mode := "normal"
	var modeReason string
	if s.fc == nil {
		mode = "degraded"
		modeReason = "框架上下文未初始化"
	} else if s.fc.MemoryManager == nil {
		modeReason = "记忆系统未加载"
	}

	// Build recent events.
	events := []component.SysEvent{
		{Time: time.Now().Format("15:04"), Message: fmt.Sprintf("Provider: %s · Model: %s", s.providerName, s.model), Level: "info"},
	}
	if s.isPlanMode() {
		events = append(events, component.SysEvent{
			Time: time.Now().Format("15:04"), Message: "计划模式已启用", Level: "info",
		})
	}
	if s.isReviewMode() {
		events = append(events, component.SysEvent{
			Time: time.Now().Format("15:04"), Message: "审核关卡已启用", Level: "info",
		})
	}
	if s.currentProject != nil {
		events = append(events, component.SysEvent{
			Time: time.Now().Format("15:04"), Message: fmt.Sprintf("当前案件: %s", s.currentProject.Alias), Level: "info",
		})
	}
	if s.currentFileWatcher != nil {
		events = append(events, component.SysEvent{
			Time: time.Now().Format("15:04"), Message: "文件监视器运行中", Level: "info",
		})
	}

	// Check storage probes.
	if s.agentStore == nil {
		events = append(events, component.SysEvent{
			Time: time.Now().Format("15:04"), Message: "会话持久化未启用", Level: "warn",
		})
	}

	// Build impact statements.
	impacts := []string{
		fmt.Sprintf("总协程: %d", runtime.NumGoroutine()),
		fmt.Sprintf("会话线程: %s", s.currentThreadID),
	}
	if s.currentProject != nil {
		impacts = append(impacts, fmt.Sprintf("工作目录: %s", s.currentProject.RootPath))
	}

	ss.SetMode(mode, modeReason)
	ss.SetEvents(events)
	ss.SetImpacts(impacts)

	var ov chat.OverlayRef
	ss.SetOnClose(func() {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
	})

	box := component.NewBox()
	box.SetBorder(component.BorderRounded)
	box.SetTitle("系统态 — [l] 日志 · Esc 关闭")
	box.SetPadding(1, 1)
	box.AddChild(ss)

	ov = s.app.OpenOverlay(box, chat.OverlayOpts{WidthPct: 55, HeightPct: 40, Dim: true, Category: chat.OverlayCatSystem})
}
