package doctmpl

import "strings"

// MarkdownRenderer is the simplest renderer: it passes through Markdown
// content with optional metadata headers.
type MarkdownRenderer struct{}

// Format returns FormatMarkdown.
func (r *MarkdownRenderer) Format() OutputFormat { return FormatMarkdown }

// Render passes through the Markdown body. When meta.Title is set, an H1
// title is prepended. When meta.StyleName is set, it is noted in an HTML
// comment for consumers.
func (r *MarkdownRenderer) Render(md string, meta RenderMeta) ([]byte, error) {
	var b strings.Builder

	if meta.StyleName != "" {
		b.WriteString("<!-- style: ")
		b.WriteString(meta.StyleName)
		b.WriteString(" -->\n")
	}
	if meta.Title != "" && !strings.HasPrefix(md, "# ") {
		b.WriteString("# ")
		b.WriteString(meta.Title)
		b.WriteString("\n\n")
	}
	b.WriteString(md)
	return []byte(b.String()), nil
}
