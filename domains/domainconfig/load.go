package domainconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadConfig 从 YAML/JSON 文件加载单个领域配置。
//
// 根据文件扩展名自动选择解析器：
//   - .yaml / .yml → yaml.v3
//   - .json        → encoding/json
func LoadConfig(path string) (*DomainConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("domainconfig: read %q: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var cfg DomainConfig

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("domainconfig: parse yaml %q: %w", path, err)
		}
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("domainconfig: parse json %q: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("domainconfig: unsupported extension %q for %q (want .yaml/.yml/.json)", ext, path)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("domainconfig: validate %q: %w", path, err)
	}

	return &cfg, nil
}

// LoadConfigs 从目录扫描所有 .yaml/.yml/.json 文件并加载。
//
// 非配置文件和子目录被静默跳过。读取或校验失败的单个文件会返回错误，
// 已成功加载的配置不会丢失——调用方应处理部分成功场景。
func LoadConfigs(dir string) ([]*DomainConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("domainconfig: read dir %q: %w", dir, err)
	}

	var configs []*DomainConfig
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		cfg, err := LoadConfig(path)
		if err != nil {
			return configs, fmt.Errorf("domainconfig: load %q: %w", path, err)
		}
		configs = append(configs, cfg)
	}

	return configs, nil
}

// DefaultConfigDir 返回默认配置目录路径。
//
// 解析优先级同 util.MadyHome():
//  1. $MADY_HOME/domains/
//  2. $HOME/.mady/domains/
//
// 目录不保证存在，调用方应自行处理 os.IsNotExist 错误。
func DefaultConfigDir() string {
	// 优先 MADY_HOME 环境变量
	if env := os.Getenv("MADY_HOME"); env != "" {
		return filepath.Join(env, "domains")
	}

	// 回退到家目录下的 .mady
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".mady", "domains")
	}

	// 最终回退：cwd 下的 .mady/domains
	return filepath.Join(".mady", "domains")
}
