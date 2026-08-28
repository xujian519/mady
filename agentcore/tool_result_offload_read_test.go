package agentcore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupOffloadRoundtrip(t *testing.T) (*ToolResultBudget, *Tool, OffloadResult, string) {
	t.Helper()
	dir := t.TempDir()
	b := NewToolResultBudget(ToolResultBudgetConfig{
		Threshold: 100,
		HeadChars: 10,
		TailChars: 10,
		RootDir:   dir,
	})
	content := strings.Repeat("中段数据", 100) + "END"
	out := b.MaybeOffload("read_file", content)
	if !out.Offloaded {
		t.Fatal("expected offload")
	}
	return b, NewOffloadReadTool(dir), out, content
}

func TestOffloadRead_Roundtrip(t *testing.T) {
	_, tool, out, content := setupOffloadRoundtrip(t)
	args, _ := json.Marshal(map[string]string{"handle": out.Handle})
	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if got != content {
		t.Errorf("roundtrip content mismatch: got %d bytes want %d bytes", len(got), len(content))
	}
	// 摘要应提示 offload_read 回读通道。
	if !strings.Contains(out.Summary, "offload_read") {
		t.Errorf("summary should hint offload_read, got %s", out.Summary)
	}
}

func TestOffloadRead_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	tool := NewOffloadReadTool(dir)
	// 目录外真实文件。
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, handle := range []string{"../secret.txt", secret, dir, ""} {
		args, _ := json.Marshal(map[string]string{"handle": handle})
		result, err := tool.Func(context.Background(), args)
		if err != nil {
			t.Fatal(err)
		}
		hr, ok := result.(HandoffResult)
		if !ok || hr.Success {
			t.Errorf("handle %q must be rejected", handle)
		}
	}
}

func TestOffloadRead_UnknownHandle(t *testing.T) {
	dir := t.TempDir()
	tool := NewOffloadReadTool(dir)
	args, _ := json.Marshal(map[string]string{"handle": filepath.Join(dir, "nonexistent.txt")})
	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	hr, ok := result.(HandoffResult)
	if !ok || hr.Success || !strings.Contains(hr.Result, "不存在") {
		t.Errorf("unknown handle must fail cleanly, got %#v", result)
	}
}
