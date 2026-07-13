package acp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/agentcore"
)

// RunOptions configures a ready-to-run ACP server assembled by RunServer.
type RunOptions struct {
	// Provider is the LLM backend used by every agent instance. Required.
	Provider agentcore.Provider
	// Model is the default model id advertised to the client.
	Model string
	// Thinking configures reasoning behavior; nil leaves the provider default.
	Thinking *agentcore.ThinkingConfig
	// AgentInfo identifies the agent in the ACP initialize handshake.
	AgentInfo AgentInfo
	// HomeDir is where session metadata is persisted (defaults to ~/.mady).
	HomeDir string
	// Logger writes diagnostics to stderr (defaults to a warn-level text logger).
	Logger *slog.Logger
	// SystemPrompt overrides the default agent system prompt when non-empty.
	SystemPrompt string
	// MaxTurns caps the agent loop iterations per prompt (default 25).
	MaxTurns int
	// Lifecycle 注入知识检索等生命周期钩子（如 Wiki RAG）。
	// 为 nil 时不注入任何钩子，保持裸 LLM 对话。
	Lifecycle agentcore.LifecycleHook
	// Extensions 注入知识扩展等可选能力（如 search_knowledge / add_document 工具）。
	Extensions []agentcore.Extension
}

// sessionModePrimary is the default (and only) mode advertised over ACP.
var sessionModePrimary = SessionMode{
	ID:          "primary",
	Name:        "Primary",
	Description: "Default conversational mode",
}

const defaultACPSystemPrompt = "你是 Mady 智能助手，一个能力完备的通用 AI 代理。" +
	"你可以调用工具、检索知识、多步推理。请用简洁清晰的中文回答用户。"

// factoryAgent adapts an agentcore.Agent into an ACP AgentInstance.
type factoryAgent struct {
	core  *agentcore.Agent
	model string
	mode  string
	opts  RunOptions
}

func (a *factoryAgent) Run(ctx context.Context, input string) (string, error) {
	return a.core.Run(ctx, input)
}

func (a *factoryAgent) Core() *agentcore.Agent { return a.core }
func (a *factoryAgent) Model() string          { return a.model }
func (a *factoryAgent) Mode() string           { return a.mode }

// Rebuild reconstructs the underlying agent with a new mode/model so the
// session/prompt loop picks up runtime mode/model changes from the client.
func (a *factoryAgent) Rebuild(mode, model string) error {
	cfg := buildAgentConfig(a.opts, model)
	next := agentcore.New(cfg)
	a.core.Close()
	a.core = next
	a.model = model
	a.mode = mode
	return nil
}

// acpAgentFactory produces agents backed by a shared provider/model config.
type acpAgentFactory struct {
	opts RunOptions
}

func (f *acpAgentFactory) CreateAgent(_ context.Context, _ string, _ string, model, mode string) (AgentInstance, error) {
	if model == "" {
		model = f.opts.Model
	}
	if mode == "" {
		mode = "primary"
	}
	cfg := buildAgentConfig(f.opts, model)
	agent := agentcore.New(cfg)
	return &factoryAgent{core: agent, model: model, mode: mode, opts: f.opts}, nil
}

func (f *acpAgentFactory) DefaultModel() string { return f.opts.Model }
func (f *acpAgentFactory) DefaultMode() string  { return "primary" }
func (f *acpAgentFactory) AvailableModes() []SessionMode {
	return []SessionMode{sessionModePrimary}
}

func buildAgentConfig(opts RunOptions, model string) agentcore.Config {
	maxTurns := opts.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 25
	}
	prompt := opts.SystemPrompt
	if prompt == "" {
		prompt = defaultACPSystemPrompt
	}
	return agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:      "mady-acp",
			Model:     model,
			Provider:  opts.Provider,
			Thinking:  opts.Thinking,
			Streaming: true,
		},
		SystemPrompt: prompt,
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          int64(maxTurns),
			ExecutionMode:     agentcore.ModeSerial,
			Concurrency:       5,
			ValidateArguments: true,
		},
		CompactionConfig: agentcore.CompactionConfig{
			ContextWindow:    128000,
			ReserveTokens:    32000,
			KeepRecentTokens: 4000,
		},
		RetryConfig: &agentcore.RetryConfig{
			MaxRetries:  3,
			BaseDelayMs: 1000,
			MaxDelayMs:  15000,
		},
		Lifecycle:  opts.Lifecycle,
		Extensions: opts.Extensions,
	}
}

// RunServer assembles an AgentFactory + SessionManager + ACP Server from the
// given options and blocks until the server stops (stdin EOF or context cancel).
// It is the single entry point for embedding Mady behind any ACP-compatible
// editor such as Zed.
func RunServer(ctx context.Context, opts RunOptions) error {
	if opts.Provider == nil {
		return fmt.Errorf("acp: RunOptions.Provider is required")
	}
	if opts.Model == "" {
		opts.Model = "default"
	}
	if opts.AgentInfo.Name == "" {
		opts.AgentInfo.Name = "mady"
	}
	if opts.AgentInfo.Version == "" {
		opts.AgentInfo.Version = "0.1.0"
	}
	if opts.HomeDir == "" {
		if dir, err := os.UserHomeDir(); err == nil {
			opts.HomeDir = filepath.Join(dir, ".mady")
		} else {
			opts.HomeDir = ".mady"
		}
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}

	if err := os.MkdirAll(opts.HomeDir, 0o755); err != nil {
		return fmt.Errorf("acp: create home dir: %w", err)
	}

	factory := &acpAgentFactory{opts: opts}
	sessionMgr := NewSessionManager(SessionManagerConfig{
		AgentFactory: factory,
		HomeDir:      opts.HomeDir,
		Logger:       opts.Logger,
	})

	server := NewServer(ServerConfig{
		SessionManager: sessionMgr,
		AgentInfo:      opts.AgentInfo,
		Logger:         opts.Logger,
	})

	opts.Logger.Info("ACP server ready", "name", opts.AgentInfo.Name, "model", opts.Model)
	return server.Run(ctx)
}
