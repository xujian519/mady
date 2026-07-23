package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval"
)

// mockWritableWithCallback extends mockWritable with callable hooks for
// verifying AddDocument calls in tests.
type mockWritableWithCallback struct {
	searchResults []retrieval.ScoredChunk
	onAddDocument func(ctx context.Context, docID, title, content string) error
}

func (m *mockWritableWithCallback) Search(_ context.Context, _ string, _ int) ([]retrieval.ScoredChunk, error) {
	return m.searchResults, nil
}

func (m *mockWritableWithCallback) AddDocument(ctx context.Context, docID, title, content string) error {
	if m.onAddDocument != nil {
		return m.onAddDocument(ctx, docID, title, content)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Lifecycle tests for KnowledgeExtension
// ---------------------------------------------------------------------------

func TestKnowledgeExtension_InitAndName(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())

	if name := ext.Name(); name != ExtensionName {
		t.Errorf("Name() = %q, want %q", name, ExtensionName)
	}

	if err := ext.Init(context.Background(), nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
}

func TestKnowledgeExtension_Layer(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())

	layer := ext.Layer()
	if layer != agentcore.LayerKnowledge {
		t.Errorf("Layer() = %q, want %q", layer, agentcore.LayerKnowledge)
	}
}

func TestKnowledgeExtension_Dispose(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())

	// Init + Dispose cycle should not panic.
	if err := ext.Init(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := ext.Dispose(); err != nil {
		t.Fatalf("Dispose failed: %v", err)
	}
}

func TestKnowledgeExtension_DisposeWithoutInit(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())

	// Dispose without Init should be safe.
	if err := ext.Dispose(); err != nil {
		t.Fatalf("Dispose without Init failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// LifecycleProvider 实现验证
// ---------------------------------------------------------------------------

func TestKnowledgeExtension_LifecycleHook(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())

	// Without backend, LifecycleHook returns RetrievalHook.
	hook := ext.LifecycleHook()
	if hook == nil {
		t.Fatal("expected non-nil LifecycleHook")
	}
}

func TestKnowledgeExtension_BackendHook_NilWithoutBackend(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())

	hook := ext.BackendHook(retrieval.DefaultRetrievalConfig())
	if hook != nil {
		t.Error("expected nil BackendHook when no backend configured")
	}
}

func TestKnowledgeExtension_BackendHook_WithBackend(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "专利法第22条", DocID: "patent_law"}, Score: 0.95},
		},
	}, nil)

	cfg := retrieval.DefaultRetrievalConfig()
	hook := ext.BackendHook(cfg)
	if hook == nil {
		t.Fatal("expected non-nil BackendHook when backend is configured")
	}

	// Verify the hook works by calling BeforeModelCall.
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

	if err := hook.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall failed: %v", err)
	}
}

func TestKnowledgeExtension_BackendHook_WithWritableOnly(t *testing.T) {
	// BackendHook should still return nil when only writable store is configured
	// (no backend).
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
// ToolProvider 注册验证
// ---------------------------------------------------------------------------

func TestKnowledgeExtension_ToolProvider_EmptyWithoutDeps(t *testing.T) {
	// Without backend, embedder, writable, or lawSearcher,
	// only search_knowledge should be exposed.
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())

	tools := ext.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool (search_knowledge), got %d", len(tools))
	}
	if tools[0].Name != "search_knowledge" {
		t.Errorf("expected search_knowledge, got %s", tools[0].Name)
	}
}

