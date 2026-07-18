// Package adapter provides a unified interface for Mady to detect, spawn,
// and communicate with external AI coding agents (Claude Code, Codex, Cursor,
// Copilot, etc.). This enables Mady to delegate subtasks to other agents
// acting as tool-using assistants.
//
// Inspired by Open Design's agent composability model but adapted for Mady's
// provider-agnostic architecture. Each adapter implements AgentAdapter, and
// adapters auto-register via init() functions.
package adapter

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
)

// AgentCapabilities describes what an external agent can do.
type AgentCapabilities struct {
	// CanEdit indicates whether the agent can modify files.
	CanEdit bool
	// MaxContextTokens is the maximum context window size in tokens.
	MaxContextTokens int
	// SupportedTools lists tool names the agent supports natively.
	SupportedTools []string
	// Models lists available model identifiers.
	Models []string
}

// SpawnConfig configures how an agent session is launched.
type SpawnConfig struct {
	// Model is the model to use (agent-specific identifier).
	Model string
	// WorkingDir is the working directory for the agent session.
	WorkingDir string
	// ExtraArgs are additional CLI arguments passed to the agent.
	ExtraArgs []string
	// Env is additional environment variables for the agent process.
	Env map[string]string
}

// StreamChunk represents a single chunk in a streaming response.
type StreamChunk struct {
	Content string
	Done    bool
	Error   error
}

// AgentSession represents an active connection to an external agent.
// Sessions are created by AgentAdapter.Spawn and must be closed after use.
type AgentSession interface {
	// Send sends a prompt and waits for the complete response.
	Send(ctx context.Context, input string) (string, error)

	// Stream sends a prompt and streams chunks back via the returned channel.
	// The channel is closed when the response is complete or on error.
	Stream(ctx context.Context, input string) (<-chan StreamChunk, error)

	// Close terminates the agent session and releases resources.
	Close() error
}

// AgentAdapter provides a unified interface for communicating with
// external AI coding agents. Each adapter is identified by name and
// registered in the global adapter registry.
type AgentAdapter interface {
	// Name returns the adapter's unique identifier (e.g., "claude", "codex").
	Name() string

	// Description returns a human-readable description of the adapter.
	Description() string

	// Detect checks whether the corresponding agent CLI is installed and
	// accessible on this machine.
	Detect(ctx context.Context) (bool, error)

	// Spawn launches a new agent session with the given configuration.
	Spawn(ctx context.Context, cfg SpawnConfig) (AgentSession, error)

	// Capabilities returns the agent's known capabilities, independent
	// of any particular session.
	Capabilities() AgentCapabilities
}

// =============================================================================
// Adapter Registry
// =============================================================================

var (
	adapterMu sync.RWMutex
	adapters  = make(map[string]AgentAdapter)
)

// RegisterAdapter adds an adapter to the global registry.
// If an adapter with the same name already exists, it is replaced.
func RegisterAdapter(a AgentAdapter) {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	adapters[a.Name()] = a
}

// =============================================================================
// CLI Adapter — shared base for CLI-based agents (Claude, Codex, etc.)
// =============================================================================

// cliAdapter is a reusable AgentAdapter implementation for CLI-based AI
// coding assistants (Claude Code, Codex, Cursor, Copilot). Individual
// adapters declare their parameters as package-level variables and register
// via init(). This eliminates ~90% code duplication between adapters.
type cliAdapter struct {
	name      string
	desc      string
	bin       string
	subcmd    string
	maxTokens int
	models    []string
	canEdit   bool
}

func (a cliAdapter) Name() string        { return a.name }
func (a cliAdapter) Description() string { return a.desc }

func (a cliAdapter) Detect(ctx context.Context) (bool, error) {
	return detectCLI(ctx, a.bin)
}

func (a cliAdapter) Capabilities() AgentCapabilities {
	return AgentCapabilities{
		CanEdit:          a.canEdit,
		MaxContextTokens: a.maxTokens,
		SupportedTools:   []string{"read_file", "write_file", "bash", "grep", "search"},
		Models:           a.models,
	}
}

func (a cliAdapter) Spawn(ctx context.Context, cfg SpawnConfig) (AgentSession, error) {
	return newCLISession(ctx, a.bin, a.subcmd, cfg)
}

// detectCLI checks whether bin exists on PATH and responds to --version.
func detectCLI(ctx context.Context, bin string) (bool, error) {
	_, err := exec.LookPath(bin)
	if err != nil {
		return false, nil
	}
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return false, fmt.Errorf("%s --version: %w", bin, err)
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(bin)), nil
}

// LookupAdapter returns the registered adapter by name, or nil if not found.
func LookupAdapter(name string) AgentAdapter {
	adapterMu.RLock()
	defer adapterMu.RUnlock()
	return adapters[name]
}

// ListAdapters returns all registered adapters sorted by name.
func ListAdapters() []AgentAdapter {
	adapterMu.RLock()
	defer adapterMu.RUnlock()
	all := make([]AgentAdapter, 0, len(adapters))
	for _, a := range adapters {
		all = append(all, a)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Name() < all[j].Name()
	})
	return all
}

// DetectAll returns a map of adapter names to their detection results.
// Any detection errors are included in the returned map as the error value.
func DetectAll(ctx context.Context) map[string]struct {
	Available bool
	Error     error
} {
	adapterMu.RLock()
	defer adapterMu.RUnlock()
	results := make(map[string]struct {
		Available bool
		Error     error
	}, len(adapters))
	for _, a := range adapters {
		ok, err := a.Detect(ctx)
		results[a.Name()] = struct {
			Available bool
			Error     error
		}{Available: ok, Error: err}
	}
	return results
}

// AdapterIndex returns a human-readable summary of all registered adapters
// and their detection status. Detection runs concurrently via DetectAll.
// Useful for CLI diagnostics.
func AdapterIndex(ctx context.Context) string {
	all := ListAdapters()
	if len(all) == 0 {
		return "No agent adapters registered."
	}
	results := DetectAll(ctx)
	var result string
	result = fmt.Sprintf("%-12s %-8s %s\n", "ADAPTER", "STATUS", "DESCRIPTION")
	result += "------------------------------------------------------------\n"
	for _, a := range all {
		r, ok := results[a.Name()]
		status := "✗"
		if ok && r.Available {
			status = "✓"
		} else if ok && r.Error != nil {
			status = "✗ error"
		}
		result += fmt.Sprintf("%-12s %-8s %s\n", a.Name(), status, a.Description())
	}
	return result
}
