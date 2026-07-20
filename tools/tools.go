// Package tools provides a set of built-in filesystem and shell tools for mady agents.
//
// This package is designed as a mady Extension. Register it with an agent to add
// read, edit, ls, grep, find, and bash tools to the agent's tool registry.
//
// Usage:
//
//	import "github.com/xujian519/mady/tools"
//
//	ext := tools.NewExtension(tools.ExtensionConfig{WorkingDir: "/path/to/project"})
//	agent := agentcore.New(agentcore.NewConfig(agentcore.WithExtensions(ext)))
//
// Each tool supports pluggable operations so you can delegate to remote systems (e.g. SSH).
package tools

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tools/desktop"
)

// 工具名常量，用于 DisableTools 配置。
// 使用常量而非裸字符串可在工具重命名时获得编译期安全——
// 若工具构造函数内 Name 字段变更而常量未更新，引用的地方会编译报错。
const (
	ToolBash        = "bash"
	ToolGitStatus   = "git_status"
	ToolGitDiff     = "git_diff"
	ToolGitLog      = "git_log"
	ToolBrowser     = "browser"
	ToolExecuteCode = "execute_code"
	ToolComputerUse = "computer_use"
	ToolProcess     = "process"
	ToolVision      = "vision_analyze"
)

// ComputerUseToolConfig is a type alias for backward compatibility.
// New code should use desktop.ComputerUseToolConfig directly.
type ComputerUseToolConfig = desktop.ComputerUseToolConfig

// NewComputerUseTool is re-exported from the desktop package for backward compatibility.
var NewComputerUseTool = desktop.NewComputerUseTool

// Shared default limits used across tool configurations.
const (
	DefaultMaxLines = 2000
	DefaultMaxBytes = 50 * 1024 // 50KB
)

