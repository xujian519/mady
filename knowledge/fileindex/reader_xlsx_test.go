package fileindex

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestReader_ReadXLSX verifies pure-Go XLSX parsing via excelize.
func TestReader_ReadXLSX(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xlsx")

	f := excelize.NewFile()
	defer f.Close()
	f.SetCellValue("Sheet1", "A1", "姓名")
	f.SetCellValue("Sheet1", "B1", "年龄")
	f.SetCellValue("Sheet1", "A2", "张三")
	f.SetCellValue("Sheet1", "B2", "30")
	f.SetCellValue("Sheet1", "A3", "李四")
	f.SetCellValue("Sheet1", "B3", "25")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save xlsx: %v", err)
	}

	fr := NewFileReader(dir)
	result, err := fr.readXLSX(context.Background(), path)
	if err != nil {
		t.Fatalf("readXLSX: %v", err)
	}
	if result.Confidence != 1.0 {
		t.Fatalf("expected confidence 1.0, got %f", result.Confidence)
	}
	if !strings.Contains(result.Content, "姓名") || !strings.Contains(result.Content, "张三") {
		t.Fatalf("expected content to contain header and data, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "| --- |") {
		t.Fatal("expected markdown table separator")
	}
	if result.Metadata["parser"] != "excelize" {
		t.Fatalf("expected parser=excelize, got %q", result.Metadata["parser"])
	}
	if result.Metadata["rows"] != "3" { // header + 2 data rows
		t.Fatalf("expected rows=3, got %q", result.Metadata["rows"])
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least one section")
	}
}

// TestReader_ReadXLSX_MultiSheet verifies multi-sheet workbooks get per-sheet headings.
func TestReader_ReadXLSX_MultiSheet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.xlsx")

	f := excelize.NewFile()
	defer f.Close()
	f.SetCellValue("Sheet1", "A1", "A1数据")
	// Add a second sheet.
	idx, err := f.NewSheet("数据表2")
	if err != nil {
		t.Fatalf("new sheet: %v", err)
	}
	f.SetActiveSheet(idx)
	f.SetCellValue("数据表2", "A1", "B1数据")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save xlsx: %v", err)
	}

	fr := NewFileReader(dir)
	result, err := fr.readXLSX(context.Background(), path)
	if err != nil {
		t.Fatalf("readXLSX: %v", err)
	}
	if !strings.Contains(result.Content, "### Sheet1") || !strings.Contains(result.Content, "### 数据表2") {
		t.Fatalf("expected per-sheet headings, got %q", result.Content)
	}
	if result.Metadata["sheets"] != "2" {
		t.Fatalf("expected sheets=2, got %q", result.Metadata["sheets"])
	}
}

// TestReader_ReadXLSX_EmptySheet verifies graceful handling of empty sheets.
func TestReader_ReadXLSX_EmptySheet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.xlsx")

	f := excelize.NewFile()
	defer f.Close()
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save xlsx: %v", err)
	}

	fr := NewFileReader(dir)
	result, err := fr.readXLSX(context.Background(), path)
	if err != nil {
		t.Fatalf("readXLSX: %v", err)
	}
	if result.CostNotice == "" {
		t.Fatal("expected cost notice for empty workbook")
	}
}
