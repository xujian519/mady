// Package agentconfig assembles the agent provider/model/thinking configuration
// from environment variables. It centralizes the wiring shared across all
// entry points (cli-chat, acp-server, the unified mady command) so every binary
// honors the same PROVIDER / API_KEY / BASE_URL / THINKING_* conventions.
//
// Supported providers (all via the OpenAI Chat Completions compatible protocol):
//
//	PROVIDER=deepseek → DeepSeek (deepseek-v4-flash / deepseek-v4-pro)
//	PROVIDER=zhipu    → Zhipu GLM (glm-5.2 / glm-5v-turbo 多模态)
//	PROVIDER=kimi     → Kimi Moonshot (kimi-k2.6 多模态 / kimi-k2.7-code)
//	PROVIDER=generic  → any OpenAI-compatible endpoint (set OPENAI_BASE_URL + MODEL)
package agentconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/provider/chatcompat"
)

// BuildProvider reads PROVIDER / API_KEY / BASE_URL from the environment and
// returns a chatcompat provider wired to the correct backend. Provider-specific
// fallback keys (DEEPSEEK_API_KEY, ZHIPU_API_KEY, KIMI_API_KEY) are honored.
// Returns an error when no API key is configured.
func BuildProvider() (agentcore.Provider, error) {
	providerType := util.EnvOrDefault("PROVIDER", "deepseek")

	apiKey := os.Getenv("API_KEY")
	baseURL := os.Getenv("BASE_URL")

	switch providerType {
	case "deepseek":
		if apiKey == "" {
			apiKey = os.Getenv("DEEPSEEK_API_KEY")
		}
		if baseURL == "" {
			baseURL = "https://api.deepseek.com/v1"
		}
	case "zhipu":
		if apiKey == "" {
			apiKey = os.Getenv("ZHIPU_API_KEY")
		}
		if baseURL == "" {
			baseURL = "https://open.bigmodel.cn/api/coding/paas/v4"
		}
	case "kimi":
		// Kimi Code coding 端点在 KIMI_CODE_API_KEY 和 KIMI_CODE_BASE_URL
		// 中配置（coding 专属额度）。未配置时回退到 Moonshot 标准 API。
		codeKey := os.Getenv("KIMI_CODE_API_KEY")
		codeURL := os.Getenv("KIMI_CODE_BASE_URL")
		if codeKey != "" {
			apiKey = codeKey
			if codeURL == "" {
				codeURL = "https://api.kimi.com/coding/v1"
			}
			baseURL = codeURL
		} else {
			if apiKey == "" {
				apiKey = os.Getenv("KIMI_API_KEY")
			}
			if baseURL == "" {
				baseURL = "https://api.moonshot.cn/v1"
			}
		}

	default:
		// Generic OpenAI-compatible provider.
		if baseURL == "" {
			baseURL = os.Getenv("OPENAI_BASE_URL")
		}
	}

	if apiKey == "" {
		return nil, fmt.Errorf("API_KEY (or provider-specific env var) is required")
	}
	return chatcompat.New(chatcompat.Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
	}), nil
}

// DefaultModel returns the conventional model id for the configured provider.
// For the "generic" provider the MODEL env var is used (empty when unset).
func DefaultModel() string {
	switch util.EnvOrDefault("PROVIDER", "deepseek") {
	case "deepseek":
		return "deepseek-v4-flash"
	case "zhipu":
		return "glm-5.2"
	case "kimi":
		return "kimi-k2.6"
	default:
		return os.Getenv("MODEL")
	}
}

// ThinkingFromEnv reads the THINKING_INCLUDE_THOUGHTS / THINKING_DISPLAY /
// THINKING_EFFORT / THINKING_BUDGET variables and returns a ThinkingConfig.
// Returns nil when none are set, leaving thinking behavior at the provider default.
func ThinkingFromEnv() *agentcore.ThinkingConfig {
	includeRaw := strings.TrimSpace(os.Getenv("THINKING_INCLUDE_THOUGHTS"))
	displayRaw := strings.TrimSpace(os.Getenv("THINKING_DISPLAY"))
	effortRaw := strings.TrimSpace(os.Getenv("THINKING_EFFORT"))
	budgetRaw := strings.TrimSpace(os.Getenv("THINKING_BUDGET"))
	if includeRaw == "" && displayRaw == "" && effortRaw == "" && budgetRaw == "" {
		return nil
	}

	cfg := &agentcore.ThinkingConfig{}
	if includeRaw != "" {
		if v, err := strconv.ParseBool(includeRaw); err == nil {
			cfg.IncludeThoughts = v
		}
	}
	if displayRaw != "" {
		cfg.Display = agentcore.ThinkingDisplay(strings.ToLower(displayRaw))
	}
	if effortRaw != "" {
		cfg.Effort = agentcore.ThinkingEffort(strings.ToLower(effortRaw))
	}
	if budgetRaw != "" {
		if v, err := strconv.ParseInt(budgetRaw, 10, 64); err == nil {
			cfg.Budget = v
		}
	}
	return cfg
}
