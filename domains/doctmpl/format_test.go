package doctmpl

import (
	"strings"
	"testing"
)

func TestOutputFormat_IsValid(t *testing.T) {
	tests := []struct {
		format OutputFormat
		valid  bool
	}{
		{FormatMarkdown, true},
		{FormatDOCX, true},
		{FormatPDF, true},
		{FormatHTML, true},
		{FormatEmail, true},
		{OutputFormat(""), false},
		{OutputFormat("unknown"), false},
		{OutputFormat("json"), false},
	}
	for _, tt := range tests {
		if got := tt.format.IsValid(); got != tt.valid {
			t.Errorf("%q.IsValid() = %v, want %v", tt.format, got, tt.valid)
		}
	}
}

func TestOutputFormat_Ext(t *testing.T) {
	tests := []struct {
		format OutputFormat
		ext    string
	}{
		{FormatMarkdown, ".md"},
		{FormatDOCX, ".docx"},
		{FormatPDF, ".pdf"},
		{FormatHTML, ".html"},
		{FormatEmail, ".eml"},
		{OutputFormat("unknown"), ""},
	}
	for _, tt := range tests {
		if got := tt.format.Ext(); got != tt.ext {
			t.Errorf("%q.Ext() = %q, want %q", tt.format, got, tt.ext)
		}
	}
}

func TestOutputFormat_MIME(t *testing.T) {
	tests := []struct {
		format OutputFormat
		mime   string
	}{
		{FormatMarkdown, "text/markdown"},
		{FormatDOCX, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{FormatPDF, "application/pdf"},
		{FormatHTML, "text/html"},
		{FormatEmail, "message/rfc822"},
		{OutputFormat("unknown"), "application/octet-stream"},
	}
	for _, tt := range tests {
		if got := tt.format.MIME(); got != tt.mime {
			t.Errorf("%q.MIME() = %q, want %q", tt.format, got, tt.mime)
		}
	}
}

func TestParseDocTemplate_Formats(t *testing.T) {
	// Template with formats field using YAML list syntax.
	data := []byte(`---
name: test
title: 测试
category: claims
description: 测试模板
domain: patent
formats:
  - markdown
  - docx
---
# {{title}}
`)
	tmpl, err := parseDocTemplate("test.md", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tmpl.SupportedFormats) != 2 {
		t.Fatalf("SupportedFormats len = %d, want 2", len(tmpl.SupportedFormats))
	}
	if tmpl.SupportedFormats[0] != FormatMarkdown {
		t.Errorf("SupportedFormats[0] = %q", tmpl.SupportedFormats[0])
	}
	if tmpl.SupportedFormats[1] != FormatDOCX {
		t.Errorf("SupportedFormats[1] = %q", tmpl.SupportedFormats[1])
	}
}

func TestParseDocTemplate_FormatsDefault(t *testing.T) {
	data := []byte(`---
name: test
title: 测试
category: claims
domain: patent
---
# {{title}}
`)
	tmpl, err := parseDocTemplate("test.md", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tmpl.SupportedFormats) != 1 {
		t.Fatalf("len = %d, want 1", len(tmpl.SupportedFormats))
	}
	if tmpl.SupportedFormats[0] != FormatMarkdown {
		t.Errorf("default = %q, want markdown", tmpl.SupportedFormats[0])
	}
}

func TestParseDocTemplate_StyleName(t *testing.T) {
	data := []byte(`---
name: test
title: 测试
category: claims
domain: patent
style: patent-standard
---
# {{title}}
`)
	tmpl, err := parseDocTemplate("test.md", data)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.StyleName != "patent-standard" {
		t.Errorf("StyleName = %q", tmpl.StyleName)
	}
}

func TestParseDocTemplate_Vars(t *testing.T) {
	data := []byte(`---
name: test
title: 测试
category: claims
domain: patent
vars:
  - name: title
    type: string
    required: true
    description: 发明名称
  - name: count
    type: number
    required: false
    default: "1"
---
# {{title}}
`)
	tmpl, err := parseDocTemplate("test.md", data)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.VarSchema == nil {
		t.Fatal("VarSchema is nil")
	}
	if tmpl.VarSchema.Empty() {
		t.Fatal("VarSchema is empty")
	}
	if len(tmpl.VarSchema.Names()) != 2 {
		t.Fatalf("names = %v", tmpl.VarSchema.Names())
	}
	def, ok := tmpl.VarSchema.Get("count")
	if !ok {
		t.Fatal("count not found")
	}
	if def.Default != "1" {
		t.Errorf("default = %q", def.Default)
	}
}

func TestMarkdownRenderer(t *testing.T) {
	r := &MarkdownRenderer{}
	if r.Format() != FormatMarkdown {
		t.Errorf("Format() = %q", r.Format())
	}

	t.Run("plain pass-through", func(t *testing.T) {
		out, err := r.Render("hello world", RenderMeta{})
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != "hello world" {
			t.Errorf("output = %q", string(out))
		}
	})

	t.Run("with style disclaimer", func(t *testing.T) {
		style := &RenderStyle{Name: "patent-standard", Disclaimer: "本分析由AI辅助生成。"}
		out, err := r.Render("content", RenderMeta{Style: style})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "本分析由AI辅助生成") {
			t.Error("missing disclaimer")
		}
		if !strings.Contains(string(out), "content") {
			t.Error("missing content")
		}
	})

	t.Run("with nil style", func(t *testing.T) {
		out, err := r.Render("content", RenderMeta{Style: nil})
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != "content" {
			t.Errorf("output = %q", string(out))
		}
	})

	t.Run("with title", func(t *testing.T) {
		out, err := r.Render("content", RenderMeta{Title: "测试文档"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "# 测试文档") {
			t.Errorf("missing title: %q", string(out))
		}
	})

	t.Run("title not duplicated", func(t *testing.T) {
		out, err := r.Render("# Already has title\ncontent", RenderMeta{Title: "Should Not Appear"})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(out), "# Already has title") != 1 {
			t.Error("title was duplicated")
		}
	})
}

