package search

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval/domain"
)

// fakeRetriever 固定返回两篇文档的测试检索器。
type fakeRetriever struct {
	docs []domain.DomainDocument
}

func (f *fakeRetriever) Search(_ context.Context, q domain.DomainQuery) (*domain.DomainResults, error) {
	return &domain.DomainResults{
		Query:      q,
		Documents:  f.docs,
		TotalCount: len(f.docs),
		Source:     "fake",
	}, nil
}

func (f *fakeRetriever) GetDocument(_ context.Context, _ string) (*domain.DomainDocument, error) {
	return nil, nil
}

func (f *fakeRetriever) SourceName() string { return "fake" }

func TestNewCommanderToolNilRetriever(t *testing.T) {
	if tool := NewCommanderTool(nil); tool != nil {
		t.Fatal("expected nil tool for nil retriever")
	}
}

func TestNewCommanderToolFunc(t *testing.T) {
	f := &fakeRetriever{docs: []domain.DomainDocument{
		doc("CN100A", "深度学习图像识别装置", "华为"),
	}}
	tool := NewCommanderTool(&CommanderToolConfig{Retriever: f})
	if tool == nil {
		t.Fatal("tool is nil")
	}
	if tool.Name != ToolName {
		t.Errorf("name = %q, want %q", tool.Name, ToolName)
	}
	if !tool.ReadOnly {
		t.Error("commander tool should be read-only")
	}

	args, _ := json.Marshal(map[string]any{
		"query":   "深度学习图像识别",
		"scene":   "oa",
		"ipcs":    []string{"G06F 17/30"},
		"country": "cn",
	})
	raw, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	md, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string markdown, got %T", raw)
	}
	for _, want := range []string{"# 检索指挥官报告", "深度学习图像识别", "CN100A"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
}

func TestNewCommanderToolEmptyQuery(t *testing.T) {
	f := &fakeRetriever{}
	tool := NewCommanderTool(&CommanderToolConfig{Retriever: f})
	args, _ := json.Marshal(map[string]any{"query": ""})
	if _, err := tool.Func(context.Background(), args); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestCommanderExtensionRegistration(t *testing.T) {
	f := &fakeRetriever{docs: []domain.DomainDocument{doc("CN1", "甲", "华为")}}
	ext := NewCommanderExtension(f)
	if ext.Name() != "search-commander" {
		t.Errorf("extension name = %q", ext.Name())
	}
	tools := ext.Tools()
	if len(tools) != 1 || tools[0].Name != ToolName {
		t.Fatalf("Tools() = %+v, want [patent_search_commander]", tools)
	}

	// 通过 agentcore.Agent 注册闭环验证（无 Provider 的纯工具注册）。
	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{Name: "test-agent", Model: "test-model"},
	})
	if err := ext.Init(context.Background(), agent); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, ok := agent.GetTool(ToolName); !ok {
		t.Fatal("patent_search_commander should be registered on agent")
	}
	if err := ext.Dispose(); err != nil {
		t.Fatalf("Dispose: %v", err)
	}
}

func TestCommanderExtensionNilRetriever(t *testing.T) {
	ext := NewCommanderExtension(nil)
	if len(ext.Tools()) != 0 {
		t.Fatalf("expected no tools for nil retriever, got %d", len(ext.Tools()))
	}
	if err := ext.Init(context.Background(), nil); err != nil {
		t.Fatalf("Init with nil agent should be no-op, got %v", err)
	}
}
