package browser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BrowserRetrieverConfig 配置 ego-browser 检索器的运行参数。
type BrowserRetrieverConfig struct {
	// EgoBrowserPath 是 ego-browser CLI 的路径。为空时自动从 PATH 与
	// ~/.local/bin（ego lite 安装后注册的默认位置）解析。
	EgoBrowserPath string

	// Timeout 是单次 ego-browser 调用超时（默认 90s，浏览器启动与页面
	// 加载较慢）。ego-browser 的 wait/timeout 参数单位为秒。
	Timeout time.Duration
}

// DefaultConfig 返回带默认值的配置，并自动解析 ego-browser 路径。
// 解析失败时 EgoBrowserPath 保持空串，由工厂函数决定降级。
func DefaultConfig() *BrowserRetrieverConfig {
	return &BrowserRetrieverConfig{
		EgoBrowserPath: FindEgoBrowser(),
		Timeout:        90 * time.Second,
	}
}

// FindEgoBrowser 查找 ego-browser CLI 可执行文件。查找顺序：
// PATH → ~/.local/bin/ego-browser（ego lite onboarding 注册位置）。
// 未找到返回空串。
func FindEgoBrowser() string {
	if p := os.Getenv("EGO_BROWSER_PATH"); p != "" {
		return p
	}
	if p, err := exec.LookPath("ego-browser"); err == nil {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	candidates := []string{
		filepath.Join(home, ".local", "bin", "ego-browser"),
		filepath.Join(home, "bin", "ego-browser"),
	}
	homePrefix := home + string(filepath.Separator)
	for _, c := range candidates {
		clean := filepath.Clean(c)
		// 路径必须位于用户主目录内（防 taint 逃逸）。
		if !strings.HasPrefix(clean, homePrefix) {
			continue
		}
		if st, err := os.Stat(clean); err == nil && !st.IsDir() {
			return clean
		}
	}
	return ""
}

// IsAvailable 报告 ego-browser 是否可用（非空且可执行）。
func (c *BrowserRetrieverConfig) IsAvailable() bool {
	return c != nil && c.EgoBrowserPath != ""
}

// RetrieversEnabled 报告在线专利数据库检索器（ego-browser 驱动）是否启用。
// 环境变量 MADY_BROWSER_RETRIEVERS 控制：空（未设置）视为启用；明确禁用值
// 为 off/false/0/disabled/no（大小写不敏感，容忍常见 falsy 写法），其余视为启用。
// 语义为保密性隔离（未公开发明不联网），所有检索器工厂与工具注册应统一读取
// 本函数，避免门控不一致。
func RetrieversEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MADY_BROWSER_RETRIEVERS"))) {
	case "off", "false", "0", "disabled", "no":
		return false
	default:
		return true
	}
}