// ExtensionConfig configures the built-in tools extension.
type ExtensionConfig struct {
	// WorkingDir is the base directory for all relative paths. Defaults to the
	// current working directory if empty.
	WorkingDir string

	// Read configures the read tool. If nil, default local filesystem operations are used.
	Read *ReadToolConfig

	// Edit configures the edit tool. If nil, default local filesystem operations are used.
	Edit *EditToolConfig

	// Ls configures the ls tool. If nil, default local filesystem operations are used.
	Ls *LsToolConfig

	// Grep configures the grep tool. If nil, default local filesystem + ripgrep is used.
	Grep *GrepToolConfig

	// Find configures the find tool. If nil, default local filesystem + fd is used.
	Find *FindToolConfig

	// Bash configures the bash tool. If nil, default local shell execution is used.
	Bash *BashToolConfig

	// WriteFile configures the write_file tool. If nil, default local filesystem operations are used.
	WriteFile *WriteFileToolConfig

	// Patch configures the patch tool. If nil, default local filesystem operations are used.
	Patch *PatchToolConfig

	// Process configures the process tool. If nil, default local process operations are used.
	Process *ProcessToolConfig

	// Vision configures the vision_analyze tool. If nil, default operations are used.
	Vision *VisionToolConfig

	// View configures the view tool. If nil, default local filesystem operations are used.
	View *ViewToolConfig

	// Glob configures the glob tool. If nil, default local filesystem operations are used.
	Glob *GlobToolConfig

	// Delete configures the delete tool. If nil, default local filesystem operations are used.
	Delete *DeleteToolConfig

	// Move configures the move tool. If nil, default local filesystem operations are used.
	Move *MoveToolConfig

	// Git configures the git tools. If nil, default local git operations are used.
	Git *GitToolConfig

	// Browser configures the browser tools. If nil, browser tools are not included.
	Browser *BrowserToolConfig

	// WebSearch configures the web_search tool. If nil, default auto-selecting search is used.
	WebSearch *WebSearchToolConfig

	// WebFetch configures the web_fetch tool. If nil, default HTTP fetch is used.
	WebFetch *WebFetchToolConfig

	// ScholarSearch configures the scholar_search tool (学术论文检索).
	// Uses Semantic Scholar API. Set SEMANTIC_SCHOLAR_API_KEY env var for higher rate limits.
	ScholarSearch *ScholarSearchConfig

	PatentTool *PatentToolConfig
	// NuoPatentPath defaults to PATH-based resolution ("nuo-patent" or "npx nuo-patent").
	// PatentTool configures patent lookup/download tools via nuo-patent CLI.

	// ExecuteCode configures the execute_code tool. If nil, a default configuration is used.
	ExecuteCode *ExecuteCodeToolConfig

	// ComputerUse enables the computer_use tool (macOS desktop control). macOS only.
	ComputerUse bool

	// ComputerUseConfig configures the computer_use tool. Nil uses sensible defaults.
	ComputerUseConfig *desktop.ComputerUseToolConfig

	// SandboxEnabled enforces the WorkingDir boundary for file tools when true.
	// When enabled, read/write/edit/etc. tools reject paths that escape the
	// WorkingDir subtree. Default is false (Go bool zero value); domain
	// factory functions (AssistantAgentConfig, BuildProjectAgent, etc.) must
	// explicitly set this to true. Propagated to individual tool configs in
	// BuildTools.
	SandboxEnabled bool

	// EnabledTools is a positive allowlist of tool names to include.
	// When non-empty, only tools in this list are registered (overrides
	// DisableTools). Use this for fine-grained per-project tool sets
	// where only a known subset should be available.
	EnabledTools []string

	// DisableTools is a list of tool names to exclude from registration.
	// Use this to limit the tool set for specific agent roles (e.g., disable
	// bash/git/browser for a retrieval-only assistant). An empty list means
	// all tools are enabled (backward compatible).
	DisableTools []string

	// ExtraTools is an optional set of add-on tools to register on top of
	// the built-in set. Use this for domain-specific tools that live outside
	// the tools package (e.g. workflows/patent.NewPatentNoveltyTool).
	// These tools bypass the EnabledTools/DisableTools filtering: if you
	// pass an extra tool, it WILL be registered regardless of the allow/block
	// lists. This is intentional — extra tools are explicitly opted-in by the
	// caller (the domain config factory) and should not be silently suppressed.
	ExtraTools []*agentcore.Tool

	// MaxBytes is the default output byte limit shared across all tools (default: 50KB).
	MaxBytes int64

	// MaxLines is the default output line limit shared across all tools (default: 2000).
	MaxLines int64
}

func (c *ExtensionConfig) setDefaults() {
	if c.MaxBytes <= 0 {
		c.MaxBytes = DefaultMaxBytes
	}
	if c.MaxLines <= 0 {
		c.MaxLines = DefaultMaxLines
	}
}

// Extension is a mady Extension that registers built-in filesystem and shell tools.
type Extension struct {
	config ExtensionConfig
	tools  []*agentcore.Tool
}

// Compile-time check: tools.Extension satisfies agentcore.Extension.
var _ agentcore.Extension = (*Extension)(nil)

// NewExtension creates a new built-in tools extension with the given configuration.
func NewExtension(cfg ExtensionConfig) *Extension {
	cfg.setDefaults()
	return &Extension{config: cfg}
}

// Name returns the extension name.
func (e *Extension) Name() string { return "builtin-tools" }

// WithVision configures the vision_analyze tool to use the given provider and
// model. 必须在 Init() 之前调用（即在 agentcore.New 之前），否则配置不生效。
// 对 nil 接收者安全（no-op）。
// 浏览器工具（browser vision action / browser_vision）共享同一视觉配置；
// 显式设置的 Browser.Vision 不被覆盖。
func (e *Extension) WithVision(provider agentcore.Provider, model string) {
	if e == nil {
		return
	}
	e.config.Vision = &VisionToolConfig{
		Provider: provider,
		Model:    model,
	}
	if e.config.Browser != nil && e.config.Browser.Vision == nil {
		e.config.Browser.Vision = &VisionToolConfig{
			Provider: provider,
			Model:    model,
		}
	}
}

