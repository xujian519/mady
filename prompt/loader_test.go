package prompt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPrompts_Valid(t *testing.T) {
	root := t.TempDir()
	mustWritePromptTemplate(t, filepath.Join(root, "analysis", "novelty.json"), PromptTemplate{
		Name:               "novelty-analysis",
		Title:              "新颖性分析",
		Version:            "0.1.0",
		Description:        "专利新颖性分析",
		Domain:             "patent",
		Category:           "analysis",
		Triggers:           []string{"新颖性分析", "novelty search"},
		SystemPrompt:       "你是一个专利审查员",
		UserPromptTemplate: "请分析：{{description}}",
	})
	mustWritePromptTemplate(t, filepath.Join(root, "search", "keyword.json"), PromptTemplate{
		Name:               "keyword-search",
		Title:              "关键词检索",
		Version:            "0.1.0",
		Description:        "关键词检索式生成",
		Domain:             "patent",
		Category:           "search",
		Triggers:           []string{"检索式生成", "keyword search"},
		SystemPrompt:       "你是一个检索分析师",
		UserPromptTemplate: "技术领域：{{field}}",
	})

	templates, err := LoadPrompts(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(templates))
	}
}

func TestLoadPrompts_SkipsInvalid(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "bad.json"), []byte(`{"title":"bad"}`), 0o644)

	templates, err := LoadPrompts(root)
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
	if templates != nil {
		t.Fatal("expected nil templates on error")
	}
}

func TestResolvePrompt(t *testing.T) {
	tmpl := PromptTemplate{
		Name:               "test",
		SystemPrompt:       "You are a helpful assistant.",
		UserPromptTemplate: "请分析专利{{patent_number}}，技术方案：{{description}}",
	}
	resolved := ResolvePrompt(tmpl, map[string]string{
		"patent_number": "CN109690000A",
		"description":   "一种智能温控装置",
	})
	if !strings.Contains(resolved.UserPrompt, "CN109690000A") {
		t.Fatalf("expected patent number: %s", resolved.UserPrompt)
	}
	if !strings.Contains(resolved.UserPrompt, "智能温控装置") {
		t.Fatalf("expected description: %s", resolved.UserPrompt)
	}
	if strings.Contains(resolved.UserPrompt, "{{") {
		t.Fatalf("unresolved variable: %s", resolved.UserPrompt)
	}
	if resolved.SystemPrompt != "You are a helpful assistant." {
		t.Fatalf("system prompt changed: %s", resolved.SystemPrompt)
	}
}

func TestFindPromptByTrigger(t *testing.T) {
	templates := []PromptTemplate{
		{Name: "novelty", Triggers: []string{"新颖性分析", "novelty search"}},
		{Name: "keyword", Triggers: []string{"检索式生成"}},
		{Name: "infringement", Triggers: []string{"侵权分析", "FTO"}},
	}
	matched := FindPromptByTrigger(templates, "retrieval")
	if len(matched) != 0 {
		t.Fatalf("expected no match, got %d", len(matched))
	}
	matched = FindPromptByTrigger(templates, "新颖")
	if len(matched) != 1 || matched[0].Name != "novelty" {
		t.Fatalf("expected novelty, got %v", matched)
	}
}

func TestFindPromptByName(t *testing.T) {
	templates := []PromptTemplate{
		{Name: "novelty-analysis"},
		{Name: "keyword-search"},
	}
	tmpl, ok := FindPromptByName(templates, "novelty-analysis")
	if !ok || tmpl.Name != "novelty-analysis" {
		t.Fatal("not found")
	}
	_, ok = FindPromptByName(templates, "nonexistent")
	if ok {
		t.Fatal("unexpected found")
	}
}

func TestPromptIndex(t *testing.T) {
	templates := []PromptTemplate{
		{Name: "novelty", Category: "analysis", Description: "新颖性分析"},
		{Name: "keyword", Category: "search", Description: "关键词检索"},
	}
	idx := PromptIndex(templates)
	if !strings.Contains(idx, "novelty") || !strings.Contains(idx, "keyword") {
		t.Fatalf("index = %s", idx)
	}
	if PromptIndex(nil) != "" {
		t.Fatal("expected empty index")
	}
}

func TestLoadPrompts_DuplicateName(t *testing.T) {
	root := t.TempDir()
	mustWritePromptTemplate(t, filepath.Join(root, "a", "first.json"), PromptTemplate{
		Name:               "my-tmpl",
		SystemPrompt:       "first",
		UserPromptTemplate: "test",
	})
	mustWritePromptTemplate(t, filepath.Join(root, "b", "second.json"), PromptTemplate{
		Name:               "my-tmpl",
		SystemPrompt:       "second",
		UserPromptTemplate: "test",
	})

	templates, err := LoadPrompts(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 (first wins), got %d", len(templates))
	}
	if templates[0].SystemPrompt != "first" {
		t.Fatalf("expected first, got %s", templates[0].SystemPrompt)
	}
}

func mustWritePromptTemplate(t *testing.T, path string, tmpl PromptTemplate) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
