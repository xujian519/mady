package main

// settings_panel.go wires the SettingsList component into the TUI as a
// centered overlay. It is the template for how app-level panels (settings,
// session picker, todo, …) mount via ChatApp.OpenOverlay: build the
// component, wrap it in a Box, push it as an overlay, and wire OnSubmit to
// close + apply.

import (
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
)

// openSettings builds a settings panel from the current session state and
// opens it as a focused, dimmed overlay. Changes apply immediately via the
// OnChange hook (theme/plan/review/thinking toggle live); Enter submits and
// closes the panel.
func (s *tuiSession) openSettings() {
	entries := s.buildSettingEntries()
	settings := component.NewSettingsList(entries)

	box := component.NewBox()
	box.SetBorder(component.BorderRounded)
	box.SetTitle("Settings — ←/→ 切换 · Enter 确认")
	box.SetPadding(1, 1)
	box.AddChild(settings)

	// Track the overlay handle so OnSubmit can close it. The closure captures
	// ov by reference; assignment below happens before any user interaction
	// can fire the callbacks, so this ordering is safe.
	var ov chat.OverlayRef
	// OnChange: apply the cycled value immediately so the user sees the effect
	// without having to submit (matches the /theme /plan etc. live behavior).
	// NOTE: SettingsList.submitOrCycle (Enter) fires BOTH OnChange and
	// OnSubmit, so OnSubmit must NOT re-apply — doing so would double-trigger
	// the plan/review toggles (toggle twice = no-op) and rebuild the agent
	// twice. OnSubmit's only job is to close the panel.
	settings.OnChange(func(e component.SettingEntry) { s.applySettingEntry(e) })
	settings.OnSubmit(func(_ component.SettingEntry) {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
	})
	ov = s.app.OpenOverlay(box, chat.OverlayOpts{WidthPct: 60, HeightPct: 50, Dim: true})
}

// buildSettingEntries derives the current setting values from session state.
func (s *tuiSession) buildSettingEntries() []component.SettingEntry {
	// Theme entry reflects the current palette name.
	themeCur := int64(0)
	if s.currentThemeName == "light" {
		themeCur = 1
	}
	// Plan mode: 0=off, 1=on.
	var planCur int64
	if s.planMode {
		planCur = 1
	}
	// Review mode: 0=off, 1=on.
	var reviewCur int64
	if s.reviewMode {
		reviewCur = 1
	}
	// Thinking: 0=default, 1=summarized, 2=omitted.
	thinkingCur := int64(0)
	if s.currentThinking != nil {
		switch s.currentThinking.Display {
		case agentcore.ThinkingDisplaySummarized:
			thinkingCur = 1
		case agentcore.ThinkingDisplayOmitted:
			thinkingCur = 2
		}
	}
	return []component.SettingEntry{
		{
			Key: "theme", Label: "主题",
			Options: []component.SettingOption{
				{Value: "dark", Label: "深色"},
				{Value: "light", Label: "浅色"},
			},
			Current: themeCur,
		},
		{
			Key: "plan", Label: "计划模式",
			Options: []component.SettingOption{
				{Value: "off", Label: "关闭"},
				{Value: "on", Label: "开启"},
			},
			Current: planCur,
		},
		{
			Key: "review", Label: "审核关卡",
			Options: []component.SettingOption{
				{Value: "off", Label: "关闭"},
				{Value: "on", Label: "开启"},
			},
			Current: reviewCur,
		},
		{
			Key: "thinking", Label: "推理显示",
			Options: []component.SettingOption{
				{Value: "default", Label: "默认"},
				{Value: "summarized", Label: "摘要"},
				{Value: "omitted", Label: "隐藏"},
			},
			Current: thinkingCur,
		},
	}
}

// applySettingEntry reacts to a settings change by delegating to the
// existing slash-command handlers, so the panel and the command line stay in
// sync (single behavior, two entry points).
func (s *tuiSession) applySettingEntry(e component.SettingEntry) {
	switch e.Key {
	case "theme":
		val := e.Options[e.Current].Value
		s.handleThemeCommand("/theme " + val)
	case "plan":
		s.handlePlanCommand()
	case "review":
		s.handleReviewCommand()
	case "thinking":
		val := e.Options[e.Current].Value
		s.handleThinkingCommand("/thinking " + val)
	}
}
