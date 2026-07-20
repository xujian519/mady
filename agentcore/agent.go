package agentcore

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
)

const defaultMaxTurns = 20

// ToolCallOverride controls how a loop-level BeforeToolCall can block or replace
// the execution of a tool call.
type ToolCallOverride struct {
	Block   bool   // if true, skip execution
	Result  string // result to use when blocked (empty = default error message)
	IsError bool   // whether the override result should be treated as an error
}

// Config defines the parameters for constructing an Agent.
//
// Config is composed of embedded sub-configs that group related fields:
//   - ModelConfig:      LLM model selection and generation parameters
//   - SkillConfig:      skill loading, selection, and API control
//   - ExecutionConfig:  execution mode, concurrency, middleware, and hooks
//   - CompactionConfig: context window management and compaction behavior
//
// Because sub-configs are embedded, fields are promoted to the top level:
// you can access c.Model or c.ModelConfig.Model interchangeably.
// Both struct literal construction and functional options (NewConfig) are supported.
type Config struct {
	ModelConfig
	SkillConfig
	ExecutionConfig
	CompactionConfig

	// Top-level agent configuration not belonging to a specific sub-config.
	Tools        []*Tool
	SystemPrompt string

	// WorkspaceDir 是应用数据目录（~/.mady/workspace），供 AgentStore 等
	// 基础设施使用。不作为工具沙箱边界。
	WorkspaceDir string

	// ProjectDir 是用户当前项目文件夹（os.Getwd()），作为工具沙箱边界。
	// 领域工厂函数（如 AssistantAgentConfig）在构造文件工具时读取此字段，
	// 避免硬编码相对路径导致非项目目录启动时沙箱错位。
	// 案件模式下覆盖为 RootPath。空字符串时工具回退到 WorkspaceDir。
	ProjectDir string

	Store Store // optional: enables SaveState / LoadState
	// Checkpoint optional durable snapshots per thread (see CheckpointSettings).
	Checkpoint *CheckpointSettings

	Handoffs []HandoffConfig // optional: sub-agents reachable via handoff
	Tracer   Tracer          // optional: distributed tracing

	// LLM-level retry with exponential backoff.
	// Context overflow errors trigger compaction instead of retry.
	RetryConfig *RetryConfig

	// TransformContext is called before ConvertToLLM to filter/modify/inject messages.
	TransformContext func(ctx context.Context, msgs []Message) []Message

	// ContextBuilder replaces TransformContext as the primary context assembly mechanism.
	// When set, Build() is called instead of TransformContext.
	// If nil, the legacy TransformContext → ConvertToLLM path is used (backward compatible).
	ContextBuilder ContextBuilder `json:"-"`

	// LayerConfigs provides per-layer configuration for ContextBuilder.
	LayerConfigs map[ContextLayer]LayerConfig `json:"layer_configs,omitempty"`

	// ConvertToLLM converts internal message types to standard LLM messages.
	// If nil, DefaultConvertToLLM is used which strips custom types.
	ConvertToLLM ConvertToLLMFunc

	// Deprecated: use Lifecycle.BeforeToolExecution instead. This hook is
	// auto-adapted to the Lifecycle chain in New() for backward compatibility.
	// Will be removed in v0.6.0.
	BeforeToolCall func(ctx context.Context, tc ToolCall) *ToolCallOverride

	// Deprecated: use Lifecycle.AfterToolExecution instead. This hook is
	// auto-adapted to the Lifecycle chain in New() for backward compatibility.
	// Will be removed in v0.6.0.
	AfterToolCall func(ctx context.Context, tc ToolCall, result *ToolResult) *ToolResult

	// Deprecated: use Lifecycle.AfterToolExecution instead (it receives the
	// full Results slice and can modify it in-place). This hook is auto-adapted
	// to the Lifecycle chain in New() for backward compatibility.
	// Will be removed in v0.6.0.
	PostProcessResults func(ctx context.Context, calls []ToolCall, results []ToolResult) []ToolResult

	// Extensions are registered during New() and contribute tools, hooks, etc.
	Extensions []Extension

	// Lifecycle hooks intercept every stage of agent execution.
	// Multiple hooks are composed via LifecycleChain.
	Lifecycle LifecycleHook
}

// Agent is the core runtime that orchestrates LLM calls and tool execution.
type Agent struct {
	config        Config
	configMu      sync.RWMutex
	configErr     error // set by New() when Validate() fails; checked in Run()
	state         *AgentState
	registry      *Registry
	executor      *Executor
	eventBus      *EventBus
	ownsEventBus  bool
	steering      *messageQueue
	followUp      *messageQueue
	extensions    *ExtensionRegistry
	contextEngine ContextEngine
	engineReg     *EngineRegistry
	interrupted   atomic.Pointer[InterruptReason]
}

