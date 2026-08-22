package agentconfig

import (
	"os"
	"testing"
)

func TestBuildProvider_MissingAPIKey(t *testing.T) {
	// Save and clear relevant env vars.
	envKeys := []string{
		"API_KEY",
		"DEEPSEEK_API_KEY",
		"ZHIPU_API_KEY",
		"KIMI_API_KEY",
		"KIMI_CODE_API_KEY",
	}
	saved := make(map[string]string, len(envKeys))
	for _, k := range envKeys {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	provider, err := BuildProvider()
	if err == nil {
		t.Fatal("expected error when API key is missing, got nil")
	}
	if provider != nil {
		t.Fatalf("expected nil provider, got %v", provider)
	}
}
