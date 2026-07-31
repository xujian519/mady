package domains

import (
	"github.com/xujian519/mady/domains/config"
)

// 案件注册表的实现位于 domains/config（config/project.go），本文件为
// domains 父包提供类型别名与函数转发，保持既有调用方（TUI/desktop/
// bootstrap）的导入路径不变。类型别名下 ProjectRegistry 的全部接收者
// 方法（Register/Lookup/List/Delete/Touch/RefreshStatus/SaveMeta/LoadMeta）
// 对调用方直接可用，无需逐一转发。

// ProjectRegistry 案件注册表（见 config.ProjectRegistry）。
type ProjectRegistry = config.ProjectRegistry

// ProjectRecord 案件记录（见 config.ProjectRecord）。
type ProjectRecord = config.ProjectRecord

// ProjectMeta 案件元数据（见 config.ProjectMeta）。
type ProjectMeta = config.ProjectMeta

// Deadline 法定期限提醒（见 config.Deadline）。
type Deadline = config.Deadline

// Project status constants.
const (
	StatusActive      = config.StatusActive
	StatusArchived    = config.StatusArchived
	StatusUnreachable = config.StatusUnreachable
)

// NewProjectRegistry 创建案件注册表（见 config.NewProjectRegistry）。
func NewProjectRegistry(baseDir string) (*ProjectRegistry, error) {
	return config.NewProjectRegistry(baseDir)
}

// NewProjectRegistryOrEmpty 创建案件注册表，失败时返回空注册表
// （见 config.NewProjectRegistryOrEmpty）。
func NewProjectRegistryOrEmpty(baseDir string) *ProjectRegistry {
	return config.NewProjectRegistryOrEmpty(baseDir)
}

// ValidateProjectPath 校验项目路径合法性（见 config.ValidateProjectPath）。
func ValidateProjectPath(p string) error {
	return config.ValidateProjectPath(p)
}
