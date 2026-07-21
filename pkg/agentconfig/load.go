package agentconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadConfig 从 JSON 或 YAML 文件加载 Config。
// 文件扩展名决定解析格式：.json → JSON, .yaml/.yml → YAML。
func LoadConfig(path string) (*Config, error) {
	// Sanitize file path for security scanning.
	safePath := filepath.Clean(path)
	data, err := os.ReadFile(safePath)
	if err != nil {
		return nil, fmt.Errorf("agentconfig: read %s: %w", path, err)
	}

	var cfg Config

	switch {
	case strings.HasSuffix(path, ".json"):
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("agentconfig: parse %s (JSON): %w", path, err)
		}
	case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("agentconfig: parse %s (YAML): %w", path, err)
		}
	default:
		return nil, fmt.Errorf("agentconfig: unsupported config file format: %s (use .json, .yaml, or .yml)", path)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("agentconfig: validate %s: %w", path, err)
	}

	return &cfg, nil
}

// FromEnv 从环境变量构建 Config。
// 环境变量名的优先级高于 Config 文件中的值，可通过 EnvOverride 应用。
func FromEnv() *Config {
	cfg := &Config{}

	if v := os.Getenv("MODEL"); v != "" {
		cfg.Model = v
	}

	if v := os.Getenv("TEMPERATURE"); v != "" {
		if f, err := parseFloat(v); err == nil {
			cfg.Temperature = f
		}
	}

	if v := os.Getenv("MAX_TOKENS"); v != "" {
		if n, err := parseInt64(v); err == nil {
			cfg.MaxTokens = n
		}
	}

	if v := os.Getenv("STREAMING"); v != "" {
		cfg.Streaming = v == "true" || v == "1" || v == "yes"
	}

	if v := os.Getenv("MAX_TURNS"); v != "" {
		if n, err := parseInt64(v); err == nil {
			cfg.MaxTurns = n
		}
	}

	if v := os.Getenv("CONCURRENCY"); v != "" {
		if n, err := parseInt64(v); err == nil {
			cfg.Concurrency = n
		}
	}

	if v := os.Getenv("CONTEXT_WINDOW"); v != "" {
		if n, err := parseInt64(v); err == nil {
			cfg.ContextWindow = n
		}
	}

	if v := os.Getenv("RESERVE_TOKENS"); v != "" {
		if n, err := parseInt64(v); err == nil {
			cfg.ReserveTokens = n
		}
	}

	if v := os.Getenv("KEEP_RECENT_TOKENS"); v != "" {
		if n, err := parseInt64(v); err == nil {
			cfg.KeepRecentTokens = n
		}
	}

	if v := os.Getenv("STRUCTURED_COMPACTION"); v != "" {
		cfg.StructuredCompaction = v == "true" || v == "1"
	}

	if v := os.Getenv("CONTEXT_ENGINE"); v != "" {
		cfg.ContextEngine = v
	}

	if v := os.Getenv("SYSTEM_PROMPT"); v != "" {
		cfg.SystemPrompt = v
	}

	// Env override for file paths.
	if v := os.Getenv("SKILL_PATHS"); v != "" {
		cfg.SkillPaths = splitEnvList(v)
	}

	return cfg
}

// EnvOverride 将环境变量中的值覆盖到 Config 上。
// 这是 FromEnv + Merge 的组合。
func EnvOverride(cfg *Config) {
	envCfg := FromEnv()
	cfg.Merge(envCfg)
}

// ConfigPathEnv 是配置文件的默认环境变量名。
const ConfigPathEnv = "MADY_CONFIG"

// LoadOrDefault 从 MADY_CONFIG 环境变量指定的路径加载配置，
// 如果未设置或文件不存在则返回空配置（零值）。
func LoadOrDefault() *Config {
	path := os.Getenv(ConfigPathEnv)
	if path == "" {
		return &Config{}
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		return &Config{}
	}

	// Apply environment variable overrides.
	EnvOverride(cfg)

	return cfg
}

// --- helpers ---

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func splitEnvList(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
