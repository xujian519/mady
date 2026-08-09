package component

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/xujian519/mady/tui/core"
	apitheme "github.com/xujian519/mady/tui/theme"
)

var (
	// reInlineHashes matches ATX-style hash runs of 2+ appearing mid-line
	// inside a paragraph. Line-start hashes are classified as headings by the
	// block parser, so any hash run that reaches paragraph text is a stray LLM
	// artifact. Requiring at least two hashes keeps legitimate single hashes
	// ("C#", "F#", patent drawing references like "1#") intact.
	reInlineHashes = regexp.MustCompile(`([^#\s])#{2,6}(\s|$)`)
)

// renderBlock renders a single Block to width using theme.
func renderBlock(b Block, width int64, theme MarkdownTheme) []string {
	switch b.Kind {
	case kindFence:
		return renderFenceBlock(b.Lang, b.Lines, width, theme)
	case kindHR:
		return []string{theme.HRFn(core.PadToWidth(strings.Repeat("─", int(width)), width))}
	case kindHeading:
		level, _, ok := parseATXHeading(b.Lines[0])
		if !ok {
			return nil
		}
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		text := renderInline(extractHeadingText(b.Lines[0], level), theme)
		fn := theme.HeadingFn[level-1]
		return wrapMarkdownText(fn(text), width)
	case kindQuote:
		qm := reQuote.FindStringSubmatch(b.Lines[0])
		if qm == nil {
			return nil
		}
		text := renderInline(qm[1], theme)
		out := make([]string, 0, 2)
		for _, w := range wrapMarkdownText(text, width-2) {
			line := theme.QuoteFn("│ ") + w
			out = append(out, line)
		}
		return out
	case kindTable:
		return renderTable(b.Lines, width, theme)
	case kindBullet:
		if len(b.Lines) == 0 {
			return nil
		}
		bm := reBullet.FindStringSubmatch(b.Lines[0])
		if bm == nil {
			return nil
		}
		indent := len(bm[1])
		text := renderInline(bm[3], theme)
		bullet := theme.ListBulletFn("• ")
		prefixWidth := int64(indent) + core.VisibleWidth(bullet)
		wrapW := width - prefixWidth
		if wrapW < 1 {
			wrapW = 1
		}
		indentStr := strings.Repeat(" ", int(prefixWidth))
		out := make([]string, 0, len(b.Lines)*2)
		for k, w := range wrapMarkdownText(text, wrapW) {
			prefix := indentStr
			if k == 0 {
				prefix = strings.Repeat(" ", indent) + bullet
			}
			out = append(out, prefix+w)
		}
		// Render continuation lines. Strip the source indentation that marks
		// the line as a continuation so the rendered text aligns with the first
		// line's text column rather than being indented twice.
		for _, cl := range b.Lines[1:] {
			ct := strings.TrimLeft(cl, " \t")
			// A nested list line (deeper indentation + bullet/ordered
			// marker) is not real continuation text of this item. Strip its
			// marker so it renders as indented plain text instead of
			// leaking a raw "- " or "1. " into the output (P2-16). Full
			// nested-list rendering is a separate feature.
			ct = stripNestedListMarker(ct)
			ct = renderInline(ct, theme)
			for _, w := range wrapMarkdownText(ct, wrapW) {
				out = append(out, indentStr+w)
			}
		}
		return out
	case kindOrdered:
		if len(b.Lines) == 0 {
			return nil
		}
		om := reOrdered.FindStringSubmatch(b.Lines[0])
		if om == nil {
			return nil
		}
		indent := len(om[1])
		num := om[2] + ". "
		text := renderInline(om[3], theme)
		styledNum := theme.ListBulletFn(num)
		prefixWidth := int64(indent) + core.VisibleWidth(styledNum)
		wrapW := width - prefixWidth
		if wrapW < 1 {
			wrapW = 1
		}
		indentStr := strings.Repeat(" ", int(prefixWidth))
		out := make([]string, 0, len(b.Lines)*2)
		for k, w := range wrapMarkdownText(text, wrapW) {
			prefix := indentStr
			if k == 0 {
				prefix = strings.Repeat(" ", indent) + styledNum
			}
			out = append(out, prefix+w)
		}
		// Strip source continuation indentation so wrapped/continuation lines
		// align with the first line's text column.
		for _, cl := range b.Lines[1:] {
			ct := stripNestedListMarker(strings.TrimLeft(cl, " \t"))
			ct = renderInline(ct, theme)
			for _, w := range wrapMarkdownText(ct, wrapW) {
				out = append(out, indentStr+w)
			}
		}
		return out
	case kindChineseOrdered:
		if len(b.Lines) == 0 {
			return nil
		}
		om := reChineseOrdered.FindStringSubmatch(b.Lines[0])
		if om == nil {
			return nil
		}
		indent := len(om[1])
		// Normalize the Chinese enumeration comma to a period so the marker is
		// treated like other ordered lists and does not leave raw punctuation
		// (e.g. "一、") in the rendered output.
		num := om[2] + ". "
		text := renderInline(om[3], theme)
		styledNum := theme.ListBulletFn(num)
		prefixWidth := int64(indent) + core.VisibleWidth(styledNum)
		wrapW := width - prefixWidth
		if wrapW < 1 {
			wrapW = 1
		}
		indentStr := strings.Repeat(" ", int(prefixWidth))
		out := make([]string, 0, len(b.Lines)*2)
		for k, w := range wrapMarkdownText(text, wrapW) {
			prefix := indentStr
			if k == 0 {
				prefix = strings.Repeat(" ", indent) + styledNum
			}
			out = append(out, prefix+w)
		}
		// Strip source continuation indentation so wrapped/continuation lines
		// align with the first line's text column.
		for _, cl := range b.Lines[1:] {
			ct := stripNestedListMarker(strings.TrimLeft(cl, " \t"))
			ct = renderInline(ct, theme)
			for _, w := range wrapMarkdownText(ct, wrapW) {
				out = append(out, indentStr+w)
			}
		}
		return out
	case kindBlank:
		return []string{""}
	case kindParagraph:
		var out []string
		for _, ln := range b.Lines {
			text := renderInline(sanitizeParagraphArtifacts(ln), theme)
			out = append(out, wrapMarkdownText(text, width)...)
		}
		return out
	}
	return nil
}

