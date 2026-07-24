package doctmpl

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// HTMLRenderer renders Markdown to a standalone HTML document.
// Uses goldmark (CommonMark + GFM) for robust Markdown parsing.
type HTMLRenderer struct{}

// Format returns FormatHTML.
func (r *HTMLRenderer) Format() OutputFormat { return FormatHTML }

// Render converts the Markdown body into a standalone HTML5 document.
// meta.Style disclaimer and meta.Title are injected into the document.
func (r *HTMLRenderer) Render(md string, meta RenderMeta) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("doctmpl: nil HTML renderer")
	}
	if meta.Style != nil {
		md = meta.Style.ApplyDisclaimer(md)
	}

	converter := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(
			gmhtml.WithHardWraps(),
			gmhtml.WithUnsafe(),
		),
	)

	var bodyBuf bytes.Buffer
	if err := converter.Convert([]byte(md), &bodyBuf); err != nil {
		return nil, fmt.Errorf("doctmpl: markdown to HTML failed: %w", err)
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"")
	b.WriteString(langAttr(meta))
	b.WriteString("\">\n<head>\n<meta charset=\"UTF-8\">\n")
	if meta.Title != "" {
		fmt.Fprintf(&b, "<title>%s</title>\n", html.EscapeString(meta.Title))
	}
	if meta.Author != "" {
		fmt.Fprintf(&b, "<meta name=\"author\" content=\"%s\">\n", html.EscapeString(meta.Author))
	}
	b.WriteString(htmlStyleBlock)
	b.WriteString("</head>\n<body>\n")

	if meta.Title != "" {
		fmt.Fprintf(&b, "<h1>%s</h1>\n", html.EscapeString(meta.Title))
	}

	b.WriteString(bodyBuf.String())
	b.WriteString("\n</body>\n</html>\n")
	return []byte(b.String()), nil
}

func langAttr(meta RenderMeta) string {
	if meta.Language != "" {
		return meta.Language
	}
	return "zh-CN"
}

const htmlStyleBlock = `<style>
:root { --fg:#222; --bg:#fff; --muted:#666; --border:#ddd; --code-bg:#f5f5f5; }
* { box-sizing:border-box; }
body { font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","Noto Sans CJK SC",
  "PingFang SC","Microsoft YaHei",sans-serif; color:var(--fg); background:var(--bg);
  max-width:820px; margin:2rem auto; padding:0 1.5rem; line-height:1.7; }
h1,h2,h3,h4,h5,h6 { font-weight:600; line-height:1.3; margin:1.6em 0 .6em; }
h1 { font-size:1.8rem; border-bottom:2px solid var(--border); padding-bottom:.3rem; }
h2 { font-size:1.5rem; border-bottom:1px solid var(--border); padding-bottom:.2rem; }
h3 { font-size:1.25rem; }
table { border-collapse:collapse; width:100%; margin:1em 0; }
th,td { border:1px solid var(--border); padding:.5em .75em; text-align:left; }
th { background:#f8f8f8; font-weight:600; }
tr:nth-child(even) { background:#fafafa; }
code { font-family:"SF Mono","Fira Code","JetBrains Mono",Consolas,monospace;
  background:var(--code-bg); padding:.15em .35em; border-radius:3px; font-size:.9em; }
pre { background:var(--code-bg); padding:1em; border-radius:6px; overflow-x:auto; }
pre code { background:none; padding:0; }
blockquote { margin:1em 0; padding:.5em 1em; border-left:4px solid var(--border);
  color:var(--muted); }
img { max-width:100%; }
a { color:#2563eb; }
hr { border:none; border-top:1px solid var(--border); margin:2em 0; }
@media print { body { max-width:none; margin:0; padding:1cm; } }
</style>`
