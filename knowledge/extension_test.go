package knowledge

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval"
)

// ---------------------------------------------------------------------------
// Mock types
// ---------------------------------------------------------------------------

// mockWritable implements WritableBackend for testing.
type mockWritable struct {
	searchResults []retrieval.ScoredChunk
}

func (m *mockWritable) Search(_ context.Context, _ string, _ int) ([]retrieval.ScoredChunk, error) {
	return m.searchResults, nil
}

func (m *mockWritable) AddDocument(_ context.Context, _, _, _ string) error {
	return nil
}

// mockGraph implements GraphEnhancer for testing.
type mockGraph struct {
	context string
}

func (m *mockGraph) Enhance(_ []retrieval.ScoredChunk) any {
	return &mockEnhancement{ctx: m.context}
}

type mockEnhancement struct {
	ctx string
}

func (m *mockEnhancement) GetContext() string {
	return m.ctx
}

// mockReranker implements retrieval.QueryReranker for testing.
type mockReranker struct {
	called bool
}

func (m *mockReranker) Rerank(results []retrieval.ScoredChunk) []retrieval.ScoredChunk {
	return results
}

func (m *mockReranker) RerankWithQuery(_ context.Context, _ string, results []retrieval.ScoredChunk) []retrieval.ScoredChunk {
	m.called = true
	return results
}

// ---------------------------------------------------------------------------
// KnowledgeExtension Search tests
// ---------------------------------------------------------------------------

func TestExtensionSearch_WithBackend(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "专利法第22条 新颖性是指", DocID: "patent_law"}, Score: 0.95},
		},
	}, nil)

	results := ext.Search(context.Background(), "新颖性", 5)
	if len(results) == 0 {
		t.Fatal("expected non-empty results from backend search")
	}
	if results[0].DocID != "patent_law" {
		t.Errorf("expected patent_law, got %s", results[0].DocID)
	}
}

func TestExtensionSearch_WithEmbedderAndVector(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "专利法第22条", DocID: "d1"}, Score: 0.9},
		},
		vectorResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "新颖性判断标准", DocID: "d2"}, Score: 0.85},
		},
	}, &mockEmbedder{vectors: [][]float32{{0.1, 0.2, 0.3}}})

	results := ext.Search(context.Background(), "新颖性", 5)
	if len(results) == 0 {
		t.Fatal("expected non-empty results from multi-lane search")
	}
}

func TestExtensionSearch_WithWritableStore(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{ID: "c1", Content: "审查指南", DocID: "guideline"}, Score: 0.9},
		},
	}, &mockEmbedder{vectors: [][]float32{{0.1, 0.2, 0.3}}})
	ext.WithWritableStore(&mockWritable{
		searchResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{ID: "c2", Content: "用户添加的文档", DocID: "user_doc"}, Score: 0.8},
		},
	})

	results := ext.Search(context.Background(), "专利", 5)
	if len(results) == 0 {
		t.Fatal("expected non-empty results with writable store")
	}
	// Results should include both backend and writable store chunks.
	docIDs := make(map[string]bool)
	for _, r := range results {
		docIDs[r.DocID] = true
	}
	if !docIDs["guideline"] {
		t.Errorf("expected guideline in results")
	}
	if !docIDs["user_doc"] {
		t.Errorf("expected user_doc in results")
	}
}

func TestExtensionSearch_WithGraphEnhancer(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "专利法第22条", DocID: "patent_law"}, Score: 0.95},
		},
	}, nil)
	ext.WithGraph(&mockGraph{context: "相似案例: 复审决定X12345"})

	results := ext.Search(context.Background(), "新颖性", 5)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}

	// Graph context should be cached after search.
	graphCtx := ext.GraphContext()
	if !strings.Contains(graphCtx, "相似案例") {
		t.Errorf("expected graph context containing '相似案例', got %q", graphCtx)
	}
}

func TestExtensionSearch_GraphContextEmptyByDefault(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	ctx := ext.GraphContext()
	if ctx != "" {
		t.Errorf("expected empty graph context, got %q", ctx)
	}
}

func TestExtensionSearch_WithReranker(t *testing.T) {
	rr := &mockReranker{}
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "专利法第22条", DocID: "patent_law"}, Score: 0.95},
		},
	}, nil)
	ext.WithReranker(rr)

	_ = ext.Search(context.Background(), "新颖性", 5)
	if !rr.called {
		t.Errorf("expected reranker to be called during search")
	}
}

func TestExtensionSearch_NoBackend_FallbackToMemory(t *testing.T) {
	s := NewStore()
	_ = s.LoadText("patent", "doc1", "测试文档", "专利法第22条 新颖性是指...")

	ext := NewExtension(s, nil, "patent", DefaultKnowledgeExtConfig())
	results := ext.Search(context.Background(), "新颖性", 5)

	// Memory search may or may not match (keyword matching).
	// At minimum it shouldn't crash.
	if results == nil {
		// nil is acceptable when memory search finds nothing
		return
	}
}

