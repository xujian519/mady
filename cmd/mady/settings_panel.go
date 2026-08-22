package main

// settings_panel.go wires the SettingsList component into the TUI as a
// centered overlay. It is the template for how app-level panels (settings,
// session picker, todo, …) mount via ChatApp.OpenOverlay: build the
// component, wrap it in a Box, push it as an overlay, and wire OnSubmit to
// close + apply.

import (
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/bootstrap/agentconfig"
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

	var ov chat.OverlayRef
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
	themeCur := int64(0)
	if s.themeName() == valDark {
		themeCur = 0
	} else {
		themeCur = 1
	}
	var planCur int64
	if s.isPlanMode() {
		planCur = 1
	}
	var reviewCur int64
	if s.isReviewMode() {
		reviewCur = 1
	}
	thinkingCur := int64(0)
	if s.thinkingConfig() != nil {
		switch s.thinkingConfig().Display {
		case agentcore.ThinkingDisplaySummarized:
			thinkingCur = 1
		case agentcore.ThinkingDisplayOmitted:
			thinkingCur = 2
		}
	}

	allProviders := agentconfig.ProviderCatalog()
	providerCur := int64(0)
	for i, p := range allProviders {
		if p.Name == s.providerName {
			providerCur = int64(i)
			break
		}
	}

	models := agentconfig.ModelsForProvider(s.providerName)
	modelCur := int64(0)
	for i, m := range models {
		if m.Name == s.model {
			modelCur = int64(i)
			break
		}
	}

	return []component.SettingEntry{
		{
			Key: SettingKeyTheme, Label: "主题",
			Options: []component.SettingOption{
				{Value: valDark, Label: "深色"},
				{Value: DefaultTheme, Label: "浅色"},
			},
			Current: themeCur,
		},
		{
			Key: SettingKeyPlan, Label: "计划模式",
			Options: []component.SettingOption{
				{Value: DefaultPlan, Label: "关闭"},
				{Value: "on", Label: "开启"},
			},
			Current: planCur,
		},
		{
			Key: SettingKeyReview, Label: "审核关卡",
			Options: []component.SettingOption{
				{Value: DefaultReview, Label: "关闭"},
				{Value: "on", Label: "开启"},
			},
			Current: reviewCur,
		},
		{
			Key: SettingKeyThinking, Label: "推理显示",
			Options: []component.SettingOption{
				{Value: DefaultThinking, Label: "默认"},
				{Value: valSummarized, Label: "摘要"},
				{Value: valOmitted, Label: "隐藏"},
			},
			Current: thinkingCur,
		},
		{
			Key: SettingKeyProvider, Label: "模型提供方",
			Options: buildProviderOptions(),
			Current: providerCur,
		},
		{
			Key: SettingKeyModel, Label: "模型",
			Options: buildModelOptions(s.providerName),
			Current: modelCur,
		},
	}
}

func buildProviderOptions() []component.SettingOption {
	var opts []component.SettingOption
	for _, p := range agentconfig.ProviderCatalog() {
		opts = append(opts, component.SettingOption{Value: p.Name, Label: p.Label})
	}
	return opts
}

func buildModelOptions(providerName string) []component.SettingOption {
	models := agentconfig.ModelsForProvider(providerName)
	var opts []component.SettingOption
	for _, m := range models {
		opts = append(opts, component.SettingOption{Value: m.Name, Label: m.Label})
	}
	if len(opts) == 0 {
		opts = []component.SettingOption{{Value: "", Label: "(无可用模型)"}}
	}
	return opts
}

// applySettingEntry reacts to a settings change by delegating to the
// existing slash-command handlers, so the panel and the command line stay in
// sync (single behavior, two entry points).
func (s *tuiSession) applySettingEntry(e component.SettingEntry) {
	if e.Current < 0 || e.Current >= int64(len(e.Options)) {
		return // 边界检查：无有效选项
	}
	val := e.Options[e.Current].Value
	switch e.Key {
	case SettingKeyTheme:
		s.handleThemeCommand("/theme " + val)
	case SettingKeyPlan:
		s.handlePlanCommandEx(val)
	case SettingKeyReview:
		s.handleReviewCommandEx(val)
	case SettingKeyThinking:
		s.handleThinkingCommand("/thinking " + val)
	case SettingKeyProvider:
		if err := s.switchProvider(val); err != nil {
			s.app.PrintError(fmt.Errorf("切换 Provider 失败: %w\n请确认 API Key 已通过环境变量配置", err))
		}
	case SettingKeyModel:
		if err := s.switchModel(val); err != nil {
			s.app.PrintError(fmt.Errorf("切换模型失败: %w", err))
		}
	default:
		// 未知设置键，静默忽略（未来扩展安全兜底）
	}
}

