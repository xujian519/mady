package fileindex

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readSpreadsheet extracts text from spreadsheet files.
// For CSV: reads all rows as text.
// For XLSX/XLS: returns a cost notice (full parsing is M3+).
func (fr *FileReader) readSpreadsheet(ctx context.Context, path string) (*FileReadResult, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".csv":
		return fr.readCSV(ctx, path)
	case ".xlsx", ".xls":
		// XLSX parsing requires a library (like excelize). For now, return notice.
		info, _ := os.Stat(path)
		return &FileReadResult{
			Content:    "",
			Confidence: 0.0,
			Metadata: map[string]string{
				"type":       ext,
				"size_bytes": fmt.Sprintf("%d", info.Size()),
			},
			CostNotice: fmt.Sprintf("%s 格式暂不支持自动提取。建议将文件另存为 CSV 后重试。", ext),
		}, nil
	default:
		return fr.readText(ctx, path)
	}
}

// readCSV reads a CSV file and formats it as text.
func (fr *FileReader) readCSV(ctx context.Context, path string) (*FileReadResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 CSV 文件失败: %w", err)
	}
	defer f.Close()

	// Detect encoding by reading the first few bytes (for BOM).
	reader := bufio.NewReader(f)
	bom, err := reader.Peek(3)
	if err == nil && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
		reader.Discard(3) // skip UTF-8 BOM
	}

	csvReader := csv.NewReader(reader)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1 // variable columns

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析 CSV 失败: %w", err)
	}

	if len(records) == 0 {
		return &FileReadResult{
			Content:    "",
			Confidence: 1.0,
			Metadata:   map[string]string{"rows": "0"},
		}, nil
	}

	// Format as markdown table for readability.
	var sb strings.Builder
	maxRows := 1000 // limit to prevent context overflow

	// Header row.
	headers := records[0]
	sb.WriteString("| " + strings.Join(headers, " | ") + " |\n")
	sb.WriteString("|" + strings.Repeat(" --- |", len(headers)) + "\n")

	for i := 1; i < len(records) && i < maxRows; i++ {
		row := records[i]
		// Pad or trim to match header count.
		for len(row) < len(headers) {
			row = append(row, "")
		}
		if len(row) > len(headers) {
			row = row[:len(headers)]
		}
		// Escape pipe characters in cell values.
		for j, cell := range row {
			row[j] = strings.ReplaceAll(cell, "|", "\\|")
		}
		sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}

	content := sb.String()
	if len(records) > maxRows {
		content += fmt.Sprintf("\n[共 %d 行，仅显示前 %d 行]", len(records), maxRows)
	}

	return &FileReadResult{
		Content:    content,
		Confidence: 1.0,
		Sections: []Section{
			{Content: fmt.Sprintf("CSV 表格：%d 行 x %d 列", len(records), len(headers))},
			{Content: content},
		},
		Metadata: map[string]string{
			"rows":    fmt.Sprintf("%d", len(records)),
			"columns": fmt.Sprintf("%d", len(headers)),
		},
	}, nil
}
