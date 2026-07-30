//go:build darwin

package main

// app_settings.go — AI 服务设置（Q9：全局切换 + 新会话生效）
// 与 sandbox/path 辅助函数。
//
// AISettings 持久化到 ~/.mady/desktop-settings.json；运行时切换仅对后续
// 新建会话生效，已有会话保持原有模型。

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"

	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/util"
)

// AISettings 是设置面板读写的 AI 服务配置。
// 持久化到 ~/.mady/desktop-settings.json；运行时切换仅对后续新建会话
// 生效，已有会话保持原有模型（Q9 语义）。
type AISettings struct {
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	LastProjectID string `json:"last_project_id,omitempty"`
}

// aiSettingsPath 返回桌面端设置文件路径。
func aiSettingsPath(madyHome string) string {
	return filepath.Join(madyHome, "desktop-settings.json")
}

// loadAISettingsFrom 从指定路径读取 AI 设置。文件不存在或解析失败时
// 返回零值（视为无用户覆盖），不视为错误。
func loadAISettingsFrom(path string) AISettings {
	data, err := os.ReadFile(path) //nolint:gosec // path 由 MadyHome 派生
	if err != nil {
		return AISettings{}
	}
	var s AISettings
	if err := json.Unmarshal(data, &s); err != nil {
		log.Printf("[mady-desktop] invalid AI settings file %s: %v", path, err)
		return AISettings{}
	}
	return s
}

// saveAISettingsTo 将 AI 设置原子写入指定路径（tmp + rename）。
func saveAISettingsTo(path string, s AISettings) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// resolveMadyHome 返回 MadyHome；优先取 framework 上下文（便于测试注入），
// 回退到 util.MadyHome()。
func (a *App) resolveMadyHome() string {
	if a.fc != nil && a.fc.MadyHome != "" {
		return a.fc.MadyHome
	}
	home, err := util.MadyHome()
	if err != nil {
		return ""
	}
	return home
}

// applyLastProject 在启动时恢复上次使用的项目。
// 如果 LastProjectID 存在且对应案件目录仍可用，则将其设为当前 ProjectDir。
func (a *App) applyLastProject(saved AISettings) {
	if saved.LastProjectID == "" || a.fc == nil || a.fc.ProjectRegistry == nil {
		return
	}
	rec, ok := a.fc.ProjectRegistry.Lookup(saved.LastProjectID)
	if !ok {
		log.Printf("[mady-desktop] last project %q not found in registry", saved.LastProjectID)
		return
	}
	if err := domains.ValidateProjectPath(rec.RootPath); err != nil {
		log.Printf("[mady-desktop] last project %q path %s unreachable: %v", rec.ProjectID, rec.RootPath, err)
		return
	}
	a.fc.BaseConfig.ProjectDir = rec.RootPath
	a.fc.ProjectRegistry.Touch(a.ctx, rec.ProjectID)
	log.Printf("[mady-desktop] restored last project: %s (%s)", rec.Alias, rec.RootPath)
}

// GetAISettings 返回当前生效的 Provider/Model，供设置面板展示。
func (a *App) GetAISettings() (AISettings, error) {
	a.aiMu.RLock()
	defer a.aiMu.RUnlock()
	if a.aiProvider == "" && a.aiModel == "" {
		return AISettings{}, fmt.Errorf("GetAISettings: AI settings not initialized")
	}
	return AISettings{Provider: a.aiProvider, Model: a.aiModel}, nil
}

// SetAISettings 切换全局 Provider/Model（Q9：全局切换 + 新会话生效）。
//
// 行为契约：
//  1. 持久化到 ~/.mady/desktop-settings.json（重启后仍生效）；
//  2. 运行时立即对后续新建会话生效；已有会话保持原有模型；
//  3. Provider 切换会依据目标 Provider 的 API Key 重建 Provider 实例，
//     重建失败时返回错误且不变更任何状态（环境变量一并回滚）。
func (a *App) SetAISettings(s AISettings) error {
	if s.Provider == "" && s.Model == "" {
		return fmt.Errorf("SetAISettings: provider 或 model 至少一项必填")
	}

	a.aiMu.Lock()
	defer a.aiMu.Unlock()

	newProvider := a.aiProvider
	if s.Provider != "" {
		newProvider = s.Provider
	}
	newModel := a.aiModel
	if s.Model != "" {
		newModel = s.Model
	}

	// Provider 变化时重建 Provider 实例（依据目标 Provider 的 API Key）。
	var providerIface agentcore.Provider
	if newProvider != a.aiProvider {
		prev := os.Getenv("PROVIDER")
		_ = os.Setenv("PROVIDER", newProvider)
		p, err := agentconfig.BuildProvider()
		if err != nil {
			_ = os.Setenv("PROVIDER", prev) // 回滚环境变量
			return fmt.Errorf("SetAISettings: 重建 Provider 失败（请确认 %s 的 API Key 已配置）: %w", newProvider, err)
		}
		providerIface = p
	}

	// 持久化（原子写）；失败时不变更运行时状态。
	// 保留已有的 last_project_id，避免 AI 设置保存覆盖项目状态。
	if home := a.resolveMadyHome(); home != "" {
		saved := loadAISettingsFrom(aiSettingsPath(home))
		saved.Provider = newProvider
		saved.Model = newModel
		if err := saveAISettingsTo(aiSettingsPath(home), saved); err != nil {
			return fmt.Errorf("SetAISettings: 保存配置失败: %w", err)
		}
	}

	// 运行时生效：更新 framework 上下文与 server 全局配置。
	// server.SwitchModel 仅影响后续新建 agent；池中已有会话保持不变。
	ctxWindow := agentconfig.ResolveContextWindow(newModel)
	if a.fc != nil {
		if providerIface != nil {
			a.fc.BaseConfig.Provider = providerIface
		}
		a.fc.BaseConfig.Model = newModel
		a.fc.BaseConfig.ContextWindow = ctxWindow
	}
	if a.server != nil {
		a.server.SwitchModel(providerIface, newModel, ctxWindow)
	}

	a.aiProvider = newProvider
	a.aiModel = newModel
	log.Printf("[mady-desktop] AI settings updated: provider=%s model=%s", newProvider, newModel)
	return nil
}

// --- 项目树操作（T3.2b） ---

// resolveProjectDir 返回当前可用的项目根目录。
// 优先使用 ProjectDir（由 CWD 解析），回退到 WorkspaceDir。
func (a *App) resolveProjectDir() (string, error) {
	cwd := a.fc.BaseConfig.ProjectDir
	if cwd == "" {
		cwd = a.fc.BaseConfig.WorkspaceDir
	}
	if cwd == "" {
		return "", fmt.Errorf("no working directory available")
	}
	return cwd, nil
}

// isPathWithinSandbox 检查 target 是否位于 sandboxRoot 之下。
// 防止路径穿越攻击（path traversal）。
func isPathWithinSandbox(target, sandboxRoot string) bool {
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	cleanRoot, err := filepath.Abs(sandboxRoot)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil {
		return false
	}
	// rel 以 ".." 开头说明 target 在 sandboxRoot 之外
	return len(rel) < 2 || rel[:2] != ".."
}