func TestKnowledgeExtension_ToolProvider_WithLawSearcher(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithLawSearcher(func(keyword string, topK int) ([]LawRecord, error) {
		return nil, nil
	})

	tools := ext.Tools()
	if len(tools) < 2 {
		t.Fatalf("expected at least 2 tools (search + laws), got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names["search_laws"] {
		t.Error("missing search_laws tool")
	}
	if !names["search_knowledge"] {
		t.Error("missing search_knowledge tool")
	}
}

func TestKnowledgeExtension_ToolProvider_WithWritable(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithWritableStore(&mockWritable{})

	tools := ext.Tools()
	if len(tools) < 2 {
		t.Fatalf("expected at least 2 tools (search + add_document), got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names["add_document"] {
		t.Error("missing add_document tool")
	}
}

func TestKnowledgeExtension_ToolProvider_AllThree(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithWritableStore(&mockWritable{})
	ext.WithLawSearcher(func(keyword string, topK int) ([]LawRecord, error) {
		return nil, nil
	})

	tools := ext.Tools()
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools (search + laws + add), got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
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

func TestKnowledgeExtension_Tools_Disabled(t *testing.T) {
	cfg := DefaultKnowledgeExtConfig()
	cfg.ExposeTool = false

	ext := NewExtension(nil, nil, "patent", cfg)
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithWritableStore(&mockWritable{})
	ext.WithLawSearcher(func(keyword string, topK int) ([]LawRecord, error) {
		return nil, nil
	})

	tools := ext.Tools()
	if tools != nil {
		t.Errorf("expected nil tools when ExposeTool=false, got %d tools", len(tools))
	}
}

// ---------------------------------------------------------------------------
// SearchLaws tool handler tests
// ---------------------------------------------------------------------------

func TestKnowledgeExtension_HandleSearchLaws(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithLawSearcher(func(keyword string, topK int) ([]LawRecord, error) {
		return []LawRecord{
			{
				ID:       "law1",
				Name:     "专利法",
				Content:  "第22条 新颖性是指该发明或者实用新型不属于现有技术",
				Level:    "法律",
				Category: "知识产权",
			},
		}, nil
	})

	tools := ext.Tools()
	var searchLawsTool *agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "search_laws" {
			searchLawsTool = tool
			break
		}
	}
	if searchLawsTool == nil {
		t.Fatal("search_laws tool not found")
	}

	// Verify the handler returns proper results.
	result, err := searchLawsTool.Func(context.Background(), []byte(`{"query":"新颖性","top_k":3}`))
	if err != nil {
		t.Fatalf("search_laws handler failed: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	if len(resultStr) == 0 {
		t.Error("expected non-empty result")
	}
}

func TestKnowledgeExtension_HandleSearchLaws_EmptyQuery(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithLawSearcher(func(keyword string, topK int) ([]LawRecord, error) {
		return nil, nil
	})

	tools := ext.Tools()
	var searchLawsTool *agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "search_laws" {
			searchLawsTool = tool
			break
		}
	}
	if searchLawsTool == nil {
		t.Fatal("search_laws tool not found")
	}

	// Empty query should return a help message, not an error.
	result, err := searchLawsTool.Func(context.Background(), []byte(`{"query":"","top_k":3}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok || resultStr == "" {
		t.Errorf("expected non-empty help message for empty query")
	}
}

func TestKnowledgeExtension_HandleSearchLaws_NoLawSearcher(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	// No law searcher configured.

	tools := ext.Tools()
	// Without law searcher, search_laws should not be exposed.
	for _, tool := range tools {
		if tool.Name == "search_laws" {
			t.Fatal("search_laws should not be exposed without LawSearcher")
		}
	}
}

func TestKnowledgeExtension_HandleSearchLaws_LawSearcherError(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithLawSearcher(func(keyword string, topK int) ([]LawRecord, error) {
		return nil, errors.New("database connection failed")
	})

	tools := ext.Tools()
	var searchLawsTool *agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "search_laws" {
			searchLawsTool = tool
			break
		}
	}
	if searchLawsTool == nil {
		t.Fatal("search_laws tool not found")
	}

	result, err := searchLawsTool.Func(context.Background(), []byte(`{"query":"专利法","top_k":3}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	if !containsSubstr(resultStr, "失败") {
		t.Errorf("expected error message in result, got: %s", resultStr)
	}
}

// ---------------------------------------------------------------------------
// HandleAddDocument tests
// ---------------------------------------------------------------------------

func TestKnowledgeExtension_HandleAddDocument_Success(t *testing.T) {
	var addedDocID, addedTitle, addedContent string
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithWritableStore(&mockWritableWithCallback{
		searchResults: []retrieval.ScoredChunk{},
		onAddDocument: func(_ context.Context, docID, title, content string) error {
			addedDocID = docID
			addedTitle = title
			addedContent = content
			return nil
		},
	})

	tools := ext.Tools()
	var addDocTool *agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "add_document" {
			addDocTool = tool
			break
		}
	}
	if addDocTool == nil {
		t.Fatal("add_document tool not found")
	}

	result, err := addDocTool.Func(context.Background(),
		[]byte(`{"doc_id":"doc-001","title":"测试文档","content":"这是测试文档的内容"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	if !containsSubstr(resultStr, "doc-001") {
		t.Errorf("expected doc_id in result, got: %s", resultStr)
	}
	if addedDocID != "doc-001" {
		t.Errorf("added doc_id = %q, want doc-001", addedDocID)
	}
	if addedTitle != "测试文档" {
		t.Errorf("added title = %q, want 测试文档", addedTitle)
	}
	if addedContent != "这是测试文档的内容" {
		t.Errorf("added content = %q", addedContent)
	}
}

func TestKnowledgeExtension_HandleAddDocument_MissingFields(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithWritableStore(&mockWritable{})

	tools := ext.Tools()
	var addDocTool *agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "add_document" {
			addDocTool = tool
			break
		}
	}

	// Empty doc_id.
	result, err := addDocTool.Func(context.Background(),
		[]byte(`{"doc_id":"","title":"test","content":"content"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok || resultStr == "" {
		t.Errorf("expected help message for missing doc_id")
	}

	// Empty content.
	result, err = addDocTool.Func(context.Background(),
		[]byte(`{"doc_id":"doc-002","title":"test","content":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultStr, ok = result.(string)
	if !ok || resultStr == "" {
		t.Errorf("expected help message for missing content")
	}
}

func TestKnowledgeExtension_HandleAddDocument_NoWritable(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	// No writable store configured.

	tools := ext.Tools()
	for _, tool := range tools {
		if tool.Name == "add_document" {
			t.Fatal("add_document should not be exposed without writable store")
		}
	}
}

func TestKnowledgeExtension_HandleAddDocument_WritableError(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{vectors: [][]float32{{1.0}}})
	ext.WithWritableStore(&mockWritableWithCallback{
		onAddDocument: func(_ context.Context, _, _, _ string) error {
			return errors.New("write failed")
		},
	})

	tools := ext.Tools()
	var addDocTool *agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "add_document" {
			addDocTool = tool
			break
		}
	}

	result, err := addDocTool.Func(context.Background(),
		[]byte(`{"doc_id":"doc-003","title":"test","content":"content"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok || !containsSubstr(resultStr, "失败") {
		t.Errorf("expected error message in result, got: %s", resultStr)
	}
}

// ---------------------------------------------------------------------------
// Tool handler: search_knowledge
// ---------------------------------------------------------------------------

func TestKnowledgeExtension_HandleSearch_EmptyQuery(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "test", DocID: "d1"}, Score: 0.9},
		},
	}, nil)

	tools := ext.Tools()
	var searchTool *agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "search_knowledge" {
			searchTool = tool
			break
		}
	}
	if searchTool == nil {
		t.Fatal("search_knowledge tool not found")
	}

	// Empty query returns help message.
	result, err := searchTool.Func(context.Background(), []byte(`{"query":"","top_k":5}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok || resultStr == "" {
		t.Errorf("expected non-empty help message for empty query")
	}
}

func TestKnowledgeExtension_HandleSearch_NoResults(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults:    nil,
		vectorResults: nil,
	}, &mockEmbedder{vectors: [][]float32{{0.1, 0.2, 0.3}}})

	tools := ext.Tools()
	var searchTool *agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "search_knowledge" {
			searchTool = tool
			break
		}
	}
	if searchTool == nil {
		t.Fatal("search_knowledge tool not found")
	}

	result, err := searchTool.Func(context.Background(), []byte(`{"query":"nonexistent_term","top_k":5}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok || !containsSubstr(resultStr, "未找到") {
		t.Errorf("expected 'not found' message, got: %s", resultStr)
	}
}

// ---------------------------------------------------------------------------
// Provide / Layer integration
// ---------------------------------------------------------------------------

func TestKnowledgeExtension_Provide_NoStoreOrBackend(t *testing.T) {
	cfg := DefaultKnowledgeExtConfig()
	cfg.Enabled = true
	ext := NewExtension(nil, nil, "patent", cfg)
	// No store and no backend.

	msgs, err := ext.Provide(context.Background(), agentcore.BuildInput{
		Messages: []agentcore.Message{{Role: agentcore.RoleUser, Content: "测试"}},
	}, agentcore.LayerConfig{})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	if msgs != nil {
		t.Errorf("expected nil messages when no store and no backend, got %d", len(msgs))
	}
}

func TestKnowledgeExtension_Provide_Disabled(t *testing.T) {
	cfg := DefaultKnowledgeExtConfig()
	cfg.Enabled = false

	ext := NewExtension(nil, nil, "patent", cfg)
	s := NewStore()
	_ = s.LoadText("patent", "doc1", "test", "专利法内容")

	msgs, err := ext.Provide(context.Background(), agentcore.BuildInput{
		Messages: []agentcore.Message{{Role: agentcore.RoleUser, Content: "测试"}},
	}, agentcore.LayerConfig{})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	if msgs != nil {
		t.Errorf("expected nil messages when disabled, got %d", len(msgs))
	}
}

func TestKnowledgeExtension_Provide_WithBackend(t *testing.T) {
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
	if len(msgs) == 0 {
		t.Fatal("expected non-empty messages from Provide with backend")
	}
	if msgs[0].Role != agentcore.RoleSystem {
		t.Errorf("expected RoleSystem, got %v", msgs[0].Role)
	}
}

func TestKnowledgeExtension_Provide_NoUserMsg(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "test", DocID: "d1"}, Score: 0.9},
		},
	}, nil)

	// No user message in input.
	msgs, err := ext.Provide(context.Background(), agentcore.BuildInput{
		Messages: []agentcore.Message{{Role: agentcore.RoleAssistant, Content: "回答"}},
	}, agentcore.LayerConfig{})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	if msgs != nil {
		t.Errorf("expected nil messages when no user input, got %d", len(msgs))
	}
}

