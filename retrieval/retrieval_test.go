package retrieval

import (
	"context"
	"testing"
)

func TestChunkDocument(t *testing.T) {
	text := `## 权利要求 1
一种基于中观哲学的多领域智能调度方法，其特征在于，包括以下步骤：
步骤一，发现事实，收集用户输入和相关文档；
步骤二，获取规则，检索相关法律法规和审查指南；
步骤三，规划，基于事实和规则制定行动方案；
步骤四，执行，逐步执行计划，调用工具生成文书；
步骤五，检查，验证执行结果并纠正偏差。

## 权利要求 2
根据权利要求1所述的方法，其特征在于，所述步骤二中还包括通过语义检索和关键词匹配相结合的混合检索方式。

## 背景技术
现有的智能调度系统多采用单一领域模型，无法有效处理跨领域的专业任务。特别是在专利和法律领域，知识边界和风险等级差异显著。`

	opts := ChunkOptions{MaxChars: 300, OverlapChars: 50, SplitBySection: true}
	chunks := ChunkDocument("test-doc", text, opts)

	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks (3 sections), got %d", len(chunks))
	}

	// Verify chunk positions are sequential.
	for i, c := range chunks {
		if c.Position != i {
			t.Errorf("chunk %d: expected Position %d, got %d", i, i, c.Position)
		}
		if c.DocID != "test-doc" {
			t.Errorf("chunk %d: expected DocID 'test-doc', got %q", i, c.DocID)
		}
		if c.ID == "" {
			t.Errorf("chunk %d: ID is empty", i)
		}
	}
}

func TestKeywordSearcher(t *testing.T) {
	chunks := []Chunk{
		{ID: "c0", DocID: "patent-1", Content: "专利检索方法包括关键词检索和语义检索。关键词检索基于IPC分类号和申请人信息。", Position: 0},
		{ID: "c1", DocID: "patent-1", Content: "权利要求分析需要逐项比对现有技术，判断新颖性和创造性。这需要专利代理人具备专业知识。", Position: 1},
		{ID: "c2", DocID: "patent-2", Content: "商标检索关注近似性和类别区分。与专利检索有本质不同，主要依据尼斯分类。", Position: 0},
	}

	searcher := NewKeywordSearcher()

	// Test patent search.
	results := searcher.Search(context.Background(), "专利检索 关键词 IPC", chunks, 3)
	if len(results) == 0 {
		t.Fatal("expected results for patent search")
	}
	if results[0].ID != "c0" {
		t.Errorf("expected c0 as top result, got %s", results[0].ID)
	}

	// Test trademark search (should find trademark chunk, not patent).
	results2 := searcher.Search(context.Background(), "商标 近似性", chunks, 3)
	if len(results2) == 0 {
		t.Fatal("expected results for trademark search")
	}
	if results2[0].ID != "c2" {
		t.Errorf("expected c2 as top result, got %s", results2[0].ID)
	}

	// Test empty query.
	results3 := searcher.Search(context.Background(), "", chunks, 3)
	if len(results3) != 0 {
		t.Errorf("expected no results for empty query, got %d", len(results3))
	}
}

func TestPositionReranker(t *testing.T) {
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "c0", Position: 0}, Score: 0.8},
		{Chunk: Chunk{ID: "c5", Position: 5}, Score: 0.9}, // higher raw score but later position
		{Chunk: Chunk{ID: "c3", Position: 3}, Score: 0.7},
	}

	reranker := NewPositionReranker()
	reranked := reranker.Rerank(results)

	// c0 (pos 0, score 0.8) with position boost should outrank c5 (pos 5, score 0.9).
	// c0 boosted: 0.8 * (1.0 + 0.3 * 1.0) = 0.8 * 1.3 = 1.04
	// c5 boosted: 0.9 * (1.0 + 0.3 * 0.5) = 0.9 * 1.15 = 1.035
	if reranked[0].ID != "c0" {
		t.Errorf("expected c0 (boosted by position) to be top result, got %s", reranked[0].ID)
	}
}

func TestChunkDocument_Empty(t *testing.T) {
	chunks := ChunkDocument("empty", "", DefaultChunkOptions())
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty document, got %d", len(chunks))
	}
}

func TestChunkDocument_SmallText(t *testing.T) {
	text := "这是一个很短的文本片段。"
	chunks := ChunkDocument("small", text, DefaultChunkOptions())
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for small text, got %d", len(chunks))
	}
	if chunks[0].Content != text {
		t.Errorf("chunk content mismatch: got %q, want %q", chunks[0].Content, text)
	}
}

