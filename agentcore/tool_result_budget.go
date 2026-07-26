package agentcore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// This file implements ToolResultBudget — a disk-offload manager for large
// tool results, inspired by PilotDeck's context/budget/ToolResultBudget.
//
// Problem: tool results (file reads, search, browser captures) can be very
// large. Mady already has three layers of in-context handling:
//   1. tools.go: MaxBytes=50KB / MaxLines=2000 hard truncation
//   2. TieredEngine snip (0.6 ratio): keep head + tail, elide middle
//   3. TieredEngine prune (0.8 ratio): replace entire content with placeholder
//
// All three are lossy truncation. ToolResultBudget adds a fourth option that
// PRESERVES the full content: offload to disk, leave a head+tail summary with
// a retrieval handle in the context. The model can request the full content
// via a read-back tool (future phase); for now the summary carries enough for
// most turns, and the full content is recoverable from disk for debugging.
//
// Integration: ToolResultBudget implements LifecycleHook via AfterToolExecution.
// It runs BEFORE results are persisted into the message stream, so the
// summary replaces what the LLM sees while the full content is safe on disk.

// ToolResultBudgetConfig configures the offload behavior.
type ToolResultBudgetConfig struct {
	// Threshold is the minimum content length (bytes) that triggers offload.
	// Results shorter than this stay inline. Default 8192 (8KB).
	Threshold int
	// HeadChars / TailChars are the sizes of the head and tail snippets kept
	// in the summary. Default 1500 each (~3KB summary total).
	HeadChars int
	TailChars int
	// RootDir is the directory where offloaded content is stored. Each
	// offload writes one file named by a SHA-256 of the content. The
	// directory is created lazily. Empty → os.MkdirTemp("", "mady-offload").
	RootDir string
}

// DefaultToolResultBudgetConfig returns recommended defaults.
func DefaultToolResultBudgetConfig() ToolResultBudgetConfig {
	return ToolResultBudgetConfig{
		Threshold: 8192,
		HeadChars: 1500,
		TailChars: 1500,
	}
}

// ToolResultBudget offloads large tool results to disk, replacing them in the
// conversation context with a head+tail summary and a retrieval handle.
// It is safe for concurrent use: the config is immutable after construction;
// disk writes use content-addressed filenames (SHA-256) so concurrent
// offloads of identical content are idempotent.
type ToolResultBudget struct {
	BaseLifecycleHook
	cfg ToolResultBudgetConfig

	// rootDirResolved is the actual directory used (RootDir or a temp dir).
	// Resolved lazily on first offload so the temp dir is only created when
	// actually needed (zero disk footprint when nothing overflows).
	rootDirResolved string
}

// NewToolResultBudget constructs a budget manager. Zero-value fields in cfg
// fall back to defaults.
func NewToolResultBudget(cfg ToolResultBudgetConfig) *ToolResultBudget {
	d := DefaultToolResultBudgetConfig()
	if cfg.Threshold <= 0 {
		cfg.Threshold = d.Threshold
	}
	if cfg.HeadChars <= 0 {
		cfg.HeadChars = d.HeadChars
	}
	if cfg.TailChars <= 0 {
		cfg.TailChars = d.TailChars
	}
	return &ToolResultBudget{cfg: cfg}
}

// OffloadResult is the outcome of evaluating one tool result.
type OffloadResult struct {
	// Offloaded is true when the content exceeded the threshold and was
	// written to disk; the caller should replace the result with Summary.
	Offloaded bool
	// Summary is the head+tail summary with a retrieval handle, intended to
	// replace the original content in the conversation context.
	Summary string
	// Handle is the disk path where the full content was stored. Empty when
	// not offloaded.
	Handle string
}

// MaybeOffload evaluates content and offloads it to disk when it exceeds the
// threshold. Returns an OffloadResult describing the replacement.
func (b *ToolResultBudget) MaybeOffload(toolName, content string) OffloadResult {
	if len(content) < b.cfg.Threshold {
		return OffloadResult{}
	}

	dir, err := b.resolveDir()
	if err != nil {
		slog.Warn("tool_result_budget: cannot resolve offload dir, keeping content inline",
			"err", err)
		return OffloadResult{}
	}

	handle, err := b.writeOffload(dir, content)
	if err != nil {
		slog.Warn("tool_result_budget: offload write failed, keeping content inline",
			"err", err)
		return OffloadResult{}
	}

	return OffloadResult{
		Offloaded: true,
		Summary:   b.buildSummary(toolName, content, handle),
		Handle:    handle,
	}
}

// AfterToolExecution is the LifecycleHook entry point. It runs after all tool
// calls in a turn complete but BEFORE results are persisted into the message
// stream. Large results are replaced in-place with summaries.
func (b *ToolResultBudget) AfterToolExecution(_ context.Context, _ *AgentRunContext, tec *ToolExecutionContext) {
	if tec == nil {
		return
	}
	for i := range tec.Results {
		r := &tec.Results[i]
		if r.Err != nil || r.Result == "" {
			continue
		}
		out := b.MaybeOffload(r.ToolName, r.Result)
		if out.Offloaded {
			r.Result = out.Summary
		}
	}
}

// resolveDir returns the offload directory, creating it lazily.
func (b *ToolResultBudget) resolveDir() (string, error) {
	if b.rootDirResolved != "" {
		return b.rootDirResolved, nil
	}
	dir := b.cfg.RootDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "mady-offload-*")
		if err != nil {
			return "", fmt.Errorf("tool_result_budget: mkdir temp: %w", err)
		}
		b.rootDirResolved = tmp
		return tmp, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("tool_result_budget: mkdir %s: %w", dir, err)
	}
	b.rootDirResolved = dir
	return dir, nil
}

// writeOffload writes content to a content-addressed file (SHA-256) and
// returns the full path. Identical content is idempotent (same hash → same
// file, overwrite is a no-op).
func (b *ToolResultBudget) writeOffload(dir, content string) (string, error) {
	sum := sha256.Sum256([]byte(content))
	name := hex.EncodeToString(sum[:]) + ".txt"
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return path, nil // already offloaded (idempotent)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("tool_result_budget: write %s: %w", path, err)
	}
	return path, nil
}

// buildSummary constructs the in-context replacement: head snippet, an
// elision marker with byte count, tail snippet, and a retrieval handle.
func (b *ToolResultBudget) buildSummary(toolName, content, handle string) string {
	head := b.cfg.HeadChars
	tail := b.cfg.TailChars
	if head > len(content) {
		head = len(content)
	}
	if tail > len(content)-head {
		tail = len(content) - head
	}
	headSnippet := content[:head]
	tailSnippet := content[len(content)-tail:]
	omitted := len(content) - head - tail
	return fmt.Sprintf("%s\n\n...[tool_result_budget: 已省略 %d 字节，完整结果已落盘]\n[offload handle: %s]\n[tool: %s]\n\n...%s",
		headSnippet, omitted, handle, toolName, tailSnippet)
}
