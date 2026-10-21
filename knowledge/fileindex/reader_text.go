package fileindex

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// readText reads a plain text file (txt, md, go, etc.). This is the cheapest
// extraction path — just read the file and return its entire content.
func (fr *FileReader) readText(_ context.Context, path string) (*FileReadResult, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from file index, validated by caller
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	content := string(data)

	// Truncate extremely large files (>1MB) to prevent context overflow.
	const maxBytes = 1_000_000
	if len(content) > maxBytes {
		// Keep the byte budget, but back off to a UTF-8 boundary so the cut
		// never lands inside a multi-byte rune.
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(content[cut]) {
			cut--
		}
		content = content[:cut] + "\n\n[文件过长，已截断至前1MB]"
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
