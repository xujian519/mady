package cache

import "time"

// Strategy 按 Provider 类型区分缓存策略。
type Strategy string

const (
	StrategyAnthropicPrefix Strategy = "anthropic-prefix" // Claude 前缀缓存
	StrategyOpenAIPrompt    Strategy = "openai-prompt"    // GPT 提示缓存
	StrategyGeneric         Strategy = "generic"          // 通用无缓存
)

// Policy 定义一条缓存策略。
type Policy struct {
	// Strategy 是按 Provider 的缓存策略类型。
	Strategy Strategy `json:"strategy"`
	// TTL 是缓存条目的有效期（0 = 不会过期）。
	TTL time.Duration `json:"ttl"`
	// Priority 是缓存条目的优先级（0-10，10 最高）。
	Priority int `json:"priority"`
	// InvalidationHooks 是失效触发条件列表。
	InvalidationHooks []string `json:"invalidation_hooks,omitempty"`
}

// DefaultPolicy 返回适合当前 Provider 的默认策略。
func DefaultPolicy(strategy Strategy) Policy {
	switch strategy {
	case StrategyAnthropicPrefix:
		return Policy{
			Strategy:          strategy,
			TTL:               5 * time.Minute,
			Priority:          8,
			InvalidationHooks: []string{"compress", "system_prompt_change"},
		}
	case StrategyOpenAIPrompt:
		return Policy{
			Strategy:          strategy,
			TTL:               10 * time.Minute,
			Priority:          7,
			InvalidationHooks: []string{"compress"},
		}
	default:
		return Policy{
			Strategy: strategy,
			TTL:      0,
			Priority: 0,
		}
	}
}