func TestRendererRegistry(t *testing.T) {
	reg := NewRendererRegistry()

	// Empty registry.
	if reg.Has(FormatMarkdown) {
		t.Error("should not have markdown yet")
	}
	_, ok := reg.Get(FormatMarkdown)
	if ok {
		t.Error("should not get markdown")
	}
	if len(reg.Formats()) != 0 {
		t.Error("should be empty")
	}

	// Register.
	reg.Register(&MarkdownRenderer{})
	if !reg.Has(FormatMarkdown) {
		t.Error("should have markdown")
	}
	r, ok := reg.Get(FormatMarkdown)
	if !ok || r.Format() != FormatMarkdown {
		t.Error("bad get")
	}
	if len(reg.Formats()) != 1 {
		t.Fatal("len =", len(reg.Formats()))
	}
	if reg.Formats()[0] != FormatMarkdown {
		t.Errorf("formats[0] = %q", reg.Formats()[0])
	}

	// Register nil is no-op.
	reg.Register(nil)
	if len(reg.Formats()) != 1 {
		t.Error("nil should not be registered")
	}

	// Overwrite.
	reg.Register(&MarkdownRenderer{})
	if len(reg.Formats()) != 1 {
		t.Error("overwrite should not increase count")
	}
}

func TestRendererRegistry_Render(t *testing.T) {
	reg := NewRendererRegistry()
	reg.Register(&MarkdownRenderer{})

	out, err := reg.Render(FormatMarkdown, "test", RenderMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "test" {
		t.Errorf("output = %q", string(out))
	}

	// Unregistered format.
	_, err = reg.Render(FormatPDF, "test", RenderMeta{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pdf") {
		t.Errorf("error = %v", err)
	}
}

// Verify embedded templates all parse with at least markdown format.
func TestEmbeddedTemplates_SupportedFormats(t *testing.T) {
	templates, err := LoadDocTemplatesFromFS(embeddedTemplatesFS, embeddedTemplatesDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tmpl := range templates {
		if len(tmpl.SupportedFormats) == 0 {
			t.Errorf("%s: SupportedFormats is empty", tmpl.Name)
		}
		found := false
		for _, f := range tmpl.SupportedFormats {
			if f == FormatMarkdown {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: SupportedFormats must include markdown, got %v", tmpl.Name, tmpl.SupportedFormats)
		}
	}
}
