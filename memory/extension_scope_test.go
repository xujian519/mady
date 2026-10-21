package memory

import (
	"context"
	"testing"
)

func TestMemoryExtension_ListSessionMemoriesForSummaryScoped(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	mgr := NewManager(store, nil, nil, DefaultManagerConfig())

	scopeA := MemoryScope{UserID: "u1", AgentID: "agent", SessionID: "session_a", ProjectID: "p1"}
	scopeB := MemoryScope{UserID: "u1", AgentID: "agent", SessionID: "session_b", ProjectID: "p1"}
	otherUser := MemoryScope{UserID: "u2", AgentID: "agent", SessionID: "session_a", ProjectID: "p1"}

	for _, tc := range []struct {
		content string
		scope   MemoryScope
		layer   MemoryLayer
	}{
		{"会话A的事实", scopeA, LayerSession},
		{"会话B的事实", scopeB, LayerSession},
		{"其他用户的会话事实", otherUser, LayerSession},
		{"会话A的长期事实", scopeA, LayerLongTerm},
	} {
		if _, err := store.Remember(ctx, tc.content, tc.scope, tc.layer, nil); err != nil {
			t.Fatalf("remember %q: %v", tc.content, err)
		}
	}

	ext := NewExtension(mgr, scopeA, DefaultExtensionConfig(), WithSharedManager())
	entries, err := ext.listSessionMemoriesForSummary(ctx)
	if err != nil {
		t.Fatalf("list session memories: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected only current scope's session memory, got %d", len(entries))
	}
	if entries[0].Content != "会话A的事实" {
		t.Errorf("unexpected memory content: %q", entries[0].Content)
	}
}
