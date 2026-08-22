package bootstrap

import (
	"log/slog"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/bootstrap/agentconfig"
)

func ExtSlice(ext agentcore.Extension) []agentcore.Extension {
	if ext == nil {
		return nil
	}
	return []agentcore.Extension{ext}
}

// AgentThinking 将 agentconfig.ThinkingConfig 转换为 agentcore.ThinkingConfig。
func AgentThinking(cfg *agentconfig.ThinkingConfig) *agentcore.ThinkingConfig {
	if cfg == nil {
		return nil
	}
	return &agentcore.ThinkingConfig{
		IncludeThoughts: cfg.IncludeThoughts,
		Display:         agentcore.ThinkingDisplay(cfg.Display),
		Effort:          agentcore.ThinkingEffort(cfg.Effort),
		Budget:          cfg.Budget,
	}
}

// DefaultMaxTokens 是 LLM 响应的默认最大 token 数。
//
// 取 8192 的原因：当前支持的主流模型（DeepSeek-V4 / Kimi-K2 / GLM-5）均为
// 1M 上下文、数百 K 最大输出，8192 足以覆盖普通调研、分析、表格输出，
// 同时避免 provider 默认限制导致长内容在表格/代码块中间被截断。
// 用户可通过 MADY_CONFIG 中的 max_tokens 或 MAX_TOKENS 环境变量覆盖。
const DefaultMaxTokens = 8192

// DefaultFrequencyPenalty 是抑制模型退化重复（"整句 ×N" 循环）的默认频率惩罚。
//
// 取值范围 OpenAI 兼容：0..2，通常 0.1~0.3 即可明显改善重复输出而几乎不影响质量。
// 取 0.2 作为统一入口默认，覆盖所有未显式配置 frequency_penalty 的运行
// （包括未设置 MADY_CONFIG 的配置无关启动），因为重复退化正是在这类默认运行下
// 被观测到的。显式 -1 可关闭（由 agentconfig 校验与 NewBaseConfig 透传保证）。
const DefaultFrequencyPenalty = 0.2

// ResolveMaxTokens 决定最终使用的 LLM 最大输出 token 数。
//
// 优先级：
//  1. 用户显式配置（MADY_CONFIG 中的 max_tokens 或 MAX_TOKENS 环境变量）。
//  2. 未配置时回退到 DefaultMaxTokens。
func ResolveMaxTokens(userCfg *agentconfig.Config) int64 {
	if userCfg != nil && userCfg.MaxTokens > 0 {
		return userCfg.MaxTokens
	}
	return DefaultMaxTokens
}

// ResolveFrequencyPenalty 决定最终使用的频率惩罚。
//
// 优先级：
//  1. 用户显式配置（MADY_CONFIG 的 frequency_penalty 或 FREQUENCY_PENALTY 环境变量）。
//     其中 -1 是"显式关闭"哨兵：非 0，会被透传，但 chatcompat 仅当 >0 才发送，
//     因此 -1 实际等价于不发送。
//  2. 未配置时回退到 DefaultFrequencyPenalty（0.2），以缓解模型退化重复。
func ResolveFrequencyPenalty(userCfg *agentconfig.Config) float64 {
	if userCfg != nil && userCfg.FrequencyPenalty != 0 {
		return userCfg.FrequencyPenalty
	}
	return DefaultFrequencyPenalty
}

// ResolveRepetitionPenalty 决定最终使用的重复惩罚（本地/vLLM 类端点如 oMLX）。
//
// 与 FrequencyPenalty 不同，RepetitionPenalty 为 opt-in：OpenAI 官方端点可能
// 不支持该字段，因此 0 = 不发送（沿用 ProviderRequest → chatcompat 的 >0 判定）。
// 仅当用户显式配置（repetition_penalty / REPETITION_PENALTY）时才透传，不做默认值。
func ResolveRepetitionPenalty(userCfg *agentconfig.Config) float64 {
	if userCfg != nil && userCfg.RepetitionPenalty != 0 {
		return userCfg.RepetitionPenalty
	}
	return 0
}

// NewBaseConfig 构造所有入口（tui/serve/acp/desktop）共享的基础 Agent 配置。
// 它统一处理 max_tokens 默认值、用户配置覆盖和 fallback 候选链，避免各入口
// 重复维护一份 BaseConfig 构造逻辑。
func NewBaseConfig(model string, provider agentcore.Provider, userCfg *agentconfig.Config) agentcore.Config {
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:              "mady-router",
			Model:             model,
			Provider:          provider,
			Streaming:         true,
			MaxTokens:         ResolveMaxTokens(userCfg),
			FrequencyPenalty:  ResolveFrequencyPenalty(userCfg),
			RepetitionPenalty: ResolveRepetitionPenalty(userCfg),
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          25,
			ExecutionMode:     agentcore.ModeSerial,
			ValidateArguments: true,
		},
		CompactionConfig: agentcore.CompactionConfig{
			ContextWindow:    agentconfig.ResolveContextWindow(model),
			ReserveTokens:    32000,
			KeepRecentTokens: 4000,
		},
		RetryConfig: &agentcore.RetryConfig{
			MaxRetries:  3,
			BaseDelayMs: 1000,
			MaxDelayMs:  15000,
		},
	}
	if fbCfg := LoadFallbackConfig(userCfg); fbCfg != nil {
		cfg.FallbackConfig = fbCfg
	}
	return cfg
}

// LoadFallbackConfig 从 agentconfig 读取模型级联回退候选链。
func LoadFallbackConfig(ac *agentconfig.Config) *agentcore.FallbackConfig {
	if ac == nil || ac.Fallback == nil || len(ac.Fallback.Candidates) == 0 {
		return nil
	}
	candidates := make(map[agentcore.Complexity][]string, len(ac.Fallback.Candidates))
	for level, models := range ac.Fallback.Candidates {
		var c agentcore.Complexity
		switch strings.ToLower(level) {
		case "low":
			c = agentcore.ComplexityLow
		case "medium":
			c = agentcore.ComplexityMedium
		case "high":
			c = agentcore.ComplexityHigh
		default:
			slog.Debug("framework: ignoring unknown fallback complexity level", "level", level)
			continue
		}
		candidates[c] = models
	}
	if len(candidates) == 0 {
		return nil
	}
	return &agentcore.FallbackConfig{
		Candidates:    candidates,
		StickySession: ac.Fallback.StickySession,
	}
}

// BuildCitationSource 从 wiki 拆分法条文件构建 S2 知识源索引，与 S1
