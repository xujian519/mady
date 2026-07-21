package domainconfig

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/xujian519/mady/pkg/agentconfig"
)

const testdataDir = "testdata"

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(testdataDir, name)
}

// TestLoadConfig_YAML 验证 YAML 配置加载。
func TestLoadConfig_YAML(t *testing.T) {
	cfg, err := LoadConfig(testdataPath(t, "patent.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig(patent.yaml) 失败: %v", err)
	}

	if cfg.Name != "patent-agent" {
		t.Errorf("Name = %q, 期望 %q", cfg.Name, "patent-agent")
	}
	if cfg.Domain != "patent" {
		t.Errorf("Domain = %q, 期望 %q", cfg.Domain, "patent")
	}
	if cfg.GuardrailLevel != "strict" {
		t.Errorf("GuardrailLevel = %q, 期望 %q", cfg.GuardrailLevel, "strict")
	}
	if cfg.KnowledgeDomain != "patent" {
		t.Errorf("KnowledgeDomain = %q, 期望 %q", cfg.KnowledgeDomain, "patent")
	}
	if cfg.Config.Model != "gpt-4o" {
		t.Errorf("Config.Model = %q, 期望 %q", cfg.Config.Model, "gpt-4o")
	}
	if cfg.Config.Temperature != 0.1 {
		t.Errorf("Config.Temperature = %v, 期望 %v", cfg.Config.Temperature, 0.1)
	}
	if cfg.Config.MaxTokens != 4096 {
		t.Errorf("Config.MaxTokens = %d, 期望 %d", cfg.Config.MaxTokens, 4096)
	}
	if cfg.Config.MaxTurns != 50 {
		t.Errorf("Config.MaxTurns = %d, 期望 %d", cfg.Config.MaxTurns, 50)
	}
	if len(cfg.HandoffTargets) != 2 {
		t.Errorf("HandoffTargets 长度 = %d, 期望 %d", len(cfg.HandoffTargets), 2)
	}
	if len(cfg.Config.Tools) != 3 {
		t.Errorf("Config.Tools 长度 = %d, 期望 %d", len(cfg.Config.Tools), 3)
	}
	if len(cfg.Config.SkillPaths) != 1 {
		t.Errorf("Config.SkillPaths 长度 = %d, 期望 %d", len(cfg.Config.SkillPaths), 1)
	}
}

// TestLoadConfig_JSON 验证 JSON 配置加载。
func TestLoadConfig_JSON(t *testing.T) {
	cfg, err := LoadConfig(testdataPath(t, "patent.json"))
	if err != nil {
		t.Fatalf("LoadConfig(patent.json) 失败: %v", err)
	}

	if cfg.Name != "patent-agent" {
		t.Errorf("Name = %q, 期望 %q", cfg.Name, "patent-agent")
	}
	if cfg.Domain != "patent" {
		t.Errorf("Domain = %q, 期望 %q", cfg.Domain, "patent")
	}
	if cfg.GuardrailLevel != "strict" {
		t.Errorf("GuardrailLevel = %q, 期望 %q", cfg.GuardrailLevel, "strict")
	}
	if cfg.Config.Model != "gpt-4o" {
		t.Errorf("Config.Model = %q, 期望 %q", cfg.Config.Model, "gpt-4o")
	}
	if cfg.Config.Temperature != 0.1 {
		t.Errorf("Config.Temperature = %v, 期望 %v", cfg.Config.Temperature, 0.1)
	}
	if cfg.Config.MaxTokens != 4096 {
		t.Errorf("Config.MaxTokens = %d, 期望 %d", cfg.Config.MaxTokens, 4096)
	}
	if cfg.Config.MaxTurns != 50 {
		t.Errorf("Config.MaxTurns = %d, 期望 %d", cfg.Config.MaxTurns, 50)
	}
}

// TestLoadConfig_FileNotFound 验证文件不存在时返回错误。
func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig(testdataPath(t, "nonexistent.yaml"))
	if err == nil {
		t.Fatal("期望文件不存在错误，但得到 nil")
	}
}

// TestLoadConfigs 验证多文件批量加载。
func TestLoadConfigs(t *testing.T) {
	configs, err := LoadConfigs(testdataDir)
	if err != nil {
		t.Fatalf("LoadConfigs(testdata) 失败: %v", err)
	}

	if len(configs) == 0 {
		t.Fatal("期望至少加载一个配置，实际为 0")
	}

	found := map[string]bool{"patent-agent": false, "legal-advisor": false, "chat-agent": false, "assistant-agent": false}
	for _, cfg := range configs {
		found[cfg.Name] = true
	}

	for name, ok := range found {
		if !ok {
			t.Errorf("配置 %q 未加载", name)
		}
	}
}

// TestLoadConfigs_DirNotExist 验证目录不存在时返回错误。
func TestLoadConfigs_DirNotExist(t *testing.T) {
	_, err := LoadConfigs(testdataPath(t, "nonexistent_dir"))
	if err == nil {
		t.Fatal("期望目录不存在错误，但得到 nil")
	}
}

