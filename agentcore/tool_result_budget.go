package agentcore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
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
	// HeadChars / TailChars are the sizes of the head and tail snippets
	// (in RUNES, not bytes) kept in the summary. Reuses SnipMessageContent
	// for rune-safe truncation that never splits multi-byte UTF-8 characters.
	// Default 1500 each.
	HeadChars int
	TailChars int
	// RootDir is the directory where offloaded content is stored. Each
	// offload writes one file named by a SHA-256 of the content. The
	// directory is created lazily on first offload. REQUIRED: when empty,
	// offload is disabled (content stays inline) to avoid leaking temp dirs.
	// Callers integrating into a runtime should set this to a managed path
	// (e.g. $MADY_HOME/offload/) with a cleanup strategy.
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
//
// It is safe for concurrent use: the config is immutable after construction,
// and the offload directory is resolved exactly once via sync.Once. Disk
// writes use content-addressed filenames (SHA-256) so concurrent offloads of
// identical content are idempotent.
type ToolResultBudget struct {
	BaseLifecycleHook
	cfg ToolResultBudgetConfig

	// resolveOnce ensures the offload directory is created at most once,
	// even under concurrent MaybeOffload calls. rootDir holds the resolved
	// path; resolveErr holds any creation error. Both are stable after the
	// first resolveDir call returns.
	resolveOnce sync.Once
	rootDir     string
	resolveErr  error
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

// resolveDir returns the offload directory, creating it exactly once. Returns
// an error when RootDir is empty (offload disabled) or directory creation
// fails. Thread-safe via sync.Once.
func (b *ToolResultBudget) resolveDir() (string, error) {
	b.resolveOnce.Do(func() {
		if b.cfg.RootDir == "" {
			b.resolveErr = fmt.Errorf("tool_result_budget: RootDir is empty (offload disabled; set RootDir to enable)")
			return
		}
		if err := os.MkdirAll(b.cfg.RootDir, 0o750); err != nil {
			b.resolveErr = fmt.Errorf("tool_result_budget: mkdir %s: %w", b.cfg.RootDir, err)
			return
		}
		b.rootDir = b.cfg.RootDir
	})
	return b.rootDir, b.resolveErr
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

// buildSummary constructs the in-context replacement by reusing the canonical
// SnipMessageContent for rune-safe head+tail truncation (never splits
// multi-byte UTF-8 — critical for CJK patent/legal content), then appending
// the offload metadata so the model knows the full content is recoverable.
func (b *ToolResultBudget) buildSummary(toolName, content, handle string) string {
	snipped := SnipMessageContent(content, b.cfg.HeadChars, b.cfg.TailChars)
	return snipped + fmt.Sprintf(
		"\n[tool_result_budget: 完整结果已落盘 | handle: %s | tool: %s]",
		handle, toolName)
}
