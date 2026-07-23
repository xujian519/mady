package fileindex

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// readSpreadsheet extracts text from spreadsheet files.
// For CSV: reads all rows as text.
// For XLSX/XLS: returns a cost notice (full parsing is M3+).
func (fr *FileReader) readSpreadsheet(ctx context.Context, path string) (*FileReadResult, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".csv":
		return fr.readCSV(ctx, path)
	case ".xlsx":
		return fr.readXLSX(ctx, path)
	case ".xls":
		// .xls（Excel 97-2003）是 OLE 复合二进制格式，纯 Go 解析库成熟度低，
		// 故降级提示。如需处理，建议另存为 .xlsx 或 .csv。
		info, _ := os.Stat(path)
		return &FileReadResult{
			Content:    "",
			Confidence: 0.0,
			Metadata: map[string]string{
				"type":       ext,
				"size_bytes": fmt.Sprintf("%d", info.Size()),
			},
			CostNotice: fmt.Sprintf("%s（Excel 97-2003）为旧版二进制格式，暂不支持自动提取。建议另存为 .xlsx 或 .csv 后重试。", ext),
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
		_, _ = reader.Discard(3) // skip UTF-8 BOM
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

// readXLSX 用 excelize（纯 Go 库）读取 .xlsx 文件，按 sheet 转 markdown 表格。
// 多 sheet 时每个表前加三级标题；单 sheet 不加标题以保持与 CSV 输出一致。
func (fr *FileReader) readXLSX(ctx context.Context, path string) (*FileReadResult, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("打开 XLSX 文件失败: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return &FileReadResult{
			Content:    "",
			Confidence: 1.0,
			Metadata:   map[string]string{"sheets": "0"},
		}, nil
	}

	const maxRowsPerSheet = 1000
	var allTables []string
	totalRows := 0
	maxCols := 0

	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil || len(rows) == 0 {
			continue
		}

		// 过滤全空行。
		cleaned := make([][]string, 0, len(rows))
		for _, r := range rows {
			empty := true
			for _, c := range r {
				if strings.TrimSpace(c) != "" {
					empty = false
					break
				}
			}
			if !empty {
				cleaned = append(cleaned, r)
			}
		}
		if len(cleaned) == 0 {
			continue
		}

		headers := cleaned[0]
		var sb strings.Builder
		if len(sheets) > 1 {
			sb.WriteString("### " + sheet + "\n\n")
		}
		sb.WriteString("| " + strings.Join(headers, " | ") + " |\n")
		sb.WriteString("|" + strings.Repeat(" --- |", len(headers)) + "\n")

		limit := len(cleaned)
		if limit > maxRowsPerSheet+1 {
			limit = maxRowsPerSheet + 1
		}
		for i := 1; i < limit; i++ {
			row := cleaned[i]
			for len(row) < len(headers) {
				row = append(row, "")
			}
			if len(row) > len(headers) {
				row = row[:len(headers)]
			}
			for j, cell := range row {
				row[j] = strings.ReplaceAll(cell, "|", "\\|")
			}
			sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
		}
		allTables = append(allTables, sb.String())
		totalRows += len(cleaned)
		if len(headers) > maxCols {
			maxCols = len(headers)
		}
	}

	if len(allTables) == 0 {
		return &FileReadResult{
			Content:    "",
			Confidence: 1.0,
			Metadata:   map[string]string{"sheets": fmt.Sprintf("%d", len(sheets))},
			CostNotice: "工作簿所有 sheet 均为空。",
		}, nil
	}

	content := strings.Join(allTables, "\n")
	sections := make([]Section, 0, len(allTables))
	for _, t := range allTables {
		sections = append(sections, Section{Content: t})
	}

	return &FileReadResult{
		Content:    content,
		Confidence: 1.0,
		Sections:   sections,
		Metadata: map[string]string{
			"sheets":  fmt.Sprintf("%d", len(sheets)),
			"rows":    fmt.Sprintf("%d", totalRows),
			"columns": fmt.Sprintf("%d", maxCols),
			"parser":  "excelize",
		},
	}, nil
}
