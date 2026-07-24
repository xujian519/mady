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

	// Track the overlay handle so OnSubmit/OnCancel can close it.
	var ov chat.OverlayRef
	// OnChange: apply immediately via store, which guarantees idempotent writes.
	// SettingsList.submitOrCycle (Enter) fires both OnChange and OnSubmit;
	// re-applying via the store is safe (same value = no-op).
	settings.OnChange(func(e component.SettingEntry) { s.applySettingEntry(e) })
	settings.OnSubmit(func(_ component.SettingEntry) {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
	})
	settings.OnCancel(func() {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
	})
	ov = s.app.OpenOverlay(box, chat.OverlayOpts{WidthPct: 60, HeightPct: 50, Dim: true, Category: chat.OverlayCatReview})
}

// buildSettingEntries derives the current setting values from session state.
func (s *tuiSession) buildSettingEntries() []component.SettingEntry {
	// Theme entry reflects the current palette name.
	themeCur := int64(0)
	if s.themeName() == "dark" {
		themeCur = 0 // dark (品牌冷色)
	} else {
		themeCur = 1 // light
	}
	// Plan mode: 0=off, 1=on.
	var planCur int64
	if s.isPlanMode() {
		planCur = 1
	}
	// Review mode: 0=off, 1=on.
	var reviewCur int64
	if s.isReviewMode() {
		reviewCur = 1
	}
	// Thinking: 0=default, 1=summarized, 2=omitted.
	thinkingCur := int64(0)
	if s.thinkingConfig() != nil {
		switch s.thinkingConfig().Display {
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
	val := e.Options[e.Current].Value
	// 直接通过子命令 handler 应用设置，handler 内部负责写入 store 并重建 agent。
	// 注意：不要在 handler 之前写 store，否则 handler 的幂等检查会误判"已在目标状态"而跳过重建。
	switch e.Key {
	case "theme":
		s.handleThemeCommand("/theme " + val)
	case "plan":
		s.handlePlanCommandEx(val) // "on" or "off" — idempotent
	case "review":
		s.handleReviewCommandEx(val) // "on" or "off" — idempotent
	case "thinking":
		s.handleThinkingCommand("/thinking " + val)
	}
}

// openCommandCenter builds a CommandCenter from the slash registry and opens
// it as a focused, dimmed overlay. Use /cmd or Ctrl+P to invoke.
// filter is an optional initial search term (empty = no pre-filter).
func (s *tuiSession) openCommandCenter(filter ...string) {
	items := s.buildCommandItems()
	cc := component.NewCommandCenter(items)
	cc.OnExecute(func(item component.CommandItem) {
		s.app.PrintSystem("▸ " + item.Label)
		s.handleSubmit(item.Label)
	})
	cc.OnClose(func() {
		// Overlay close handled by the overlay system
	})

	// Pre-fill search filter when provided (e.g. from a misspelled slash command)
	if len(filter) > 0 && filter[0] != "" {
		cc.SetFilter(filter[0])
	}

	box := component.NewBox()
	box.SetBorder(component.BorderRounded)
	box.SetTitle("命令中心 — 搜索 / 执行  ·  Esc 关闭")
	box.SetPadding(1, 1)
	box.AddChild(cc)

	_ = s.app.OpenOverlay(box, chat.OverlayOpts{WidthPct: 70, HeightPct: 60, Dim: true, Category: chat.OverlayCatReview})
}

// buildCommandItems converts slash registry commands to CommandItems.
func (s *tuiSession) buildCommandItems() []component.CommandItem {
	categoryNames := map[string]string{
		"mode":     "⚙ 模式",
		"session":  "📂 会话",
		"case":     "📋 案件",
		"settings": "🔧 设置",
		"general":  "📌 通用",
	}
	var items []component.CommandItem
	for _, cmd := range s.slashReg.cmds {
		avail, reason := true, ""
		if cmd.Available != nil {
			avail, reason = cmd.Available(s)
		}
		cat := cmd.Category
		if name, ok := categoryNames[cat]; ok {
			cat = name
		}
		label := "/" + cmd.Name
		if cmd.Usage != "" {
			label = cmd.Usage
		}

		// 为模式/开关类命令填充当前状态
		status := s.resolveCommandStatus(cmd.Name)
		items = append(items, component.CommandItem{
			Name:        cmd.Name,
			Label:       label,
			Category:    cat,
			Description: cmd.Desc,
			Status:      status,
			Available:   avail,
			Reason:      reason,
		})
	}
	return items
}

// resolveCommandStatus 返回命令的当前状态文本（用于命令中心展示）。
func (s *tuiSession) resolveCommandStatus(name string) string {
	switch name {
	case "plan":
		if s.isPlanMode() {
			return "开启"
		}
		return "关闭"
	case "review":
		if s.isReviewMode() {
			return "开启"
		}
		return "关闭"
	case "thinking":
		cfg := s.thinkingConfig()
		if cfg == nil || cfg.Display == "" || cfg.Display == agentcore.ThinkingDisplayDefault {
			return "默认"
		}
		switch cfg.Display {
		case agentcore.ThinkingDisplaySummarized:
			return "摘要"
		case agentcore.ThinkingDisplayOmitted:
			return "隐藏"
		default:
			return "默认"
		}
	case "theme":
		if s.themeName() == "dark" {
			return "深色"
		}
		return "浅色"
	}
	return ""
}