func TestExtractTerms(t *testing.T) {
	tests := []struct {
		query    string
		minTerms int
	}{
		{"专利检索方法", 1},
		{"IPC 分类号 检索", 3},
		{"hello world search", 3},
		{"a b c d", 0}, // single chars filtered
	}

	for _, tt := range tests {
		terms := extractTerms(tt.query)
		if len(terms) < tt.minTerms {
			t.Errorf("extractTerms(%q): got %d terms, want >= %d: %v", tt.query, len(terms), tt.minTerms, terms)
		}
	}
}

func TestLegalReranker_BoostsHigherRank(t *testing.T) {
	reranker := NewLegalReranker()
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "law1", Content: "宪法", Metadata: map[string]string{"law_source": "宪法"}}, Score: 0.5},
		{Chunk: Chunk{ID: "guide1", Content: "指导案例", Metadata: map[string]string{"law_source": "指导性案例"}}, Score: 0.5},
	}
	reranked := reranker.Rerank(results)
	if len(reranked) != 2 {
		t.Fatalf("expected 2 results, got %d", len(reranked))
	}
	if reranked[0].Score <= reranked[1].Score {
		t.Errorf("宪法 should rank higher: %f vs %f", reranked[0].Score, reranked[1].Score)
	}
}

func TestLegalReranker_EmptyResults(t *testing.T) {
	reranker := NewLegalReranker()
	if result := reranker.Rerank(nil); result != nil {
		t.Errorf("expected nil for empty results, got %v", result)
	}
}

func TestDefaultLegalHierarchy(t *testing.T) {
	h := DefaultLegalHierarchy()
	if h["宪法"] <= h["指导性案例"] {
		t.Error("宪法 should rank higher than 指导性案例")
	}
	if h["法律"] <= h["行政法规"] {
		t.Error("法律 should rank higher than 行政法规")
	}
}

func TestPatentReranker_BoostsGuidelinesOverLiterature(t *testing.T) {
	reranker := NewPatentReranker()
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "lit", Metadata: map[string]string{"doc_type": "技术文献"}}, Score: 0.6},
		{Chunk: Chunk{ID: "guide", Metadata: map[string]string{"doc_type": "审查指南"}}, Score: 0.6},
	}
	reranked := reranker.Rerank(results)
	// 审查指南 (rank 100) should outrank 技术文献 (rank 50) despite equal base score.
	if reranked[0].ID != "guide" {
		t.Errorf("expected 审查指南 to rank first, got %s (score %.3f vs %.3f)",
			reranked[0].ID, reranked[0].Score, reranked[1].Score)
	}
}

func TestPatentReranker_EmptyResults(t *testing.T) {
	reranker := NewPatentReranker()
	if result := reranker.Rerank(nil); result != nil {
		t.Errorf("expected nil for empty results, got %v", result)
	}
}

func TestPatentReranker_SuppressesFutureDated(t *testing.T) {
	reranker := NewPatentReranker()
	reranker.ApplicationDate = "2024-01-01"
	reranker.SuppressFutureDateKey = "date"
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "valid", Metadata: map[string]string{"date": "2023-06-01", "doc_type": "专利法"}}, Score: 0.8},
		{Chunk: Chunk{ID: "future", Metadata: map[string]string{"date": "2025-01-01", "doc_type": "专利法"}}, Score: 0.8},
	}
	reranked := reranker.Rerank(results)
	// The future-dated chunk should be suppressed (penalty multiplier applied).
	var futureScore, validScore float64
	for _, r := range reranked {
		if r.ID == "future" {
			futureScore = r.Score
		}
		if r.ID == "valid" {
			validScore = r.Score
		}
	}
	if futureScore >= validScore {
		t.Errorf("future-dated (2025) should be suppressed below valid (2023): %.3f vs %.3f", futureScore, validScore)
	}
}

func TestPatentReranker_NoMetadataNoCrash(t *testing.T) {
	reranker := NewPatentReranker()
	// Chunks without doc_type metadata should pass through unchanged (no boost, no crash).
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "plain", Metadata: nil}, Score: 0.5},
	}
	reranked := reranker.Rerank(results)
	if len(reranked) != 1 || reranked[0].Score != 0.5 {
		t.Errorf("plain chunk should be unchanged, got score %.3f", reranked[0].Score)
	}
}

func TestDefaultPatentDocTypeRank(t *testing.T) {
	h := DefaultPatentDocTypeRank()
	if h["审查指南"] <= h["技术文献"] {
		t.Error("审查指南 should rank higher than 技术文献")
	}
	if h["专利法"] <= h["判例"] {
		t.Error("专利法 (statute) should rank higher than 判例 (case law)")
	}
}
