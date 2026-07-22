package domains

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/pkg/util"
)

// 案件工作区子目录名。在用户案卷文件夹下创建 .mady/ 隐藏目录，
// 用于存放 AI 中间产物（草稿、分析报告、检查点、索引缓存）。
// 目录名本身复用 util.AppDirName 保持全仓一致。
const (
	MadyCheckpoints = "checkpoints"
	MadyDrafts      = "drafts"
	MadyAnalysis    = "analysis"
	MadyIndexDB     = "index.db"
)

// EnsureCaseWorkspace 在指定案卷目录下创建 .mady/ 工作区子目录。
// 如果已存在则幂等返回。返回 .mady/ 的完整路径。
func EnsureCaseWorkspace(caseRootPath string) (string, error) {
	madyDir := filepath.Join(caseRootPath, util.AppDirName)
	for _, sub := range []string{MadyCheckpoints, MadyDrafts, MadyAnalysis} {
		if err := os.MkdirAll(filepath.Join(madyDir, sub), 0o755); err != nil {
			return "", fmt.Errorf("case workspace: create %s: %w", sub, err)
		}
	}
	return madyDir, nil
}

// IsCaseWorkspace 检查指定目录下是否已有 .mady/ 工作区。
func IsCaseWorkspace(path string) bool {
	info, err := os.Stat(filepath.Join(path, util.AppDirName))
	return err == nil && info.IsDir()
}

// CaseWorkspacePath 返回案件工作区中某个子目录的完整路径。
// subDir 为 MadyCheckpoints/MadyDrafts/MadyAnalysis 之一。
func CaseWorkspacePath(caseRootPath, subDir string) string {
	return filepath.Join(caseRootPath, util.AppDirName, subDir)
}

// CaseIndexDBPath 返回案件索引数据库的路径。
func CaseIndexDBPath(caseRootPath string) string {
	return filepath.Join(caseRootPath, util.AppDirName, MadyIndexDB)
}

// DraftPath 返回案件工作区中草稿文件的完整路径。
// filename 不含路径分隔符。
func DraftPath(caseRootPath, filename string) string {
	return filepath.Join(caseRootPath, util.AppDirName, MadyDrafts, filename)
}

// AnalysisPath 返回案件工作区中分析报告的完整路径。
func AnalysisPath(caseRootPath, filename string) string {
	return filepath.Join(caseRootPath, util.AppDirName, MadyAnalysis, filename)
}
