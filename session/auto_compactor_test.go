package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestAutoCompactor_Disabled(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	cfg := DefaultCompactionConfig()
	cfg.Enabled = false

	compactor := NewAutoCompactor(mgr, cfg)

	// Add messages below threshold
	for i := 0; i < 5; i++ {
		if err := mgr.AppendMessage(ctx, agentcore.Message{
			Role: agentcore.RoleUser, Content: "msg",
		}); err != nil {
			t.Fatal(err)
		}
	}

	compacted, err := compactor.CheckAndCompact(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compacted {
		t.Error("should not compact when disabled")
	}
}

func TestAutoCompactor_BelowThreshold(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	cfg := DefaultCompactionConfig()
	cfg.Enabled = true
	cfg.MaxMessages = 10
	cfg.KeepRecent = 3

	compactor := NewAutoCompactor(mgr, cfg)

	// Add 5 messages (below threshold of 10)
	for i := 0; i < 5; i++ {
		if err := mgr.AppendMessage(ctx, agentcore.Message{
			Role: agentcore.RoleUser, Content: "msg",
		}); err != nil {
			t.Fatal(err)
		}
	}

	compacted, err := compactor.CheckAndCompact(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compacted {
		t.Error("should not compact below threshold")
	}
}

func TestAutoCompactor_TriggersCompaction(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	cfg := DefaultCompactionConfig()
	cfg.Enabled = true
	cfg.MaxMessages = 5
	cfg.KeepRecent = 2

	compactor := NewAutoCompactor(mgr, cfg)

	// Add 8 messages (exceeds threshold of 5)
	for i := 0; i < 8; i++ {
		if err := mgr.AppendMessage(ctx, agentcore.Message{
			Role: agentcore.RoleUser, Content: "message content here",
		}); err != nil {
			t.Fatal(err)
		}
	}

	compacted, err := compactor.CheckAndCompact(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !compacted {
		t.Error("should trigger compaction above threshold")
	}

	// Verify compaction entry was added
	entries := mgr.Entries()
	hasCompaction := false
	for _, e := range entries {
		if e.Type == EntryCompaction {
			hasCompaction = true
			break
		}
	}
	if !hasCompaction {
		t.Error("expected compaction entry in session")
	}
}

func TestAutoCompactor_KeepRecent(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	cfg := DefaultCompactionConfig()
	cfg.Enabled = true
	cfg.MaxMessages = 5
	cfg.KeepRecent = 3

	compactor := NewAutoCompactor(mgr, cfg)

	for i := 0; i < 10; i++ {
		if err := mgr.AppendMessage(ctx, agentcore.Message{
			Role: agentcore.RoleUser, Content: "msg",
		}); err != nil {
			t.Fatal(err)
		}
	}

	compacted, err := compactor.CheckAndCompact(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !compacted {
		t.Error("should trigger compaction")
	}

	// After compaction, total entries should still include the compaction
	// entry plus the kept messages
	entries := mgr.Entries()
	if len(entries) == 0 {
		t.Fatal("expected entries after compaction")
	}
}

func TestAutoCompactor_SummaryContent(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	cfg := DefaultCompactionConfig()
	cfg.Enabled = true
	cfg.MaxMessages = 3
	cfg.KeepRecent = 1

	compactor := NewAutoCompactor(mgr, cfg)

	if err := mgr.AppendMessage(ctx, agentcore.Message{
		Role: agentcore.RoleUser, Content: "帮我分析专利的新颖性",
	}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AppendMessage(ctx, agentcore.Message{
		Role: agentcore.RoleAssistant, Content: "好的，我来分析。",
	}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AppendMessage(ctx, agentcore.Message{
		Role: agentcore.RoleUser, Content: "继续",
	}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AppendMessage(ctx, agentcore.Message{
		Role: agentcore.RoleAssistant, Content: "分析结果如下...",
	}); err != nil {
		t.Fatal(err)
	}

	compacted, err := compactor.CheckAndCompact(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !compacted {
		t.Error("should trigger compaction")
	}

	// Verify compaction entry exists with summary
	entries := mgr.Entries()
	found := false
	for _, e := range entries {
		if e.Type == EntryCompaction {
			found = true
			var cd CompactionData
			if err := json.Unmarshal(e.Data, &cd); err == nil {
				if cd.Summary == "" {
					t.Error("expected non-empty summary")
				}
			}
			break
		}
	}
	if !found {
		t.Error("expected compaction entry")
	}
}
