package domain

import (
	"testing"

	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/retrieval"
)

func TestDomainDocument_Fields(t *testing.T) {
	doc := DomainDocument{
		ID:       "cn-20240001",
		Title:    "一种测试方法",
		Snippet:  "本专利涉及...",
		Content:  "完整的权利要求文本",
		URL:      "https://patents.example.com/cn-20240001",
		Metadata: map[string]string{"ipc": "G06F17/30", "applicant": "测试公司"},
		Score:    0.85,
	}
	if doc.ID != "cn-20240001" {
		t.Errorf("ID = %q", doc.ID)
	}
	if doc.Metadata["ipc"] != "G06F17/30" {
		t.Errorf("ipc = %q", doc.Metadata["ipc"])
	}
}

func TestDomainQuery_Filters(t *testing.T) {
	q := DomainQuery{
		Text:     "深度学习图像识别",
		Keywords: []string{"深度学习", "图像识别", "CNN"},
		Filters:  map[string]string{"ipc": "G06F17/30", "applicant": "华为"},
		MaxResults: 20,
	}
	if q.MaxResults != 20 {
		t.Errorf("MaxResults = %d", q.MaxResults)
	}
	if len(q.Keywords) != 3 {
		t.Errorf("Keywords = %d, want 3", len(q.Keywords))
	}
	if q.Filters["applicant"] != "华为" {
		t.Errorf("Filters[applicant] = %q", q.Filters["applicant"])
	}
}

func TestDomainReranker_BoostsMatchingMetadata(t *testing.T) {
	reranker := &DomainReranker{
		MetadataKey:     "ipc",
		PreferredValues: []string{"G06F17/30", "G06N3/08"},
		Boost:           2.0,
	}

	results := []retrieval.ScoredChunk{
		{
			Chunk: retrieval.Chunk{
				ID:      "c1",
				DocID:   "d1",
				Content: "匹配IPC的区块",
				Metadata: map[string]string{"ipc": "G06F17/30"},
			},
			Score: 0.5,
		},
		{
			Chunk: retrieval.Chunk{
				ID:      "c2",
				DocID:   "d2",
				Content: "不匹配的区块",
				Metadata: map[string]string{"ipc": "A61K31/00"},
			},
			Score: 0.5,
		},
		{
			Chunk: retrieval.Chunk{
				ID:      "c3",
				DocID:   "d3",
				Content: "无IPC元数据",
			},
			Score: 0.5,
		},
	}

	reranked := reranker.Rerank(results)

	if len(reranked) != 3 {
		t.Fatalf("reranked count = %d, want 3", len(reranked))
	}

	// c1 should be boosted (matches preferred IPC).
	if reranked[0].ID != "c1" {
		t.Errorf("c1 should rank first after boost, got %s (score %f)", reranked[0].ID, reranked[0].Score)
	}
	expected := 0.5 * 2.0
	if reranked[0].Score < expected-0.001 || reranked[0].Score > expected+0.001 {
		t.Errorf("c1 score = %f, want ~%f (0.5 * 2.0)", reranked[0].Score, expected)
	}
	// c2 and c3 should keep original score.
	if reranked[1].Score != 0.5 || reranked[2].Score != 0.5 {
		t.Errorf("unboosted scores should remain 0.5: got %f, %f", reranked[1].Score, reranked[2].Score)
	}
}

func TestDomainReranker_DefaultBoost(t *testing.T) {
	reranker := &DomainReranker{
		MetadataKey:     "type",
		PreferredValues: []string{"judgment"},
	}
	results := []retrieval.ScoredChunk{
		{
			Chunk: retrieval.Chunk{
				ID:       "c1",
				Content:  "判决书内容",
				Metadata: map[string]string{"type": "judgment"},
			},
			Score: 0.4,
		},
	}
	reranked := reranker.Rerank(results)
	// Default boost is 1.5.
	expected := 0.4 * 1.5
	if reranked[0].Score < expected-0.001 || reranked[0].Score > expected+0.001 {
		t.Errorf("default boost: score = %f, want ~%f (0.4 * 1.5)", reranked[0].Score, expected)
	}
}

func TestImportToStore_AddsDocuments(t *testing.T) {
	store := knowledge.NewStore()
	results := &DomainResults{
		Source: "test-source",
		Documents: []DomainDocument{
			{
				ID:      "doc-1",
				Title:   "测试文档",
				Content: "这是测试文档的完整内容，包含足够多的文本用于分块测试。",
				URL:     "https://example.com/doc-1",
				Score:   0.9,
			},
		},
	}

	count, err := ImportToStore(store, results, "patent")
	if err != nil {
		t.Fatalf("ImportToStore: %v", err)
	}
	if count != 1 {
		t.Errorf("imported = %d, want 1", count)
	}

	doc, ok := store.GetDocument("doc-1")
	if !ok {
		t.Fatal("document not found after import")
	}
	if doc.Domain != "patent" {
		t.Errorf("domain = %q, want %q", doc.Domain, "patent")
	}
	if doc.Source != "https://example.com/doc-1" {
		t.Errorf("source = %q", doc.Source)
	}
}

func TestImportToStore_SkipsDuplicates(t *testing.T) {
	store := knowledge.NewStore()
	results := &DomainResults{
		Documents: []DomainDocument{
			{ID: "doc-1", Title: "First", Content: "Content 1"},
		},
	}

	// First import.
	count, _ := ImportToStore(store, results, "patent")
	if count != 1 {
		t.Errorf("first import = %d, want 1", count)
	}

	// Second import with same ID should skip.
	count, _ = ImportToStore(store, results, "patent")
	if count != 0 {
		t.Errorf("second import = %d, want 0 (should skip duplicate)", count)
	}
}

func TestImportToStore_UsesSnippetFallback(t *testing.T) {
	store := knowledge.NewStore()
	results := &DomainResults{
		Documents: []DomainDocument{
			{
				ID:      "snippet-only",
				Title:   "仅摘要",
				Snippet: "这是文档的摘要内容",
				Content: "", // empty content
			},
		},
	}

	count, err := ImportToStore(store, results, "legal")
	if err != nil {
		t.Fatalf("ImportToStore: %v", err)
	}
	if count != 1 {
		t.Errorf("imported = %d, want 1", count)
	}

	doc, ok := store.GetDocument("snippet-only")
	if !ok {
		t.Fatal("document not found")
	}
	if doc.Content != "这是文档的摘要内容" {
		t.Errorf("content = %q, want snippet fallback", doc.Content)
	}
}

func TestImportToStore_NilResults(t *testing.T) {
	count, err := ImportToStore(nil, nil, "patent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}
