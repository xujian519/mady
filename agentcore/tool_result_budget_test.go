package agentcore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- construction & defaults ---

func TestNewToolResultBudget_Defaults(t *testing.T) {
	b := NewToolResultBudget(ToolResultBudgetConfig{})
	if b.cfg.Threshold != 8192 {
		t.Errorf("default Threshold: got %d, want 8192", b.cfg.Threshold)
	}
	if b.cfg.HeadChars != 1500 {
		t.Errorf("default HeadChars: got %d, want 1500", b.cfg.HeadChars)
	}
	if b.cfg.TailChars != 1500 {
		t.Errorf("default TailChars: got %d, want 1500", b.cfg.TailChars)
	}
}

func TestNewToolResultBudget_CustomConfig(t *testing.T) {
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 100,
		HeadChars: 20,
		TailChars: 20,
		RootDir:   t.TempDir(),
	})
	if b.cfg.Threshold != 100 {
		t.Errorf("custom Threshold: got %d", b.cfg.Threshold)
	}
}

// --- MaybeOffload: below threshold (no offload) ---

func TestMaybeOffload_BelowThreshold_NoOffload(t *testing.T) {
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 100,
		RootDir:   t.TempDir(),
	})
	out := b.MaybeOffload("read", "short content")
	if out.Offloaded {
		t.Error("short content should not be offloaded")
	}
	if out.Summary != "" || out.Handle != "" {
		t.Errorf("expected empty OffloadResult, got %+v", out)
	}
}

// --- MaybeOffload: above threshold (offload + summary) ---

func TestMaybeOffload_AboveThreshold_Offloaded(t *testing.T) {
	dir := t.TempDir()
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 100,
		HeadChars: 10,
		TailChars: 10,
		RootDir:   dir,
	})
	content := strings.Repeat("x", 250)
	out := b.MaybeOffload("read_file", content)
	if !out.Offloaded {
		t.Fatal("expected offload for content above threshold")
	}
	if out.Handle == "" {
		t.Error("Handle should be set when offloaded")
	}
	// Summary must contain head + tail + omission marker.
	if !strings.HasPrefix(out.Summary, strings.Repeat("x", 10)) {
		t.Error("summary should start with head snippet")
	}
	if !strings.Contains(out.Summary, "已省略") {
		t.Error("summary should contain omission marker")
	}
	if !strings.Contains(out.Summary, "offload handle:") {
		t.Error("summary should contain retrieval handle reference")
	}
}

// --- disk persistence: content-addressed & idempotent ---

func TestMaybeOffload_DiskContentMatches(t *testing.T) {
	dir := t.TempDir()
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 50,
		RootDir:   dir,
	})
	content := "hello tool result budget " + strings.Repeat("data ", 20)
	out := b.MaybeOffload("search", content)
	if !out.Offloaded {
		t.Fatal("expected offload")
	}
	data, err := os.ReadFile(out.Handle)
	if err != nil {
		t.Fatalf("reading offloaded file: %v", err)
	}
	if string(data) != content {
		t.Error("disk content must match original exactly")
	}
}

func TestMaybeOffload_Idempotent_SameContent(t *testing.T) {
	dir := t.TempDir()
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 50,
		RootDir:   dir,
	})
	content := strings.Repeat("idempotent-", 20)
	out1 := b.MaybeOffload("read", content)
	out2 := b.MaybeOffload("read", content)
	if out1.Handle != out2.Handle {
		t.Errorf("same content should produce same handle: %q vs %q", out1.Handle, out2.Handle)
	}
	// Only one file should exist (content-addressed).
	files, _ := filepath.Glob(filepath.Join(dir, "*.txt"))
	if len(files) != 1 {
		t.Errorf("expected 1 offloaded file, got %d", len(files))
	}
}

// --- temp dir fallback when RootDir empty ---

func TestMaybeOffload_EmptyRootDir_UsesTempDir(t *testing.T) {
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 50,
		// RootDir intentionally empty
	})
	content := strings.Repeat("temp-", 20)
	out := b.MaybeOffload("read", content)
	if !out.Offloaded {
		t.Fatal("expected offload even with empty RootDir")
	}
	if _, err := os.Stat(out.Handle); err != nil {
		t.Errorf("temp offload file should exist: %v", err)
	}
}

// --- AfterToolExecution lifecycle hook ---

func TestAfterToolExecution_OffloadsLargeResults(t *testing.T) {
	dir := t.TempDir()
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 50,
		HeadChars: 5,
		TailChars: 5,
		RootDir:   dir,
	})
	original := strings.Repeat("big", 50) // 150 bytes, well above threshold 50
	tec := &ToolExecutionContext{
		Results: []ToolResult{
			{ToolCallID: "tc1", ToolName: "read", Result: original},
			{ToolCallID: "tc2", ToolName: "calc", Result: "small"}, // below threshold
		},
	}
	b.AfterToolExecution(context.Background(), &AgentRunContext{}, tec)

	if tec.Results[0].Result == original {
		t.Error("large result should be replaced with summary")
	}
	if !strings.Contains(tec.Results[0].Result, "已省略") {
		t.Error("large result should contain omission marker")
	}
	// Small result should be untouched.
	if tec.Results[1].Result != "small" {
		t.Errorf("small result should be untouched, got %q", tec.Results[1].Result)
	}
}

func TestAfterToolExecution_NilTEC_NoPanic(t *testing.T) {
	b := NewToolResultBudget(ToolResultBudgetConfig{Threshold: 10})
	b.AfterToolExecution(context.Background(), &AgentRunContext{}, nil)
}

func TestAfterToolExecution_SkipsErrors(t *testing.T) {
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 10,
		RootDir:   t.TempDir(),
	})
	bigErr := strings.Repeat("e", 100)
	tec := &ToolExecutionContext{
		Results: []ToolResult{
			{ToolCallID: "tc1", ToolName: "fail", Result: bigErr, Err: context.Canceled},
		},
	}
	b.AfterToolExecution(context.Background(), &AgentRunContext{}, tec)
	// Error results should NOT be offloaded (the error message is what the
	// model needs to see, and it's already short in practice).
	if tec.Results[0].Result != bigErr {
		t.Error("error results should not be offloaded")
	}
}

// --- summary structure ---

func TestBuildSummary_ContainsHeadTailHandleTool(t *testing.T) {
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 10,
		HeadChars: 8,
		TailChars: 8,
		RootDir:   t.TempDir(),
	})
	content := "HEAD" + strings.Repeat("M", 100) + "TAIL"
	out := b.MaybeOffload("grep", content)
	if !out.Offloaded {
		t.Fatal("expected offload")
	}
	s := out.Summary
	if !strings.HasPrefix(s, "HEADMMMM") {
		t.Errorf("summary should start with head 8 chars, got prefix %q", s[:min(8, len(s))])
	}
	if !strings.Contains(s, "MMMMTAIL") {
		t.Error("summary should contain tail snippet")
	}
	if !strings.Contains(s, "[tool: grep]") {
		t.Error("summary should record tool name")
	}
}

func min(a, b int) int { //nolint:revive // test helper
	if a < b {
		return a
	}
	return b
}
