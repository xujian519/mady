package legal

import (
	"os"
	"path/filepath"
)

// 数据目录与文件名约定。
const (
	dataDirName = "legal-knowledge"
	queryDBName = "legal.db"
)

// ResolvePaths 返回查询库路径与安装源路径。
//
// 查询库优先使用安装产物 $MADY_HOME/legal-knowledge/legal.db；未安装时直接
// 只读使用现有的 laws-full*.db——先找带 FTS 索引的 laws-full-local.db，
// 再退回 laws-full.db。两者都不存在时返回安装目标路径并让调用方降级。
func ResolvePaths(madyHome string) (queryPath, sourcePath string) {
	if madyHome == "" {
		return "", ""
	}
	installed := DefaultInstallPath(madyHome)
	for _, name := range []string{"laws-full-local.db", "laws-full.db"} {
		candidate := filepath.Join(madyHome, "knowledge", name)
		if fileExists(candidate) {
			sourcePath = candidate
			break
		}
	}
	if fileExists(installed) {
		return installed, sourcePath
	}
	if sourcePath != "" {
		return sourcePath, sourcePath
	}
	return installed, ""
}

// DefaultInstallPath 返回安装产物的默认路径。
func DefaultInstallPath(madyHome string) string {
	return filepath.Join(madyHome, dataDirName, queryDBName)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
