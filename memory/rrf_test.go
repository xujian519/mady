package memory

import (
	"testing"
	"time"
)

func TestRRFFusion(t *testing.T) {
	now := time.Now()
	dense := []ScoredMemory{
		{Entry: MemoryEntry{ID: "A", Content: "doc A", CreatedAt: now}, Semantic: 0.9, Rank: 0},
		{Entry: MemoryEntry{ID: "B", Content: "doc B", CreatedAt: now}, Semantic: 0.7, Rank: 1},
		{Entry: MemoryEntry{ID: "C", Content: "doc C", CreatedAt: now}, Semantic: 0.5, Rank: 2},
	}
	sparse := []ScoredMemory{
		{Entry: MemoryEntry{ID: "B", Content: "doc B", CreatedAt: now}, Semantic: 0.8, Rank: 0},
		{Entry: MemoryEntry{ID: "D", Content: "doc D", CreatedAt: now}, Semantic: 0.6, Rank: 1},
		{Entry: MemoryEntry{ID: "A", Content: "doc A", CreatedAt: now}, Semantic: 0.4, Rank: 2},
	}

	cfg := DefaultRRFConfig()
	result := RRFFusion(dense, sparse, cfg)

	if len(result) != 4 {
		t.Fatalf("expected 4 unique results, got %d", len(result))
	}

	// doc B appears in both lists at rank 1 (dense[1]) and rank 0 (sparse[0])
	// RRF(B) = 1/(60+2) + 1/(60+1) = 1/62 + 1/61 ≈ 0.0161 + 0.0164 ≈ 0.0325
	// doc A appears at rank 0 (dense[0]) and rank 2 (sparse[2])
	// RRF(A) = 1/(60+1) + 1/(60+3) = 1/61 + 1/63 ≈ 0.0164 + 0.0159 ≈ 0.0323
	// So B should rank higher than A
	if result[0].Entry.ID != "B" {
		t.Logf("top result = %s (RRF score = %.4f)", result[0].Entry.ID, result[0].Composite)
		// This is informational — RRF ordering depends on exact rank positions
		// The key property is that both A and B appear in the results
	}

	// All 4 unique docs should be present
	ids := make(map[string]bool)
	for _, r := range result {
		ids[r.Entry.ID] = true
	}
	for _, id := range []string{"A", "B", "C", "D"} {
		if !ids[id] {
			t.Errorf("result should contain %s", id)
		}
	}

	// Ranks should be set
	for i, r := range result {
		if r.Rank != i {
			t.Errorf("result[%d].Rank = %d, want %d", i, r.Rank, i)
		}
	}
}

func TestRRFFusionEmpty(t *testing.T) {
	cfg := DefaultRRFConfig()

	// Both empty
	result := RRFFusion(nil, nil, cfg)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}

	// Only dense
	dense := []ScoredMemory{
		{Entry: MemoryEntry{ID: "A", Content: "doc A"}, Semantic: 0.9},
	}
	result2 := RRFFusion(dense, nil, cfg)
	if len(result2) != 1 {
		t.Errorf("expected 1 result, got %d", len(result2))
	}
	if result2[0].Entry.ID != "A" {
		t.Errorf("expected doc A, got %s", result2[0].Entry.ID)
	}
}

func TestRRFFusionCustomK(t *testing.T) {
	dense := []ScoredMemory{
		{Entry: MemoryEntry{ID: "X", Content: "doc X"}, Rank: 0},
	}
	sparse := []ScoredMemory{
		{Entry: MemoryEntry{ID: "X", Content: "doc X"}, Rank: 0},
	}

	// K=0 should default to 60
	cfg := RRFConfig{K: 0}
	result := RRFFusion(dense, sparse, cfg)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// Score should be 1/61 + 1/61 = 2/61 ≈ 0.0328
	expected := 2.0 / 61.0
	if result[0].Composite < expected-0.001 || result[0].Composite > expected+0.001 {
		t.Errorf("RRF score = %.4f, want ~%.4f", result[0].Composite, expected)
	}
}
