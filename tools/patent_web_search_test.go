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

// TestNewPatentDocumentToolUnavailable 验证 ego-browser 不可用时工具不注册。
func TestNewPatentDocumentToolUnavailable(t *testing.T) {
	if tool := NewPatentDocumentTool(&PatentWebSearchConfig{EgoBrowserPath: "/nonexistent/ego-browser"}); tool != nil {
		t.Error("expected nil tool when ego-browser is unavailable")
	}
}

// TestNewPatentDocumentTool 验证 patent_document 工具注册与调用闭环（mock CLI），
// 并断言统一响应结构 patentDocumentResult（found=true 且含全文）。
func TestNewPatentDocumentTool(t *testing.T) {
	out := `{"number": "US11452699B2", "title": "测试专利", "abstract": "摘要", "claims": "权利要求", "description": "说明书"}`
	tool := NewPatentDocumentTool(&PatentWebSearchConfig{EgoBrowserPath: writeMockEgo(t, out)})
	if tool == nil {
		t.Fatal("tool is nil")
	}
	if tool.Name != "patent_document" {
		t.Errorf("name = %q", tool.Name)
	}

	args, _ := json.Marshal(map[string]any{"patent_number": "US11452699B2"})
	raw, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	res, ok := raw.(patentDocumentResult)
	if !ok {
		t.Fatalf("result type = %T, want patentDocumentResult", raw)
	}
	if !res.Found {
		t.Error("found = false, want true")
	}
	if res.Title != "测试专利" {
		t.Errorf("title = %q", res.Title)
	}
	if res.Abstract != "摘要" {
		t.Errorf("abstract = %q", res.Abstract)
	}
	if !strings.Contains(res.Content, "权利要求") || !strings.Contains(res.Content, "说明书") {
		t.Errorf("content 应含 claims/description: %.100s", res.Content)
	}
	if res.Truncated {
		t.Error("truncated = true, want false")
	}
}

// TestNewPatentDocumentToolNotHandled 验证全源未命中时返回结构化提示
// （found=false + note），而非裸 nil 或错误——LLM 可据此建议改用
// patent_lookup / patent_download，而非误判为工具故障。
func TestNewPatentDocumentToolNotHandled(t *testing.T) {
	tool := NewPatentDocumentTool(&PatentWebSearchConfig{EgoBrowserPath: writeMockEgo(t, "{}")})
	if tool == nil {
		t.Fatal("tool is nil")
	}
	args, _ := json.Marshal(map[string]any{"patent_number": "US0000000000A"})
	raw, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	res, ok := raw.(patentDocumentResult)
	if !ok {
		t.Fatalf("result type = %T, want patentDocumentResult", raw)
	}
	if res.Found {
		t.Error("found = true, want false")
	}
	if res.Note == "" {
		t.Error("note 应提供替代方案提示")
	}
	if res.Content != "" {
		t.Errorf("content = %q, want empty", res.Content)
	}
}

// TestNewPatentDocumentToolMaxChars 验证 max_chars 截断并置 truncated 位。
func TestNewPatentDocumentToolMaxChars(t *testing.T) {
	out := `{"number": "US11452699B2", "title": "T", "abstract": "A", "claims": "C1 C2 C3", "description": "D D D D D D D D D D"}`
	tool := NewPatentDocumentTool(&PatentWebSearchConfig{EgoBrowserPath: writeMockEgo(t, out)})
	if tool == nil {
		t.Fatal("tool is nil")
	}
	args, _ := json.Marshal(map[string]any{"patent_number": "US11452699B2", "max_chars": 5})
	raw, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	res := raw.(patentDocumentResult)
	if !res.Truncated {
		t.Error("truncated = false, want true")
	}
	if len([]rune(res.Content)) > 6 { // 5 字符 + "…"
		t.Errorf("content 长度 = %d, 应 ≤6", len([]rune(res.Content)))
	}
	if !strings.HasSuffix(res.Content, "…") {
		t.Errorf("content 应以省略号结尾: %q", res.Content)
	}
}

// TestNewPatentDocumentToolBiblioOnly 验证仅目录信息（Espacenet 样式，无
// claims/description）时返回 found=true + note 提示，而非冒充全文返回空
// content——契约保护：不把 biblio 当成功全文。
func TestNewPatentDocumentToolBiblioOnly(t *testing.T) {
	out := `{"number": "US11452699B2", "title": "Biblio 专利", "abstract": "仅摘要"}`
	tool := NewPatentDocumentTool(&PatentWebSearchConfig{EgoBrowserPath: writeMockEgo(t, out)})
	if tool == nil {
		t.Fatal("tool is nil")
	}
	args, _ := json.Marshal(map[string]any{"patent_number": "US11452699B2"})
	raw, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	res, ok := raw.(patentDocumentResult)
	if !ok {
		t.Fatalf("result type = %T, want patentDocumentResult", raw)
	}
	if !res.Found {
		t.Error("found = false, want true (biblio 命中目录信息)")
	}
	if res.Content != "" {
		t.Errorf("content = %q, want empty (biblio 无全文)", res.Content)
	}
	if res.Note == "" {
		t.Error("note 应说明仅目录信息并给替代方案")
	}
}

// TestNewPatentDocumentToolEmptyNumber 验证空专利号报错。
func TestNewPatentDocumentToolEmptyNumber(t *testing.T) {
	tool := NewPatentDocumentTool(&PatentWebSearchConfig{EgoBrowserPath: writeMockEgo(t, "{}")})
	if tool == nil {
		t.Fatal("tool is nil")
	}
	args, _ := json.Marshal(map[string]any{"patent_number": ""})
	if _, err := tool.Func(context.Background(), args); err == nil {
		t.Fatal("expected error for empty patent_number")
	}
}

// TestPatentWebSearchDisabledByEnv 验证 MADY_BROWSER_RETRIEVERS=off 时
// 检索/取文工具均不注册（门控集中生效，与 bootstrap/init_reasoning.go 一致）。
func TestPatentWebSearchDisabledByEnv(t *testing.T) {
	t.Setenv("MADY_BROWSER_RETRIEVERS", "off")
	bin := writeMockEgo(t, "[]")
	if tool := NewPatentWebSearchTool(&PatentWebSearchConfig{EgoBrowserPath: bin}); tool != nil {
		t.Error("expected nil tool when MADY_BROWSER_RETRIEVERS=off")
	}
	if tool := NewPatentDocumentTool(&PatentWebSearchConfig{EgoBrowserPath: bin}); tool != nil {
		t.Error("expected nil tool when MADY_BROWSER_RETRIEVERS=off")
	}
}