func New(cfg Config) *Agent {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = defaultMaxTurns
	}

	// Validate configuration early. The error is stored and checked at
	// Run() time — this lets the Agent be constructed (callers don't need
	// to change their New() signature) while still preventing an invalid
	// Agent from executing.
	configErr := cfg.Validate()
	if configErr != nil {
		slog.Warn("agentcore: config validation failed", "err", configErr)
	}

	reg := NewRegistry()
	reg.Register(cfg.Tools...)

	unknownHandler := cfg.UnknownToolHandler
	if unknownHandler == nil {
		unknownHandler = DynamicUnknownToolHandler(reg)
	}

	// Adapt deprecated BeforeToolCall/AfterToolCall/PostProcessResults to Lifecycle hooks.
	if cfg.BeforeToolCall != nil || cfg.AfterToolCall != nil || cfg.PostProcessResults != nil {
		adapter := &deprecatedHookAdapter{
			beforeToolCall:     cfg.BeforeToolCall,
			afterToolCall:      cfg.AfterToolCall,
			postProcessResults: cfg.PostProcessResults,
		}
		cfg.Lifecycle = appendLifecycleHook(cfg.Lifecycle, adapter)
	}

	engineReg := NewEngineRegistry()

	var ctxEngine ContextEngine
	if cfg.CustomEngine != nil {
		ctxEngine = cfg.CustomEngine
	} else if cfg.ContextWindow > 0 {
		engineName := cfg.Engine
		if engineName == "" {
			engineName = engineReg.Default()
		}
		engineCfg := ContextEngineConfig{
			Model:                cfg.Model,
			BaseURL:              "",
			APIKey:               "",
			Provider:             cfg.Provider,
			ContextWindow:        cfg.ContextWindow,
			ReserveTokens:        cfg.ReserveTokens,
			KeepRecentTokens:     cfg.KeepRecentTokens,
			ProtectFirstN:        cfg.ProtectFirstN,
			CompressionThreshold: cfg.CompressionThreshold,
			AutoCompactLimit:     cfg.AutoCompactTokenLimit,
			StructuredCompaction: cfg.StructuredCompaction,
			CompressionModel:     cfg.CompressionModel,
			CompressionProvider:  cfg.CompressionProvider,
			CompressionBaseURL:   cfg.CompressionBaseURL,
			CompressionAPIKey:    cfg.CompressionAPIKey,
		}
		var err error
		ctxEngine, err = engineReg.Create(engineName, engineCfg)
		if err != nil {
			slog.Warn("agentcore: context engine init failed, falling back to default", "engine", engineName, "err", err)
			ctxEngine, err = engineReg.Create(engineReg.Default(), engineCfg)
			if err != nil {
				slog.Error("agentcore: default context engine also failed", "err", err)
			}
		}
	}

	a := &Agent{
		config:        cfg,
		configErr:     configErr,
		state:         NewState(),
		registry:      reg,
		eventBus:      NewEventBus(),
		ownsEventBus:  true,
		steering:      newMessageQueue(cfg.SteeringMode),
		followUp:      newMessageQueue(cfg.FollowUpMode),
		extensions:    NewExtensionRegistry(),
		contextEngine: ctxEngine,
		engineReg:     engineReg,
	}

	a.registerHandoffs()

	if len(cfg.AvailableSkills) > 0 {
		cfg.Extensions = append(cfg.Extensions, NewSkillExtension(cfg.AvailableSkills, cfg.SelectedSkills))
		a.config = cfg
	}

	if len(cfg.Extensions) > 0 {
		if err := a.extensions.Register(context.Background(), a, cfg.Extensions...); err != nil {
			slog.Error("agentcore: extension registration failed", "err", err)
		}
	}

	// Build executor AFTER extension registration so HookProvider hooks are included.
	execCfg := ExecutorConfig{
		Mode:               cfg.ExecutionMode,
		Concurrency:        cfg.Concurrency,
		Middleware:         cfg.Middleware,
		Before:             cfg.GlobalBefore,
		After:              cfg.GlobalAfter,
		ValidateArguments:  cfg.ValidateArguments,
		UnknownToolHandler: unknownHandler,
	}
	a.executor = NewExecutor(reg, execCfg)

	return a
}

// --- event subscriptions ---

func (a *Agent) On(t EventType, h EventHandler) func() { return a.eventBus.On(t, h) }
func (a *Agent) OnAll(h EventHandler) func()           { return a.eventBus.OnAll(h) }
func (a *Agent) EmitEvent(e Event)                     { a.eventBus.Emit(e) }
func (a *Agent) EmitExtensionSnapshots() {
	for _, e := range a.extensions.SnapshotEvents() {
		a.eventBus.Emit(e)
	}
}

