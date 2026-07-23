package doctmpl

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// DOCXRenderer 渲染 Markdown 为 DOCX（纯 Go，标准库 zip+xml，零外部依赖）。
// 支持标题（六级）、段落、无序列表、表格（首行表头加粗）、行内加粗与等宽字体。
// 不依赖 pandoc 或任何 Office 库，适合无外部工具的部署环境。
type DOCXRenderer struct{}

// Format 返回 FormatDOCX。
func (r *DOCXRenderer) Format() OutputFormat { return FormatDOCX }

// Render 将 Markdown 正文转换为 DOCX 字节流。
// meta.Style 的免责声明、meta.Title 会注入文档开头。
func (r *DOCXRenderer) Render(md string, meta RenderMeta) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("doctmpl: nil DOCX renderer")
	}
	if meta.Style != nil {
		md = meta.Style.ApplyDisclaimer(md)
	}
	var prefix string
	if meta.Title != "" {
		prefix = docxHeadingXML(meta.Title, 1)
	}
	body := markdownToDocxBody(md)
	return buildDocxZip(docxWrapDocument(prefix + body))
}

// ── markdown → OOXML body ──────────────────────────────────────────

// markdownToDocxBody 把 Markdown 文本转为 word/document.xml 的 body 片段。
// 连续的普通文本行合并为同一段落（符合 Markdown 段落语义）。
func markdownToDocxBody(md string) string {
	lines := strings.Split(md, "\n")
	var b strings.Builder
	var para []string
	flushPara := func() {
		if len(para) == 0 {
			return
		}
		b.WriteString(docxParagraphXML(strings.Join(para, " ")))
		para = para[:0]
	}

	i := 0
	for i < len(lines) {
		line := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(line)

		// 表格块：连续以 | 开头的行。
		if strings.HasPrefix(trimmed, "|") {
			flushPara()
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
				b.WriteString(docxTableXML(tableRows))
			}
			continue
		}

		// 标题
		if lvl, text, ok := parseHeading(trimmed); ok {
			flushPara()
			b.WriteString(docxHeadingXML(text, lvl))
			i++
			continue
		}

		// 无序列表项
		if isListItem(trimmed) {
			flushPara()
			b.WriteString(docxListItemXML(strings.TrimSpace(trimmed[2:])))
			i++
			continue
		}

		// 空行：段落分隔
		if trimmed == "" {
			flushPara()
			i++
			continue
		}

		// 普通文本行：累积到当前段落
		para = append(para, trimmed)
		i++
	}
	flushPara()
	return b.String()
}

// ── OOXML 片段构造 ─────────────────────────────────────────────────

func docxHeadingXML(text string, level int) string {
	sz := 26
	switch {
	case level <= 1:
		sz = 44
	case level == 2:
		sz = 36
	case level == 3:
		sz = 30
	}
	return fmt.Sprintf(
		`<w:p><w:pPr><w:spacing w:before="240" w:after="120"/><w:outlineLvl w:val="%d"/></w:pPr>`+
			`<w:r><w:rPr><w:b/><w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`,
		level-1, sz, sz, docxXMLEscape(text),
	)
}

func docxParagraphXML(text string) string {
	return fmt.Sprintf(`<w:p>%s</w:p>`, docxInlineRunsXML(text, false))
}

func docxListItemXML(text string) string {
	return fmt.Sprintf(`<w:p><w:pPr><w:ind w:left="420"/></w:pPr>`+
		`<w:r><w:t xml:space="preserve">• </w:t></w:r>%s</w:p>`, docxInlineRunsXML(text, false))
}

// docxTableXML 生成带边框的表格；首行作为表头（加粗）。
func docxTableXML(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/><w:tblBorders>`)
	for _, edge := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		fmt.Fprintf(&b, `<w:%s w:val="single" w:sz="4" w:space="0" w:color="auto"/>`, edge)
	}
	b.WriteString(`</w:tblBorders></w:tblPr>`)
	for ri, row := range rows {
		b.WriteString(`<w:tr>`)
		for ci := 0; ci < cols; ci++ {
			cell := ""
			if ci < len(row) {
				cell = row[ci]
			}
			fmt.Fprintf(&b, `<w:tc><w:tcPr><w:tcW w:w="0" w:type="auto"/></w:tcPr><w:p>%s</w:p></w:tc>`,
				docxInlineRunsXML(cell, ri == 0))
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl><w:p/>`) // OOXML 要求 tbl 后跟随一个 p
	return b.String()
}

