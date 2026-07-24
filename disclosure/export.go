package disclosure

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DOCXConverter 将 Markdown 转换为 DOCX 格式。
// 定义在此避免 disclosure 包反向依赖 domains/doctmpl（架构边界）。
type DOCXConverter interface {
	Render(markdownBody string) ([]byte, error)
}

var defaultDOCXConverter DOCXConverter

// SetDOCXConverter 设置 DOCX 转换器，由 composition root（cmd/mady）注入。
func SetDOCXConverter(c DOCXConverter) { defaultDOCXConverter = c }

// ExportFormat represents the output format for report export.
type ExportFormat string

const (
	FormatMarkdown ExportFormat = "markdown"
	FormatDOCX     ExportFormat = "docx"
)

// ExportReport 将分析报告导出为指定格式。
// Markdown 格式直接生成；DOCX 格式优先使用纯 Go 渲染器（无外部依赖），
// 若失败则降级到 pandoc（需安装）。
func ExportReport(report *AnalysisReport, format ExportFormat) ([]byte, error) {
	md := buildMarkdownReport(report)

	switch format {
	case FormatMarkdown:
		return []byte(md), nil

	case FormatDOCX:
		docx, err := convertToDOCX(md)
		if err != nil {
			return nil, fmt.Errorf("DOCX conversion: %w", err)
		}
		return docx, nil

	default:
		return []byte(md), nil
	}
}

// buildMarkdownReport 构建 Markdown 格式的分析报告。
func buildMarkdownReport(report *AnalysisReport) string {
	var b strings.Builder
	if report == nil {
		return "（空报告）"
	}

	// Header
	b.WriteString("# 技术交底书分析报告\n\n")
	fmt.Fprintf(&b, "**报告ID**: %s  \n", report.ID)
	fmt.Fprintf(&b, "**生成时间**: %s  \n", report.GeneratedAt.Format("2006-01-02 15:04:05"))
	b.WriteString("\n---\n\n")

	// 1. Document overview
	if report.Document != nil {
		b.WriteString("## 一、文档概况\n\n")
		if report.Document.Title != "" {
			fmt.Fprintf(&b, "- **标题**: %s\n", report.Document.Title)
		}
		fmt.Fprintf(&b, "- **格式**: %s\n", report.Document.Format)
		fmt.Fprintf(&b, "- **段落数**: %d\n", len(report.Document.Sections))
		if report.Document.HasDrawings {
			b.WriteString("- **附图**: 有\n")
		}
		if len(report.Document.FigureRefs) > 0 {
			fmt.Fprintf(&b, "- **附图标记**: %s\n", strings.Join(report.Document.FigureRefs, "、"))
		}
		b.WriteString("\n")
	}

	// 2. Technical features
	if report.Extraction != nil && len(report.Extraction.Features) > 0 {
		b.WriteString("## 二、技术特征分析\n\n")
		b.WriteString("| ID | 描述 | 分类 | 功能 | 现有技术状态 | 重要度 |\n")
		b.WriteString("|----|------|------|------|-------------|--------|\n")
		for _, f := range report.Extraction.Features {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
				f.ID, f.Description, f.Category, f.Function, f.PriorArtStatus, f.Importance)
		}
		b.WriteString("\n")
	}

	// 3. Technical problems and effects
	if report.Extraction != nil {
		if len(report.Extraction.Problems) > 0 {
			b.WriteString("### 技术问题\n\n")
			for _, p := range report.Extraction.Problems {
				fmt.Fprintf(&b, "- %s\n", p)
			}
			b.WriteString("\n")
		}
		if len(report.Extraction.Effects) > 0 {
			b.WriteString("### 技术效果\n\n")
			for _, e := range report.Extraction.Effects {
				fmt.Fprintf(&b, "- %s\n", e)
			}
			b.WriteString("\n")
		}
	}

	// 4. Consistency
	if report.Consistency != nil {
		b.WriteString("## 三、一致性校验\n\n")
		fmt.Fprintf(&b, "**得分**: %.0f%%\n\n", report.Consistency.OverallScore*100)
		if report.Consistency.Pass {
			b.WriteString("✅ 校验通过\n\n")
		} else {
			b.WriteString("⚠️ 存在一致性问题\n\n")
		}
		if len(report.Consistency.Issues) > 0 {
			b.WriteString("| 严重程度 | 描述 |\n")
			b.WriteString("|----------|------|\n")
			for _, issue := range report.Consistency.Issues {
				sev := issue.Severity
				switch sev {
				case "error":
					sev = "🔴 " + sev
				case "warning":
					sev = "🟡 " + sev
				}
				fmt.Fprintf(&b, "| %s | %s |\n", sev, issue.Description)
			}
			b.WriteString("\n")
		}
	}

	// 5. Search keywords
	if len(report.SearchKeywords) > 0 {
		b.WriteString("## 四、检索关键词\n\n")
		b.WriteString(strings.Join(report.SearchKeywords, "、"))
		b.WriteString("\n\n")
	}

	// 6. Novelty assessment
	if report.Novelty != nil {
		b.WriteString("## 五、新颖性评估\n\n")
		if report.Novelty.Assessed {
			fmt.Fprintf(&b, "**结论**: %s\n\n", report.Novelty.Conclusion)
			if report.Novelty.Notes != "" {
				b.WriteString(report.Novelty.Notes)
				b.WriteString("\n")
			}
		} else {
			b.WriteString("未完成评估。\n")
			if report.Novelty.Notes != "" {
				b.WriteString(report.Novelty.Notes)
				b.WriteString("\n")
			}
		}
	}

	// 7. Report text
	if report.ReportText != "" {
		b.WriteString("## 六、详细分析\n\n")
		b.WriteString(report.ReportText)
		b.WriteString("\n\n")
	}

	// Footer
	b.WriteString("---\n\n")
	b.WriteString("**免责声明**\n\n")
	b.WriteString("本报告由 AI 辅助生成，仅供内部参考，不构成正式法律意见。\n")
	if !report.ReviewedByHuman {
		b.WriteString("\n⚠️ **本报告尚未经人工复核**\n")
	}
	fmt.Fprintf(&b, "\n*报告生成时间: %s*\n", report.GeneratedAt.Format("2006-01-02 15:04:05"))

	return b.String()
}