// wrapMarkdownText chooses between the generic ANSI wrapper and the CJK-aware
// wrapper based on whether the text contains CJK ideographs.
func wrapMarkdownText(text string, width int64) []string {
	if containsCJK(text) {
		return core.WrapAnsiCJK(text, width)
	}
	return core.WrapAnsi(text, width)
}

// containsCJK reports whether s contains any CJK Unified Ideographs.
func containsCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF, // CJK Unified Ideographs
			r >= 0x3400 && r <= 0x4DBF,   // CJK Ext A
			r >= 0xF900 && r <= 0xFAFF,   // CJK Compatibility
			r >= 0x20000 && r <= 0x2FFFD, // CJK Ext B..F
			r >= 0x30000 && r <= 0x3FFFD: // CJK Ext G
			return true
		}
	}
	return false
}

// renderFenceBlock renders a fenced code block. Split out of renderBlock so
// the fence-specific highlighter logic (diff coloring, syntax Highlight)
// stays readable on its own. Behavior is identical to the old inline branch.
func renderFenceBlock(lang string, codeLines []string, width int64, theme MarkdownTheme) []string {
	var out []string
	if lang != "" {
		out = append(out, theme.CodeFenceFn(core.PadToWidth("  "+lang, width)))
	}

	// If a language is specified and a highlighter exists, run it
	// over the whole block so multi-line block comments/strings
	// resolve correctly.
	var rendered []string
	if lang == "diff" {
		rendered = make([]string, len(codeLines))
		var oldLine, newLine int
		for k, cl := range codeLines {
			switch {
			case strings.HasPrefix(cl, "@@ "):
				// Parse hunk header to extract line numbers.
				rendered[k] = apitheme.CurrentPalette().Accent.Render(theme.CodeBlockFn(cl))
				if _, err := fmt.Sscanf(cl, "@@ -%d", &oldLine); err == nil {
					newLine = oldLine
					if idx := strings.Index(cl, "+"); idx > 0 {
						if _, err2 := fmt.Sscanf(cl[idx:], "+%d", &newLine); err2 != nil {
							newLine = oldLine
						}
					}
				}
			case strings.HasPrefix(cl, "+++ ") || strings.HasPrefix(cl, "--- "):
				rendered[k] = theme.CodeBlockFn(cl)
			case strings.HasPrefix(cl, "+") && !strings.HasPrefix(cl, "++"):
				rendered[k] = apitheme.CurrentPalette().Success.Render(fmt.Sprintf("%4d %s", newLine, theme.CodeBlockFn(cl)))
				newLine++
			case strings.HasPrefix(cl, "-") && !strings.HasPrefix(cl, "--"):
				rendered[k] = apitheme.CurrentPalette().Error.Render(fmt.Sprintf("%4d %s", oldLine, theme.CodeBlockFn(cl)))
				oldLine++
			default:
				rendered[k] = fmt.Sprintf("     %s", theme.CodeBlockFn(cl))
				oldLine++
				newLine++
			}
		}
	} else if spec := LookupLanguage(lang); spec != nil {
		rendered = Highlight(strings.Join(codeLines, "\n"), lang, syntaxThemeFromMarkdown(theme))
	} else {
		rendered = make([]string, len(codeLines))
		for k, cl := range codeLines {
			rendered[k] = theme.CodeBlockFn(cl)
		}
	}
	for _, cl := range rendered {
		if cl == "" {
			continue
		}
		line := "  " + cl
		if core.VisibleWidth(line) > width {
			// 超宽代码行软换行，确保内容完整显示而非被上层视口或
			// 引擎的 normalizeLine 截断为省略号（丢失代码末尾）。
			for _, w := range core.WrapAnsi(line, width) {
				out = append(out, core.PadToWidth(w, width))
			}
		} else {
			out = append(out, core.PadToWidth(line, width))
		}
	}
	return out
}

