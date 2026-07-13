package fileindex

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// readText reads a plain text file (txt, md, go, etc.). This is the cheapest
// extraction path — just read the file and return its entire content.
func (fr *FileReader) readText(_ context.Context, path string) (*FileReadResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	content := string(data)

	// Truncate extremely large files (>1MB) to prevent context overflow.
	const maxBytes = 1_000_000
	if len(content) > maxBytes {
		// Slice at rune boundary to avoid splitting multi-byte UTF-8 characters.
		runes := []rune(content)
		content = string(runes[:maxBytes]) + "\n\n[文件过长，已截断至前1MB]"
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

	return &FileReadResult{
		Content:    content,
		Confidence: 1.0,
		Sections:   sections,
		Metadata: map[string]string{
			"chars":    fmt.Sprintf("%d", len(content)),
			"sections": fmt.Sprintf("%d", len(sections)),
			"encoding": "utf-8",
		},
	}, nil
}
