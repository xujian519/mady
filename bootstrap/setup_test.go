package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/agentconfig"
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
