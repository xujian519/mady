package disclosure

import (
	"testing"
)

func TestBuildExtractionConfig_DisablesSchemaForDefaultDeepSeek(t *testing.T) {
	t.Setenv("PROVIDER", "")

	cfg := buildExtractionConfig(nil, extractProblems)
	if cfg.ResponseFormat != nil {
		t.Fatalf("ResponseFormat = %#v, want nil for default deepseek env", cfg.ResponseFormat)
	}
}
