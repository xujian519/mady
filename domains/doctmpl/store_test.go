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
