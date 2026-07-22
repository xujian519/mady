package retrieval

import (
	"testing"
)

// ---------------------------------------------------------------------------
// PositionReranker tests
// ---------------------------------------------------------------------------

func TestPositionReranker_Empty(t *testing.T) {
	pr := NewPositionReranker()
	got := pr.Rerank(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestPositionReranker_BoostsEarlyChunks(t *testing.T) {
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "c0", Position: 0}, Score: 1.0},
		{Chunk: Chunk{ID: "c1", Position: 5}, Score: 1.0},
		{Chunk: Chunk{ID: "c2", Position: 10}, Score: 1.0},
		{Chunk: Chunk{ID: "c3", Position: 20}, Score: 1.0},
	}

	pr := NewPositionReranker()
	got := pr.Rerank(results)

	if len(got) != 4 {
		t.Fatalf("expected 4 results, got %d", len(got))
	}

	if got[0].ID != "c0" {
		t.Errorf("expected c0 first, got %s", got[0].ID)
	}
	if got[0].Score != 1.3 {
		t.Errorf("c0 expected score 1.3, got %f", got[0].Score)
	}
	if got[3].ID != "c3" {
		t.Errorf("expected c3 last, got %s", got[3].ID)
	}
	if got[3].Score != 1.0 {
		t.Errorf("c3 expected score 1.0, got %f", got[3].Score)
	}
}

func TestPositionReranker_ZeroWeight(t *testing.T) {
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "c0", Position: 0}, Score: 2.0},
		{Chunk: Chunk{ID: "c1", Position: 1}, Score: 1.0},
	}

	pr := &PositionReranker{PositionWeight: 0}
	got := pr.Rerank(results)

	if got[0].ID != "c0" || got[0].Score != 2.0 {
		t.Errorf("expected c0 score 2.0 unchanged, got %s score %f", got[0].ID, got[0].Score)
	}
}

func TestPositionReranker_ReordersByBoostedScore(t *testing.T) {
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "late-high", Position: 15}, Score: 10.0},
		{Chunk: Chunk{ID: "early-low", Position: 0}, Score: 9.0},
	}

	pr := NewPositionReranker()
	got := pr.Rerank(results)

	if got[0].ID != "early-low" {
		t.Errorf("expected early-low first, got %s", got[0].ID)
	}
}

// ---------------------------------------------------------------------------
// DeduplicatingReranker tests
// ---------------------------------------------------------------------------

func TestDeduplicatingReranker_Empty(t *testing.T) {
	dr := NewDeduplicatingReranker()
	got := dr.Rerank(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestDeduplicatingReranker_Single(t *testing.T) {
	dr := NewDeduplicatingReranker()
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "c0", Content: "unique"}, Score: 1.0},
	}
	got := dr.Rerank(results)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
}

func TestDeduplicatingReranker_RemovesDuplicates(t *testing.T) {
	dr := NewDeduplicatingReranker()
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "c0", Content: "这是一段重复内容"}, Score: 1.0},
		{Chunk: Chunk{ID: "c1", Content: "这是一段重复内容"}, Score: 0.9},
		{Chunk: Chunk{ID: "c2", Content: "独特内容"}, Score: 0.8},
	}

	got := dr.Rerank(results)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique results, got %d", len(got))
	}
	if got[0].ID != "c0" {
		t.Errorf("expected c0 (higher score) kept, got %s", got[0].ID)
	}
}

