package doctmpl

// OutputFormat represents the target file format for a rendered document.
type OutputFormat string

const (
	FormatMarkdown OutputFormat = "markdown" // .md — 对话内预览、版本控制
	FormatDOCX     OutputFormat = "docx"     // .docx — 提交专利局、客户交付
	FormatPDF      OutputFormat = "pdf"      // .pdf — 归档、邮件附件
	FormatHTML     OutputFormat = "html"     // .html — Web 预览、嵌入页面
	FormatEmail    OutputFormat = "email"    // 邮件正文 — 审查流转、客户沟通
)

// IsValid reports whether f is a known output format.
func (f OutputFormat) IsValid() bool {
	switch f {
	case FormatMarkdown, FormatDOCX, FormatPDF, FormatHTML, FormatEmail:
		return true
	default:
		return false
	}
}

// Ext returns the conventional file extension for this format.
func (f OutputFormat) Ext() string {
	switch f {
	case FormatMarkdown:
		return ".md"
	case FormatDOCX:
		return ".docx"
	case FormatPDF:
		return ".pdf"
	case FormatHTML:
		return ".html"
	case FormatEmail:
		return ".eml"
	default:
		return ""
	}
}

// MIME returns the MIME type for this format.
func (f OutputFormat) MIME() string {
	switch f {
	case FormatMarkdown:
		return "text/markdown"
	case FormatDOCX:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case FormatPDF:
		return "application/pdf"
	case FormatHTML:
		return "text/html"
	case FormatEmail:
		return "message/rfc822"
	default:
		return "application/octet-stream"
	}
}

// Renderer converts resolved template content to a target output format.
type Renderer interface {
	// Format returns the target format this renderer produces.
	Format() OutputFormat

	// Render converts the resolved Markdown body into the target format.
	// meta carries optional metadata (style name, title, author, date).
	Render(markdownBody string, meta RenderMeta) ([]byte, error)
}

// RenderMeta carries rendering-time metadata.
type RenderMeta struct {
	Style    *RenderStyle // 风格信息（可空），由调用方从 DocumentStyle 转换
	Title    string       // 文档标题
	Author   string       // 作者/代理人
	Date     string       // 日期
	Filename string       // 建议文件名（不含扩展名）
}

// RenderStyle 是 DocumentStyle 的精简投影，包含渲染时需要的风格信息。
// 避免 doctmpl 包直接依赖 domains 包（避免循环依赖）。
type RenderStyle struct {
	Name       string // 风格名，如 "patent-standard"
	Disclaimer string // 适用当前模板的免责声明
}

// ApplyDisclaimer 在 Markdown 输出前附加免责声明。
func (s *RenderStyle) ApplyDisclaimer(md string) string {
	if s == nil || s.Disclaimer == "" {
		return md
	}
	return "> ⚠️ " + s.Disclaimer + "\n\n---\n\n" + md
}
