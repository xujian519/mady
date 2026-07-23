package doctmpl

import (
	"strings"
	"testing"
)

func TestPDFRenderer_Format(t *testing.T) {
	r := &PDFRenderer{}
	if r.Format() != FormatPDF {
		t.Fatalf("expected FormatPDF, got %v", r.Format())
	}
}

// TestPDFRenderer_Render 验证中文 Markdown 能生成有效 PDF（依赖本机中文字体）。
func TestPDFRenderer_Render(t *testing.T) {
	r := &PDFRenderer{}
	md := "# 测试报告\n\n这是一段中文段落，用于验证 PDF 渲染能力。\n\n" +
		"- 项目一\n- 项目二\n\n" +
		"| 名称 | 数值 |\n| --- | --- |\n| 甲 | 1 |\n| 乙 | 2 |\n"
	data, err := r.Render(md, RenderMeta{Title: "交底书分析报告"})
	if err != nil {
		// 无中文字体的环境跳过，而非失败。
		if strings.Contains(err.Error(), "未找到") {
			t.Skipf("跳过：本机无可用中文字体 (%v)", err)
		}
		t.Fatalf("Render: %v", err)
	}
	if len(data) < 100 {
		t.Fatalf("PDF too small: %d bytes", len(data))
	}
	if string(data[:4]) != "%PDF" {
		t.Fatalf("expected PDF magic header, got %q", data[:8])
	}
	if !bytesEndWithEOF(data) {
		t.Error("expected PDF to contain EOF marker")
	}
}

// bytesEndWithEOF 检查 PDF 是否以 %%EOF 结尾（宽松匹配尾部）。
func bytesEndWithEOF(data []byte) bool {
	if len(data) > 1024 {
		data = data[len(data)-1024:]
	}
	return strings.Contains(string(data), "%%EOF")
}

func TestPDFRenderer_NoFont(t *testing.T) {
	r := &PDFRenderer{FontPath: "/nonexistent/font.ttf"}
	_, err := r.Render("测试", RenderMeta{})
	if err == nil {
		t.Fatal("expected error for missing font file")
	}
	if !strings.Contains(err.Error(), "字体") {
		t.Fatalf("expected font-related error message, got: %v", err)
	}
}

func TestPDFRenderer_Nil(t *testing.T) {
	var r *PDFRenderer
	_, err := r.Render("x", RenderMeta{})
	if err == nil {
		t.Fatal("expected error for nil renderer")
	}
}
