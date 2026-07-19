package doctmpl

import "strings"

// MarkdownRenderer is the simplest renderer: it passes through Markdown
// content with optional metadata and disclaimer injection.
type MarkdownRenderer struct{}

// Format returns FormatMarkdown.
func (r *MarkdownRenderer) Format() OutputFormat { return FormatMarkdown }

// Render passes through the Markdown body. When meta.Style is set, a
// disclaimer is prepended. When meta.Title is set, an H1 is prepended
// (unless the body already starts with one).
func (r *MarkdownRenderer) Render(md string, meta RenderMeta) ([]byte, error) {
	var b strings.Builder

	// Capture whether the original body starts with a title before any
	// disclaimer injection changes the prefix.
	hasBodyTitle := strings.HasPrefix(md, "# ")

	if meta.Style != nil {
		md = meta.Style.ApplyDisclaimer(md)
	}

	if meta.Title != "" && !hasBodyTitle {
		b.WriteString("# ")
		b.WriteString(meta.Title)
		b.WriteString("\n\n")
	}
	b.WriteString(md)
	return []byte(b.String()), nil
}
