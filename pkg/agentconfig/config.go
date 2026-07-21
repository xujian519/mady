package agentconfig

import (
	"fmt"
)

// =============================================================================
// UnstructuredConfig 是以 YAML/JSON 格式表示的 Agent 配置。
// 这个结构体可以被序列化为 YAML 或 JSON 文件，通过环境变量覆盖字段，
// 从而实现声明式的配置管理。
//
// 使用示例（YAML）：
//
//	model: deepseek-v4-flash
//	temperature: 0.7
//	max_tokens: 4096
//	max_turns: 20
//	context_window: 1048576
//	streaming: true
//
// 所有字段都有清晰的零值语义：零值表示"使用默认值"，只有非零字段会生效。
// =============================================================================

// Config 是统一的 Agent 配置，支持从 JSON/YAML 反序列化。
type Config struct {
	// Model 是 LLM 模型标识（如 "deepseek-v4-flash", "gpt-4o-mini"）。
	// 对应 agentcore.ModelConfig.Model。
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// Temperature 是采样温度（0.0-2.0）。0=确定性。
	// 对应 agentcore.ModelConfig.Temperature。
	Temperature float64 `json:"temperature,omitempty" yaml:"temperature,omitempty"`

	// MaxTokens 是 LLM 响应的最大 token 数。
	// 对应 agentcore.ModelConfig.MaxTokens。
	MaxTokens int64 `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`

	// Streaming 启用流式响应。
	// 对应 agentcore.ModelConfig.Streaming。
	Streaming bool `json:"streaming,omitempty" yaml:"streaming,omitempty"`

	// Thinking 是扩展思考/推理配置。
	// 对应 agentcore.ModelConfig.Thinking。
	Thinking *Thinking `json:"thinking,omitempty" yaml:"thinking,omitempty"`

	// SystemPrompt 是 Agent 的系统提示词。
	SystemPrompt string `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`

	// MaxTurns 是 Agent 循环的最大轮数。
	// 对应 agentcore.ExecutionConfig.MaxTurns。
	MaxTurns int64 `json:"max_turns,omitempty" yaml:"max_turns,omitempty"`

	// Concurrency 是最大并发工具调用数。
	// 对应 agentcore.ExecutionConfig.Concurrency。
	Concurrency int64 `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`

	// ContextWindow 是模型上下文窗口大小（token 数）。
	// 对应 agentcore.CompactionConfig.ContextWindow。
	ContextWindow int64 `json:"context_window,omitempty" yaml:"context_window,omitempty"`

	// ReserveTokens 是为响应生成保留的 token 数。
	// 对应 agentcore.CompactionConfig.ReserveTokens。
	ReserveTokens int64 `json:"reserve_tokens,omitempty" yaml:"reserve_tokens,omitempty"`

	// KeepRecentTokens 是压缩时保留的最近 token 数。
	// 对应 agentcore.CompactionConfig.KeepRecentTokens。
	KeepRecentTokens int64 `json:"keep_recent_tokens,omitempty" yaml:"keep_recent_tokens,omitempty"`

	// StructuredCompaction 启用结构化 JSON 压缩摘要。
	// 对应 agentcore.CompactionConfig.StructuredCompaction。
	StructuredCompaction bool `json:"structured_compaction,omitempty" yaml:"structured_compaction,omitempty"`

	// ContextEngine 是上下文引擎名称（compressor/chunked/tiered/truncate）。
	// 对应 agentcore.CompactionConfig.Engine。
	ContextEngine string `json:"context_engine,omitempty" yaml:"context_engine,omitempty"`

	// ValidateArguments 启用工具参数的 JSON Schema 校验。
	// 对应 agentcore.ExecutionConfig.ValidateArguments。
	ValidateArguments bool `json:"validate_arguments,omitempty" yaml:"validate_arguments,omitempty"`

	// DisableSkillRegistryAPI 禁用技能注册 HTTP API。
	// 对应 agentcore.SkillConfig.DisableSkillRegistryAPI。
	DisableSkillRegistryAPI bool `json:"disable_skill_registry_api,omitempty" yaml:"disable_skill_registry_api,omitempty"`

	// DisableSkillReloadAPI 禁用技能重载 HTTP API。
	// 对应 agentcore.SkillConfig.DisableSkillReloadAPI。
	DisableSkillReloadAPI bool `json:"disable_skill_reload_api,omitempty" yaml:"disable_skill_reload_api,omitempty"`

	// Tools 是启用的内置工具名列表。空列表 = 全部启用。
	Tools []string `json:"tools,omitempty" yaml:"tools,omitempty"`

	// Extensions 是启用的扩展名列表。空列表 = 全部启用。
	Extensions []string `json:"extensions,omitempty" yaml:"extensions,omitempty"`

	// SkillPaths 是技能目录路径列表，支持热重载。
	SkillPaths []string `json:"skill_paths,omitempty" yaml:"skill_paths,omitempty"`
}

