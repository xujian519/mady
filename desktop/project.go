//go:build darwin

package main

// project.go — 桌面端项目（案件）管理。
//
// 提供“打开现有文件夹作为项目”和“新建文件夹作为项目”的 Wails Binding，
// 并与 domains.ProjectRegistry 共享项目历史。

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/pkg/util"
)

// ProjectInfo 是桌面端展示的项目摘要。
type ProjectInfo struct {
	ID           string    `json:"id"`
	Alias        string    `json:"alias"`
	Path         string    `json:"path"`
	Status       string    `json:"status"`
	LastAccessed time.Time `json:"lastAccessed"`
}

// projectInfoFromRecord 将 domains.ProjectRecord 转换为桌面端展示结构。
func projectInfoFromRecord(rec domains.ProjectRecord) ProjectInfo {
	return ProjectInfo{
		ID:           rec.ProjectID,
		Alias:        rec.Alias,
		Path:         rec.RootPath,
		Status:       rec.Status,
		LastAccessed: rec.LastAccessed,
	}
}

// ListProjects 返回已注册的项目列表，按最近访问时间倒序。
func (a *App) ListProjects() ([]ProjectInfo, error) {
	if a.fc == nil || a.fc.ProjectRegistry == nil {
		return nil, fmt.Errorf("listProjects: 项目注册表未初始化")
	}

	a.fc.ProjectRegistry.RefreshStatus(a.ctxOrNil())

	recs := a.fc.ProjectRegistry.List()
	// 按 LastAccessed 倒序，零值放最后
	sortProjectRecords(recs)

	result := make([]ProjectInfo, 0, len(recs))
	for _, rec := range recs {
		result = append(result, projectInfoFromRecord(rec))
	}
	return result, nil
}

// GetCurrentProject 返回当前生效的项目。未选择项目时返回零值，不报错。
func (a *App) GetCurrentProject() (ProjectInfo, error) {
	if a.fc == nil {
		return ProjectInfo{}, fmt.Errorf("getCurrentProject: framework 未初始化")
	}

	rootDir, err := a.resolveProjectDir()
	if err != nil {
		return ProjectInfo{}, nil // 未选择项目视为空，不报错
	}

	// 优先从注册表查找匹配 RootPath 的项目
	if a.fc.ProjectRegistry != nil {
		for _, rec := range a.fc.ProjectRegistry.List() {
			if filepath.Clean(rec.RootPath) == filepath.Clean(rootDir) {
				return projectInfoFromRecord(rec), nil
			}
		}
	}

	// 未注册：回退到目录名作为别名
	return ProjectInfo{
		ID:     "",
		Alias:  filepath.Base(rootDir),
		Path:   rootDir,
		Status: domains.StatusActive,
	}, nil
}

// SelectProjectFolder 弹出系统文件夹选择对话框，将选中的文件夹注册为项目并设为当前项目。
func (a *App) SelectProjectFolder() (ProjectInfo, error) {
	ctx := a.ctxOrNil()
	if ctx == nil {
		return ProjectInfo{}, fmt.Errorf("selectProjectFolder: 应用尚未启动")
	}
	if a.fc == nil || a.fc.ProjectRegistry == nil {
		return ProjectInfo{}, fmt.Errorf("selectProjectFolder: 项目注册表未初始化")
	}

	opts := runtime.OpenDialogOptions{
		Title:                "选择项目文件夹",
		CanCreateDirectories: true,
		ResolvesAliases:      true,
	}

	selected, err := runtime.OpenDirectoryDialog(ctx, opts)
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("selectProjectFolder: %w", err)
	}
	if selected == "" {
		return ProjectInfo{}, fmt.Errorf("selectProjectFolder: 未选择文件夹")
	}

	info, err := a.registerAndSwitch(selected)
	if err != nil {
		return ProjectInfo{}, err
	}

	if ctx := a.ctxOrNil(); ctx != nil {
		runtime.WindowReloadApp(ctx)
	}
	return info, nil
}

// CreateProjectFolder 在 workspace/projects 下新建一个文件夹并注册为项目。
// name 为项目名称，将自动清理为合法目录名。
func (a *App) CreateProjectFolder(name string) (ProjectInfo, error) {
	if a.fc == nil || a.fc.ProjectRegistry == nil {
		return ProjectInfo{}, fmt.Errorf("createProjectFolder: 项目注册表未初始化")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return ProjectInfo{}, fmt.Errorf("createProjectFolder: 项目名称不能为空")
	}

	safeName := sanitizeProjectName(name)
	if safeName == "" {
		return ProjectInfo{}, fmt.Errorf("createProjectFolder: 项目名称不合法: %s", name)
	}

	// 使用 workspace/projects 作为默认根目录
	baseDir := filepath.Join(a.fc.WorkspaceDir, "projects")
	if err := util.EnsureDir(baseDir); err != nil {
		return ProjectInfo{}, fmt.Errorf("createProjectFolder: 创建工作区目录失败: %w", err)
	}

	projectDir := filepath.Join(baseDir, safeName)
	if _, err := os.Stat(projectDir); err == nil {
		return ProjectInfo{}, fmt.Errorf("createProjectFolder: 项目文件夹已存在: %s", projectDir)
	}
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		return ProjectInfo{}, fmt.Errorf("createProjectFolder: 创建项目文件夹失败: %w", err)
	}

	info, err := a.registerAndSwitch(projectDir)
	if err != nil {
		return ProjectInfo{}, err
	}

	if ctx := a.ctxOrNil(); ctx != nil {
		runtime.WindowReloadApp(ctx)
	}
	return info, nil
}

