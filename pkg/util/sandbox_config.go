package util

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SandboxConfig 是 ~/.mady/config.yaml 中 sandbox 段的结构化表示。
type SandboxConfig struct {
	AllowRead  []string `yaml:"allow_read"`
	AllowWrite []string `yaml:"allow_write"`
	// SiblingCasesVisible 控制 workspace 内同级案件是否互可见（只读）。
	// 默认 true。设为 false 则严格隔离。
	SiblingCasesVisible *bool `yaml:"sibling_cases_visible"`
}

// LoadSandboxConfig 从 ~/.mady/config.yaml 加载沙箱白名单配置。
// 如果文件不存在或无 sandbox 段，返回空配置（不报错）。
func LoadSandboxConfig() (*SandboxConfig, error) {
	home, err := MadyHome()
	if err != nil {
		return &SandboxConfig{}, nil
	}
	return LoadSandboxConfigFromPath(filepath.Join(home, "config.yaml"))
}

// LoadSandboxConfigFromPath 从指定路径加载配置文件。
// 文件不存在视为合法状态（返回空配置）；文件存在但读取/解析失败时
// 输出警告日志但仍返回空配置（不阻断启动），因为配置问题不应导致
// 整个应用拒绝启动。
func LoadSandboxConfigFromPath(path string) (*SandboxConfig, error) {
	cfg := &SandboxConfig{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // 合法：配置文件尚未创建
		}
		slog.Warn("sandbox: config read failed, using empty allowlist", "path", path, "err", err)
		return cfg, nil
	}

	var raw struct {
		Sandbox *SandboxConfig `yaml:"sandbox"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		slog.Warn("sandbox: config YAML parse failed, using empty allowlist", "path", path, "err", err)
		return cfg, nil
	}
	if raw.Sandbox != nil {
		cfg = raw.Sandbox
	}
	return cfg, nil
}

// SaveSandboxConfig 将沙箱配置写入 ~/.mady/config.yaml。
// 如果文件已存在，保留其他段，仅合并 sandbox 段。
func SaveSandboxConfig(cfg *SandboxConfig) error {
	home, err := MadyHome()
	if err != nil {
		return err
	}
	return SaveSandboxConfigToPath(filepath.Join(home, "config.yaml"), cfg)
}

// SaveSandboxConfigToPath 将配置写入指定路径，保留已有非 sandbox 段。
func SaveSandboxConfigToPath(path string, cfg *SandboxConfig) error {
	// 读取已有配置，保留非 sandbox 段。
	existing := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &existing); err != nil {
			slog.Warn("sandbox: existing config parse failed, will overwrite", "path", path, "err", err)
		}
	}
	existing["sandbox"] = cfg

	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal sandbox config: %w", err)
	}
	return os.WriteFile(path, out, 0600)
}

// AddKnowledgeDir 向配置文件的 allow_read 列表追加一条路径（去重）。
// 如果路径已存在则不重复添加。
func AddKnowledgeDir(path string) error {
	cfg, err := LoadSandboxConfig()
	if err != nil {
		return err
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	for _, existing := range cfg.AllowRead {
		if existing == abs {
			return nil // 已存在
		}
	}

	cfg.AllowRead = append(cfg.AllowRead, abs)
	return SaveSandboxConfig(cfg)
}

// LoadKnowledgeDirsFromEnv 从环境变量 KNOWLEDGE_DIRS 加载知识库路径列表。
// 格式：冒号（Unix）或分号（Windows）分隔的绝对路径。
// 返回的路径经过 filepath.Abs 规范化。
func LoadKnowledgeDirsFromEnv() []string {
	raw := os.Getenv("KNOWLEDGE_DIRS")
	if raw == "" {
		return nil
	}

	// Unix 用冒号，Windows 用分号
	separator := ":"
	if filepath.Separator == '\\' {
		separator = ";"
	}

	var dirs []string
	for _, d := range strings.Split(raw, separator) {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		// 展开 ~ 前缀
		if d == "~" || strings.HasPrefix(d, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				if d == "~" {
					d = home
				} else {
					d = filepath.Join(home, d[2:])
				}
			}
		}
		abs, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		dirs = append(dirs, abs)
	}
	return dirs
}

// MergeAllowRead 合并多个 AllowRead 来源：配置文件 + 环境变量 + 自动白名单。
// 去重后返回。
func MergeAllowRead(sources ...[]string) []string {
	seen := make(map[string]bool)
	var merged []string
	for _, src := range sources {
		for _, dir := range src {
			if !seen[dir] {
				seen[dir] = true
				merged = append(merged, dir)
			}
		}
	}
	return merged
}
