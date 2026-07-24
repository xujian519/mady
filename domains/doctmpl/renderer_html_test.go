package doctmpl

import (
	"strings"
	"testing"
)

func TestHTMLRenderer_Format(t *testing.T) {
	r := &HTMLRenderer{}
	if r.Format() != FormatHTML {
		t.Fatalf("expected FormatHTML, got %v", r.Format())
	}
}

func TestHTMLRenderer_NilReceiver(t *testing.T) {
	var r *HTMLRenderer
	_, err := r.Render("# test", RenderMeta{})
	if err == nil {
		t.Fatal("expected error for nil receiver")
	}
}

func TestHTMLRenderer_BasicMarkdown(t *testing.T) {
	r := &HTMLRenderer{}
	md := "# 标题\n\n段落文本。\n\n- 项目一\n- 项目二\n"
	out, err := r.Render(md, RenderMeta{})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html := string(out)
	for _, want := range []string{"<h1", "项目一", "<ul", "<li"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected %q in output", want)
		}
	}
}

func TestHTMLRenderer_FullDocument(t *testing.T) {
	r := &HTMLRenderer{}
	md := "# 技术标题\n\n这是一段**加粗**文字。\n\n| 名称 | 数值 |\n| --- | --- |\n| A | 1 |\n\n```go\nfmt.Println()\n```\n"
	out, err := r.Render(md, RenderMeta{
		Title:  "测试文档",
		Author: "张三",
		Style:  &RenderStyle{Disclaimer: "免责内容"},
	})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html := string(out)

	for _, want := range []string{
		"<!DOCTYPE html>",
		`<html lang="zh-CN">`,
		"<meta charset=\"UTF-8\">",
		"<title>测试文档</title>",
		`<meta name="author" content="张三">`,
		"<style>",
		"<body>",
		"<h1>测试文档</h1>",
		"<strong>加粗</strong>",
		"<table>",
		"<th",
		"<pre><code",
		"免责内容",
		"</body>\n</html>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected %q in HTML output", want)
		}
	}
}

func TestHTMLRenderer_Language(t *testing.T) {
	r := &HTMLRenderer{}
	out, err := r.Render("hello", RenderMeta{Language: "en"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(string(out), `<html lang="en">`) {
		t.Errorf("expected lang=en in HTML output")
	}
}

func TestHTMLRenderer_GFMFeatures(t *testing.T) {
	r := &HTMLRenderer{}
	md := "~~删除线~~\n\n- [x] 已完成\n- [ ] 未完成\n"
	out, err := r.Render(md, RenderMeta{})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html := string(out)
	if !strings.Contains(html, "<del>删除线</del>") && !strings.Contains(html, "<s>删除线</s>") {
		t.Errorf("expected strikethrough for ~~删除线~~")
	}
	if !strings.Contains(html, "已完成") || !strings.Contains(html, "未完成") {
		t.Errorf("expected task list items")
	}
}
