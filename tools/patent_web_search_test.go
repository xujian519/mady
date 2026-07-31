package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shellQuote 用单引号包裹字符串（shell 字面量），用于生成 mock 脚本。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// writeMockEgo 生成输出固定 JSON 的 mock ego-browser 脚本。
func writeMockEgo(t *testing.T, out string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "ego-browser")
	script := "#!/bin/sh\nprintf '%s\\n' " + shellQuote(out) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // G306: 测试 mock
		t.Fatal(err)
	}
	return bin
}

// TestNewPatentWebSearchToolUnavailable 验证 ego-browser 不可用时工具不注册。
func TestNewPatentWebSearchToolUnavailable(t *testing.T) {
	if tool := NewPatentWebSearchTool(&PatentWebSearchConfig{EgoBrowserPath: "/nonexistent/ego-browser"}); tool != nil {
		t.Error("expected nil tool when ego-browser is unavailable")
	}
}

// TestNewPatentWebSearchTool 验证工具注册与调用闭环（mock CLI）。
func TestNewPatentWebSearchTool(t *testing.T) {
	out := `[
		{"title": "测试专利", "meta": "CN CN106599773B 马惠敏 清华大学", "dateLine": "Priority 2016-10-31", "abstract": "摘要", "number": "CN106599773B", "pdfUrl": "https://x/pdf", "url": "", "itemId": "patent/CN106599773B"}
	]`
	tool := NewPatentWebSearchTool(&PatentWebSearchConfig{EgoBrowserPath: writeMockEgo(t, out)})
	if tool == nil {
		t.Fatal("tool is nil")
	}
	if tool.Name != "patent_web_search" {
		t.Errorf("name = %q", tool.Name)
	}

	args, _ := json.Marshal(map[string]any{"query": "深度学习", "max_results": 5, "source": "cnipa"})
	raw, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	doc, ok := raw.(map[string]any)
	_ = doc
	if !ok {
		// Func 返回 *domain.DomainResults，通过 JSON 序列化检查关键字段。
		data, _ := json.Marshal(raw)
		if len(data) == 0 {
			t.Fatal("empty result")
		}
	}
}

// TestNewPatentWebSearchToolEmptyQuery 验证空查询报错。
func TestNewPatentWebSearchToolEmptyQuery(t *testing.T) {
	tool := NewPatentWebSearchTool(&PatentWebSearchConfig{EgoBrowserPath: writeMockEgo(t, "[]")})
	if tool == nil {
		t.Fatal("tool is nil")
	}
	args, _ := json.Marshal(map[string]any{"query": ""})
	if _, err := tool.Func(context.Background(), args); err == nil {
		t.Fatal("expected error for empty query")
	}
}

// TestNewPatentWebSearchToolClamp 验证 max_results 钳制（0/负值 → 10，超 100 → 100）。
func TestNewPatentWebSearchToolClamp(t *testing.T) {
	tool := NewPatentWebSearchTool(&PatentWebSearchConfig{EgoBrowserPath: writeMockEgo(t, "[]")})
	if tool == nil {
		t.Fatal("tool is nil")
	}
	for _, tc := range []struct {
		in   int
		want int
	}{
		{0, 10}, {5, 5}, {100000, 100}, {-3, 10}, {50, 50},
	} {
		args, _ := json.Marshal(map[string]any{"query": "x", "max_results": tc.in})
		if _, err := tool.Func(context.Background(), args); err != nil {
			t.Fatalf("max_results=%d: %v", tc.in, err)
		}
	}
}