// TestDomainConfig_Validate_EmptyName 验证空 name 错误。
func TestDomainConfig_Validate_EmptyName(t *testing.T) {
	cfg := &DomainConfig{Name: "", Domain: "patent"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("期望空 name 错误，但得到 nil")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("错误信息不包含 'name is required': %v", err)
	}
}

// TestDomainConfig_Validate_EmptyDomain 验证空 domain 错误。
func TestDomainConfig_Validate_EmptyDomain(t *testing.T) {
	cfg := &DomainConfig{Name: "test-agent", Domain: ""}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("期望空 domain 错误，但得到 nil")
	}
	if !strings.Contains(err.Error(), "domain is required") {
		t.Errorf("错误信息不包含 'domain is required': %v", err)
	}
}

// TestDomainConfig_YAMLRoundTrip 验证 YAML 序列化/反序列化一致性。
func TestDomainConfig_YAMLRoundTrip(t *testing.T) {
	original := &DomainConfig{
		Name:            "roundtrip-agent",
		Domain:          "test",
		Description:     "往返测试",
		GuardrailLevel:  "standard",
		KnowledgeDomain: "test",
		HandoffTargets:  []string{"other-agent"},
		Config: agentconfig.Config{
			Model:        "gpt-4o-mini",
			Temperature:  0.5,
			MaxTokens:    2048,
			MaxTurns:     15,
			Tools:        []string{"tool_a", "tool_b"},
			SkillPaths:   []string{"skills/test"},
			SystemPrompt: "你是测试助手。",
		},
		Extra: map[string]any{"key": "value", "count": float64(42)},
	}

	yamlData, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("YAML 序列化失败: %v", err)
	}

	var restored DomainConfig
	if err := yaml.Unmarshal(yamlData, &restored); err != nil {
		t.Fatalf("YAML 反序列化失败: %v", err)
	}

	if restored.Name != original.Name {
		t.Errorf("Name = %q, 期望 %q", restored.Name, original.Name)
	}
	if restored.Domain != original.Domain {
		t.Errorf("Domain = %q, 期望 %q", restored.Domain, original.Domain)
	}
	if restored.Description != original.Description {
		t.Errorf("Description = %q, 期望 %q", restored.Description, original.Description)
	}
	if restored.GuardrailLevel != original.GuardrailLevel {
		t.Errorf("GuardrailLevel = %q, 期望 %q", restored.GuardrailLevel, original.GuardrailLevel)
	}
	if restored.Config.Model != original.Config.Model {
		t.Errorf("Config.Model = %q, 期望 %q", restored.Config.Model, original.Config.Model)
	}
	if restored.Config.Temperature != original.Config.Temperature {
		t.Errorf("Config.Temperature = %v, 期望 %v", restored.Config.Temperature, original.Config.Temperature)
	}
	if restored.Config.MaxTokens != original.Config.MaxTokens {
		t.Errorf("Config.MaxTokens = %d, 期望 %d", restored.Config.MaxTokens, original.Config.MaxTokens)
	}
	if restored.Config.MaxTurns != original.Config.MaxTurns {
		t.Errorf("Config.MaxTurns = %d, 期望 %d", restored.Config.MaxTurns, original.Config.MaxTurns)
	}
	if len(restored.HandoffTargets) != len(original.HandoffTargets) {
		t.Errorf("HandoffTargets 长度 = %d, 期望 %d", len(restored.HandoffTargets), len(original.HandoffTargets))
	}
	if len(restored.Config.Tools) != len(original.Config.Tools) {
		t.Errorf("Config.Tools 长度 = %d, 期望 %d", len(restored.Config.Tools), len(original.Config.Tools))
	}
	if restored.Config.SystemPrompt != original.Config.SystemPrompt {
		t.Errorf("Config.SystemPrompt = %q, 期望 %q", restored.Config.SystemPrompt, original.Config.SystemPrompt)
	}
}

// TestDefaultConfigDir 验证默认目录路径不 panic。
func TestDefaultConfigDir(t *testing.T) {
	dir := DefaultConfigDir()
	if dir == "" {
		t.Fatal("DefaultConfigDir() 返回空字符串")
	}
	t.Logf("默认配置目录: %s", dir)
}

func TestLoadConfig_InvalidEmptyName(t *testing.T) {
	_, err := LoadConfig(filepath.Join(testdataDir, "invalid", "empty_name.yaml"))
	if err == nil {
		t.Fatal("期望校验错误，但得到 nil")
	}
}

func TestLoadConfig_InvalidEmptyDomain(t *testing.T) {
	_, err := LoadConfig(filepath.Join(testdataDir, "invalid", "empty_domain.yaml"))
	if err == nil {
		t.Fatal("期望校验错误，但得到 nil")
	}
}