func TestDeduplicatingReranker_LongContentSignature(t *testing.T) {
	dr := NewDeduplicatingReranker()
	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "a"
	}
	dupContent := ""
	for i := 0; i < 200; i++ {
		dupContent += "a"
	}

	results := []ScoredChunk{
		{Chunk: Chunk{ID: "c0", Content: longContent}, Score: 1.0},
		{Chunk: Chunk{ID: "c1", Content: dupContent}, Score: 0.9},
	}
	got := dr.Rerank(results)
	if len(got) != 1 {
		t.Fatalf("expected 1 after dedup (same prefix), got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// ChainReranker tests
// ---------------------------------------------------------------------------

func TestChainReranker_Empty(t *testing.T) {
	cr := &ChainReranker{Rerankers: nil}
	got := cr.Rerank(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestChainReranker_EmptyChain(t *testing.T) {
	cr := &ChainReranker{Rerankers: []Reranker{}}
	results := []ScoredChunk{{Chunk: Chunk{ID: "c0", Position: 0}, Score: 1.0}}
	got := cr.Rerank(results)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
}

func TestChainReranker_AppliesInOrder(t *testing.T) {
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "c0", Position: 0, Content: "unique a"}, Score: 1.0},
		{Chunk: Chunk{ID: "c1", Position: 5, Content: "unique a"}, Score: 0.9},
		{Chunk: Chunk{ID: "c2", Position: 10, Content: "unique b"}, Score: 1.0},
	}

	cr := &ChainReranker{
		Rerankers: []Reranker{
			NewDeduplicatingReranker(),
			NewPositionReranker(),
		},
	}
	got := cr.Rerank(results)

	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].ID != "c0" {
		t.Errorf("expected c0 first, got %s", got[0].ID)
	}
	if got[1].ID != "c2" {
		t.Errorf("expected c2 second, got %s", got[1].ID)
	}
}

// ---------------------------------------------------------------------------
// LegalReranker tests
// ---------------------------------------------------------------------------