// SetEventBus replaces the agent's event bus (used by sub-agents to forward
// events to a parent's bus). The agent will not close a bus it did not create.
func (a *Agent) SetEventBus(bus *EventBus) {
	a.eventBus = bus
	a.ownsEventBus = false
}

// --- state access ---

// ConfigError returns the validation error from config validation (if any).
// Returns nil when the agent configuration is valid. This can be called after
// New() to check configuration before calling Run().
func (a *Agent) ConfigError() error { return a.configErr }

func (a *Agent) Config() Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return a.config
}

// ApplyCallConfig updates the agent's Model, Thinking, ResponseFormat, and
// SelectedSkills from the given CallConfig. This is used by the server pool
// to apply thread-level or request-level overrides before reusing a cached agent.
func (a *Agent) ApplyCallConfig(cc *CallConfig) {
	if cc == nil {
		return
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if cc.Model != "" {
		a.config.Model = cc.Model
	}
	if cc.ResponseFormat != nil {
		a.config.ResponseFormat = CloneResponseFormat(cc.ResponseFormat)
	}
	if cc.Thinking != nil {
		a.config.Thinking = CloneThinkingConfig(cc.Thinking)
	}
	if len(cc.Skills) > 0 {
		a.config.SelectedSkills = CloneStringSlice(cc.Skills)
		a.extensions.Visit("skills", func(ext Extension) {
			if s, ok := ext.(interface{ SetSelected([]string) }); ok {
				s.SetSelected(CloneStringSlice(cc.Skills))
			}
		})
	}
}

// SetThinkingConfig updates thinking/reasoning configuration at runtime.
func (a *Agent) SetThinkingConfig(tc *ThinkingConfig) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.config.Thinking = tc
}

func (a *Agent) State() *AgentState { return a.state }

func (a *Agent) lifecycle() LifecycleHook {
	a.configMu.RLock()
	lc := a.config.Lifecycle
	a.configMu.RUnlock()
	return lc
}

func (a *Agent) transformContext() func(ctx context.Context, msgs []Message) []Message {
	a.configMu.RLock()
	fn := a.config.TransformContext
	a.configMu.RUnlock()
	return fn
}

// contextBuilder returns the ContextBuilder (or nil if not configured).
func (a *Agent) contextBuilder() ContextBuilder {
	a.configMu.RLock()
	cb := a.config.ContextBuilder
	a.configMu.RUnlock()
	return cb
}

func (a *Agent) systemPrompt() string {
	a.configMu.RLock()
	s := a.config.SystemPrompt
	a.configMu.RUnlock()
	return s
}

// --- tool hot-reload ---

func (a *Agent) RegisterTools(tools ...*Tool)      { a.registry.Register(tools...) }
func (a *Agent) UnregisterTools(names ...string)   { a.registry.Unregister(names...) }
func (a *Agent) ToolNames() []string               { return a.registry.Names() }
func (a *Agent) GetTool(name string) (*Tool, bool) { return a.registry.Get(name) }

// InvokeTool runs a single named tool through the exact same hook pipeline
// as a normal model-issued tool call (tool-before -> global-before ->
// middleware chain -> global-after -> tool-after), rather than calling its
// Func directly. Use this instead of GetTool+Func when a caller needs to
// invoke a tool programmatically -- e.g. from a sandboxed script via
// Programmatic Tool Calling -- while still getting audit logging,
// guardrails, and any other configured hooks applied exactly as they would
// be for the model's own tool calls.
func (a *Agent) InvokeTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	tc := ToolCall{Name: name, Arguments: string(args)}
	result := a.executor.Execute(ctx, tc, a.state)
	if result.Err != nil {
		return "", result.Err
	}
	return result.EffectiveResult(), nil
}

// --- steering & follow-up ---

// Steer injects a message that will be picked up before the next LLM call.
// Use this to redirect or interrupt the agent mid-conversation.
func (a *Agent) Steer(msg Message) { a.steering.Push(msg) }

// FollowUp queues a message that will be processed after the current
// conversation finishes (no more tool calls). The agent loop restarts
// with the follow-up as new input.
func (a *Agent) FollowUp(msg Message) { a.followUp.Push(msg) }

// --- extensions ---

func (a *Agent) ExtensionNames() []string { return a.extensions.Names() }

// Emit dispatches an event to the agent's event bus for TUI/SSE subscribers.
func (a *Agent) Emit(e Event) { a.eventBus.Emit(e) }

func (a *Agent) emit(e Event) { a.eventBus.Emit(e) }

func (a *Agent) tracer() Tracer {
	if a.config.Tracer != nil {
		return a.config.Tracer
	}
	return noopTracer{}
}