// ---------------------------------------------------------------------------
// 扩展注册验证
// ---------------------------------------------------------------------------

func TestKnowledgeExtension_InterfaceImplementation(t *testing.T) {
	// Verify KnowledgeExtension satisfies the required interfaces.
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())

	var _ agentcore.Extension = ext
	var _ agentcore.LifecycleProvider = ext
	var _ agentcore.ToolProvider = ext
	var _ agentcore.TransformContextProvider = ext
}

// ---------------------------------------------------------------------------
// GraphContext accessor
// ---------------------------------------------------------------------------

func TestKnowledgeExtension_GraphContext_Default(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	ctx := ext.GraphContext()
	if ctx != "" {
		t.Errorf("expected empty graph context by default, got %q", ctx)
	}
}

func TestKnowledgeExtension_GraphContext_AfterBackendSearch(t *testing.T) {
	ext := NewExtension(nil, nil, "patent", DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{
		ftsResults: []retrieval.ScoredChunk{
			{Chunk: retrieval.Chunk{Content: "专利法内容", DocID: "patent_law"}, Score: 0.95},
		},
	}, nil)
	ext.WithGraph(&mockGraph{context: "图增强: 相似案例"})

	results := ext.Search(context.Background(), "新颖性", 5)
	if len(results) == 0 {
		t.Fatal("expected search results")
	}

	graphCtx := ext.GraphContext()
	if !containsSubstr(graphCtx, "图增强") {
		t.Errorf("expected graph context after search, got %q", graphCtx)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// containsSubstr is a helper to check substring presence without importing strings.
func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
