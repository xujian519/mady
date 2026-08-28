package session

import (
	"context"
	"encoding/json"
	"strings"
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

func TestAutoCompactor_TokenThreshold_Triggers(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	cfg := DefaultCompactionConfig()
	cfg.Enabled = true
	cfg.MaxMessages = 1000 // 消息数不触发，只看 token
	cfg.MaxTokens = 100
	cfg.KeepRecent = 2

	compactor := NewAutoCompactor(mgr, cfg)

	// 4 条 100 字符 ASCII 消息 ≈ 每条 29 tokens，共 ≈ 116 ≥ 100。
	longMsg := strings.Repeat("a", 100)
	for i := 0; i < 4; i++ {
		if err := mgr.AppendMessage(ctx, agentcore.Message{
			Role: agentcore.RoleUser, Content: longMsg,
		}); err != nil {
			t.Fatal(err)
		}
	}

	compacted, err := compactor.CheckAndCompact(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !compacted {
		t.Error("should compact when estimated tokens exceed MaxTokens")
	}
}

func TestAutoCompactor_TokenThreshold_Below(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	cfg := DefaultCompactionConfig()
	cfg.Enabled = true
	cfg.MaxMessages = 1000
	cfg.MaxTokens = 100000 // 远超估算值
	cfg.KeepRecent = 2

	compactor := NewAutoCompactor(mgr, cfg)

	longMsg := strings.Repeat("a", 100)
	for i := 0; i < 4; i++ {
		if err := mgr.AppendMessage(ctx, agentcore.Message{
			Role: agentcore.RoleUser, Content: longMsg,
		}); err != nil {
			t.Fatal(err)
		}
	}

	compacted, err := compactor.CheckAndCompact(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compacted {
		t.Error("should not compact below token threshold")
	}
}

func TestAutoCompactor_TokenThreshold_NothingToCompact(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	cfg := DefaultCompactionConfig()
	cfg.Enabled = true
	cfg.MaxMessages = 1000
	cfg.MaxTokens = 10 // 极低阈值，估算 tokens 必然超过
	cfg.KeepRecent = 10

	compactor := NewAutoCompactor(mgr, cfg)

	// 只有 4 条消息（<= KeepRecent=10），没有可压缩的旧消息：
	// 即使 token 超阈也不应空转压缩（防抖）。
	longMsg := strings.Repeat("a", 100)
	for i := 0; i < 4; i++ {
		if err := mgr.AppendMessage(ctx, agentcore.Message{
			Role: agentcore.RoleUser, Content: longMsg,
		}); err != nil {
			t.Fatal(err)
		}
	}

	compacted, err := compactor.CheckAndCompact(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compacted {
		t.Error("should not compact when nothing older than KeepRecent exists")
	}
}