// openCommandCenter builds a CommandCenter from the slash registry and opens
// it as a focused, dimmed overlay. Use /cmd or Ctrl+P to invoke.
func (s *tuiSession) openCommandCenter(filter ...string) {
	items := s.buildCommandItems()
	cc := component.NewCommandCenter(items)

	var ov chat.OverlayRef
	cc.OnExecute(func(item component.CommandItem) {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
		s.app.PrintSystem("▸ " + item.Label)
		s.handleSubmit(item.Label)
	})
	cc.OnClose(func() {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
	})

	if len(filter) > 0 && filter[0] != "" {
		cc.SetFilter(filter[0])
	}

	box := component.NewBox()
	box.SetBorder(component.BorderRounded)
	box.SetTitle("命令中心 — 搜索 / 执行  ·  Esc 关闭")
	box.SetPadding(1, 1)
	box.AddChild(cc)

	ov = s.app.OpenOverlay(box, chat.OverlayOpts{WidthPct: 70, HeightPct: 60, Dim: true, Category: chat.OverlayCatReview})
}

func (s *tuiSession) buildCommandItems() []component.CommandItem {
	categoryNames := map[string]string{
		catMode:     "⚙ 模式",
		catSession:  "📂 会话",
		catCase:     "📋 案件",
		catSettings: "🔧 设置",
		catGeneral:  "📌 通用",
		catInspect:  "🔍 查看",
	}
	items := make([]component.CommandItem, 0, len(s.slashReg.cmds))
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
		status := s.resolveCommandStatus(cmd.Name)
		items = append(items, component.CommandItem{
			Name: cmd.Name, Label: label, Category: cat,
			Description: cmd.Desc, Status: status,
			Available: avail, Reason: reason,
		})
	}
	return items
}

func (s *tuiSession) resolveCommandStatus(name string) string {
	switch name {
	case SettingKeyPlan:
		if s.isPlanMode() {
			return "开启"
		}
		return "关闭"
	case SettingKeyReview:
		if s.isReviewMode() {
			return "开启"
		}
		return "关闭"
	case SettingKeyThinking:
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
	case SettingKeyTheme:
		if s.themeName() == valDark {
			return "深色"
		}
		return "浅色"
	case SettingKeyProvider:
		return agentconfig.ProviderNameLabel(s.providerName)
	case SettingKeyModel:
		return s.model
	}
	return ""
}

// openModelPicker 打开模型浏览器 overlay，使用 SelectList 模糊搜索。
func (s *tuiSession) openModelPicker() {
	items := buildModelSelectItems(s.providerName)
	picker := component.NewSelectList(items)
	picker.SetMaxVisible(12)

	box := component.NewBox()
	box.SetBorder(component.BorderRounded)
	box.SetTitle("模型选择 — 输入关键词筛选 · Enter 确认 · Esc 取消")
	box.SetPadding(1, 1)
	box.AddChild(picker)

	var ov chat.OverlayRef
	picker.OnSelect(func(item component.SelectItem) {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
		if err := s.switchModel(item.Value); err != nil {
			s.app.PrintError(fmt.Errorf("切换模型失败: %w", err))
			return
		}
		s.app.PrintSystem("🤖 已切换模型: " + s.normalModel)
	})
	picker.OnCancel(func() {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
	})

	ov = s.app.OpenOverlay(box, chat.OverlayOpts{WidthPct: 60, HeightPct: 60, Dim: true, Category: chat.OverlayCatReview})
}

func buildModelSelectItems(providerName string) []component.SelectItem {
	var items []component.SelectItem
	models := agentconfig.ModelsForProvider(providerName)
	if len(models) > 0 {
		for _, m := range models {
			items = append(items, component.SelectItem{
				Value: m.Name, Label: m.Label,
				Description: m.Description,
				Group:       m.Group,
			})
		}
	}
	if len(items) == 0 {
		items = append(items, component.SelectItem{
			Value: "", Label: "(无可选模型)",
			Description: "该 Provider 当前无可用模型",
		})
	}
	return items
}
