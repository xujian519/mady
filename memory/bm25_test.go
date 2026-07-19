package memory

import (
	"testing"
)

func TestBM25Tokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		min   int // minimum expected tokens
	}{
		{"chinese", "专利创造性审查标准", 8},                // 4 single + 3 bigram >= 7
		{"english", "patent invalidity search", 3}, // 3 words
		{"mixed", "专利 patent 审查 review", 8},        // CJK singles + bigrams + 2 words
		{"numbers", "test123 abc456", 2},           // 2 alphanumeric words
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := bm25Tokenize(tt.input)
			if len(tokens) < tt.min {
				t.Errorf("got %d tokens, want >= %d: %v", len(tokens), tt.min, tokens)
			}
		})
	}
}

func TestBM25IndexAddSearch(t *testing.T) {
	idx := NewBM25Index(DefaultBM25Config())

	// Add documents
	idx.Add("doc1", "专利创造性审查标准与判断方法")
	idx.Add("doc2", "用户偏好使用表格展示数据分析结果")
	idx.Add("doc3", "审查意见答复的三步法策略：理解、反驳、修改")
	idx.Add("doc4", "商标侵权判断标准与混淆可能性分析")

	if idx.Size() != 4 {
		t.Fatalf("size = %d, want 4", idx.Size())
	}

	// Search for patent-related content
	results := idx.Search("专利审查", 3)
	if len(results) == 0 {
		t.Fatal("expected results for '专利审查'")
	}

	// doc1 (专利创造性审查) and doc3 (审查意见答复) should rank high
	foundDoc1 := false
	foundDoc3 := false
	for _, r := range results {
		if r.EntryID == "doc1" {
			foundDoc1 = true
		}
		if r.EntryID == "doc3" {
			foundDoc3 = true
		}
	}
	if !foundDoc1 {
		t.Error("doc1 should match '专利审查'")
	}
	if !foundDoc3 {
		t.Error("doc3 should match '专利审查'")
	}

	// Search for unrelated content
	results2 := idx.Search("商标混淆", 2)
	if len(results2) == 0 {
		t.Fatal("expected results for '商标混淆'")
	}
	if results2[0].EntryID != "doc4" {
		t.Errorf("top result = %s, want doc4", results2[0].EntryID)
	}
}

func TestBM25IndexRemove(t *testing.T) {
	idx := NewBM25Index(DefaultBM25Config())
	idx.Add("doc1", "专利审查标准")
	idx.Add("doc2", "商标侵权判断")

	if idx.Size() != 2 {
		t.Fatalf("size = %d, want 2", idx.Size())
	}

	idx.Remove("doc1")
	if idx.Size() != 1 {
		t.Fatalf("size after remove = %d, want 1", idx.Size())
	}

	results := idx.Search("专利", 5)
	if len(results) != 0 {
		t.Errorf("expected no results for '专利' after removing doc1, got %d", len(results))
	}
}

func TestBM25IndexRebuild(t *testing.T) {
	idx := NewBM25Index(DefaultBM25Config())
	idx.Add("old1", "完全不同的主题内容alpha")
	idx.Add("old2", "另一个旧数据条目beta")

	entries := []MemoryEntry{
		{ID: "new1", Content: "专利审查标准与判断方法"},
		{ID: "new2", Content: "商标侵权判断与混淆分析"},
	}
	idx.Rebuild(entries)

	if idx.Size() != 2 {
		t.Fatalf("size after rebuild = %d, want 2", idx.Size())
	}

	// Old data should be gone
	results := idx.Search("完全不同的旧数据", 5)
	if len(results) != 0 {
		t.Error("old data should be removed after rebuild")
	}

	// New data should be searchable
	results2 := idx.Search("专利审查", 5)
	if len(results2) == 0 {
		t.Error("new data should be searchable after rebuild")
	}
}

func TestBM25IndexEmptySearch(t *testing.T) {
	idx := NewBM25Index(DefaultBM25Config())
	results := idx.Search("anything", 5)
	if len(results) != 0 {
		t.Errorf("empty index should return no results, got %d", len(results))
	}
}
