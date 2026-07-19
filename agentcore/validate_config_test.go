package agentcore

import (
	"testing"
)

func TestConfigValidate_EmptyModel(t *testing.T) {
	cfg := Config{
		ModelConfig: ModelConfig{
			Model:    "",
			Provider: &stubProvider{},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty Model")
	}
}

func TestConfigValidate_NilProvider(t *testing.T) {
	cfg := Config{
		ModelConfig: ModelConfig{
			Model:    "gpt-4",
			Provider: nil,
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for nil Provider")
	}
}

func TestConfigValidate_Valid(t *testing.T) {
	cfg := Config{
		ModelConfig: ModelConfig{
			Model:    "gpt-4",
			Provider: &stubProvider{},
		},
		ExecutionConfig: ExecutionConfig{
			ExecutionMode: ModeSerial,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestConfigValidate_ExecutionMode(t *testing.T) {
	valid := []ExecutionMode{"", ModeSerial, ModeParallel}
	for _, mode := range valid {
		cfg := Config{
			ModelConfig: ModelConfig{Model: "gpt-4", Provider: &stubProvider{}},
			ExecutionConfig: ExecutionConfig{
				ExecutionMode: mode,
			},
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("mode %q should be valid: %v", mode, err)
		}
	}

	cfg := Config{
		ModelConfig: ModelConfig{Model: "gpt-4", Provider: &stubProvider{}},
		ExecutionConfig: ExecutionConfig{
			ExecutionMode: "concurrent", // invalid
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unknown execution mode")
	}
}

func TestConfigValidate_DuplicateHandoffNames(t *testing.T) {
	cfg := Config{
		ModelConfig: ModelConfig{Model: "gpt-4", Provider: &stubProvider{}},
		Handoffs: []HandoffConfig{
			{Name: "patent"},
			{Name: "patent"}, // duplicate
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate handoff names")
	}
}

func TestConfigValidate_EmptyHandoffName(t *testing.T) {
	cfg := Config{
		ModelConfig: ModelConfig{Model: "gpt-4", Provider: &stubProvider{}},
		Handoffs: []HandoffConfig{
			{Name: ""},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty handoff name")
	}
}

func TestConfigValidate_CompactionThreshold(t *testing.T) {
	// Valid: threshold in [0, 1].
	cfg := Config{
		ModelConfig: ModelConfig{Model: "gpt-4", Provider: &stubProvider{}},
		CompactionConfig: CompactionConfig{
			ContextWindow:        128000,
			CompressionThreshold: 0.75,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid: %v", err)
	}

	// Invalid: threshold > 1.
	cfg.CompressionThreshold = 1.5
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for threshold > 1")
	}

	// Invalid: threshold < 0.
	cfg.CompressionThreshold = -0.1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for threshold < 0")
	}
}

func TestConfigValidate_CompactionDisabled(t *testing.T) {
	// ContextWindow=0 means compaction disabled — should be valid.
	cfg := Config{
		ModelConfig: ModelConfig{Model: "gpt-4", Provider: &stubProvider{}},
		CompactionConfig: CompactionConfig{
			ContextWindow: 0, // disabled
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("compaction disabled should be valid: %v", err)
	}
}

func TestConfigValidate_StubConfig(t *testing.T) {
	// StubConfig should always pass validation.
	cfg := StubConfig(&stubProvider{})
	if err := cfg.Validate(); err != nil {
		t.Fatalf("StubConfig should validate: %v", err)
	}
}

func TestConfigValidate_AllowedSourcesEmpty(t *testing.T) {
	cfg := Config{
		ModelConfig: ModelConfig{Model: "gpt-4", Provider: &stubProvider{}},
		Handoffs: []HandoffConfig{
			{Name: "patent", AllowedSources: []string{"chat", ""}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty AllowedSources entry")
	}
}

func TestConfigValidate_AllowedSourcesSelfReference(t *testing.T) {
	cfg := Config{
		ModelConfig: ModelConfig{Model: "gpt-4", Provider: &stubProvider{}},
		Handoffs: []HandoffConfig{
			{Name: "patent", AllowedSources: []string{"patent"}}, // self
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for self-referential AllowedSources")
	}
}

func TestConfigValidate_AllowedSourcesValid(t *testing.T) {
	cfg := Config{
		ModelConfig: ModelConfig{Model: "gpt-4", Provider: &stubProvider{}},
		Handoffs: []HandoffConfig{
			{Name: "patent", AllowedSources: []string{"chat", "*"}},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid AllowedSources should pass: %v", err)
	}
}
