package retrieval

import (
	"testing"
)

func TestRRFFuse_BasicFusion(t *testing.T) {
	chunk1 := Chunk{ID: "a", Content: "alpha"}
	chunk2 := Chunk{ID: "b", Content: "beta"}
	chunk3 := Chunk{ID: "c", Content: "gamma"}

	list1 := []ScoredChunk{
		{Chunk: chunk1, Score: 0.9},
		{Chunk: chunk2, Score: 0.7},
		{Chunk: chunk3, Score: 0.5},
	}
	list2 := []ScoredChunk{
		{Chunk: chunk2, Score: 0.8},
		{Chunk: chunk1, Score: 0.6},
		{Chunk: chunk3, Score: 0.4},
	}

	fuser := NewRRFFuser()
	results := fuser.Fuse([][]ScoredChunk{list1, list2}, 3)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// chunk1: 1/(60+1) + 1/(60+2) = 0.01639 + 0.01613 = 0.03252
	// chunk2: 1/(60+2) + 1/(60+1) = 0.01613 + 0.01639 = 0.03252
	// chunk3: 1/(60+3) + 1/(60+3) = 0.01587 + 0.01587 = 0.03175
	// chunk1 and chunk2 tie at top, chunk3 last
	if results[0].ID != "a" && results[0].ID != "b" {
		t.Errorf("expected 'a' or 'b' first, got %s", results[0].ID)
	}
	if results[len(results)-1].ID != "c" {
		t.Errorf("expected 'c' last, got %s", results[len(results)-1].ID)
	}
}

func TestRRFFuse_TopK(t *testing.T) {
	list := []ScoredChunk{
		{Chunk: Chunk{ID: "a"}, Score: 0.9},
		{Chunk: Chunk{ID: "b"}, Score: 0.7},
		{Chunk: Chunk{ID: "c"}, Score: 0.5},
		{Chunk: Chunk{ID: "d"}, Score: 0.3},
	}

	fuser := NewRRFFuser()
	results := fuser.Fuse([][]ScoredChunk{list}, 2)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "a" {
		t.Errorf("expected 'a' first, got %s", results[0].ID)
	}
}

func TestRRFFuse_UniqueFromOneList(t *testing.T) {
	list1 := []ScoredChunk{
		{Chunk: Chunk{ID: "a"}, Score: 0.9},
		{Chunk: Chunk{ID: "b"}, Score: 0.7},
	}
	list2 := []ScoredChunk{
		{Chunk: Chunk{ID: "c"}, Score: 0.8},
		{Chunk: Chunk{ID: "d"}, Score: 0.6},
	}

	fuser := NewRRFFuser()
	results := fuser.Fuse([][]ScoredChunk{list1, list2}, 4)
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
}

func TestRRFFuse_EmptyInput(t *testing.T) {
	fuser := NewRRFFuser()
	if results := fuser.Fuse(nil, 5); len(results) != 0 {
		t.Errorf("expected 0 results for nil input, got %d", len(results))
	}
	if results := fuser.Fuse([][]ScoredChunk{{}}, 5); len(results) != 0 {
		t.Errorf("expected 0 results for empty list, got %d", len(results))
	}
}