// docxInlineRunsXML 解析行内 **加粗** 和 `等宽` 标记，生成一个或多个 <w:r>。
// forceBold 强制所有 run 加粗（用于表头）。
func docxInlineRunsXML(text string, forceBold bool) string {
	var b strings.Builder
	i := 0
	for i < len(text) {
		// **bold**
		if strings.HasPrefix(text[i:], "**") {
			if end := strings.Index(text[i+2:], "**"); end >= 0 {
				b.WriteString(docxRunXML(text[i+2:i+2+end], true, false))
				i += 2 + end + 2
				continue
			}
		}
		// `code`
		if text[i] == '`' {
			if end := strings.Index(text[i+1:], "`"); end >= 0 {
				b.WriteString(docxRunXML(text[i+1:i+1+end], false, true))
				i += 1 + end + 1
				continue
			}
		}
		// 普通文本直到下一个标记
		next := len(text)
		for _, m := range []string{"**", "`"} {
			if pos := strings.Index(text[i:], m); pos >= 0 && i+pos < next {
				next = i + pos
			}
		}
		if next <= i {
			next = i + 1
		}
		b.WriteString(docxRunXML(text[i:next], forceBold, false))
		i = next
	}
	return b.String()
}

func docxRunXML(content string, bold, code bool) string {
	if content == "" {
		return ""
	}
	var rpr strings.Builder
	if bold {
		rpr.WriteString("<w:b/>")
	}
	if code {
		rpr.WriteString(`<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/>`)
	}
	rprOpen := ""
	if rpr.Len() > 0 {
		rprOpen = "<w:rPr>" + rpr.String() + "</w:rPr>"
	}
	return fmt.Sprintf(`<w:r>%s<w:t xml:space="preserve">%s</w:t></w:r>`, rprOpen, docxXMLEscape(content))
}

// ── Markdown 行解析辅助 ────────────────────────────────────────────

func parseHeading(line string) (level int, text string, ok bool) {
	if !strings.HasPrefix(line, "#") {
		return 0, "", false
	}
	lvl := 0
	for lvl < len(line) && line[lvl] == '#' {
		lvl++
	}
	if lvl > 6 {
		return 0, "", false
	}
	rest := line[lvl:]
	if !strings.HasPrefix(rest, " ") {
		return 0, "", false
	}
	return lvl, strings.TrimSpace(rest), true
}

func isListItem(line string) bool {
	return strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")
}

func isTableSeparator(line string) bool {
	if !strings.Contains(line, "-") || !strings.Contains(line, "|") {
		return false
	}
	s := strings.ReplaceAll(line, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "|", "")
	return s == ""
}

func parseTableRow(line string) []string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	parts := strings.Split(s, "|")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// ── DOCX 包装配 ────────────────────────────────────────────────────

const docxXMLHead = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
	`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`

const docxXMLTail = `<w:sectPr/></w:body></w:document>`

func docxWrapDocument(body string) string {
	return docxXMLHead + body + docxXMLTail
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`</Types>`

const rootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

// buildDocxZip 把 document.xml 装配为最小可用 DOCX 包（zip）。
func buildDocxZip(documentXML string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", rootRelsXML},
		{"word/document.xml", documentXML},
	}
	for _, p := range parts {
		fw, err := zw.Create(p.name)
		if err != nil {
			return nil, fmt.Errorf("docx: create %s: %w", p.name, err)
		}
		if _, err := fw.Write([]byte(p.content)); err != nil {
			return nil, fmt.Errorf("docx: write %s: %w", p.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("docx: close zip: %w", err)
	}
	return buf.Bytes(), nil
}

// docxXMLEscape 转义 XML 特殊字符。
func docxXMLEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
