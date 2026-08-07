package agentcore

import (
	"time"

	"github.com/xujian519/mady/skill"
)

// ModelConfig groups LLM model selection and generation parameters.
type ModelConfig struct {
	Name        string   // optional: identifies this agent in events and handoff logs
	Model       string   // model identifier (e.g. "gpt-4o-mini")
	Provider    Provider // LLM provider implementation
	Temperature float64  // sampling temperature; 0 = deterministic
	MaxTokens   int64    // max tokens in response; 0 = provider default
	// FrequencyPenalty suppresses repeated tokens (OpenAI-compatible, ~0..2).
	// Mitigates degenerate model output that loops the same sentence (the
	// "整句 ×N" repetition observed in streaming agents). 0 = provider default.
	FrequencyPenalty float64
	// RepetitionPenalty suppresses repeated n-grams (local/vLLM endpoints such
	// as oMLX). OpenAI's official API may not accept this field; only set it
	// when targeting a compatible endpoint. 0 = not sent.
	RepetitionPenalty float64
	ResponseFormat    *ResponseFormat // optional: force JSON mode etc.
	Thinking          *ThinkingConfig // optional: extended thinking / reasoning
	Streaming         bool            // enable streaming responses
}

// SkillConfig groups skill loading, selection, and API control.
//
// This config uses concrete types from the skill package (skill.Skill,
// skill.Diagnostic). This is intentional: the skill package is a lightweight
// data+parsing leaf package within the mady project with no external
// dependencies. The coupling is intra-project and benign.
type SkillConfig struct {
	AvailableSkills         []skill.Skill
	SelectedSkills          []string
	SkillPaths              []string
	SkillDiagnostics        []skill.Diagnostic
	SkillAPIAuthToken       string
	DisableSkillRegistryAPI bool
	DisableSkillReloadAPI   bool
}

// ExecutionConfig groups execution mode, concurrency, middleware, and hooks.
type ExecutionConfig struct {
	ExecutionMode ExecutionMode
	Concurrency   int64
	MaxTurns      int64
	Middleware    []Middleware
	// Deprecated: use Middleware instead. These are auto-adapted to Middleware
	// in New() for backward compatibility. Will be removed in v0.6.0.
	GlobalBefore       []BeforeHook
	GlobalAfter        []AfterHook
	ValidateArguments  bool
	UnknownToolHandler UnknownToolHandler
	SteeringMode       SteeringMode // default: SteeringAll
	FollowUpMode       SteeringMode // default: SteeringAll
}

// CompactionConfig groups context window management and compaction behavior.
// Many fields overlap with ContextEngineConfig — this is intentional.
// CompactionConfig is user-facing agent configuration while ContextEngineConfig
// is the engine-level equivalent. The mapping happens in Agent.New().
type CompactionConfig struct {
	ContextWindow          int64         // model context window size in tokens (e.g. 128000); 0 = no compaction
	ReserveTokens          int64         // tokens reserved for response generation; default = ContextWindow/4
	KeepRecentTokens       int64         // min recent tokens preserved during compaction; default = 2000
	StructuredCompaction   bool          // emit JSON summaries instead of free-form paragraphs
	ProtectFirstN          int           // number of non-system head messages to preserve verbatim; default = 3
	CompressionThreshold   float64       // compress when usage exceeds this fraction of contextWindow; default = 0.75
	AutoCompactTokenLimit  int64         // absolute token threshold (overrides CompressionThreshold when > 0); default = 0
	AntiThrashEnabled      bool          // skip compaction if recent savings < 10%; default = true
	CompressionModel       string        // optional: separate model for summarization (cheaper/faster)
	CompressionProvider    Provider      // optional: provider for compression model
	CompressionBaseURL     string        // optional: base URL for compression model
	CompressionAPIKey      string        // optional: API key for compression model
	Engine                 string        // context engine name; default = "compressor"
	CustomEngine           ContextEngine // pre-built custom engine (overrides Engine name)
	SummaryFailureCooldown time.Duration // cooldown after a summary generation failure; default = 600s
	IneffectiveCooldown    time.Duration // cooldown after ineffective compactions; default = 300s
}

// ConfigOption is a functional option for constructing a Config.
type ConfigOption func(*Config)

// NewConfig creates a Config with the given options applied.
// Zero-value defaults are used for any unset fields; MaxTurns defaults to 20
// when the Config is passed to New().
func NewConfig(opts ...ConfigOption) Config {
	cfg := Config{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// StubConfig returns a minimal Config suitable for tests.
// It sets Model="stub" and Provider=p, which are the most common test setup.
func StubConfig(p Provider, opts ...ConfigOption) Config {
	cfg := Config{
		ModelConfig: ModelConfig{
			Model:    "stub",
			Provider: p,
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// --- Model options ---

// WithProvider sets the LLM provider.
func WithProvider(p Provider) ConfigOption {
	return func(c *Config) { c.Provider = p }
}

// WithName sets the agent name.
func WithName(name string) ConfigOption {
	return func(c *Config) { c.Name = name }
}

// WithTools sets the agent's tools.
func WithTools(tools ...*Tool) ConfigOption {
	return func(c *Config) { c.Tools = tools }
}

// WithExtensions sets the extensions.
func WithExtensions(exts ...Extension) ConfigOption {
	return func(c *Config) { c.Extensions = exts }
}

// WithLifecycleObservers adds one or more observer interfaces as lifecycle hooks.
// Each argument should implement one of AgentRunObserver, TurnObserver,
// ModelCallObserver, ToolCallObserver, or MessagePersistObserver.
func WithLifecycleObservers(observers ...any) ConfigOption {
	return func(c *Config) {
		if hook := ObserversToHook(observers...); hook != nil {
			c.Lifecycle = appendLifecycleHook(c.Lifecycle, hook)
		}
	}
}
