package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

const defaultMaxTurns = 20

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

	// SessionID 是当前对话会话标识，供 LifecycleHook（如 ApprovalGate）在
	// 持久化审批记录时关联到正确会话。空字符串时表示无会话上下文。
	SessionID string

	// CaseID 是当前案件标识，供 LifecycleHook 在持久化审批记录时关联案件。
	// 空字符串时表示无案件上下文。
	CaseID string

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

	// Extensions are registered during New() and contribute tools, hooks, etc.
	Extensions []Extension

	// Lifecycle hooks intercept every stage of agent execution.
	// Multiple hooks are composed via LifecycleChain.
	Lifecycle LifecycleHook

	// FallbackRouter routes to alternative models when the primary fails.
	// When non-nil, callModelWithFallback iterates through the fallback chain
	// defined per complexity level. Gateway sets this field when it creates
	// the FallbackRouter internally; callers normally leave it nil and let
	// newDefaultGateway populate it.
	FallbackRouter *FallbackRouter

	// FallbackConfig is pure data describing the fallback candidate chains
	// per complexity. When non-nil, newDefaultGateway uses it to construct
	// the FallbackRouter with real candidates; nil → empty candidates
	// (safe no-op, the model from the request is kept).
	// This decouples configuration (data) from the router instance that
	// Gateway owns.
	FallbackConfig *FallbackConfig
}

// A2UIAction represents a user action from the A2UI protocol (e.g. approval
// approve/reject, button click). It is intentionally decoupled from the a2ui
// package to keep agentcore dependency-free. Server.desktop.go converts
// *a2ui.ClientAction to *A2UIAction before delivering via A2UIPromise.
type A2UIAction struct {
	Name    string
	Context map[string]any
}

// A2UIPromise provides goroutine-safe one-shot delivery of an A2UI action
// from the SendAction caller (a UI goroutine) to the agent run loop.
//
// Usage:
//
//	promise := NewA2UIPromise()
//	agent.SetA2UIPromise(promise)
//	// ... on some UI event:
//	promise.Set(action)
//	// ... agent's runPreTurn calls promise.TryGet() — returns once.
//
// TryGet returns the action exactly once (the second call returns nil),
// preventing the same action from being processed in multiple turns.
type A2UIPromise struct {
	mu       sync.Mutex
	action   *A2UIAction
	consumed bool
}

// NewA2UIPromise creates a promise ready for Set/TryGet.
func NewA2UIPromise() *A2UIPromise {
	return &A2UIPromise{}
}

// Set delivers an action to the promise. Idempotent: the first Set wins,
// subsequent calls are silently ignored.
func (p *A2UIPromise) Set(action *A2UIAction) {
	if action == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.action != nil {
		return // first Set wins
	}
	p.action = action
}

// TryGet returns the action if one has been Set and not yet consumed.
// Consumed actions are never returned again.
func (p *A2UIPromise) TryGet() *A2UIAction {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.consumed || p.action == nil {
		return nil
	}
	p.consumed = true
	return p.action
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
	a2uiPromise   *A2UIPromise // optional, set by desktop/adapter for UI→agent action delivery
	// intentCacheMu + intentCache provide a per-Agent LLM intent summary
	// cache. Previously this was a package-level global, which caused
	// cross-agent cache pollution in multi-agent setups.
	intentCacheMu sync.Mutex
	intentCache   map[string]intentCacheEntry
	todopad       *TodoPad // lightweight ordered scratchpad, reset per Run()
}

// New creates an Agent with the given configuration, registering tools,
// extensions, and setting up the context engine and executor.
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
		steering:      newMessageQueue(cfg.SteeringMode, 0),
		followUp:      newMessageQueue(cfg.FollowUpMode, 0),
		extensions:    NewExtensionRegistry(),
		contextEngine: ctxEngine,
		engineReg:     engineReg,
		todopad:       NewTodoPad(),
	}

	a.registerHandoffs()
	a.RegisterTools(
		newTodoSetupTool(a.todopad),
		newTodoTickTool(a.todopad),
		newTodoListTool(a.todopad),
	)

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

// On subscribes to events of the given type. Returns an unregister function.
func (a *Agent) On(t EventType, h EventHandler) func() { return a.eventBus.On(t, h) }

// OnAll subscribes to all events. Returns an unregister function.
func (a *Agent) OnAll(h EventHandler) func() { return a.eventBus.OnAll(h) }

// EmitEvent dispatches an event to the agent's event bus.
func (a *Agent) EmitEvent(e Event) { a.eventBus.Emit(e) }

// EmitExtensionSnapshots emits snapshot events from all registered extensions.
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

// EventBus returns the agent's event bus for registering external handlers
// (e.g. EventLogger, custom listeners).
func (a *Agent) EventBus() *EventBus { return a.eventBus }

// SetA2UIPromise installs an A2UIPromise so that A2UI actions (e.g. approval
// decisions, button clicks) can be delivered from UI goroutines to the agent's
// run loop. Pass nil to clear a previously installed promise.
// A2UIPromise is an opt-in mechanism: the TUI path does not set it, so the
// agent's runPreTurn simply skips the nil check with zero overhead.
func (a *Agent) SetA2UIPromise(p *A2UIPromise) { a.a2uiPromise = p }

