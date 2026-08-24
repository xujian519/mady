package patenttools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	ksqlite "github.com/xujian519/mady/knowledge/sqlite"
)

// testEmbedder implements retrieval.Embedder deterministically so that
// OpenWritable can build a scratch writable store in t.TempDir() without
// external services.
type testEmbedder struct{ dim int }

func (e *testEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
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

func (e *testEmbedder) Dimensions() int { return e.dim }

// callTool invokes a tool's Func with the given JSON args, returning the value
// and error directly. It exists to keep the nil-store/error-path assertions
// terse.
func callTool(t *testing.T, tool *agentcore.Tool, argsJSON string) (any, error) {
	t.Helper()
	raw := json.RawMessage(argsJSON)
	return tool.Func(context.Background(), raw)
}

// failResult asserts the returned value is a HandoffResult and returns it.
func failResult(t *testing.T, v any) agentcore.HandoffResult {
	t.Helper()
	hr, ok := v.(agentcore.HandoffResult)
	if !ok {
		t.Fatalf("want HandoffResult, got %T: %v", v, v)
	}
	return hr
}

func TestPatentWikiSearchTool_NilStore(t *testing.T) {
	tool := NewPatentWikiSearchTool(nil)
	v, err := callTool(t, tool, `{"query":"新颖性"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hr := failResult(t, v)
	if hr.Success {
		t.Fatalf("want failure, got success: %+v", hr)
	}
	if hr.Action != "wiki 搜索不可用" {
		t.Errorf("Action = %q, want wiki 搜索不可用", hr.Action)
	}
}

func TestPatentWikiSearchTool_InvalidArgs(t *testing.T) {
	tool := NewPatentWikiSearchTool(nil)
	v, err := callTool(t, tool, `{not-json`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hr := failResult(t, v)
	if hr.Success {
		t.Fatalf("want failure, got success: %+v", hr)
	}
	if hr.Action != "参数解析失败" {
		t.Errorf("Action = %q, want 参数解析失败", hr.Action)
	}
}

func TestPatentCaseSearchTool_NilStore(t *testing.T) {
	tool := NewPatentCaseSearchTool(nil)
	v, err := callTool(t, tool, `{"query":"创造性 三步法"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hr := failResult(t, v)
	if hr.Success {
		t.Fatalf("want failure, got success: %+v", hr)
	}
	if hr.Action != "判例搜索不可用" {
		t.Errorf("Action = %q, want 判例搜索不可用", hr.Action)
	}
}

func TestKnowledgeNoteSaveTool_NilWritable(t *testing.T) {
	tool := NewKnowledgeNoteSaveTool(nil)
	v, err := callTool(t, tool, `{"title":"t","content":"c"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hr := failResult(t, v)
	if hr.Success {
		t.Fatalf("want failure, got success: %+v", hr)
	}
	if hr.Action != "笔记保存不可用" {
		t.Errorf("Action = %q, want 笔记保存不可用", hr.Action)
	}
}

func TestKnowledgeNoteSaveTool_Persist(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "user.db")
	w, err := ksqlite.OpenWritable(dbPath, &testEmbedder{dim: 8}, "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer func() { _ = w.Close() }()

	tool := NewKnowledgeNoteSaveTool(w)
	v, err := callTool(t, tool, `{"title":"OA要点","content":"新颖性判断三步法"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("want string result, got %T: %v", v, v)
	}
	if !strings.Contains(s, "已沉淀笔记") {
		t.Errorf("Result = %q, want substring 已沉淀笔记", s)
	}
}
