package knowledge

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval"
)

// mockBackend implements KnowledgeBackend for testing.
type mockBackend struct {
	ftsResults    []retrieval.ScoredChunk
	vectorResults []retrieval.ScoredChunk
}

func (m *mockBackend) FTSSearch(query string, topK int) ([]retrieval.ScoredChunk, error) {
	return m.ftsResults, nil
}

func (m *mockBackend) VectorSearch(queryVec []float32, topK int) ([]retrieval.ScoredChunk, error) {
	return m.vectorResults, nil
}

// mockEmbedder implements retrieval.Embedder for testing.
type mockEmbedder struct {
	vectors [][]float32
}

func (m *mockEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return m.vectors, nil
}

func (m *mockEmbedder) Dimensions() int {
	if len(m.vectors) > 0 && len(m.vectors[0]) > 0 {
		return len(m.vectors[0])
	}
	return 1024
}

func TestBackendHook_NilWhenNoBackend(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	hook := ext.BackendHook(retrieval.DefaultRetrievalConfig())
	if hook != nil {
		t.Fatal("expected nil hook when no backend configured")
	}
}

func TestBackendHook_NonNilWithBackend(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "test", DocID: "d1"}, Score: 0.9},
		},
	}, nil)
	hook := ext.BackendHook(retrieval.DefaultRetrievalConfig())
	if hook == nil {
		t.Fatal("expected non-nil hook with backend configured")
	}
}

func TestBackendHook_InjectsContext(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "专利法第22条 新颖性是指...", DocID: "patent_law"}, Score: 0.95},
		},
	}, nil)

	hook := NewBackendRetrievalHook(ext, retrieval.RetrievalConfig{
		TopK:       5,
		MaxChars:   4000,
		DomainHint: "patent",
		Prefix:     "测试前缀\n",
	})

	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: "system prompt"},
			{Role: agentcore.RoleUser, Content: "什么是新颖性"},
		},
	}
	arc := &agentcore.AgentRunContext{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "什么是新颖性"},
		},
	}
	mcc := &agentcore.ModelCallContext{Request: req}

	if err := hook.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall failed: %v", err)
	}

	if len(req.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(req.Messages))
	}
	injected := req.Messages[1]
	if injected.Role != agentcore.RoleSystem {
		t.Fatalf("expected RoleSystem, got %s", injected.Role)
	}
	if !strings.Contains(injected.Content, "新颖性") {
		t.Fatalf("expected '新颖性' in content, got: %s", injected.Content)
	}
	if !strings.Contains(injected.Content, "测试前缀") {
		t.Fatalf("expected prefix in content, got: %s", injected.Content)
	}
}

func TestBackendHook_EmptyQueryNoOp(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "should not appear"}, Score: 0.9},
		},
	}, nil)

	hook := NewBackendRetrievalHook(ext, retrieval.DefaultRetrievalConfig())
	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleAssistant, Content: "reply"},
		},
	}
	arc := &agentcore.AgentRunContext{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleAssistant, Content: "reply"},
		},
	}
	mcc := &agentcore.ModelCallContext{Request: req}

	if err := hook.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall failed: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message (no injection), got %d", len(req.Messages))
	}
}

func TestBackendHook_NoResultsNoOp(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, nil)

	hook := NewBackendRetrievalHook(ext, retrieval.DefaultRetrievalConfig())
	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "查询"},
		},
	}
	arc := &agentcore.AgentRunContext{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "查询"},
		},
	}
	mcc := &agentcore.ModelCallContext{Request: req}

	if err := hook.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall failed: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message (no results), got %d", len(req.Messages))
	}
}

func TestBackendHook_NilMCCNoOp(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, nil)

	hook := NewBackendRetrievalHook(ext, retrieval.DefaultRetrievalConfig())
	if err := hook.BeforeModelCall(context.Background(), &agentcore.AgentRunContext{}, nil); err != nil {
		t.Fatalf("BeforeModelCall failed: %v", err)
	}
}

