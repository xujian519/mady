package domains

import (
	"os"
	"path/filepath"

	"github.com/xujian519/mady/pkg/util"
)

// BuildSandboxAllowLists 构建沙箱只读/读写白名单。
// 合并三个来源（优先级从低到高）：
//  1. 自动白名单：$MADY_HOME/doc-templates（只读）、os.TempDir()/mady-*（读写）
//  2. config.yaml：用户持久化配置（~/.mady/config.yaml 的 sandbox.allow_read）
//  3. 环境变量：KNOWLEDGE_DIRS（只读，优先级最高，临时覆盖）
//
// 返回 (allowRead, allowWrite)。
func BuildSandboxAllowLists() (allowRead, allowWrite []string) {
	// 1. 自动白名单：$MADY_HOME/doc-templates（只读）
	if home, err := util.MadyHome(); err == nil {
		docTmpl := filepath.Join(home, "doc-templates")
		if info, err := os.Stat(docTmpl); err == nil && info.IsDir() {
			allowRead = append(allowRead, docTmpl)
		}
	}

	// 2. config.yaml 持久化配置
	if cfg, err := util.LoadSandboxConfig(); err == nil && cfg != nil {
		allowRead = append(allowRead, cfg.AllowRead...)
		allowWrite = append(allowWrite, cfg.AllowWrite...)
	}

	// 3. 环境变量 KNOWLEDGE_DIRS（只读）
	envDirs := util.LoadKnowledgeDirsFromEnv()

	// 合并去重
	allowRead = util.MergeAllowRead(allowRead, envDirs)

	// 4. 自动白名单：临时目录（读写）。
	// 使用具体目录名而非 glob — isWithin 做字面路径匹配，不支持通配符。
	// 目录由 util.EnsureDir 创建，保证存在。
	if tmpDir := os.TempDir(); tmpDir != "" {
		madyTmp := filepath.Join(tmpDir, "mady")
		if err := util.EnsureDir(madyTmp); err == nil {
			allowWrite = append(allowWrite, madyTmp)
		}
	}

	return allowRead, allowWrite
}
