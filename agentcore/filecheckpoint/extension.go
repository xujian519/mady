package filecheckpoint

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName is the registration name for the file checkpoint extension.
const ExtensionName = "file_checkpoint"

// writerTools is the set of built-in tools that modify files and should be
// snapshotted before execution.
var writerTools = map[string]bool{
	"edit":       true,
	"write_file": true,
	"patch":      true,
	"delete":     true,
	"move":       true,
}

// FileCheckpointExtension integrates file-level checkpointing into the agent
// lifecycle. It snapshots files before writer tools modify them, enabling
// one-click rewind to any previous turn.
type FileCheckpointExtension struct {
	store *Store
}

var (
	_ agentcore.Extension         = (*FileCheckpointExtension)(nil)
	_ agentcore.HookProvider      = (*FileCheckpointExtension)(nil)
	_ agentcore.LifecycleProvider = (*FileCheckpointExtension)(nil)
)

// NewExtension creates a file checkpoint extension for the given workspace root.
func NewExtension(root string) *FileCheckpointExtension {
	return &FileCheckpointExtension{store: New(OSFileSystem{}, root)}
}

// NewExtensionWithFS creates a file checkpoint extension with a custom FileSystem
// (for testing).
func NewExtensionWithFS(fs FileSystem, root string) *FileCheckpointExtension {
	return &FileCheckpointExtension{store: New(fs, root)}
}

// Store returns the underlying checkpoint store for direct access.
func (e *FileCheckpointExtension) Store() *Store { return e.store }

// Name implements agentcore.Extension.
func (e *FileCheckpointExtension) Name() string { return ExtensionName }

// Init implements agentcore.Extension.
func (e *FileCheckpointExtension) Init(_ context.Context, _ *agentcore.Agent) error {
	return nil
}

// Dispose implements agentcore.Extension.
func (e *FileCheckpointExtension) Dispose() error { return nil }

// BeforeHooks implements agentcore.HookProvider, injecting a snapshot hook
// before every writer tool.
func (e *FileCheckpointExtension) BeforeHooks() []agentcore.BeforeHook {
	return []agentcore.BeforeHook{e.beforeWriteHook}
}

// AfterHooks implements agentcore.HookProvider (no-op for checkpointing).
func (e *FileCheckpointExtension) AfterHooks() []agentcore.AfterHook {
	return nil
}

// LifecycleHook implements agentcore.LifecycleProvider.
func (e *FileCheckpointExtension) LifecycleHook() agentcore.LifecycleHook {
	return &checkpointHook{ext: e}
}

func (e *FileCheckpointExtension) beforeWriteHook(_ context.Context, hc *agentcore.HookContext) error {
	if hc == nil {
		return nil
	}
	var paths []string
	switch hc.ToolName {
	case "bash":
		// bash is not in writerTools because its target paths are not
		// declared in args; extract best-effort from redirections.
		paths = extractBashWritePaths(hc.Arguments)
	default:
		if !writerTools[hc.ToolName] {
			return nil
		}
		paths = extractPathsFromArgs(hc.Arguments)
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		if err := e.store.SnapshotFile(p); err != nil {
			// Log the failure so operators know the file won't be restorable.
			// Non-fatal: don't block the tool call itself.
			slog.Warn("filecheckpoint: snapshot failed, file will not be restorable",
				"path", p,
				"error", err,
			)
			continue
		}
	}
	return nil
}

type checkpointHook struct {
	agentcore.BaseLifecycleHook
	ext *FileCheckpointExtension
}

func (h *checkpointHook) BeforeTurn(_ context.Context, arc *agentcore.AgentRunContext) error {
	if arc == nil {
		return nil
	}
	// The current turn's user message is the last entry in Messages at
	// BeforeTurn time; its index is len-1. Guard against empty slices.
	idx := len(arc.Messages) - 1
	if idx < 0 {
		idx = 0
	}
	h.ext.store.BeginTurn(arc.Turn, arc.Input, idx)
	return nil
}

func (h *checkpointHook) AfterTurn(_ context.Context, _ *agentcore.AgentRunContext, _ agentcore.TurnInfo) {
	h.ext.store.EndTurn()
}

// extractPathsFromArgs extracts file paths from the JSON arguments of writer
// tools (edit, write_file, patch, delete, move).
func extractPathsFromArgs(args json.RawMessage) []string {
	if len(args) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return nil
	}
	var paths []string
	for _, key := range []string{"path", "file_path", "source_path", "destination_path"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			s = strings.TrimSpace(s)
			if s != "" {
				paths = append(paths, s)
			}
		}
	}
	for _, key := range []string{"paths", "file_paths"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var vals []string
		if err := json.Unmarshal(raw, &vals); err == nil {
			for _, v := range vals {
				v = strings.TrimSpace(v)
				if v != "" {
					paths = append(paths, v)
				}
			}
		}
	}
	return paths
}

// extractBashWritePaths extracts file paths that a bash command is likely to
// modify, focusing on output redirections (> file, >> file, and the attached
// >file form). This is a BEST-EFFORT heuristic — bash is Turing-complete and
// paths derived from command substitution, variables, heredocs, or tools like
// sed -i/awk are NOT captured. Callers requiring guaranteed rollback must
// avoid bash for file mutation or snapshot the whole workspace. Despite
// being incomplete, this covers the most common redirection patterns that
// would otherwise silently escape checkpointing.
func extractBashWritePaths(args json.RawMessage) []string {
	if len(args) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return nil
	}
	raw, ok := fields["command"]
	if !ok {
		return nil
	}
	var cmd string
	if err := json.Unmarshal(raw, &cmd); err != nil {
		return nil
	}
	var paths []string
	tokens := strings.Fields(cmd)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch {
		case tok == ">" || tok == ">>":
			// "cmd > file": target is the next token.
			if i+1 < len(tokens) {
				paths = append(paths, tokens[i+1])
				i++ // skip the consumed target token
			}
		case strings.HasPrefix(tok, ">"):
			// "cmd >file": target attached to the operator.
			target := strings.TrimLeft(tok, ">")
			if target != "" {
				paths = append(paths, target)
			}
		}
	}
	return paths
}