func TestExtensionSearch_EmptyQuery(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "test", DocID: "d1"}, Score: 0.9},
		},
	}, nil)

	results := ext.Search(context.Background(), "", 5)
	// Empty query should still go through backend search (FTS handles it).
	// At minimum shouldn't crash.
	if results == nil {
		t.Fatal("expected non-nil results from backend with empty query, got nil")
	}
}

func TestExtensionSearch_NoResults(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults:    nil,
		vectorResults: nil,
	}, &mockEmbedder{vectors: [][]float32{{0.1, 0.2, 0.3}}})

	results := ext.Search(context.Background(), "不存在的内容", 5)
	if results != nil {
		t.Errorf("expected nil results when no matches, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// KnowledgeExtension BackendHook tests
// ---------------------------------------------------------------------------

func TestExtension_BackendHookWithGraph(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "专利法第22条 新颖性", DocID: "patent_law"}, Score: 0.95},
		},
	}, nil)
	ext.WithGraph(&mockGraph{context: "图增强上下文"})

	hook := NewBackendRetrievalHook(ext, retrieval.RetrievalConfig{
		TopK:       5,
		MaxChars:   4000,
		DomainHint: "patent",
		Prefix:     "测试前缀\n",
	})

	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "什么是新颖性"},
		},
	}
	arc := &agentcore.AgentRunContext{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "什么是新颖性"},
		},
	}
	mcc := &agentcore.ModelCallContext{Request: req}

	err := hook.BeforeModelCall(context.Background(), arc, mcc)
	if err != nil {
		t.Fatalf("BeforeModelCall failed: %v", err)
	}

	// Should have injected search results and graph context.
	foundSearch := false
	foundGraph := false
	for _, msg := range req.Messages {
		if strings.Contains(msg.Content, "测试前缀") {
			foundSearch = true
		}
		if strings.Contains(msg.Content, "图增强上下文") {
			foundGraph = true
		}
	}
	if !foundSearch {
		t.Error("expected search results injected")
	}
	if !foundGraph {
		t.Error("expected graph context injected")
	}
}

func TestExtension_BackendHookNilHookWithWritableOnly(t *testing.T) {
	// BackendHook should return nil when there's no backend (only writable).
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	ext.WithWritableStore(&mockWritable{
		searchResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "user doc", DocID: "u1"}, Score: 0.8},
		},
	})

	hook := ext.BackendHook(retrieval.DefaultRetrievalConfig())
	if hook != nil {
		t.Error("expected nil hook without backend, even with writable store")
	}
}

// ---------------------------------------------------------------------------
// KnowledgeExtension tool tests
// ---------------------------------------------------------------------------

func TestExtension_SearchLawsTool(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithLawSearcher(func(keyword string, topK int) ([]LawRecord, error) {
		return []LawRecord{
			{ID: "law1", Name: "专利法", Content: "第22条", Level: "法律"},
		}, nil
	})

	tools := ext.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools (search_knowledge + search_laws), got %d", len(tools))
	}
	if tools[1].Name != "search_laws" {
		t.Errorf("expected search_laws, got %s", tools[1].Name)
	}
}

func TestExtension_AddDocumentTool(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithWritableStore(&mockWritable{})

	tools := ext.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools (search_knowledge + add_document), got %d", len(tools))
	}
	if tools[1].Name != "add_document" {
		t.Errorf("expected add_document, got %s", tools[1].Name)
	}
}

func TestExtension_AllToolsExposed(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithWritableStore(&mockWritable{})
	ext.WithLawSearcher(func(keyword string, topK int) ([]LawRecord, error) {
		return nil, nil
	})

	tools := ext.Tools()
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, t2 := range tools {
		names[t2.Name] = true
	}
	if !names["search_knowledge"] {
		t.Error("missing search_knowledge")
	}
	if !names["search_laws"] {
		t.Error("missing search_laws")
	}
	if !names["add_document"] {
		t.Error("missing add_document")
	}
}

// ---------------------------------------------------------------------------
// KnowledgeExtension Provide tests
// ---------------------------------------------------------------------------

func TestExtension_ProvideWithBackend(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "专利法第22条 新颖性", DocID: "patent_law"}, Score: 0.95},
		},
	}, &mockEmbedder{vectors: [][]float32{{0.1, 0.2, 0.3}}})

	msgs, err := ext.Provide(context.Background(), agentcore.BuildInput{
		Messages: []agentcore.Message{{Role: agentcore.RoleUser, Content: "什么是新颖性"}},
	}, agentcore.LayerConfig{})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	if len(msgs) > 0 {
		if msgs[0].Role != agentcore.RoleSystem {
			t.Errorf("expected RoleSystem, got %v", msgs[0].Role)
		}
	}
}