// SetA2UIAction delivers a user action from the A2UI protocol into the agent's
// promise. The action is consumed by consumePendingA2UIActions during the next
// runPreTurn. If no promise is installed this is a no-op (safe for TUI path).
func (a *Agent) SetA2UIAction(action *A2UIAction) {
	if a.a2uiPromise != nil {
		a.a2uiPromise.Set(action)
	}
}

// --- state access ---

// ConfigError returns the validation error from config validation (if any).
// Returns nil when the agent configuration is valid. This can be called after
// New() to check configuration before calling Run().
func (a *Agent) ConfigError() error { return a.configErr }

// Config returns a shallow copy of the agent's configuration, safe for concurrent access.
func (a *Agent) Config() Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	// Shallow-copy slice/map fields so callers cannot race against
	// the agent's internal state by mutating the returned Config.
	cfg := a.config
	cfg.Tools = append([]*Tool(nil), cfg.Tools...)
	cfg.Handoffs = append([]HandoffConfig(nil), cfg.Handoffs...)
	cfg.Extensions = append([]Extension(nil), cfg.Extensions...)
	cfg.Middleware = append([]Middleware(nil), cfg.Middleware...)
	cfg.GlobalBefore = append([]BeforeHook(nil), cfg.GlobalBefore...)
	cfg.GlobalAfter = append([]AfterHook(nil), cfg.GlobalAfter...)
	if cfg.LayerConfigs != nil {
		cfg.LayerConfigs = cloneMap(cfg.LayerConfigs)
	}
	return cfg
}

// cloneMap shallow-copies a ContextLayer→LayerConfig map.
func cloneMap(src map[ContextLayer]LayerConfig) map[ContextLayer]LayerConfig {
	dst := make(map[ContextLayer]LayerConfig, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
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

// State returns the agent's mutable conversation state.
func (a *Agent) State() *AgentState { return a.state }

// lifecycle 返回当前配置的 LifecycleHook。
// 每次调用执行一次 configMu.RLock→RUnlock。在 runLoop 执行期间 configMu
// 无写锁竞争（配置在运行时仅通过 SetThinkingConfig 等外部入口变更，不会
// 在循环中触发），单次锁开销约 25–50 ns，与 LLM 秒级延迟相比可忽略。
// 有意保留 RWMutex 而非原子指针：配置在生命周期外可能热更新，
// RLock 提供了语义正确的并发契约。

func (a *Agent) lifecycle() LifecycleHook {
	a.configMu.RLock()
	lc := a.config.Lifecycle
	a.configMu.RUnlock()
	return lc
}

// newRunContext 创建填充了会话/案件上下文的 AgentRunContext。
// SessionID 和 CaseID 从 Config 自动传播，LifecycleHook 无需自行查找。
func (a *Agent) newRunContext(input string, messages []Message, turn int64) *AgentRunContext {
	a.configMu.RLock()
	sid, cid := a.config.SessionID, a.config.CaseID
	a.configMu.RUnlock()
	return &AgentRunContext{
		Agent: a, Input: input, Messages: messages, Turn: turn,
		SessionID: sid, CaseID: cid,
	}
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

// RegisterTools adds one or more tools to the agent's registry at runtime.
func (a *Agent) RegisterTools(tools ...*Tool) { a.registry.Register(tools...) }

// UnregisterTools removes one or more tools from the agent's registry by name.
func (a *Agent) UnregisterTools(names ...string) { a.registry.Unregister(names...) }

// ToolNames returns the names of all registered tools.
func (a *Agent) ToolNames() []string { return a.registry.Names() }

// GetTool returns a tool by name, or false if not found.
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
	if a.configErr != nil {
		return "", fmt.Errorf("agentcore: agent configuration is invalid: %w", a.configErr)
	}
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
func (a *Agent) Steer(msg Message) {
	if err := a.steering.Push(msg); err != nil {
		slog.Warn("agent: failed to push steering message", "error", err)
	}
}

// FollowUp queues a message that will be processed after the current
// conversation finishes (no more tool calls). The agent loop restarts
// with the follow-up as new input.
func (a *Agent) FollowUp(msg Message) {
	if err := a.followUp.Push(msg); err != nil {
		slog.Warn("agent: failed to push follow-up message", "error", err)
	}
}

// --- extensions ---

// ExtensionNames returns the names of all registered extensions.
func (a *Agent) ExtensionNames() []string { return a.extensions.Names() }

// Emit dispatches an event to the agent's event bus for TUI/SSE subscribers.
func (a *Agent) Emit(e Event) { a.eventBus.Emit(e) }

func (a *Agent) emit(e Event) { a.eventBus.Emit(e) }

// emitMustDeliver 使用有界阻塞语义发射终态事件（完成/错误/中断）。
// 与 emit 不同，此方法保证在超时到期前尽力投递，适用于必须到达处理器的关键事件。
func (a *Agent) emitMustDeliver(ctx context.Context, e Event) {
	a.eventBus.EmitMustDeliver(ctx, e)
}

func (a *Agent) tracer() Tracer {
	if a.config.Tracer != nil {
		return a.config.Tracer
	}
	return noopTracer{}
}
