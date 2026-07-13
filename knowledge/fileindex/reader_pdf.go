package fileindex

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// readPDF extracts text from a PDF by calling pdftotext (poppler-utils).
// pdftotext is the gold standard for PDF text extraction and handles CJK
// documents well when the correct cMap is available.
//
// Fallback: if pdftotext is not available, returns empty content with
// a cost notice.
func (fr *FileReader) readPDF(ctx context.Context, path string) (*FileReadResult, error) {
	// Check if pdftotext is available.
	pdftotextPath, err := exec.LookPath("pdftotext")
	if err != nil {
		return &FileReadResult{
			Content:    "",
			Confidence: 0.0,
			Metadata:   map[string]string{"warning": "pdftotext not found"},
			CostNotice: "系统未安装 pdftotext（poppler-utils）。请安装后重试，或手动查看 PDF。",
		}, nil
	}

	// Run pdftotext to extract text.
	cmd := exec.CommandContext(ctx, pdftotextPath, "-nopgbrk", "-enc", "UTF-8", path, "-")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &FileReadResult{
			Content:    "",
			Confidence: 0.0,
			Metadata:   map[string]string{"error": err.Error(), "stderr": stderr.String()},
			CostNotice: fmt.Sprintf("pdftotext 提取失败: %s", err.Error()),
		}, nil
	}

	content := stdout.String()
	if content == "" {
		// Could also try extracting metadata only.
		info, err := os.Stat(path)
		if err != nil {
			return &FileReadResult{
				Content:    "",
				Confidence: 0.0,
				Metadata:   map[string]string{"error": err.Error()},
				CostNotice: "无法读取 PDF 文件信息",
			}, nil
		}
		return &FileReadResult{
			Content:    "",
			Confidence: 0.0,
			Metadata: map[string]string{
				"size_bytes": fmt.Sprintf("%d", info.Size()),
				"empty":      "true",
			},
			CostNotice: "此 PDF 未提取到文本内容（可能是扫描件或纯图片 PDF）。建议人工查看。",
		}, nil
	}

	// Truncate extremely large PDF extracts (>500KB of text).
	const maxRunes = 500_000
	runes := []rune(content)
	truncated := false
	if len(runes) > maxRunes {
		content = string(runes[:maxRunes])
		truncated = true
	}

	// Split into sections by double newlines.
	rawSections := strings.Split(content, "\n\n")
	sections := make([]Section, 0, len(rawSections))
	for _, s := range rawSections {
		s = strings.TrimSpace(s)
		if s != "" {
			sections = append(sections, Section{Content: s})
		}
	}

	result := &FileReadResult{
		Content:    strings.TrimSpace(content),
		Confidence: 1.0,
		Sections:   sections,
		Metadata: map[string]string{
			"chars":    fmt.Sprintf("%d", len(content)),
			"sections": fmt.Sprintf("%d", len(sections)),
			"parser":   "pdftotext",
		},
	}

	if truncated {
		result.Metadata["truncated"] = "true"
		result.Content += "\n\n[内容过长，已截断]"
	}

	if stderr.Len() > 0 {
		result.Metadata["stderr"] = strings.TrimSpace(stderr.String())
	}

	return result, nil
}