// Init initializes the extension and registers all tools with the agent.
func (e *Extension) Init(_ context.Context, agent *agentcore.Agent) error {
	e.tools = BuildTools(e.config)
	agent.RegisterTools(e.tools...)
	return nil
}

// Dispose tears down the extension.
func (e *Extension) Dispose() error { return nil }

// Tools returns the tools managed by this extension.
func (e *Extension) Tools() []*agentcore.Tool { return e.tools }

// BuildTools constructs the full set of built-in tools from the given config.
// Tools listed in cfg.DisableTools are excluded from the result.
// This is useful when you want the tools but not the extension lifecycle.
func BuildTools(cfg ExtensionConfig) []*agentcore.Tool {
	cfg.setDefaults()
	disabled := disabledSet(cfg.DisableTools)
	enabled := enabledSet(cfg.EnabledTools)
	useAllowList := len(cfg.EnabledTools) > 0

	// Propagate sandbox configuration to all file tool configs.
	sbx := WorkingDirSandbox{
		Enabled:    cfg.SandboxEnabled,
		WorkingDir: cfg.WorkingDir,
	}
	if cfg.Read == nil {
		cfg.Read = &ReadToolConfig{}
	}
	cfg.Read.Sandbox = sbx
	if cfg.Edit == nil {
		cfg.Edit = &EditToolConfig{}
	}
	cfg.Edit.Sandbox = sbx
	if cfg.WriteFile == nil {
		cfg.WriteFile = &WriteFileToolConfig{}
	}
	cfg.WriteFile.Sandbox = sbx
	if cfg.Patch == nil {
		cfg.Patch = &PatchToolConfig{}
	}
	cfg.Patch.Sandbox = sbx
	if cfg.Delete == nil {
		cfg.Delete = &DeleteToolConfig{}
	}
	cfg.Delete.Sandbox = sbx
	if cfg.Move == nil {
		cfg.Move = &MoveToolConfig{}
	}
	cfg.Move.Sandbox = sbx

	// Inject sandbox into read-only tools and bash.
	if cfg.Ls == nil {
		cfg.Ls = &LsToolConfig{}
	}
	cfg.Ls.Sandbox = sbx
	if cfg.Grep == nil {
		cfg.Grep = &GrepToolConfig{}
	}
	cfg.Grep.Sandbox = sbx
	if cfg.Find == nil {
		cfg.Find = &FindToolConfig{}
	}
	cfg.Find.Sandbox = sbx
	if cfg.Glob == nil {
		cfg.Glob = &GlobToolConfig{}
	}
	cfg.Glob.Sandbox = sbx
	if cfg.View == nil {
		cfg.View = &ViewToolConfig{}
	}
	cfg.View.Sandbox = sbx
	if cfg.Bash == nil {
		cfg.Bash = &BashToolConfig{}
	}
	cfg.Bash.Sandbox = sbx

	var tools []*agentcore.Tool

	addTool := func(t *agentcore.Tool) {
		if t == nil {
			return
		}
		if useAllowList {
			if enabled[t.Name] {
				tools = append(tools, t)
			}
			return
		}
		if !disabled[t.Name] {
			tools = append(tools, t)
		}
	}

	// readOnly marks a tool as side-effect-free before registration.
	readOnly := func(t *agentcore.Tool) *agentcore.Tool {
		if t != nil {
			t.ReadOnly = true
		}
		return t
	}

	addTool(readOnly(NewReadTool(cfg.WorkingDir, cfg.Read)))
	addTool(NewEditTool(cfg.WorkingDir, cfg.Edit))
	addTool(readOnly(NewLsTool(cfg.WorkingDir, cfg.Ls)))
	addTool(readOnly(NewGrepTool(cfg.WorkingDir, cfg.Grep)))
	addTool(readOnly(NewFindTool(cfg.WorkingDir, cfg.Find)))
	addTool(NewBashTool(cfg.WorkingDir, cfg.Bash))
	addTool(NewWriteFileTool(cfg.WorkingDir, cfg.WriteFile))
	addTool(NewPatchTool(cfg.WorkingDir, cfg.Patch))
	// Ensure Process tool has a shared registry so handleStatus/handleWait/
	// handleKill/handleList can look up spawned processes.
	if cfg.Process == nil {
		cfg.Process = &ProcessToolConfig{}
	}
	if cfg.Process.Operations == nil {
		reg := NewProcessRegistry()
		cfg.Process.Operations = NewDefaultProcessOperations(reg)
	}
	if cfg.Process.Registry == nil {
		if dpo, ok := cfg.Process.Operations.(*DefaultProcessOperations); ok && dpo.registry != nil {
			cfg.Process.Registry = dpo.registry
		} else {
			cfg.Process.Registry = NewProcessRegistry()
		}
	}
	addTool(NewProcessTool(cfg.WorkingDir, cfg.Process))
	if cfg.Vision == nil {
		cfg.Vision = &VisionToolConfig{}
	}
	cfg.Vision.Sandbox = sbx
	addTool(readOnly(NewVisionTool(cfg.WorkingDir, cfg.Vision)))
	addTool(readOnly(NewViewTool(cfg.WorkingDir, cfg.View)))
	addTool(readOnly(NewGlobTool(cfg.WorkingDir, cfg.Glob)))
	addTool(NewDeleteTool(cfg.WorkingDir, cfg.Delete))
	addTool(NewMoveTool(cfg.WorkingDir, cfg.Move))
	addTool(readOnly(NewGitStatusTool(cfg.WorkingDir, cfg.Git)))
	addTool(readOnly(NewGitDiffTool(cfg.WorkingDir, cfg.Git)))
	addTool(readOnly(NewGitLogTool(cfg.WorkingDir, cfg.Git)))

	addTool(readOnly(NewWebSearchTool(cfg.WebSearch)))
	addTool(readOnly(NewWebFetchTool(cfg.WebFetch)))
	addTool(readOnly(NewScholarSearchTool(cfg.ScholarSearch)))
	addTool(readOnly(NewPatentScrapeTool(cfg.PatentTool)))
	addTool(NewPatentDownloadTool(cfg.PatentTool))
	addTool(NewPatentLegalStatusTool(cfg.PatentTool))

	if cfg.Browser != nil {
		// 浏览器视觉分析与 vision_analyze 共用同一套视觉配置
		// （显式设置的 Browser.Vision 优先）。
		if cfg.Browser.Vision == nil {
			cfg.Browser.Vision = cfg.Vision
		}
		addTool(NewBrowserTool(cfg.Browser))
	}

	if cfg.ExecuteCode != nil {
		addTool(NewExecuteCodeTool(cfg.ExecuteCode))
	}

	if cfg.ComputerUse {
		addTool(desktop.NewComputerUseTool(cfg.ComputerUseConfig))
	}

	// ExtraTools: domain-specific tools injected by the caller (e.g.
	// workflows/patent.NewPatentNoveltyTool). These bypass the allow/block
	// list filtering because they are explicitly opted-in at the call site.
	for _, t := range cfg.ExtraTools {
		if t != nil {
			tools = append(tools, t)
		}
	}

	return tools
}

// disabledSet converts a disable list to a set for O(1) lookup.
func disabledSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func enabledSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// ToolResult is the standard result type for all built-in tools.
// It serializes cleanly to JSON for the LLM.
type ToolResult struct {
	Content string `json:"content"`
	Details any    `json:"details,omitempty"`
}

func result(content string, details any) (any, error) {
	return ToolResult{Content: content, Details: details}, nil
}

func resultErrf(format string, args ...any) (any, error) {
	return nil, fmt.Errorf(format, args...)
}
