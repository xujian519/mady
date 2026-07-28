// Package patent provides Pregel-based patent analysis workflows.
//
// The export functions in this file handle saving novelty analysis and OA
// response results to Markdown files, mirroring the disclosure/export.go
// pattern but for the simpler case of already-rendered Markdown text.
package patent

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ExportNoveltyReport 将新颖性分析报告（已渲染的 Markdown 文本）导出为完整文件。
// 返回带文件头的标准 Markdown 字节流。
func ExportNoveltyReport(output string) ([]byte, error) {
	if output == "" {
		return nil, fmt.Errorf("patent: empty novelty report")
	}

	var b strings.Builder

	// Header metadata
	b.WriteString("<!--\n")
	b.WriteString("生成时间: ")
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString("\n")
	b.WriteString("类型: 专利新颖性/创造性分析报告\n")
	b.WriteString("-->\n\n")

	// Content is already rendered Markdown from the Pregel graph.
	b.WriteString(output)
	b.WriteString("\n\n---\n\n")
	b.WriteString("*报告生成时间: ")
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString("*\n")

	return []byte(b.String()), nil
}

// ExportOAResponse 将 OA 答复书（已渲染的 Markdown 文本）导出为完整文件。
// 返回带文件头的标准 Markdown 字节流。
func ExportOAResponse(output string) ([]byte, error) {
	if output == "" {
		return nil, fmt.Errorf("patent: empty OA response")
	}

	var b strings.Builder

	// Header metadata
	b.WriteString("<!--\n")
	b.WriteString("生成时间: ")
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString("\n")
	b.WriteString("类型: 审查意见（OA）答复书\n")
	b.WriteString("-->\n\n")

	// Content is already rendered Markdown from the Pregel graph.
	b.WriteString(output)
	b.WriteString("\n\n---\n\n")
	b.WriteString("*报告生成时间: ")
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString("*\n")

	return []byte(b.String()), nil
}

// SaveNoveltyReport 将新颖性分析报告保存到磁盘文件。
// 路径以 .md 结尾或自动添加 .md 后缀。
func SaveNoveltyReport(output, filePath string) error {
	if !strings.HasSuffix(filePath, ".md") {
		filePath += ".md"
	}
	data, err := ExportNoveltyReport(output)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("write %s: %w", filePath, err)
	}
	return nil
}

// SaveOAResponse 将 OA 答复书保存到磁盘文件。
// 路径以 .md 结尾或自动添加 .md 后缀。
func SaveOAResponse(output, filePath string) error {
	if !strings.HasSuffix(filePath, ".md") {
		filePath += ".md"
	}
	data, err := ExportOAResponse(output)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("write %s: %w", filePath, err)
	}
	return nil
}
