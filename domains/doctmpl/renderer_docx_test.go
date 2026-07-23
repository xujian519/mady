package doctmpl

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestDOCXRenderer_Format(t *testing.T) {
	r := &DOCXRenderer{}
	if r.Format() != FormatDOCX {
		t.Fatalf("expected FormatDOCX, got %v", r.Format())
	}
}

func TestDOCXRenderer_Render(t *testing.T) {
	md := "# 技术标题\n\n这是一段**加粗**文字。\n\n- 列表项一\n- 列表项二\n\n" +
		"| 名称 | 数值 |\n| --- | --- |\n| A | 1 |\n| B | 2 |\n"
	r := &DOCXRenderer{}
	data, err := r.Render(md, RenderMeta{Title: "交底书报告", Author: "测试"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty docx output")
	}

	// 读回 zip，提取 document.xml。
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip read: %v", err)
	}
	var docXML string
	foundCT := false
	for _, f := range zr.File {
		switch f.Name {
		case "[Content_Types].xml":
			foundCT = true
		case "word/document.xml":
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open document.xml: %v", err)
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("read document.xml: %v", err)
			}
			docXML = string(b)
		}
	}
	if !foundCT {
		t.Fatal("missing [Content_Types].xml")
	}
	if docXML == "" {
		t.Fatal("missing word/document.xml")
	}

	// 标题注入
	if !strings.Contains(docXML, "交底书报告") {
		t.Error("expected injected meta.Title")
	}
	// 正文标题
	if !strings.Contains(docXML, "技术标题") {
		t.Error("expected heading text")
	}
	// 行内加粗
	if !strings.Contains(docXML, "<w:b/>") {
		t.Error("expected bold run (w:b)")
	}
	// 表格内容
	for _, want := range []string{"名称", "数值", "名称"} {
		if !strings.Contains(docXML, want) {
			t.Errorf("expected table content %q in document.xml", want)
		}
	}
	// 表格边框
	if !strings.Contains(docXML, "<w:tbl>") {
		t.Error("expected <w:tbl>")
	}
	// 列表项
	if !strings.Contains(docXML, "列表项一") || !strings.Contains(docXML, "•") {
		t.Error("expected list items with bullet")
	}
	// XML 特殊字符转义（标题里的"加粗"不在标题，但文字里有）
}

func TestDOCXRenderer_XMLEscape(t *testing.T) {
	r := &DOCXRenderer{}
	data, err := r.Render("a < b & c > d", RenderMeta{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			s := string(b)
			if !strings.Contains(s, "&lt;") || !strings.Contains(s, "&amp;") || !strings.Contains(s, "&gt;") {
				t.Errorf("expected escaped XML entities, got %q", s)
			}
			// 不应出现裸的特殊字符（在文本内容里）
			if strings.Contains(s, "a < b") {
				t.Error("unescaped < in text")
			}
			return
		}
	}
	t.Fatal("document.xml not found")
}

func TestDOCXRenderer_Nil(t *testing.T) {
	var r *DOCXRenderer
	_, err := r.Render("x", RenderMeta{})
	if err == nil {
		t.Fatal("expected error for nil renderer")
	}
}