func TestLegalReranker_Empty(t *testing.T) {
	lr := NewLegalReranker()
	got := lr.Rerank(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestLegalReranker_BoostsAuthority(t *testing.T) {
	lr := NewLegalReranker()
	results := []ScoredChunk{
		{
			Chunk: Chunk{
				ID:       "constitution",
				Metadata: map[string]string{"law_source": "宪法"},
			},
			Score: 1.0,
		},
		{
			Chunk: Chunk{
				ID:       "regulation",
				Metadata: map[string]string{"law_source": "部门规章"},
			},
			Score: 1.0,
		},
		{
			Chunk: Chunk{
				ID:       "case",
				Metadata: map[string]string{"law_source": "指导性案例"},
			},
			Score: 1.0,
		},
	}

	got := lr.Rerank(results)
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}

	if got[0].ID != "constitution" {
		t.Errorf("expected constitution first, got %s", got[0].ID)
	}
	if got[0].Score <= 1.0 {
		t.Errorf("constitution expected boost > 1.0, got %f", got[0].Score)
	}
}

func TestLegalReranker_CustomHierarchy(t *testing.T) {
	lr := &LegalReranker{
		Hierarchy:    map[string]int{"政策文件": 50, "通知": 30},
		BoostPerRank: 0.1,
		MetadataKey:  "law_source",
	}
	results := []ScoredChunk{
		{
			Chunk: Chunk{
				ID:       "policy",
				Metadata: map[string]string{"law_source": "政策文件"},
			},
			Score: 1.0,
		},
		{
			Chunk: Chunk{
				ID:       "notice",
				Metadata: map[string]string{"law_source": "通知"},
			},
			Score: 1.0,
		},
	}

	got := lr.Rerank(results)
	if got[0].ID != "policy" {
		t.Errorf("expected policy first, got %s", got[0].ID)
	}
}

func TestLegalReranker_ReordersByBoostedScore(t *testing.T) {
	lr := NewLegalReranker()
	// Input is in REVERSE order of authority: lowest authority first.
	results := []ScoredChunk{
		{
			Chunk: Chunk{
				ID:       "case",
				Metadata: map[string]string{"law_source": "指导性案例"},
			},
			Score: 1.0,
		},
		{
			Chunk: Chunk{
				ID:       "constitution",
				Metadata: map[string]string{"law_source": "宪法"},
			},
			Score: 1.0,
		},
	}

	got := lr.Rerank(results)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	// After boost, constitution (rank 100) should be first, case (rank 40) last.
	if got[0].ID != "constitution" {
		t.Errorf("expected constitution first after reorder, got %s", got[0].ID)
	}
	if got[1].ID != "case" {
		t.Errorf("expected case last after reorder, got %s", got[1].ID)
	}
}

func TestLegalReranker_UnknownSource(t *testing.T) {
	lr := NewLegalReranker()
	results := []ScoredChunk{
		{
			Chunk: Chunk{
				ID:       "unknown",
				Metadata: map[string]string{"law_source": "公司规章"},
			},
			Score: 1.0,
		},
	}
	got := lr.Rerank(results)
	if got[0].Score != 1.0 {
		t.Errorf("expected score 1.0 unchanged, got %f", got[0].Score)
	}
}

// ---------------------------------------------------------------------------
// PatentReranker tests
// ---------------------------------------------------------------------------

func TestPatentReranker_Empty(t *testing.T) {
	pr := NewPatentReranker()
	got := pr.Rerank(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestPatentReranker_BoostsAuthority(t *testing.T) {
	pr := NewPatentReranker()
	results := []ScoredChunk{
		{
			Chunk: Chunk{
				ID:       "guideline",
				Metadata: map[string]string{"doc_type": "审查指南"},
			},
			Score: 1.0,
		},
		{
			Chunk: Chunk{
				ID:       "wiki",
				Metadata: map[string]string{"doc_type": "wiki"},
			},
			Score: 1.0,
		},
		{
			Chunk: Chunk{
				ID:       "case",
				Metadata: map[string]string{"doc_type": "判例"},
			},
			Score: 1.0,
		},
	}

	got := pr.Rerank(results)
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}

	if got[0].ID != "guideline" {
		t.Errorf("expected guideline first, got %s", got[0].ID)
	}
	if got[2].ID != "wiki" {
		t.Errorf("expected wiki last, got %s", got[2].ID)
	}
}

func TestPatentReranker_SuppressFutureDate(t *testing.T) {
	pr := &PatentReranker{
		DocTypeRank:           DefaultPatentDocTypeRank(),
		DocTypeKey:            "doc_type",
		BoostPerRank:          0.2,
		ApplicationDate:       "2024-06-01",
		SuppressFutureDateKey: "publish_date",
		FutureDatePenalty:     0.1,
	}

	results := []ScoredChunk{
		{
			Chunk: Chunk{
				ID:       "before",
				Metadata: map[string]string{"doc_type": "判例", "publish_date": "2023-01-01"},
			},
			Score: 1.0,
		},
		{
			Chunk: Chunk{
				ID:       "after",
				Metadata: map[string]string{"doc_type": "判例", "publish_date": "2025-01-01"},
			},
			Score: 1.0,
		},
	}

	got := pr.Rerank(results)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}

	if got[0].ID != "before" {
		t.Errorf("expected before (not suppressed) first, got %s", got[0].ID)
	}
	if got[1].ID != "after" {
		t.Errorf("expected after (suppressed) last, got %s", got[1].ID)
	}
	if got[1].Score != 0.1 {
		t.Errorf("after expected suppressed score 0.1, got %f", got[1].Score)
	}
}

func TestPatentReranker_CustomDocTypeRank(t *testing.T) {
	pr := &PatentReranker{
		DocTypeRank:  map[string]int{"重要文献": 80, "参考": 50},
		DocTypeKey:   "doc_type",
		BoostPerRank: 0.3,
	}
	results := []ScoredChunk{
		{
			Chunk: Chunk{
				ID:       "important",
				Metadata: map[string]string{"doc_type": "重要文献"},
			},
			Score: 1.0,
		},
		{
			Chunk: Chunk{
				ID:       "ref",
				Metadata: map[string]string{"doc_type": "参考"},
			},
			Score: 1.0,
		},
	}

	got := pr.Rerank(results)
	if got[0].ID != "important" {
		t.Errorf("expected important first, got %s", got[0].ID)
	}
	if got[0].Score < 1.05 {
		t.Errorf("important expected boosted score, got %f", got[0].Score)
	}
}

func TestPatentReranker_UnknownDocType(t *testing.T) {
	pr := NewPatentReranker()
	results := []ScoredChunk{
		{
			Chunk: Chunk{
				ID:       "unknown",
				Metadata: map[string]string{"doc_type": "未分类"},
			},
			Score: 1.0,
		},
	}
	got := pr.Rerank(results)
	if got[0].Score != 1.0 {
		t.Errorf("expected score 1.0 unchanged, got %f", got[0].Score)
	}
}
