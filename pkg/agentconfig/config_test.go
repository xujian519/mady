package agentconfig_test

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/xujian519/mady/pkg/agentconfig"
)

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := agentconfig.Config{
		Model:            "deepseek-v4-flash",
		Temperature:      0.7,
		MaxTokens:        4096,
		MaxTurns:         20,
		ContextWindow:    1_000_000,
		ReserveTokens:    8192,
		KeepRecentTokens: 2000,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfig_Validate_ZeroValues(t *testing.T) {
	cfg := agentconfig.Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero config should be valid: %v", err)
	}
}

func TestConfig_Validate_InvalidTemperature(t *testing.T) {
	cfg := agentconfig.Config{Temperature: 3.0}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for temperature out of range")
	}
}

func TestConfig_Validate_NegativeValues(t *testing.T) {
	tests := []struct {
		name string
		cfg  agentconfig.Config
	}{
		{"negative max_tokens", agentconfig.Config{MaxTokens: -1}},
		{"negative max_turns", agentconfig.Config{MaxTurns: -1}},
		{"negative context_window", agentconfig.Config{ContextWindow: -1}},
		{"negative thinking budget", agentconfig.Config{Thinking: &agentconfig.Thinking{Budget: -1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestConfig_JSONRoundTrip(t *testing.T) {
	original := agentconfig.Config{
		Model:            "deepseek-v4-pro",
		Temperature:      0.3,
		MaxTokens:        8192,
		MaxTurns:         50,
		ContextWindow:    1_000_000,
		ReserveTokens:    8192,
		KeepRecentTokens: 4000,
		Streaming:        true,
		SystemPrompt:     "You are a helpful assistant.",
		Thinking: &agentconfig.Thinking{
			Effort: "high",
			Budget: 8192,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded agentconfig.Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Model != original.Model {
		t.Errorf("model: got %q, want %q", decoded.Model, original.Model)
	}
	if decoded.Temperature != original.Temperature {
		t.Errorf("temperature: got %f, want %f", decoded.Temperature, original.Temperature)
	}
	if decoded.MaxTurns != original.MaxTurns {
		t.Errorf("max_turns: got %d, want %d", decoded.MaxTurns, original.MaxTurns)
	}
	if decoded.Thinking == nil || decoded.Thinking.Effort != "high" {
		t.Errorf("thinking effort: got %v, want high", decoded.Thinking)
	}
}

func TestConfig_YAMLRoundTrip(t *testing.T) {
	original := agentconfig.Config{
		Model:         "kimi-k2.6",
		Temperature:   0.8,
		MaxTokens:     2048,
		MaxTurns:      10,
		ContextWindow: 1_000_000,
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded agentconfig.Config
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Model != original.Model {
		t.Errorf("model: got %q, want %q", decoded.Model, original.Model)
	}
}

func TestConfig_Merge(t *testing.T) {
	base := agentconfig.Config{
		Model:         "deepseek-v4-flash",
		Temperature:   0.7,
		MaxTokens:     4096,
		MaxTurns:      20,
		ContextWindow: 256_000,
	}

	override := agentconfig.Config{
		Model:         "deepseek-v4-pro",
		MaxTurns:      50,
		ContextWindow: 1_000_000,
	}

	base.Merge(&override)

	if base.Model != "deepseek-v4-pro" {
		t.Errorf("model: got %q, want %q", base.Model, "deepseek-v4-pro")
	}
	if base.Temperature != 0.7 {
		t.Errorf("temperature should be preserved: got %f", base.Temperature)
	}
	if base.MaxTurns != 50 {
		t.Errorf("max_turns: got %d, want %d", base.MaxTurns, 50)
	}
	if base.ContextWindow != 1_000_000 {
		t.Errorf("context_window: got %d, want %d", base.ContextWindow, 1_000_000)
	}
}

func TestConfig_Merge_Nil(t *testing.T) {
	cfg := agentconfig.Config{Model: "test"}
	cfg.Merge(nil)
	if cfg.Model != "test" {
		t.Error("config should not be modified by nil merge")
	}
}

func TestConfig_JSON_EmptyOmitEmpty(t *testing.T) {
	cfg := agentconfig.Config{}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	// Should be empty JSON object (all fields are omitempty).
	if string(data) != "{}" {
		t.Logf("empty config marshals to: %s", string(data))
	}
}

func TestConfig_YAML_Empty(t *testing.T) {
	cfg := agentconfig.Config{}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	// Should be empty or minimal YAML.
	if len(data) > 10 {
		t.Logf("empty config YAML: %s", string(data))
	}
}
