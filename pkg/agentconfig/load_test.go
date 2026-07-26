package agentconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/pkg/agentconfig"
)

func TestLoadConfig_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{
		"model": "deepseek-v4-pro",
		"temperature": 0.3,
		"max_tokens": 8192,
		"max_turns": 50,
		"context_window": 1000000
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := agentconfig.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Model != "deepseek-v4-pro" {
		t.Errorf("model: got %q", cfg.Model)
	}
	if cfg.Temperature != 0.3 {
		t.Errorf("temperature: got %f", cfg.Temperature)
	}
	if cfg.MaxTurns != 50 {
		t.Errorf("max_turns: got %d", cfg.MaxTurns)
	}
}

func TestLoadConfig_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
model: kimi-k2.6
temperature: 0.8
max_tokens: 2048
max_turns: 10
context_window: 1000000
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := agentconfig.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Model != "kimi-k2.6" {
		t.Errorf("model: got %q", cfg.Model)
	}
	if cfg.MaxTurns != 10 {
		t.Errorf("max_turns: got %d", cfg.MaxTurns)
	}
}

func TestLoadConfig_InvalidExtension(t *testing.T) {
	_, err := agentconfig.LoadConfig("/tmp/config.toml")
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}

func TestLoadConfig_NonexistentFile(t *testing.T) {
	_, err := agentconfig.LoadConfig("/nonexistent/config.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestFromEnv(t *testing.T) {
	// Set env vars.
	t.Setenv("MODEL", "test-model")
	t.Setenv("TEMPERATURE", "0.5")
	t.Setenv("MAX_TOKENS", "2048")
	t.Setenv("MAX_TURNS", "10")
	t.Setenv("STREAMING", "true")
	t.Setenv("CONTEXT_WINDOW", "1000000")

	cfg := agentconfig.FromEnv()
	if cfg.Model != "test-model" {
		t.Errorf("model: got %q", cfg.Model)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("temperature: got %f", cfg.Temperature)
	}
	if cfg.MaxTokens != 2048 {
		t.Errorf("max_tokens: got %d", cfg.MaxTokens)
	}
	if cfg.MaxTurns != 10 {
		t.Errorf("max_turns: got %d", cfg.MaxTurns)
	}
	if !cfg.Streaming {
		t.Error("streaming should be true")
	}
	if cfg.ContextWindow != 1000000 {
		t.Errorf("context_window: got %d", cfg.ContextWindow)
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("MAX_TURNS", "100")
	t.Setenv("TEMPERATURE", "0.1")

	cfg := &agentconfig.Config{
		Model:       "base-model",
		Temperature: 0.7,
		MaxTurns:    20,
	}

	agentconfig.EnvOverride(cfg)

	if cfg.Model != "base-model" {
		t.Errorf("model should be preserved: got %q", cfg.Model)
	}
	if cfg.Temperature != 0.1 {
		t.Errorf("temperature should be overridden: got %f", cfg.Temperature)
	}
	if cfg.MaxTurns != 100 {
		t.Errorf("max_turns should be overridden: got %d", cfg.MaxTurns)
	}
}

func TestLoadOrDefault_NoEnv(t *testing.T) {
	cfg := agentconfig.LoadOrDefault()
	// Should return empty config without error.
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

// --- Fallback candidate chain config (Phase 6) ---

func TestFromEnv_FallbackCandidates(t *testing.T) {
	t.Setenv("FALLBACK_CANDIDATES_LOW", "gpt-4o-mini, deepseek-v4-flash")
	t.Setenv("FALLBACK_CANDIDATES_HIGH", "claude-3.5-sonnet")
	t.Setenv("FALLBACK_STICKY_SESSION", "true")

	cfg := agentconfig.FromEnv()
	if cfg.Fallback == nil {
		t.Fatal("expected Fallback config from env")
	}
	if !cfg.Fallback.StickySession {
		t.Error("StickySession should be true")
	}
	if got := cfg.Fallback.Candidates["low"]; len(got) != 2 || got[0] != "gpt-4o-mini" || got[1] != "deepseek-v4-flash" {
		t.Errorf("low candidates mismatch: %v", got)
	}
	if got := cfg.Fallback.Candidates["high"]; len(got) != 1 || got[0] != "claude-3.5-sonnet" {
		t.Errorf("high candidates mismatch: %v", got)
	}
	if _, ok := cfg.Fallback.Candidates["medium"]; ok {
		t.Error("medium should not be set")
	}
}

func TestFromEnv_NoFallbackEnv_ReturnsNil(t *testing.T) {
	// 确保没有 fallback 环境变量时 Fallback 为 nil（安全 no-op）。
	t.Setenv("FALLBACK_CANDIDATES_LOW", "")
	t.Setenv("FALLBACK_CANDIDATES_HIGH", "")
	t.Setenv("FALLBACK_STICKY_SESSION", "")
	cfg := agentconfig.FromEnv()
	if cfg.Fallback != nil {
		t.Errorf("expected nil Fallback when no env set, got %+v", cfg.Fallback)
	}
}

func TestLoadConfig_FallbackYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
fallback:
  sticky_session: true
  candidates:
    low: ["gpt-4o-mini", "deepseek-v4-flash"]
    medium: ["deepseek-v4-pro"]
    high: ["claude-3.5-sonnet", "deepseek-v4-pro", "gpt-4o"]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := agentconfig.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Fallback == nil {
		t.Fatal("expected Fallback from YAML")
	}
	if !cfg.Fallback.StickySession {
		t.Error("StickySession should be true")
	}
	if len(cfg.Fallback.Candidates["high"]) != 3 {
		t.Errorf("high candidates len: got %d, want 3", len(cfg.Fallback.Candidates["high"]))
	}
}

func TestConfig_Merge_Fallback(t *testing.T) {
	base := &agentconfig.Config{}
	other := &agentconfig.Config{
		Fallback: &agentconfig.Fallback{
			StickySession: true,
			Candidates:    map[string][]string{"low": {"m1"}},
		},
	}
	base.Merge(other)
	if base.Fallback == nil {
		t.Fatal("Merge should copy Fallback")
	}
	if !base.Fallback.StickySession {
		t.Error("StickySession not merged")
	}
}