// convertToDOCX 将 Markdown 文本转换为 DOCX 格式。
// 优先使用已注入的纯 Go 渲染器（无外部依赖）；
// 失败时降级到 pandoc（若已安装）作为备选，确保兼容已有环境。
func convertToDOCX(markdown string) ([]byte, error) {
	if defaultDOCXConverter != nil {
		if data, err := defaultDOCXConverter.Render(markdown); err == nil && len(data) > 0 {
			return data, nil
		}
	}
	// 降级：尝试 pandoc（如已安装）。
	if _, lerr := exec.LookPath("pandoc"); lerr == nil {
		return convertToDOCXViaPandoc(markdown)
	}
	return nil, fmt.Errorf("DOCX 生成失败：纯 Go 渲染器出错，且未安装 pandoc 备选工具")
}

// convertToDOCXViaPandoc 通过外部 pandoc 进程转换 DOCX（备选方案）。
func convertToDOCXViaPandoc(markdown string) ([]byte, error) {
	cmd := exec.Command("pandoc", "-f", "markdown", "-t", "docx", "--from=gfm")
	cmd.Stdin = strings.NewReader(markdown)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pandoc: %w\nstderr: %s", err, stderr.String())
	}
	return out.Bytes(), nil
}

// SaveReport 将报告以指定文件名保存到磁盘。
// .docx 后缀触发 DOCX 导出，否则为 Markdown。
func SaveReport(report *AnalysisReport, filePath string) error {
	var format ExportFormat
	if strings.HasSuffix(filePath, ".docx") {
		format = FormatDOCX
	} else {
		format = FormatMarkdown
		if !strings.HasSuffix(filePath, ".md") {
			filePath += ".md"
		}
	}

	data, err := ExportReport(report, format)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("write %s: %w", filePath, err)
	}
	return nil
}
