package agentcore

import (
	"context"
)

// ---------------------------------------------------------------------------
// ContextLayer — 上下文分层
// ---------------------------------------------------------------------------

// ContextLayer 标识上下文中的一个组装层。
type ContextLayer string

const (
	LayerSystem    ContextLayer = "system"    // 系统提示词（Static 段）
	LayerTools     ContextLayer = "tools"     // 工具定义
	LayerKnowledge ContextLayer = "knowledge" // 知识库上下文
	LayerMemory    ContextLayer = "memory"    // 记忆上下文
	LayerHistory   ContextLayer = "history"   // 对话历史
)

// ValidContextLayers 是所有支持的上下文层。
// 这是一个变量而非函数，允许扩展动态注册新层。
var ValidContextLayers = []ContextLayer{LayerSystem, LayerTools, LayerKnowledge, LayerMemory, LayerHistory}

// ---------------------------------------------------------------------------
// InjectMode — 注入策略
// ---------------------------------------------------------------------------

// InjectMode 控制上下文层的注入时机。
type InjectMode string

const (
	InjectAlways    InjectMode = "always"     // 每轮都注入（当前默认行为）
	InjectPerTurn   InjectMode = "per_turn"   // 每轮重新生成（如工具定义变化）
	InjectOnDemand  InjectMode = "on_demand"  // 仅在模型或用户明确触发时注入
	InjectByTrigger InjectMode = "by_trigger" // 条件判定触发（如复杂度门控）
)

// ---------------------------------------------------------------------------
// LayerConfig — 每层配置
// ---------------------------------------------------------------------------

// LayerConfig 控制单一上下文层的行为。
type LayerConfig struct {
	// Enabled 控制该层是否启用。默认 true。
	Enabled bool `json:"enabled"`

	// InjectMode 控制注入策略。默认 InjectAlways。
	InjectMode InjectMode `json:"inject_mode"`

	// MaxTokens 是该层可消耗的最大 token 数。0 = 使用默认配额。
	MaxTokens int64 `json:"max_tokens"`

	// Priority 是该层的保留优先级。数字越小越优先保留。
	// 当总 token 预算不足时，高 Priority 值（数字大）的层先被截断。
	Priority int `json:"priority"`

	// Position 控制该层内容在最终消息列表中的插入位置。
	// 负数表示从后往前计。默认值按层类型不同。
	Position int `json:"position"`
}

// DefaultLayerConfig 返回指定层的默认配置。
func DefaultLayerConfig(layer ContextLayer) LayerConfig {
	switch layer {
	case LayerSystem:
		return LayerConfig{
			Enabled:    true,
			InjectMode: InjectAlways,
			MaxTokens:  0,
			Priority:   1,
			Position:   0,
		}
	case LayerTools:
		return LayerConfig{
			Enabled:    true,
			InjectMode: InjectPerTurn,
			MaxTokens:  0,
			Priority:   2,
			Position:   1,
		}
	case LayerKnowledge:
		return LayerConfig{
			Enabled:    true,
			InjectMode: InjectByTrigger,
			MaxTokens:  2000,
			Priority:   4,
			Position:   2,
		}
	case LayerMemory:
		return LayerConfig{
			Enabled:    true,
			InjectMode: InjectAlways,
			MaxTokens:  1000,
			Priority:   5,
			Position:   3,
		}
	case LayerHistory:
		return LayerConfig{
			Enabled:    true,
			InjectMode: InjectAlways,
			MaxTokens:  0,
			Priority:   6,
			Position:   -1,
		}
	default:
		return LayerConfig{Enabled: true, InjectMode: InjectAlways, Priority: 10}
	}
}

// ---------------------------------------------------------------------------
// BuildInput / BuildOutput
// ---------------------------------------------------------------------------

// BuildInput 是 ContextBuilder.Build 的输入参数。
type BuildInput struct {
	Messages      []Message                    // 当前 AgentState 中的消息列表
	ToolDefs      []ToolDefinition             // 当前注册的工具定义
	SystemPrompt  string                       // Agent 的系统提示词
	ContextWindow int64                        // 模型上下文窗口（token）
	ReserveTokens int64                        // 为响应预留的 token 数
	LayerConfigs  map[ContextLayer]LayerConfig // 各层配置（nil 时使用默认）
}

// BuildOutput 是 ContextBuilder.Build 的输出。
type BuildOutput struct {
	Messages []Message        // 组装完成的消息列表
	ToolDefs []ToolDefinition // 最终的工具定义列表（可能被过滤）
	Usage    BuildUsage       // 各层 token 消耗
}

// BuildUsage 记录各层实际消耗的 token 数。
type BuildUsage struct {
	ByLayer       map[ContextLayer]int64 `json:"by_layer"`
	TotalTokens   int64                  `json:"total_tokens"`
	ToolDefTokens int64                  `json:"tool_def_tokens"`
}

// ---------------------------------------------------------------------------
// LayerProvider — 各层内容提供者
// ---------------------------------------------------------------------------

// LayerProvider 为指定的上下文层提供内容。
// 每个模块（knowledge、memory 等）可以实现此接口以参与 ContextBuilder 组装。
type LayerProvider interface {
	// Provide 返回该层要注入的消息列表。
	// contextWindow 是总上下文窗口大小，用于 Token 预算计算。
	Provide(ctx context.Context, input BuildInput, layerCfg LayerConfig) ([]Message, error)

	// Layer 返回此 Provider 负责的上下文层。
	Layer() ContextLayer
}

// ---------------------------------------------------------------------------
// ContextBuilder — 统一上下文组装器
// ---------------------------------------------------------------------------

// ContextBuilder 从多个来源组装最终的 LLM 消息列表。
//
// 它取代 TransformContext 成为核心上下文组装点，提供：
//   - 分层 Token 预算管理（借鉴 LlamaIndex chat_history_token_ratio）
//   - Static/Dynamic 边界分离（借鉴 Claude Code prefix caching）
//   - 可配置注入策略（借鉴 CrewAI 的 always/per_turn/on_demand/by_trigger）
//   - 层间优先级（低 Priority 层先被截断）
type ContextBuilder interface {
	// Build 组装最终的 LLM 请求输入。
	Build(ctx context.Context, input BuildInput) BuildOutput
}

// ---------------------------------------------------------------------------
// ContextBuilderConfig — ContextBuilder 全局配置
// ---------------------------------------------------------------------------

// ContextBuilderConfig 控制 ContextBuilder 的全局行为。
type ContextBuilderConfig struct {
	// Enabled 是否启用 ContextBuilder。false 时回退到 TransformContext。
	Enabled bool `json:"enabled"`

	// DefaultLayerConfigs 各层的默认配置。
	DefaultLayerConfigs map[ContextLayer]LayerConfig `json:"default_layer_configs,omitempty"`

	// Providers 负责各层内容提供的 LayerProvider 列表。
	Providers []LayerProvider `json:"-"`
}

// DefaultContextBuilderConfig 返回默认的 ContextBuilder 配置。
func DefaultContextBuilderConfig() ContextBuilderConfig {
	cfgs := make(map[ContextLayer]LayerConfig)
	for _, l := range ValidContextLayers {
		cfgs[l] = DefaultLayerConfig(l)
	}
	return ContextBuilderConfig{
		Enabled:             false,
		DefaultLayerConfigs: cfgs,
	}
}
