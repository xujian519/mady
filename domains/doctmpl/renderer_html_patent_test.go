package doctmpl

import (
	"strings"
	"testing"
)

func TestHTMLRenderer_PatentStyleBlock(t *testing.T) {
	r := &HTMLRenderer{}
	out, err := r.Render("# 测试", RenderMeta{Style: &RenderStyle{Name: "patent-standard"}})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html := string(out)
	for _, want := range []string{
		".verdict-table", ".doc-meta", ".callout",
		"@page { size: A4", "#1f3a5f", "FangSong", "--patent-danger",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("patent style missing %q", want)
		}
	}
	// 默认样式变量不应混入专利样式块。
	if strings.Contains(html, "--fg:#222") {
		t.Errorf("patent style must not include default vars")
	}
}

func TestHTMLRenderer_PatentStyleSatiName(t *testing.T) {
	r := &HTMLRenderer{}
	out, err := r.Render("# 测试", RenderMeta{Style: &RenderStyle{Name: "sati"}})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(string(out), ".verdict-table") {
		t.Errorf("sati style should select the patent stylesheet")
	}
}

func TestHTMLRenderer_DefaultStyleUnchanged(t *testing.T) {
	r := &HTMLRenderer{}
	out, err := r.Render("# 测试", RenderMeta{Style: &RenderStyle{Disclaimer: "免责内容"}})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html := string(out)
	if !strings.Contains(html, "--fg:#222") {
		t.Errorf("default stylesheet missing")
	}
	for _, forbid := range []string{".verdict-table", "@page { size: A4", "--patent-navy"} {
		if strings.Contains(html, forbid) {
			t.Errorf("default render must not use patent block (got %q)", forbid)
		}
	}
}
