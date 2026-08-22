package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/xujian519/mady/bootstrap/agentconfig"
)

// --- Model/Provider switching ---

// switchProvider 切换 LLM Provider。需要对应 API Key 已通过环境变量配置。
// 成功时更新 session state 并重建 agent；失败时返回错误。
func (s *tuiSession) switchProvider(providerName string) error {
	newProvider, err := agentconfig.BuildProviderFor(providerName)
	if err != nil {
		return err
	}

	// 持久化到 settings
	if err := s.store.Set(SettingKeyProvider, providerName, SettingsScopeGlobal); err != nil {
		log.Printf("settings: persist provider: %v", err)
	}
	s.provider = newProvider
	s.providerName = providerName

	// 自动切换到该 Provider 的默认模型
	defModel := agentconfig.DefaultModelForProvider(providerName)
	if defModel == "" {
		// generic 且 MODEL 未配置 — 保留当前模型不变
		defModel = s.model
	}

	if err := s.store.Set(SettingKeyModel, defModel, SettingsScopeGlobal); err != nil {
		log.Printf("settings: persist model: %v", err)
	}
	s.model = defModel
	s.normalModel = defModel

	s.rebuildAgent()
	s.app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.isPlanMode(), s.thinkingConfig()))
	return nil
}

// switchModel 在当前 Provider 下切换模型。
func (s *tuiSession) switchModel(modelName string) error {
	// 校验模型列表（generic 允许任意模型名）
	if s.providerName != "generic" && !modelBelongsToProvider(modelName, s.providerName) {
		return fmt.Errorf("模型 %q 不属于当前 Provider %q", modelName, s.providerName)
	}

	if err := s.store.Set(SettingKeyModel, modelName, SettingsScopeGlobal); err != nil {
		log.Printf("settings: persist model: %v", err)
	}
	s.model = modelName
	s.normalModel = modelName

	s.rebuildAgent()
	s.app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.isPlanMode(), s.thinkingConfig()))
	return nil
}

// handleProviderCommand 处理 /provider 命令。
func (s *tuiSession) handleProviderCommand(trimmed string) {
	sub := parseSlashSubcommand(trimmed, "provider")
	switch sub {
	case "", "status":
		label := agentconfig.ProviderNameLabel(s.providerName)
		s.app.PrintSystem(fmt.Sprintf("📡 当前模型提供方: %s (%s)", label, s.providerName))
		s.app.PrintSystem("可用提供方: " + strings.Join(agentconfig.ProviderNameList(), ", "))
		s.app.PrintSystem("切换: /provider <名称>")

	case "list":
		var b strings.Builder
		fmt.Fprintf(&b, "📡 可用模型提供方:\n")
		for _, p := range agentconfig.ProviderCatalog() {
			fmt.Fprintf(&b, "  - %s (%s)\n", p.Label, p.Name)
		}
		s.app.PrintSystem(b.String())

	default:
		if err := s.switchProvider(sub); err != nil {
			s.app.PrintError(fmt.Errorf("切换 Provider 失败: %w\n请确认 %s 的 API Key 已通过环境变量配置", err, sub))
			return
		}
		label := agentconfig.ProviderNameLabel(s.providerName)
		s.app.PrintSystem(fmt.Sprintf("📡 已切换提供方: %s · 模型: %s", label, s.normalModel))
	}
}

// handleModelCommand 处理 /model 命令。
func (s *tuiSession) handleModelCommand(trimmed string) {
	sub := parseSlashSubcommand(trimmed, "model")
	switch sub {
	case "":
		s.openModelPicker()
		return

	case "list":
		var b strings.Builder
		fmt.Fprintf(&b, "🤖 可用模型:\n")
		for _, p := range agentconfig.ProviderCatalog() {
			if len(p.Models) == 0 && p.Name == "generic" {
				if m := os.Getenv("MODEL"); m != "" {
					fmt.Fprintf(&b, "  %s: %s（通过 MODEL 环境变量）\n", p.Label, m)
				} else {
					fmt.Fprintf(&b, "  %s: 请设置 MODEL 环境变量\n", p.Label)
				}
				continue
			}
			fmt.Fprintf(&b, "  %s:\n", p.Label)
			for _, m := range p.Models {
				fmt.Fprintf(&b, "    - %s (%s)\n", m.Name, m.Description)
			}
		}
		fmt.Fprintf(&b, "\n当前: %s → %s", agentconfig.ProviderNameLabel(s.providerName), s.model)
		s.app.PrintSystem(b.String())

	default:
		if err := s.switchModel(sub); err != nil {
			s.app.PrintError(fmt.Errorf("切换模型失败: %w", err))
			return
		}
		s.app.PrintSystem(fmt.Sprintf("🤖 已切换模型: %s", s.normalModel))
	}
}

// --- Settings ---

func (s *tuiSession) handleSettingsReset() {
	if err := s.store.Reset(); err != nil {
		s.app.PrintError(fmt.Errorf("settings reset failed: %w", err))
		return
	}
	// 同步 session 层的 provider/model 字段，确保与持久层一致
	s.providerName = DefaultProvider
	s.model = DefaultModel
	s.normalModel = DefaultModel
	if savedProvider := s.store.Get(SettingKeyProvider); savedProvider != "" && savedProvider != DefaultProvider {
		if p, err := agentconfig.BuildProviderFor(savedProvider); err == nil {
			s.provider = p
			s.providerName = savedProvider
		}
	}
	if savedModel := s.store.Get(SettingKeyModel); savedModel != "" && savedModel != DefaultModel {
		s.model = savedModel
		s.normalModel = savedModel
	}
	s.rebuildAgent()
	mdl := s.normalModel
	if s.isPlanMode() {
		mdl = s.planModel
	}
	s.app.UpdateStatusBar(s.providerName, mdl, statusBarModeLabel(s.isPlanMode(), s.thinkingConfig()))
	s.app.PrintSystem("✅ 设置已恢复默认值")
	for k, v := range s.store.Export() {
		s.app.PrintSystem(fmt.Sprintf("  %s = %s", k, v))
	}
}
