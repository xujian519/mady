package knowledge_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/retrieval"
)

// mockBackend implements knowledge.KnowledgeBackend for integration tests.
// It returns pre-configured results without touching a real database.
type mockBackend struct {
	ftsResults    []retrieval.ScoredChunk
	vectorResults []retrieval.ScoredChunk
}

func (m *mockBackend) FTSSearch(_ string, _ int) ([]retrieval.ScoredChunk, error) {
	return m.ftsResults, nil
}

func (m *mockBackend) VectorSearch(_ []float32, _ int) ([]retrieval.ScoredChunk, error) {
	return m.vectorResults, nil
}

// mockEmbedder implements retrieval.Embedder for integration tests.
// It produces deterministic vectors based on text content so similar texts
// yield similar vectors, enabling meaningful vector search results.
type mockEmbedder struct{ dim int }

func (e *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i, text := range texts {
		v := make([]float32, e.dim)
		for j, ch := range text {
			v[j%e.dim] += float32(ch) / 1000
		}
		vecs[i] = v
	}
	return vecs, nil
}

func (e *mockEmbedder) Dimensions() int { return e.dim }

// TestExtension_AddDocumentToolExposed verifies that the add_document tool
// is only exposed when a WritableBackend is injected.
func TestExtension_AddDocumentToolExposed(t *testing.T) {
	// Without writable store — should only have search_knowledge.
	ext := knowledge.NewExtension(nil, nil, "patent", knowledge.DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, &mockEmbedder{dim: 8})
	tools := ext.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool without writable, got %d", len(tools))
	}
	if tools[0].Name != "search_knowledge" {
		t.Errorf("expected search_knowledge, got %s", tools[0].Name)
	}

	// With writable store — should have both tools.
	dir := t.TempDir()
	ws, err := sqlite.OpenWritable(filepath.Join(dir, "user.db"), &mockEmbedder{dim: 8}, "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer ws.Close()
	ext.WithWritableStore(ws)

	tools = ext.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools with writable, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names["search_knowledge"] || !names["add_document"] {
		t.Errorf("expected search_knowledge + add_document, got %v", names)
	}
}

// TestExtension_AddDocumentThenSearch verifies the end-to-end flow:
// add_document tool call → document persisted → search_knowledge finds it.
func TestExtension_AddDocumentThenSearch(t *testing.T) {
	dir := t.TempDir()
	emb := &mockEmbedder{dim: 8}
	ws, err := sqlite.OpenWritable(filepath.Join(dir, "user.db"), emb, filepath.Join(dir, "knowledge.db"))
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer ws.Close()

	// Extension with empty mock backend (no knowledge.db results) + writable store.
	ext := knowledge.NewExtension(nil, nil, "patent", knowledge.DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, emb)
	ext.WithWritableStore(ws)

	// 1. Call add_document tool.
	addArgs, _ := json.Marshal(map[string]string{
		"doc_id":  "user-doc-001",
		"title":   "用户测试文档",
		"content": "专利申请的审查流程包括初步审查和实质审查两个阶段。实质审查主要审查新颖性、创造性和实用性。",
	})
	addResult, err := callTool(ext, "add_document", addArgs)
	if err != nil {
		t.Fatalf("add_document call: %v", err)
	}
	if str, ok := addResult.(string); !ok || !contains(str, "成功") {
		t.Errorf("add_document result = %v, expected success message", addResult)
	}

	// 2. Search for the document.
	searchArgs, _ := json.Marshal(map[string]any{
		"query": "实质审查",
		"top_k": 5,
	})
	searchResult, err := callTool(ext, "search_knowledge", searchArgs)
	if err != nil {
		t.Fatalf("search_knowledge call: %v", err)
	}
	resultStr, ok := searchResult.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", searchResult)
	}
	if !contains(resultStr, "实质审查") {
		t.Errorf("search result does not contain user document content:\n%s", resultStr)
	}
}

