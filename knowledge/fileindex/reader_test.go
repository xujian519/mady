package fileindex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// testdataAbs returns the absolute path to the package's testdata directory.
func testdataAbs() string {
	// In Go test execution, the working directory is the package directory.
	// Fall back to a path relative to this source file if CWD is unexpected.
	abs, err := filepath.Abs("testdata")
	if err != nil {
		return "testdata"
	}
	return abs
}

func TestReader_ReadText(t *testing.T) {
	fr := NewFileReader(".")
	result, err := fr.readText(context.TODO(), filepath.Join(testdataAbs(), "test.txt"))
	if err != nil {
		t.Fatalf("readText: %v", err)
	}
	if result.Content == "" {
		t.Fatal("expected non-empty content")
	}
	if !strings.Contains(result.Content, "测试文档") {
		t.Fatalf("expected content to contain '测试文档', got %q", result.Content[:50])
	}
	if result.Confidence != 1.0 {
		t.Fatalf("expected confidence 1.0 for text, got %f", result.Confidence)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least one section")
	}
}

func TestReader_ReadCSV(t *testing.T) {
	fr := NewFileReader(".")
	result, err := fr.readSpreadsheet(context.TODO(), filepath.Join(testdataAbs(), "test.csv"))
	if err != nil {
		t.Fatalf("readSpreadsheet(CSV): %v", err)
	}
	if result.Content == "" {
		t.Fatal("expected non-empty content for CSV")
	}
	if !strings.Contains(result.Content, "华为") {
		t.Fatalf("expected CSV content to contain '华为', got %q", result.Content)
	}
}

func TestReader_ReadDocx(t *testing.T) {
	fr := NewFileReader(".")
	result, err := fr.readDocx(context.TODO(), filepath.Join(testdataAbs(), "test.docx"))
	if err != nil {
		t.Fatalf("readDocx: %v", err)
	}
	if result.Content == "" {
		t.Fatal("expected non-empty content for docx")
	}
	if !strings.Contains(result.Content, "专利权利要求") {
		t.Fatalf("expected docx content to contain '专利权利要求', got %q", result.Content)
	}
}

func TestReader_ReadImage(t *testing.T) {
	fr := NewFileReader(".")
	info, err := os.Stat(filepath.Join(testdataAbs(), "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	result := fr.readImage(filepath.Join(testdataAbs(), "scan.jpg"), info)
	if result.Content != "" {
		t.Fatal("expected empty content for image")
	}
	if result.CostNotice == "" {
		t.Fatal("expected cost notice for image")
	}
	if result.Confidence != 0.0 {
		t.Fatalf("expected confidence 0.0 for image, got %f", result.Confidence)
	}
}

func TestReader_ReadAudio(t *testing.T) {
	fr := NewFileReader(".")
	info, err := os.Stat(filepath.Join(testdataAbs(), "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	result := fr.readAudio(filepath.Join(testdataAbs(), "recording.mp3"), info)
	if result.Content != "" {
		t.Fatal("expected empty content for audio")
	}
	if result.CostNotice == "" {
		t.Fatal("expected cost notice for audio")
	}
}

func TestReader_ReadProjectFile_Dispatch(t *testing.T) {
	root := testdataAbs()
	fr := NewFileReader(root)

	tests := []struct {
		name    string
		relPath string
		want    string // substring expected in content
	}{
		{"text file", "test.txt", "测试文档"},
		{"docx file", "test.docx", "专利权利要求"},
		{"csv file", "test.csv", "华为"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := fr.ReadProjectFile(context.TODO(), tc.relPath)
			if err != nil {
				t.Fatalf("ReadProjectFile(%s): %v", tc.relPath, err)
			}
			if result.Content == "" {
				t.Fatal("expected non-empty content")
			}
			if !strings.Contains(result.Content, tc.want) {
				t.Fatalf("expected content to contain %q, got %q", tc.want, result.Content[:min(100, len(result.Content))])
			}
			if result.Category != "" && result.Category != CategoryUnknown {
				t.Logf("category=%s", result.Category)
			}
		})
	}
}

func TestReader_PathSandbox(t *testing.T) {
	fr := NewFileReader("/tmp/test-project")

	// Absolute path outside project should be rejected.
	_, err := fr.ReadProjectFile(context.TODO(), "/etc/passwd")
	if err == nil {
		t.Fatal("expected error for path outside root")
	}
	if !strings.Contains(err.Error(), "不在项目目录") {
		t.Fatalf("expected sandbox error, got: %v", err)
	}

	// Relative path with ../ escape should be rejected.
	_, err = fr.ReadProjectFile(context.TODO(), "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for relative path escape")
	}
}

func TestReader_PDF(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available, skipping PDF test")
	}

	pdfPath := filepath.Join(t.TempDir(), "test.pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		createMinimalPDF(t, pdfPath)
	}

	fr := NewFileReader(".")
	result, err := fr.readPDF(context.TODO(), pdfPath)
	if err != nil {
		t.Fatalf("readPDF: %v", err)
	}
	t.Logf("PDF result: content=%q, confidence=%f, costNotice=%q",
		truncatePreview(result.Content, 60), result.Confidence, result.CostNotice)
}

func createMinimalPDF(t *testing.T, path string) {
	t.Helper()
	content := `%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
   /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>
endobj
4 0 obj
<< /Length 44 >>
stream
BT /F1 12 Tf 100 700 Td (Test PDF Content) Tj ET
endstream
endobj
5 0 obj
<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>
endobj
xref
0 6
0000000000 65535 f
0000000009 00000 n
0000000058 00000 n
0000000115 00000 n
0000000266 00000 n
0000000360 00000 n
trailer
<< /Size 6 /Root 1 0 R >>
startxref
437
%%EOF`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