// SwitchProject 切换到指定 ID 的项目。
func (a *App) SwitchProject(projectID string) error {
	if a.fc == nil || a.fc.ProjectRegistry == nil {
		return fmt.Errorf("switchProject: 项目注册表未初始化")
	}

	rec, ok := a.fc.ProjectRegistry.Lookup(projectID)
	if !ok {
		return fmt.Errorf("switchProject: 项目 %q 不存在", projectID)
	}
	if err := domains.ValidateProjectPath(rec.RootPath); err != nil {
		return fmt.Errorf("switchProject: 项目路径不可用: %w", err)
	}

	if err := a.setCurrentProject(rec.ProjectID, rec.RootPath); err != nil {
		return err
	}

	if ctx := a.ctxOrNil(); ctx != nil {
		runtime.WindowReloadApp(ctx)
	}
	return nil
}

// registerAndSwitch 注册目录为项目，设为当前项目，并持久化 last_project_id。
func (a *App) registerAndSwitch(rootPath string) (ProjectInfo, error) {
	if err := domains.ValidateProjectPath(rootPath); err != nil {
		return ProjectInfo{}, fmt.Errorf("registerAndSwitch: %w", err)
	}

	alias := filepath.Base(rootPath)
	projectID := generateProjectID(alias)

	// 若路径已注册，复用现有 ID 和别名
	if a.fc.ProjectRegistry != nil {
		for _, rec := range a.fc.ProjectRegistry.List() {
			if filepath.Clean(rec.RootPath) == filepath.Clean(rootPath) {
				projectID = rec.ProjectID
				alias = rec.Alias
				break
			}
		}
	}

	rec := domains.ProjectRecord{
		ProjectID: projectID,
		Alias:     alias,
		RootPath:  rootPath,
		Domain:    domains.DomainPatent,
		Status:    domains.StatusActive,
	}
	if err := a.fc.ProjectRegistry.Register(a.ctxOrNil(), rec); err != nil {
		return ProjectInfo{}, fmt.Errorf("registerAndSwitch: %w", err)
	}

	if err := a.setCurrentProject(projectID, rootPath); err != nil {
		return ProjectInfo{}, err
	}

	return projectInfoFromRecord(rec), nil
}

// setCurrentProject 更新当前 ProjectDir 并持久化 last_project_id。
func (a *App) setCurrentProject(projectID, rootPath string) error {
	a.fc.BaseConfig.ProjectDir = rootPath

	home := a.resolveMadyHome()
	if home != "" {
		// settingsMu（G-I3）：与 SetAISettings 的 load-modify-save 互斥，
		// 防止并发写文件时用旧快照覆盖对方字段。
		a.settingsMu.Lock()
		settings := loadJSONFile[AISettings](aiSettingsPath(home))
		settings.LastProjectID = projectID
		saveErr := saveJSONFile(aiSettingsPath(home), settings)
		a.settingsMu.Unlock()
		if saveErr != nil {
			log.Printf("[mady-desktop] 保存 last_project_id 失败: %v", saveErr)
			return fmt.Errorf("setCurrentProject: 保存项目设置失败: %w", saveErr)
		}
	}

	// G-I2：项目切换后重建 Agent 工具扩展（WorkingDir 沙箱指向新项目根）。
	// 否则前端文件面板已切到新项目，而 LLM 的 ReadFile/WriteFile 仍沙箱在旧项目。
	if a.server != nil {
		a.server.SyncConfig(buildDesktopAgentConfig(a.fc))
		log.Printf("[mady-desktop] project switched to %s — agent sandbox synced", rootPath)
	}
	return nil
}

// generateProjectID 生成一个稳定的项目 ID。
func generateProjectID(alias string) string {
	clean := sanitizeProjectName(alias)
	if clean == "" {
		clean = "project"
	}
	// S-5：Unix 秒粒度同别名必冲突，使用纳秒粒度
	return fmt.Sprintf("%s-%d", clean, time.Now().UnixNano())
}

var projectNameInvalidChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

// sanitizeProjectName 清理项目名称，使其可作为文件/目录名。
func sanitizeProjectName(name string) string {
	name = strings.TrimSpace(name)
	name = projectNameInvalidChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-.")
	return name
}

// sortProjectRecords 按最近访问时间倒序排列；零值排最后。
func sortProjectRecords(recs []domains.ProjectRecord) {
	for i := range recs {
		for j := i + 1; j < len(recs); j++ {
			a, b := recs[i].LastAccessed, recs[j].LastAccessed
			if a.IsZero() {
				if !b.IsZero() {
					recs[i], recs[j] = recs[j], recs[i]
				}
				continue
			}
			if !b.IsZero() && b.After(a) {
				recs[i], recs[j] = recs[j], recs[i]
			}
		}
	}
}