func TestBackendHook_RRFFusionBothChannels(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "FTS结果", DocID: "d1"}, Score: 0.8},
		},
		vectorResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "向量检索结果", DocID: "d2"}, Score: 0.9},
		},
	}, &mockEmbedder{
		vectors: [][]float32{{0.1, 0.2, 0.3}},
	})

	hook := NewBackendRetrievalHook(ext, retrieval.RetrievalConfig{
		TopK:       5,
		MaxChars:   4000,
		DomainHint: "patent",
	})

	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "测试查询"},
		},
	}
	arc := &agentcore.AgentRunContext{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "测试查询"},
		},
	}
	mcc := &agentcore.ModelCallContext{Request: req}

	if err := hook.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall failed: %v", err)
	}
	if len(req.Messages) < 2 {
		t.Fatal("expected injected message")
	}
	// Find the injected system message (may be at index 0 or 1).
	injected := req.Messages[0]
	if injected.Role != agentcore.RoleSystem {
		injected = req.Messages[1]
	}
	hasFTS := strings.Contains(injected.Content, "FTS结果")
	hasVec := strings.Contains(injected.Content, "向量检索结果")
	if !hasFTS && !hasVec {
		t.Fatalf("expected RRF fusion results, got: %s", injected.Content)
	}
}

// mockQueryReranker implements retrieval.QueryReranker for testing.
type mockQueryReranker struct {
	called    bool
	queryUsed string
	reorder   []int // indices to reorder results to
}

func (m *mockQueryReranker) Rerank(results []retrieval.ScoredChunk) []retrieval.ScoredChunk {
	return results
}

func (m *mockQueryReranker) RerankWithQuery(_ context.Context, query string, results []retrieval.ScoredChunk) []retrieval.ScoredChunk {
	m.called = true
	m.queryUsed = query
	if len(m.reorder) == 0 {
		return results
	}
	reranked := make([]retrieval.ScoredChunk, 0, len(m.reorder))
	for i, idx := range m.reorder {
		if idx >= 0 && idx < len(results) {
			sc := results[idx]
			sc.Score = float64(len(results) - i)
			reranked = append(reranked, sc)
		}
	}
	return reranked
}

func TestBackendHook_RerankerApplied(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{ID: "c1", Content: "低相关结果", DocID: "d1"}, Score: 0.9},
			{Chunk: retrieval.Chunk{ID: "c2", Content: "高相关结果", DocID: "d2"}, Score: 0.5},
		},
	}, nil)

	reranker := &mockQueryReranker{reorder: []int{1, 0}}
	ext.WithReranker(reranker)

	hook := NewBackendRetrievalHook(ext, retrieval.RetrievalConfig{
		TopK:       5,
		MaxChars:   4000,
		DomainHint: "patent",
	})

	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "高相关查询"},
		},
	}
	arc := &agentcore.AgentRunContext{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "高相关查询"},
		},
	}
	mcc := &agentcore.ModelCallContext{Request: req}

	if err := hook.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall failed: %v", err)
	}

	if !reranker.called {
		t.Fatal("expected reranker to be called")
	}
	if reranker.queryUsed != "高相关查询" {
		t.Fatalf("expected query '高相关查询', got '%s'", reranker.queryUsed)
	}

	injected := req.Messages[0]
	if injected.Role != agentcore.RoleSystem {
		injected = req.Messages[1]
	}

	idxHigh := strings.Index(injected.Content, "高相关结果")
	idxLow := strings.Index(injected.Content, "低相关结果")
	if idxHigh < 0 || idxLow < 0 {
		t.Fatalf("expected both results in content, got: %s", injected.Content)
	}
	if idxHigh > idxLow {
		t.Fatalf("expected '高相关结果' before '低相关结果' after rerank, content: %s", injected.Content)
	}
}