// TestExtension_ThreeLaneRRF verifies that user documents participate in
// RRF fusion alongside knowledge FTS and knowledge Vector results.
func TestExtension_ThreeLaneRRF(t *testing.T) {
	dir := t.TempDir()
	emb := &mockEmbedder{dim: 8}
	ws, err := sqlite.OpenWritable(filepath.Join(dir, "user.db"), emb, "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer ws.Close()

	ctx := context.Background()
	// Add a user document.
	if err := ws.AddDocument(ctx, "user-1", "用户文档", "这是一个关于商标注册申请的特殊文档内容"); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	// Mock backend returns knowledge.db results.
	knowledgeFTS := []retrieval.ScoredChunk{
		{Chunk: retrieval.Chunk{ID: "k1", DocID: "law://商标法/第1条", Content: "商标法第一条规定了立法目的"}, Score: 0.9},
	}
	knowledgeVec := []retrieval.ScoredChunk{
		{Chunk: retrieval.Chunk{ID: "k2", DocID: "law://商标法/第2条", Content: "商标注册申请的审查"}, Score: 0.85},
	}

	ext := knowledge.NewExtension(nil, nil, "patent", knowledge.DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{ftsResults: knowledgeFTS, vectorResults: knowledgeVec}, emb)
	ext.WithWritableStore(ws)

	results := ext.Search(ctx, "商标注册申请", 10)
	if len(results) == 0 {
		t.Fatal("expected search results from three-lane RRF")
	}

	// Verify that both knowledge and user results are present.
	foundKnowledge := false
	foundUser := false
	for _, r := range results {
		if r.DocID == "law://商标法/第1条" || r.DocID == "law://商标法/第2条" {
			foundKnowledge = true
		}
		if r.DocID == "user-1" {
			foundUser = true
		}
	}
	if !foundKnowledge {
		t.Error("knowledge.db results missing from three-lane RRF output")
	}
	if !foundUser {
		t.Error("user.db results missing from three-lane RRF output")
	}
}

// TestExtension_AddDocumentValidation tests error handling in add_document.
func TestExtension_AddDocumentValidation(t *testing.T) {
	dir := t.TempDir()
	emb := &mockEmbedder{dim: 8}
	ws, err := sqlite.OpenWritable(filepath.Join(dir, "user.db"), emb, "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer ws.Close()

	ext := knowledge.NewExtension(nil, nil, "patent", knowledge.DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{}, emb)
	ext.WithWritableStore(ws)

	// Empty doc_id.
	args, _ := json.Marshal(map[string]string{"doc_id": "", "title": "test", "content": "content"})
	result, _ := callTool(ext, "add_document", args)
	if str, ok := result.(string); !ok || !contains(str, "doc_id") {
		t.Errorf("expected doc_id validation error, got: %v", result)
	}

	// Empty content.
	args, _ = json.Marshal(map[string]string{"doc_id": "test", "title": "test", "content": ""})
	result, _ = callTool(ext, "add_document", args)
	if str, ok := result.(string); !ok || !contains(str, "content") {
		t.Errorf("expected content validation error, got: %v", result)
	}
}

// TestExtension_CrossDBIDNoCollision verifies that chunk IDs from knowledge.db
// (numeric, e.g. "1") and user.db (prefixed, e.g. "u:1") do not collide in
// RRF fusion. Before the "u:" prefix fix, identical numeric IDs from the two
// databases would be merged by RRFFuser, causing silent result corruption.
func TestExtension_CrossDBIDNoCollision(t *testing.T) {
	dir := t.TempDir()
	emb := &mockEmbedder{dim: 8}
	ws, err := sqlite.OpenWritable(filepath.Join(dir, "user.db"), emb, "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer ws.Close()

	ctx := context.Background()
	if err := ws.AddDocument(ctx, "user-doc", "用户文档", "专利优先权期限是十二个月"); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	// Simulate knowledge.db returning chunk ID "1" — a realistic numeric ID
	// from AUTOINCREMENT that would collide with user.db's chunk ID "1"
	// before the "u:" prefix fix.
	knowledgeFTS := []retrieval.ScoredChunk{
		{Chunk: retrieval.Chunk{ID: "1", DocID: "law://专利法/第29条", Content: "申请人要求优先权的应当在申请时提出书面声明"}, Score: 0.9},
	}
	knowledgeVec := []retrieval.ScoredChunk{
		{Chunk: retrieval.Chunk{ID: "1", DocID: "law://专利法/第29条", Content: "申请人要求优先权的应当在申请时提出书面声明"}, Score: 0.85},
	}

	ext := knowledge.NewExtension(nil, nil, "patent", knowledge.DefaultKnowledgeExtConfig())
	ext.WithBackend(&mockBackend{ftsResults: knowledgeFTS, vectorResults: knowledgeVec}, emb)
	ext.WithWritableStore(ws)

	results := ext.Search(ctx, "优先权", 10)
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results (knowledge + user), got %d — IDs likely collided", len(results))
	}

	foundKnowledge := false
	foundUser := false
	for _, r := range results {
		if r.DocID == "law://专利法/第29条" {
			foundKnowledge = true
		}
		if r.DocID == "user-doc" {
			foundUser = true
		}
	}
	if !foundKnowledge {
		t.Error("knowledge.db result missing — ID collision may have merged it with user.db")
	}
	if !foundUser {
		t.Error("user.db result missing — ID collision may have merged it with knowledge.db")
	}
}

// callTool finds a tool by name and invokes it.
func callTool(ext *knowledge.KnowledgeExtension, name string, args json.RawMessage) (any, error) {
	for _, tool := range ext.Tools() {
		if tool.Name == name {
			return tool.Func(context.Background(), args)
		}
	}
	return nil, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsStr(s, substr)))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