// Thinking 是扩展思考/推理的配置。
type Thinking struct {
	IncludeThoughts bool   `json:"include_thoughts,omitempty" yaml:"include_thoughts,omitempty"`
	Display         string `json:"display,omitempty" yaml:"display,omitempty"` // "summarized" / "hidden"
	Effort          string `json:"effort,omitempty" yaml:"effort,omitempty"`   // "low" / "medium" / "high"
	Budget          int64  `json:"budget,omitempty" yaml:"budget,omitempty"`   // thinking budget in tokens
}

// Validate 检查配置字段的合法性。
// 返回 nil 表示校验通过。
func (c *Config) Validate() error {
	if c.Temperature < 0 || c.Temperature > 2 {
		return fmt.Errorf("temperature must be in [0, 2], got %f", c.Temperature)
	}
	if c.MaxTokens < 0 {
		return fmt.Errorf("max_tokens must be >= 0, got %d", c.MaxTokens)
	}
	if c.MaxTurns < 0 {
		return fmt.Errorf("max_turns must be >= 0, got %d", c.MaxTurns)
	}
	if c.Concurrency < 0 {
		return fmt.Errorf("concurrency must be >= 0, got %d", c.Concurrency)
	}
	if c.ContextWindow < 0 {
		return fmt.Errorf("context_window must be >= 0, got %d", c.ContextWindow)
	}
	if c.ReserveTokens < 0 {
		return fmt.Errorf("reserve_tokens must be >= 0, got %d", c.ReserveTokens)
	}
	if c.KeepRecentTokens < 0 {
		return fmt.Errorf("keep_recent_tokens must be >= 0, got %d", c.KeepRecentTokens)
	}
	if c.Thinking != nil {
		if c.Thinking.Budget < 0 {
			return fmt.Errorf("thinking.budget must be >= 0, got %d", c.Thinking.Budget)
		}
	}
	return nil
}

// Merge 将非零字段从 other 合并到 c 中。
// other 的字段优先（覆盖 c 中的对应字段）。
func (c *Config) Merge(other *Config) {
	if other == nil {
		return
	}
	if other.Model != "" {
		c.Model = other.Model
	}
	if other.Temperature != 0 {
		c.Temperature = other.Temperature
	}
	if other.MaxTokens != 0 {
		c.MaxTokens = other.MaxTokens
	}
	if other.Streaming {
		c.Streaming = other.Streaming
	}
	if other.Thinking != nil {
		c.Thinking = other.Thinking
	}
	if other.SystemPrompt != "" {
		c.SystemPrompt = other.SystemPrompt
	}
	if other.MaxTurns != 0 {
		c.MaxTurns = other.MaxTurns
	}
	if other.Concurrency != 0 {
		c.Concurrency = other.Concurrency
	}
	if other.ContextWindow != 0 {
		c.ContextWindow = other.ContextWindow
	}
	if other.ReserveTokens != 0 {
		c.ReserveTokens = other.ReserveTokens
	}
	if other.KeepRecentTokens != 0 {
		c.KeepRecentTokens = other.KeepRecentTokens
	}
	if other.StructuredCompaction {
		c.StructuredCompaction = other.StructuredCompaction
	}
	if other.ContextEngine != "" {
		c.ContextEngine = other.ContextEngine
	}
	if other.ValidateArguments {
		c.ValidateArguments = other.ValidateArguments
	}
	if other.DisableSkillRegistryAPI {
		c.DisableSkillRegistryAPI = other.DisableSkillRegistryAPI
	}
	if other.DisableSkillReloadAPI {
		c.DisableSkillReloadAPI = other.DisableSkillReloadAPI
	}
	if other.Tools != nil {
		c.Tools = other.Tools
	}
	if other.Extensions != nil {
		c.Extensions = other.Extensions
	}
	if other.SkillPaths != nil {
		c.SkillPaths = other.SkillPaths
	}
}
