package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/bootstrap/agentconfig"
)

// ---------------------------------------------------------------------------
// Helper functions (originally in framework package)
// ---------------------------------------------------------------------------

func TestCwdPartitionName_Deterministic(t *testing.T) {
	a := CwdPartitionName("/some/path")
	b := CwdPartitionName("/some/path")
	if a != b {
		t.Fatalf("not deterministic: %q vs %q", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("expected 16 hex chars, got %d (%q)", len(a), a)
	}
}

func TestCwdPartitionName_Distinct(t *testing.T) {
	if CwdPartitionName("/alpha") == CwdPartitionName("/beta") {
		t.Fatal("different cwds should produce different partition names")
	}
}

func TestTasklistDirForCWD(t *testing.T) {
	// Empty cwd falls back to a flat tasks directory.
	if got, want := TasklistDirForCWD("/data", ""), filepath.Join("/data", "tasks"); got != want {
		t.Fatalf("empty cwd: got %q want %q", got, want)
	}
	// Non-empty cwd is partitioned by hash to avoid collisions.
	cwd := "/home/user/proj"
	want := filepath.Join("/data", "by-cwd", CwdPartitionName(cwd), "tasks")
	if got := TasklistDirForCWD("/data", cwd); got != want {
		t.Fatalf("non-empty cwd: got %q want %q", got, want)
	}
}

// fakeExt is a minimal agentcore.Extension for testing ExtSlice.
type fakeExt struct{ name string }

func (f *fakeExt) Name() string                                 { return f.name }
func (f *fakeExt) Init(context.Context, *agentcore.Agent) error { return nil }
func (f *fakeExt) Dispose() error                               { return nil }

func TestExtSlice(t *testing.T) {
	if got := ExtSlice(nil); got != nil {
		t.Fatalf("nil input should yield nil, got %v", got)
	}
	ext := &fakeExt{name: "x"}
	got := ExtSlice(ext)
	if len(got) != 1 || got[0].Name() != ext.Name() {
		t.Fatalf("expected single-element slice containing ext(name=%q), got %d elements", ext.Name(), len(got))
	}
}

func TestAgentThinking(t *testing.T) {
	if got := AgentThinking(nil); got != nil {
		t.Fatalf("nil input should yield nil, got %+v", got)
	}
	cfg := &agentconfig.ThinkingConfig{
		IncludeThoughts: true,
		Display:         "summarized",
		Effort:          "high",
		Budget:          4096,
	}
	got := AgentThinking(cfg)
	if got == nil {
		t.Fatal("expected non-nil output for non-nil input")
	}
	if !got.IncludeThoughts {
		t.Error("IncludeThoughts not mapped")
	}
	if got.Budget != 4096 {
		t.Errorf("Budget mismatch: got %d want 4096", got.Budget)
	}
}

func TestResolveMaxTokens(t *testing.T) {
	// 用户未配置时，默认应为 DefaultMaxTokens，确保长输出不被截断。
	if got := ResolveMaxTokens(nil); got != DefaultMaxTokens {
		t.Errorf("ResolveMaxTokens(nil) = %d, want %d", got, DefaultMaxTokens)
	}
	if got := ResolveMaxTokens(&agentconfig.Config{}); got != DefaultMaxTokens {
		t.Errorf("ResolveMaxTokens(empty) = %d, want %d", got, DefaultMaxTokens)
	}

	// 用户配置应被尊重。
	if got := ResolveMaxTokens(&agentconfig.Config{MaxTokens: 16384}); got != 16384 {
		t.Errorf("ResolveMaxTokens(16384) = %d, want 16384", got)
	}

	// 零值视为未配置，回退默认值。
	if got := ResolveMaxTokens(&agentconfig.Config{MaxTokens: 0}); got != DefaultMaxTokens {
		t.Errorf("ResolveMaxTokens(0) = %d, want %d", got, DefaultMaxTokens)
	}
}

func TestNewBaseConfig(t *testing.T) {
	// 未配置时 MaxTokens 应为默认值，且其他核心字段正确填充。
	cfg := NewBaseConfig("deepseek-v4-flash", nil, nil)
	if cfg.MaxTokens != DefaultMaxTokens {
		t.Errorf("MaxTokens default = %d, want %d", cfg.MaxTokens, DefaultMaxTokens)
	}
	if cfg.Model != "deepseek-v4-flash" {
		t.Errorf("Model = %q, want deepseek-v4-flash", cfg.Model)
	}
	if cfg.Name != "mady-router" {
		t.Errorf("Name = %q, want mady-router", cfg.Name)
	}
	if !cfg.Streaming {
		t.Error("Streaming should be true")
	}

	// 频率惩罚默认应为 0.2（缓解模型退化重复），重复惩罚默认不发送（0）。
	if cfg.FrequencyPenalty != DefaultFrequencyPenalty {
		t.Errorf("FrequencyPenalty default = %f, want %f", cfg.FrequencyPenalty, DefaultFrequencyPenalty)
	}
	if cfg.RepetitionPenalty != 0 {
		t.Errorf("RepetitionPenalty default = %f, want 0", cfg.RepetitionPenalty)
	}

	// 用户配置的 MaxTokens 应被正确覆盖。
	user := &agentconfig.Config{MaxTokens: 32768}
	cfg = NewBaseConfig("deepseek-v4-flash", nil, user)
	if cfg.MaxTokens != 32768 {
		t.Errorf("MaxTokens override = %d, want 32768", cfg.MaxTokens)
	}

	// 用户配置的频率/重复惩罚应被正确覆盖。
	user = &agentconfig.Config{
		FrequencyPenalty:  0.5,
		RepetitionPenalty: 1.1,
	}
	cfg = NewBaseConfig("deepseek-v4-flash", nil, user)
	if cfg.FrequencyPenalty != 0.5 {
		t.Errorf("FrequencyPenalty override = %f, want 0.5", cfg.FrequencyPenalty)
	}
	if cfg.RepetitionPenalty != 1.1 {
		t.Errorf("RepetitionPenalty override = %f, want 1.1", cfg.RepetitionPenalty)
	}

	// 显式 -1 关闭频率惩罚应被透传（chatcompat 仅 >0 发送，故等价于不发送）。
	user = &agentconfig.Config{FrequencyPenalty: -1}
	cfg = NewBaseConfig("deepseek-v4-flash", nil, user)
	if cfg.FrequencyPenalty != -1 {
		t.Errorf("FrequencyPenalty disable = %f, want -1", cfg.FrequencyPenalty)
	}
}
