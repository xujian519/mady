package doctmpl

import (
	"strings"
	"testing"
)

func TestTemplateStore_Load(t *testing.T) {
	store, err := NewTemplateStore()
	if err != nil {
		t.Fatal(err)
	}
	// Should have at least the embedded templates.
	if store.Count() < 8 {
		t.Fatalf("count = %d, want at least 8", store.Count())
	}
}

func TestTemplateStore_List(t *testing.T) {
	store, err := NewTemplateStore()
	if err != nil {
		t.Fatal(err)
	}

	all := store.List(ListOptions{})
	if len(all) != store.Count() {
		t.Fatalf("len = %d, Count = %d", len(all), store.Count())
	}

	claims := store.List(ListOptions{Category: "claims"})
	if len(claims) < 3 {
		t.Fatalf("claims count = %d", len(claims))
	}
	for _, c := range claims {
		if c.Category != "claims" {
			t.Errorf("%s: category = %q", c.Name, c.Category)
		}
	}

	// Non-existent domain returns empty.
	none := store.List(ListOptions{Domain: "nonexistent"})
	if len(none) != 0 {
		t.Fatalf("expected 0 for unknown domain, got %d", len(none))
	}
}

func TestTemplateStore_FindByName(t *testing.T) {
	store, err := NewTemplateStore()
	if err != nil {
		t.Fatal(err)
	}

	tmpl, ok := store.FindByName("method-claim")
	if !ok {
		t.Fatal("method-claim not found")
	}
	if tmpl.Category != "claims" {
		t.Errorf("category = %q", tmpl.Category)
	}

	_, ok = store.FindByName("nonexistent-template")
	if ok {
		t.Fatal("found non-existent template")
	}
}

func TestTemplateStore_Render(t *testing.T) {
	store, err := NewTemplateStore()
	if err != nil {
		t.Fatal(err)
	}

	// Render a simple markdown template.
	output, err := store.Render("method-claim", map[string]string{
		"method_name":   "一种图像处理方法",
		"step_1":        "获取输入图像",
		"step_2":        "对图像进行预处理",
		"step_3":        "输出处理结果",
		"step_1_detail": "对输入图像进行归一化处理",
		"step_4":        "对处理结果进行后处理",
		"key_param":     "归一化系数",
		"range_value":   "0.1-0.9",
	}, FormatMarkdown, RenderMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "一种图像处理方法") {
		t.Error("missing method name")
	}
	if strings.Contains(string(output), "{{") {
		t.Error("unresolved variables remain")
	}
}

func TestTemplateStore_RenderNotFound(t *testing.T) {
	store, _ := NewTemplateStore()
	_, err := store.Render("nonexistent", nil, FormatMarkdown, RenderMeta{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTemplateStore_RenderUnsupportedFormat(t *testing.T) {
	store, _ := NewTemplateStore()
	// method-claim template supports only markdown (default).
	_, err := store.Render("method-claim", map[string]string{"method_name": "test"}, FormatPDF, RenderMeta{})
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestTemplateStore_RendererRegistry(t *testing.T) {
	store, _ := NewTemplateStore()
	reg := store.RendererRegistry()
	if !reg.Has(FormatMarkdown) {
		t.Error("default store should have markdown renderer")
	}
}

func TestTemplateStore_DocIndex(t *testing.T) {
	store, _ := NewTemplateStore()
	idx := store.DocIndex()
	if idx == "" {
		t.Fatal("DocIndex should not be empty")
	}
	if !strings.Contains(idx, "claims") {
		t.Error("missing claims in index")
	}
}

// TestStoreRender_VariablesAreHTMLEscaped 验证 HTML 输出中模板变量值被 HTML 实体转义。
// 注入包含 <script> 标签的变量值，确认不会原样出现在 HTML 中（XSS 防护）。
// 这是 R13-1 的回归测试：LLM 控制模板变量时阻断注入链。
func TestStoreRender_VariablesAreHTMLEscaped(t *testing.T) {
	store, err := NewTemplateStore()
	if err != nil {
		t.Fatalf("NewTemplateStore failed: %v", err)
	}

	// 注入一个测试模板，使用 {{payload}} 变量。
	tmpl := DocTemplate{
		Name:             "__html_injection_test__",
		Category:         "disclosure",
		Domain:           "patent",
		Language:         "zh-CN",
		Title:            "HTML 注入测试",
		Body:             "# 发明名称\n\n{{payload}}",
		SupportedFormats: []OutputFormat{FormatHTML},
		VarSchema: NewVarSchema([]VarDefinition{
			{Name: "payload", Required: true},
		}),
	}
	store.mu.Lock()
	store.add(&tmpl)
	store.mu.Unlock()

	malicious := `<script>alert('xss')</script>`
	out, err := store.Render(tmpl.Name, map[string]string{"payload": malicious}, FormatHTML, RenderMeta{})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	html := string(out)
	if strings.Contains(html, malicious) {
		t.Fatalf("malicious variable value rendered raw in HTML: %s", html)
	}
	// 实体转义后的文本应出现在输出中。
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("expected escaped script tag in HTML, got: %s", html)
	}
}