func renderMarkdown(src string, width int64, theme MarkdownTheme) []string {
	blocks := parseBlocks(src)
	var out []string
	for _, b := range blocks {
		out = append(out, renderBlock(b, width, theme)...)
	}
	return out
}

// ---------------------------------------------------------------------------
// Table rendering
// ---------------------------------------------------------------------------

func renderTable(rows []string, width int64, t MarkdownTheme) []string {
	if len(rows) < 2 {
		return nil
	}
	parse := func(r string) []string {
		r = strings.TrimSpace(r)
		r = strings.TrimPrefix(r, "|")
		r = strings.TrimSuffix(r, "|")
		parts := strings.Split(r, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}

	// Check if row 1 is a standard separator; if not, treat all rows as data
	// with the first row as header.
	hasSep := reTableSep.MatchString(rows[1])

	var header []string
	var body [][]string

	if hasSep {
		header = parse(rows[0])
		for _, r := range rows[2:] {
			body = append(body, parse(r))
		}
	} else {
		header = parse(rows[0])
		for _, r := range rows[1:] {
			body = append(body, parse(r))
		}
	}
	cols := len(header)
	// Body rows may carry more cells than the header (LLM output is
	// inconsistent about trailing columns). Take the max so extra columns
	// render instead of being silently dropped (P2-20).
	for _, r := range body {
		if len(r) > cols {
			cols = len(r)
		}
	}

	// Column widths
	colW := make([]int64, cols)
	for i, h := range header {
		colW[i] = core.VisibleWidth(h)
	}
	for _, r := range body {
		for i := 0; i < cols && i < len(r); i++ {
			if w := core.VisibleWidth(r[i]); w > colW[i] {
				colW[i] = w
			}
		}
	}

	// If the table does not fit naturally, render it vertically as key/value
	// pairs rather than squeezing columns and truncating cell content with
	// ellipsis. This keeps the information complete on narrow viewports.
	total := int64(cols)*3 + 1
	for _, w := range colW {
		total += w
	}
	// 留 2 列安全边距，避免表格边框紧贴视口边缘导致破碎感。
	if total > width-2 {
		return renderTableVertical(header, body, width, t)
	}

	out := renderTableRows(header, body, colW, width, t)
	return out
}

// renderTableRows renders a fixed-width table from header/body cells and
// pre-computed column widths. Extracted from renderTable to keep its cognitive
// load low.
func renderTableRows(header []string, body [][]string, colW []int64, width int64, t MarkdownTheme) []string {
	sep := func() string {
		var b strings.Builder
		b.WriteString("+")
		for _, w := range colW {
			b.WriteString(strings.Repeat("-", int(w)+2))
			b.WriteString("+")
		}
		return t.TableBorderFn(b.String())
	}
	renderRow := func(cells []string, headerRow bool) string {
		var b strings.Builder
		b.WriteString(t.TableBorderFn("|"))
		for i, w := range colW {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			// Header and data rows both go through renderInline so inline
			// markers (**bold**, *italic*, `code`) never leak literally.
			// renderTableVertical does the same; skipping it here produced
			// raw "**维度**" in horizontal-mode tables.
			cell = renderInline(cell, t)
			padded := core.PadToWidth(core.TruncateToWidth(cell, w, "…"), w)
			if headerRow {
				padded = t.TableHeaderFn(padded)
			}
			b.WriteString(" ")
			b.WriteString(padded)
			b.WriteString(" ")
			b.WriteString(t.TableBorderFn("|"))
		}
		return b.String()
	}
	out := []string{
		core.PadToWidth(sep(), width),
		core.PadToWidth(renderRow(header, true), width),
		core.PadToWidth(sep(), width),
	}
	for _, r := range body {
		out = append(out, core.PadToWidth(renderRow(r, false), width))
	}
	out = append(out, core.PadToWidth(sep(), width))
	return out
}

// renderTableVertical renders a table as key/value pairs when the table is too
// wide for the viewport. This preserves all cell content instead of truncating
// it with ellipsis.
func renderTableVertical(header []string, body [][]string, width int64, t MarkdownTheme) []string {
	const indent = 2
	wrapWidth := width
	if wrapWidth < 1 {
		wrapWidth = 1
	}
	indentStr := strings.Repeat(" ", indent)
	var out []string
	for rowIdx, r := range body {
		if rowIdx > 0 {
			out = append(out, "")
		}
		for i, h := range header {
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			// 与单元格一致，表头也先过 renderInline，避免 **bold** 等裸标记泄漏。
			head := t.TableHeaderFn(renderInline(h, t) + ":")
			out = append(out, wrapMarkdownText(head, wrapWidth)...)
			rendered := renderInline(cell, t)
			out = append(out, wrapMarkdownText(indentStr+rendered, wrapWidth)...)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Default theme
// ---------------------------------------------------------------------------

// sanitizeParagraphArtifacts cleans up stray markdown punctuation that LLMs
// commonly emit inside paragraph text, after legal block-level parsing has
// already happened. Paragraph lines are never headings, so any ATX hash
// sequence here is malformed and can be stripped. Unpaired emphasis markers
// are also removed to avoid raw `**` / `*` polluting the terminal output.
func sanitizeParagraphArtifacts(s string) string {
	s = reInlineHashes.ReplaceAllString(s, "$1$2")
	s = stripUnpairedMarker(s, "**")
	s = stripUnpairedMarker(s, "*")
	s = stripUnpairedMarker(s, "~~")
	return s
}

// stripUnpairedMarker removes the last occurrence of marker when it has no
// matching pair. This is a best-effort cleanup for incomplete LLM markdown.
// A marker flanked by digits on both sides is treated as a math expression
// (e.g. "2*3", "4**2", "2**10") and left untouched instead of mangled.
func stripUnpairedMarker(s, marker string) string {
	if strings.Count(s, marker)%2 != 0 {
		idx := strings.LastIndex(s, marker)
		if idx < 0 {
			return s
		}
		// 数学表达式（2*3 / 4**2）：两侧都是数字，保留。
		if isDigitByte(s, idx-1) && isDigitByte(s, idx+len(marker)) {
			return s
		}
		// 单字符 marker 紧邻另一个相同字符（如 4**2 中的单个 '*'）：
		// 它属于 "**" 片段，交由 "**" 分支处理，避免二次误删。
		if len(marker) == 1 &&
			((idx > 0 && s[idx-1] == marker[0]) || (idx+1 < len(s) && s[idx+1] == marker[0])) {
			return s
		}
		s = s[:idx] + s[idx+len(marker):]
	}
	return s
}

// isDigitByte reports whether s[i] is an ASCII digit. Out-of-range indices
// report false. Byte-wise check is sufficient: multi-byte runes never equal
// '0'..'9', so full-width digits fall through to the removal path.
func isDigitByte(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c >= '0' && c <= '9'
}

// DefaultMarkdownTheme returns the built-in markdown theme used when no

var (
	reNestedBullet  = regexp.MustCompile(`^[-*+]\s+(.*)$`)
	reNestedOrdered = regexp.MustCompile(`^\d+[.)]\s+(.*)$`)
)

// stripNestedListMarker removes a nested-list marker ("- ", "* ", "1. ")
// from a continuation line, leaving the item text. Nested lists are not
// rendered with real hierarchy (P2-16); stripping the marker prevents raw
// "- "/"1. " from leaking into the output while keeping the indented text.
func stripNestedListMarker(s string) string {
	if m := reNestedBullet.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := reNestedOrdered.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}
