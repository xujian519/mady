package agentcore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// --- construction & defaults ---

func TestNewToolResultBudget_Defaults(t *testing.T) {
	b := NewToolResultBudget(ToolResultBudgetConfig{RootDir: t.TempDir()})
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
	// Summary must contain head + tail + omission marker + offload metadata.
	if !strings.HasPrefix(out.Summary, strings.Repeat("x", 10)) {
		t.Error("summary should start with head snippet")
	}
	if !strings.Contains(out.Summary, "已截断") {
		t.Error("summary should contain SnipMessageContent's truncation marker")
	}
	if !strings.Contains(out.Summary, "tool_result_budget") {
		t.Error("summary should contain offload metadata")
	}
	if !strings.Contains(out.Summary, "handle:") {
		t.Error("summary should contain retrieval handle")
	}
}

// --- CJK rune-safe truncation (F1 fix) ---

func TestMaybeOffload_CJKContent_RuneSafeSummary(t *testing.T) {
	dir := t.TempDir()
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 30, // 低阈值触发 CJK 落盘
		HeadChars: 3,
		TailChars: 3,
		RootDir:   dir,
	})
	// 每个汉字 3 字节 UTF-8，5 个汉字 = 15 字节 < 30 阈值不会触发。
	// 用 12 个汉字 = 36 字节 > 30 触发。
	content := strings.Repeat("你好世界", 3) // 12 rune, 36 bytes
	out := b.MaybeOffload("read_file", content)
	if !out.Offloaded {
		t.Fatal("expected offload for CJK content above threshold")
	}
	// 摘要必须是有效 UTF-8（不能在多字节字符中间切断）。
	if !utf8.ValidString(out.Summary) {
		t.Errorf("summary must be valid UTF-8, got invalid string: %q", out.Summary)
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

// --- empty RootDir: disabled, content stays inline (F3 fix) ---

func TestMaybeOffload_EmptyRootDir_InlineFallback(t *testing.T) {
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 50,
		// RootDir intentionally empty → offload disabled.
	})
	content := strings.Repeat("big-", 30)
	out := b.MaybeOffload("read", content)
	if out.Offloaded {
		t.Error("empty RootDir should disable offload (no temp dir leak)")
	}
	if out.Summary != "" || out.Handle != "" {
		t.Errorf("expected empty OffloadResult when RootDir is empty, got %+v", out)
	}
	// 确保没有创建临时目录。
	if b.rootDir != "" {
		t.Errorf("rootDir should be empty when RootDir is empty, got %q", b.rootDir)
	}
}

// --- concurrent offload safety (F2 fix) ---

func TestMaybeOffload_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 50,
		HeadChars: 5,
		TailChars: 5,
		RootDir:   dir,
	})
	content := strings.Repeat("concurrent-", 20)

	var wg sync.WaitGroup
	handles := make([]string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			out := b.MaybeOffload("read", content)
			handles[idx] = out.Handle
		}(i)
	}
	wg.Wait()

	// All goroutines must get the same handle (content-addressed + sync.Once).
	first := handles[0]
	if first == "" {
		t.Fatal("expected offload to succeed")
	}
	for i, h := range handles {
		if h != first {
			t.Errorf("goroutine %d: handle %q != first %q (race in resolveDir?)", i, h, first)
		}
	}
	// Only one file on disk.
	files, _ := filepath.Glob(filepath.Join(dir, "*.txt"))
	if len(files) != 1 {
		t.Errorf("expected 1 file after concurrent offload, got %d", len(files))
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
	if !strings.Contains(tec.Results[0].Result, "tool_result_budget") {
		t.Error("large result should contain offload metadata")
	}
	// Small result should be untouched.
	if tec.Results[1].Result != "small" {
		t.Errorf("small result should be untouched, got %q", tec.Results[1].Result)
	}
}

func TestAfterToolExecution_NilTEC_NoPanic(t *testing.T) {
	b := NewToolResultBudget(ToolResultBudgetConfig{Threshold: 10, RootDir: t.TempDir()})
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
		t.Errorf("summary should start with head 8 runes, got prefix %q", s[:min(8, len(s))])
	}
	if !strings.Contains(s, "MMMMTAIL") {
		t.Error("summary should contain tail snippet")
	}
	if !strings.Contains(s, "tool: grep") {
		t.Error("summary should record tool name")
	}
}
