package agentcore

import (
	"fmt"
	"strings"
)

// Validate checks the Config for obvious misconfiguration and returns
// the first error found. It is called by New() to fail fast rather than
// deferring errors to runtime.
//
// Validation rules:
//   - Model must not be empty
//   - Provider must not be nil
//   - Handoff targets must reference unique names
//   - CompactionConfig must be internally consistent when enabled
//   - ExecutionMode must be "serial" or "parallel" (or empty, defaulting to serial)
func (cfg *Config) Validate() error {
	// Model is required for the agent to make LLM calls.
	if strings.TrimSpace(cfg.Model) == "" {
		return fmt.Errorf("agentcore: Config.Model is required (set the model identifier, e.g. \"gpt-4o-mini\")")
	}

	// Provider is required — it's how the agent reaches the LLM.
	if cfg.Provider == nil {
		return fmt.Errorf("agentcore: Config.Provider is required (provide an LLM provider implementation)")
	}

	// Handoff target names must be unique and non-empty.
	if err := validateHandoffs(cfg.Handoffs); err != nil {
		return err
	}

	// Compaction config must be internally consistent.
	if err := validateCompaction(&cfg.CompactionConfig); err != nil {
		return err
	}

	// Execution mode must be a known value.
	switch cfg.ExecutionMode {
	case "", ModeSerial, ModeParallel:
		// valid
	default:
		return fmt.Errorf("agentcore: unknown ExecutionMode %q (expected \"serial\" or \"parallel\")", cfg.ExecutionMode)
	}

	return nil
}

func validateHandoffs(handoffs []HandoffConfig) error {
	seen := make(map[string]bool, len(handoffs))
	for i, h := range handoffs {
		if strings.TrimSpace(h.Name) == "" {
			return fmt.Errorf("agentcore: Handoffs[%d].Name is required", i)
		}
		if seen[h.Name] {
			return fmt.Errorf("agentcore: duplicate handoff target %q (Handoffs[%d])", h.Name, i)
		}
		seen[h.Name] = true
	}
	return nil
}

func validateCompaction(cfg *CompactionConfig) error {
	if cfg == nil || cfg.ContextWindow <= 0 {
		return nil // compaction disabled
	}

	if cfg.ReserveTokens < 0 {
		return fmt.Errorf("agentcore: CompactionConfig.ReserveTokens must be >= 0, got %d", cfg.ReserveTokens)
	}

	if cfg.KeepRecentTokens < 0 {
		return fmt.Errorf("agentcore: CompactionConfig.KeepRecentTokens must be >= 0, got %d", cfg.KeepRecentTokens)
	}

	if cfg.CompressionThreshold < 0 || cfg.CompressionThreshold > 1 {
		return fmt.Errorf("agentcore: CompactionConfig.CompressionThreshold must be in [0, 1], got %f", cfg.CompressionThreshold)
	}

	if cfg.AutoCompactTokenLimit < 0 {
		return fmt.Errorf("agentcore: CompactionConfig.AutoCompactTokenLimit must be >= 0, got %d", cfg.AutoCompactTokenLimit)
	}

	if cfg.ProtectFirstN < 0 {
		return fmt.Errorf("agentcore: CompactionConfig.ProtectFirstN must be >= 0, got %d", cfg.ProtectFirstN)
	}

	return nil
}
