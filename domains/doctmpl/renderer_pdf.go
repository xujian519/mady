package doctmpl

import (
	"fmt"
	"os"
	"strings"

	"github.com/signintech/gopdf"
)

// PDFRenderer 渲染 Markdown 为 PDF（纯 Go，gopdf，零外部进程依赖）。
// 中文支持依赖嵌入系统 TTF/TTC 中文字体；自动搜索常见路径，也可通过
// FontPath 显式指定。找不到合适字体时返回错误并提示安装字体——此时
// 可降级使用 DOCX/Markdown 渲染器。
type PDFRenderer struct {
	// FontPath 可选：指定 TTF/TTC 字体文件路径，覆盖自动搜索。
	FontPath string
}

// Format 返回 FormatPDF。
func (r *PDFRenderer) Format() OutputFormat { return FormatPDF }

// Render 将 Markdown 正文转换为 PDF 字节流。
func (r *PDFRenderer) Render(md string, meta RenderMeta) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("doctmpl: nil PDF renderer")
	}
	if meta.Style != nil {
		md = meta.Style.ApplyDisclaimer(md)
	}

	fontPath := r.FontPath
	if fontPath == "" {
		fp, ok := findCJKFont()
		if !ok {
			return nil, fmt.Errorf("doctmpl: 未找到可用的中文字体，无法生成 PDF。" +
				"请安装中文字体（如 Noto Sans CJK、Arial Unicode MS、思源黑体），" +
				"或通过 PDFRenderer.FontPath 指定字体文件路径；亦可改用 DOCX/Markdown 格式")
		}
		fontPath = fp
	}

	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4, Unit: gopdf.UnitPT})
	pdf.SetMargins(pdfLeftMargin, pdfTopMargin, pdfLeftMargin, 42)
	pdf.AddPage()

	const family = "main"
	if err := pdf.AddTTFFont(family, fontPath); err != nil {
		return nil, fmt.Errorf("doctmpl: 加载字体 %s 失败: %w", fontPath, err)
	}
	if err := pdf.SetFont(family, "", 11); err != nil {
		return nil, fmt.Errorf("doctmpl: 设置字体失败: %w", err)
	}

	if meta.Title != "" {
		if err := pdfRenderHeading(pdf, meta.Title, 1); err != nil {
			return nil, err
		}
	}
	if err := pdfRenderMarkdown(pdf, md); err != nil {
		return nil, err
	}
	return pdf.GetBytesPdfReturnErr()
}

// ── 布局常量 ───────────────────────────────────────────────────────

const (
	pdfLeftMargin   = 50.0
	pdfTopMargin    = 50.0
	pdfBottomLimit  = 800.0 // A4 高 842pt，留 42pt 底边距
	pdfContentWidth = 495.0 // 595 - 2*50
	pdfBodySize     = 11.0
	pdfLineHeight   = 16.0
)

// ── Markdown → PDF 布局 ────────────────────────────────────────────

func pdfRenderMarkdown(pdf *gopdf.GoPdf, md string) error {
	lines := strings.Split(md, "\n")
	var para []string
	flushPara := func() error {
		if len(para) == 0 {
			return nil
		}
		if err := pdfRenderParagraph(pdf, strings.Join(para, " ")); err != nil {
			return err
		}
		para = para[:0]
		return nil
	}

	i := 0
	for i < len(lines) {
		line := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "|") {
			if err := flushPara(); err != nil {
				return err
			}
			var tableRows [][]string
			for i < len(lines) {
				l := strings.TrimSpace(strings.TrimRight(lines[i], "\r"))
				if !strings.HasPrefix(l, "|") {
					break
				}
				if isTableSeparator(l) {
					i++
					continue
				}
				tableRows = append(tableRows, parseTableRow(l))
				i++
			}
			if len(tableRows) > 0 {
				if err := pdfRenderTable(pdf, tableRows); err != nil {
					return err
				}
			}
			continue
		}

		if lvl, text, ok := parseHeading(trimmed); ok {
			if err := flushPara(); err != nil {
				return err
			}
			if err := pdfRenderHeading(pdf, text, lvl); err != nil {
				return err
			}
			i++
			continue
		}

		if isListItem(trimmed) {
			if err := flushPara(); err != nil {
				return err
			}
			if err := pdfRenderListItem(pdf, strings.TrimSpace(trimmed[2:])); err != nil {
				return err
			}
			i++
			continue
		}

		if trimmed == "" {
			if err := flushPara(); err != nil {
				return err
			}
			i++
			continue
		}

		para = append(para, trimmed)
		i++
	}
	return flushPara()
}

