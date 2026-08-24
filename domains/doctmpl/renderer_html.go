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
var _ Renderer = (*HTMLRenderer)(nil)

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
	b.WriteString(selectStyleBlock(meta.Style))
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

// selectStyleBlock returns the patent-specific stylesheet when the rendered
// document uses a patent/Sati style, falling back to the default block so
// existing HTML output is unaffected.
func selectStyleBlock(s *RenderStyle) string {
	if isPatentStyle(s) {
		return patentHTMLStyleBlock
	}
	return htmlStyleBlock
}

// isPatentStyle reports whether a render style indicates the patent/Sati domain,
// which uses the A4 patentHTMLStyleBlock.
func isPatentStyle(s *RenderStyle) bool {
	if s == nil {
		return false
	}
	n := strings.ToLower(s.Name)
	return strings.HasPrefix(n, "patent") || n == "sati" || strings.Contains(n, "patent")
}

// patentHTMLStyleBlock is the A4 print-ready stylesheet for patent report
// deliverables (Sati-aligned). Selected only for patent/Sati styles so the
// default htmlStyleBlock remains the base for other documents.
const patentHTMLStyleBlock = `<style>
@page { size: A4; margin: 20mm 25mm; }
:root { --patent-navy:#1f3a5f; --patent-danger:#b42318; --patent-warning:#b54708;
  --patent-success:#067647; --patent-muted:#666; --patent-border:#d7d7d7; }
* { box-sizing:border-box; }
body { font-family:"FangSong","仿宋","FangSong_GB2312",serif; font-size:12pt;
  line-height:1.5; color:#222; background:#fff; max-width:160mm; margin:0 auto;
  padding:0; }
h1,h2,h3,h4,h5,h6 { font-family:"SimHei","黑体",sans-serif; color:var(--patent-navy);
  font-weight:700; line-height:1.3; margin:1.4em 0 .6em; }
h1 { font-size:18pt; border-bottom:2px solid var(--patent-navy);
  padding-bottom:.3rem; }
h2 { font-size:15pt; border-bottom:1px solid var(--patent-navy);
  padding-bottom:.2rem; }
h3 { font-size:13pt; }
table { border-collapse:collapse; width:100%; margin:1em 0; font-size:11pt; }
th,td { border:1px solid var(--patent-border); padding:.45em .6em;
  text-align:left; vertical-align:top; }
th { background:#f0f3f7; font-weight:600; }
.verdict-table { margin:1.2em 0; }
.verdict-table .verdict-danger { color:var(--patent-danger); font-weight:700; }
.verdict-table .verdict-warning { color:var(--patent-warning); font-weight:700; }
.verdict-table .verdict-success { color:var(--patent-success); font-weight:700; }
.doc-meta { margin:1em 0; padding:.8em 1em; background:#f7f9fc;
  border:1px solid var(--patent-border); border-radius:4px; font-size:10.5pt; }
.doc-meta dl { display:flex; flex-wrap:wrap; gap:.4rem 2rem; margin:0; }
.doc-meta dt { font-weight:600; color:var(--patent-navy); }
.doc-meta dd { margin:0; }
.callout { margin:1em 0; padding:.7em 1em; border-left:4px solid
  var(--patent-navy); background:#f7f9fc; }
.callout.warning { border-left-color:var(--patent-warning); }
.callout.danger { border-left-color:var(--patent-danger); }
blockquote { margin:1em 0; padding:.4em 1em; border-left:4px solid
  var(--patent-border); color:#444; }
code { font-family:"SF Mono",Consolas,monospace; background:#f5f5f5;
  padding:.12em .3em; border-radius:3px; font-size:.9em; }
pre { background:#f5f5f5; padding:.8em; border-radius:4px; overflow-x:auto; }
pre code { background:none; padding:0; }
img { max-width:100%; }
a { color:#2563eb; }
hr { border:none; border-top:1px solid var(--patent-border); margin:1.5em 0; }
@media print { body { max-width:none; margin:0; } }
</style>`
