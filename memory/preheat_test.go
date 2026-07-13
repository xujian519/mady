package memory

import (
	"context"
	"testing"
)

func TestPreheatCaseMemory(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	scope := MemoryScope{SessionID: "sess-1", ProjectID: "proj-1"}

	summary := "案件记忆 | 案件编号: case-001 | 案件类型: patentability | 技术领域: G06F | 当前阶段: 3 | 已记录事实: 5条"

	id, err := PreheatCaseMemory(ctx, store, scope, "case-001", summary)
	if err != nil {
		t.Fatalf("PreheatCaseMemory failed: %v", err)
	}
	if id == "" {
		t.Fatal("returned empty ID")
	}

	entry, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if entry.Layer != LayerLongTerm {
		t.Errorf("Layer = %s, want %s", entry.Layer, LayerLongTerm)
	}
	if entry.Content != summary {
		t.Errorf("Content mismatch")
	}
	if entry.Metadata["type"] != "case_preheat" {
		t.Errorf("metadata type = %v, want case_preheat", entry.Metadata["type"])
	}
	if entry.Metadata["case_id"] != "case-001" {
		t.Errorf("metadata case_id = %v, want case-001", entry.Metadata["case_id"])
	}
}

func TestPreheatCaseMemory_EmptySummary(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	scope := MemoryScope{SessionID: "sess-1"}

	_, err := PreheatCaseMemory(ctx, store, scope, "case-x", "")
	if err == nil {
		t.Fatal("expected error for empty summary")
	}
}

func TestPreheatCaseMemory_Recallable(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	scope := MemoryScope{SessionID: "sess-1", ProjectID: "proj-1"}

	summary := "案件记忆 | 案件编号: case-99 | 案件类型: novelty | 技术领域: H04L | 当前阶段: 1 | 已记录事实: 3条"
	_, err := PreheatCaseMemory(ctx, store, scope, "case-99", summary)
	if err != nil {
		t.Fatalf("PreheatCaseMemory failed: %v", err)
	}

	results, err := store.Recall(ctx, "case-99", MemoryFilter{
		SessionID: "sess-1",
		Layer:     LayerLongTerm,
		TopK:      5,
	})
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result from Recall")
	}
}