func pdfRenderHeading(pdf *gopdf.GoPdf, text string, level int) error {
	size := pdfBodySize
	switch {
	case level <= 1:
		size = 20
	case level == 2:
		size = 16
	case level == 3:
		size = 13
	}
	if err := pdf.SetFontSize(size); err != nil {
		return err
	}
	pdfEnsureSpace(pdf, size+8)
	if pdf.GetY() > pdfTopMargin {
		pdf.Br(6)
	}
	if err := pdf.MultiCell(&gopdf.Rect{W: pdfContentWidth, H: size + 6}, pdfStripInline(text)); err != nil {
		// MultiCell 失败不致命，回退到 Text
		if te := pdf.Text(pdfStripInline(text)); te != nil {
			return te
		}
	}
	return pdf.SetFontSize(pdfBodySize)
}

func pdfRenderParagraph(pdf *gopdf.GoPdf, text string) error {
	text = pdfStripInline(text)
	if err := pdf.SetFontSize(pdfBodySize); err != nil {
		return err
	}
	h := float64(pdfEstimateLines(text, pdfBodySize, pdfContentWidth)) * pdfLineHeight
	pdfEnsureSpace(pdf, h)
	if err := pdf.MultiCell(&gopdf.Rect{W: pdfContentWidth, H: pdfLineHeight}, text); err != nil {
		return pdf.Text(text)
	}
	pdf.Br(4)
	return nil
}

func pdfRenderListItem(pdf *gopdf.GoPdf, text string) error {
	text = "• " + pdfStripInline(text)
	if err := pdf.SetFontSize(pdfBodySize); err != nil {
		return err
	}
	h := float64(pdfEstimateLines(text, pdfBodySize, pdfContentWidth-15)) * pdfLineHeight
	pdfEnsureSpace(pdf, h)
	x := pdf.GetX()
	pdf.SetX(x + 15)
	if err := pdf.MultiCell(&gopdf.Rect{W: pdfContentWidth - 15, H: pdfLineHeight}, text); err != nil {
		return pdf.Text(text)
	}
	return nil
}

// pdfRenderTable 渲染表格：表头行 + 下划线 + 数据行，按列等宽定位。
func pdfRenderTable(pdf *gopdf.GoPdf, rows [][]string) error {
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	if cols == 0 {
		return nil
	}
	colW := pdfContentWidth / float64(cols)
	if err := pdf.SetFontSize(pdfBodySize); err != nil {
		return err
	}
	for ri, row := range rows {
		pdfEnsureSpace(pdf, pdfLineHeight)
		y := pdf.GetY()
		for ci := 0; ci < cols; ci++ {
			cell := ""
			if ci < len(row) {
				cell = row[ci]
			}
			pdf.SetX(pdfLeftMargin + float64(ci)*colW)
			if err := pdf.Text(pdfStripInline(cell)); err != nil {
				return err
			}
		}
		pdf.SetY(y + pdfLineHeight)
		if ri == 0 {
			pdf.SetLineWidth(0.5)
			pdf.Line(pdfLeftMargin, pdf.GetY(), pdfLeftMargin+pdfContentWidth, pdf.GetY())
		}
	}
	pdf.Br(4)
	return nil
}

// ── 辅助 ──────────────────────────────────────────────────────────

// pdfEnsureSpace 在剩余空间不足时新起一页。
func pdfEnsureSpace(pdf *gopdf.GoPdf, needed float64) {
	if pdf.GetY()+needed > pdfBottomLimit {
		pdf.AddPage()
		pdf.SetY(pdfTopMargin)
	}
}

// pdfEstimateLines 估算文本换行后的行数（CJK 计 1 单位宽，ASCII 计 0.55 单位）。
func pdfEstimateLines(text string, fontSize, width float64) int {
	units := 0.0
	for _, r := range text {
		if r > 127 {
			units++
		} else {
			units += 0.55
		}
	}
	unitsPerLine := width / fontSize
	if unitsPerLine < 1 {
		unitsPerLine = 1
	}
	n := int(units/unitsPerLine) + 1
	if n < 1 {
		n = 1
	}
	return n
}

// pdfStripInline 移除行内 Markdown 标记（PDF 不做行内富文本）。
func pdfStripInline(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	return s
}

// cjkFontCandidates 是自动搜索的中文字体候选路径（按优先级）。
var cjkFontCandidates = []string{
	"/Library/Fonts/Arial Unicode.ttf", // macOS: Arial Unicode MS（纯 TTF，CJK 齐全）
	"/System/Library/Fonts/PingFang.ttc",
	"/System/Library/Fonts/Hiragino Sans GB.ttc",
	"/System/Library/Fonts/STHeiti Light.ttc",
	"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc", // Linux: Noto
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc", // Linux: 文泉驿
	"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
	`C:\Windows\Fonts\msyh.ttc`,   // Windows: 微软雅黑
	`C:\Windows\Fonts\simsun.ttc`, // Windows: 宋体
	`C:\Windows\Fonts\msyhbd.ttc`,
}

// findCJKFont 返回第一个存在的候选字体路径。
func findCJKFont() (string, bool) {
	for _, p := range cjkFontCandidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, true
		}
	}
	return "", false
}
