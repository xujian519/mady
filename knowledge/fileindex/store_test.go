package fileindex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	fi, err := OpenFileIndex(dir, dbPath)
	if err != nil {
		t.Fatalf("OpenFileIndex: %v", err)
	}
	defer fi.Close()

	if fi.Dir() != dir {
		t.Fatalf("Dir()=%q want %q", fi.Dir(), dir)
	}
}

func TestRefreshAndSearch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create test files.
	mustWriteFile(t, filepath.Join(dir, "OA1_rejection.txt"), "第一次审查意见通知书：权利要求1不具备创造性")
	mustWriteFile(t, filepath.Join(dir, "OA2_office_action.txt"), "第二次审查意见通知书：权利要求2-5不清楚")
	mustWriteFile(t, filepath.Join(dir, "claim_set.docx"), "权利要求书——独立权利要求和从属权利要求")
	mustWriteFile(t, filepath.Join(dir, "evidence/prior_art.pdf"), "对比文件1：US20200012345A1")

	fi, err := OpenFileIndex(dir, dbPath)
	if err != nil {
		t.Fatalf("OpenFileIndex: %v", err)
	}
	defer fi.Close()

	// Refresh index.
	if err := fi.Refresh(context.TODO()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// 2nd refresh should be idempotent (no crash, no duplicate records).
	if err := fi.Refresh(context.TODO()); err != nil {
		t.Fatalf("2nd Refresh: %v", err)
	}

	// Search by filename/query.
	results, err := fi.Search(context.TODO(), "审查意见", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result for query '审查意见'")
	}
	t.Logf("Search results for '审查意见':")
	for _, r := range results {
		t.Logf("  path=%s category=%s relevance=%.3f preview=%q",
			r.Path, r.Category, r.Relevance, truncatePreview(r.Preview, 60))
	}
}

func TestSearchByFileName(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	mustWriteFile(t, filepath.Join(dir, "novelty_report.pdf"), "新颖性对比分析报告")
	mustWriteFile(t, filepath.Join(dir, "patentability_opinion.txt"), "可专利性分析意见")
	mustWriteFile(t, filepath.Join(dir, "random_notes.txt"), "一些笔记")

	fi, err := OpenFileIndex(dir, dbPath)
	if err != nil {
		t.Fatalf("OpenFileIndex: %v", err)
	}
	defer fi.Close()

	fi.Refresh(context.TODO())

	// Search for patentability — filename match should score highly.
	results, err := fi.Search(context.TODO(), "patentability", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'patentability'")
	}
	if results[0].Path != filepath.Join(dir, "patentability_opinion.txt") {
		t.Fatalf("top result=%q want %q", results[0].Path, filepath.Join(dir, "patentability_opinion.txt"))
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	mustWriteFile(t, filepath.Join(dir, "old.txt"), "旧文件")
	mustWriteFile(t, filepath.Join(dir, "new.txt"), "新文件")

	fi, err := OpenFileIndex(dir, dbPath)
	if err != nil {
		t.Fatalf("OpenFileIndex: %v", err)
	}
	defer fi.Close()
	fi.Refresh(context.TODO())

	// Empty query should return most recently modified.
	results, err := fi.Search(context.TODO(), "", 10)
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for empty query, got %d", len(results))
	}
}

func TestFileCategory(t *testing.T) {
	tests := []struct {
		path string
		cat  FileCategory
	}{
		{"foo.txt", CategoryTextDoc},
		{"foo.md", CategoryTextDoc},
		{"foo.go", CategoryTextDoc},
		{"foo.pdf", CategoryPdf},
		{"foo.jpg", CategoryImage},
		{"foo.png", CategoryImage},
		{"foo.mp3", CategoryAudio},
		{"foo.m4a", CategoryAudio},
		{"foo.xlsx", CategorySpreadsheet},
		{"foo.csv", CategorySpreadsheet},
		{"foo.xyz", CategoryUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := classifyFile(tc.path); got != tc.cat {
				t.Fatalf("classifyFile(%q)=%q want %q", tc.path, got, tc.cat)
			}
		})
	}
}

func TestRefresh_NewFileAfterInitialScan(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	fi, err := OpenFileIndex(dir, dbPath)
	if err != nil {
		t.Fatalf("OpenFileIndex: %v", err)
	}
	defer fi.Close()

	// Initial scan with one file.
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "File A")
	if err := fi.Refresh(context.TODO()); err != nil {
		t.Fatalf("initial Refresh: %v", err)
	}

	r1, _ := fi.Search(context.TODO(), "a.txt", 10)
	if len(r1) != 1 {
		t.Fatalf("expected 1 file after initial refresh, got %d", len(r1))
	}

	// Add a new file and re-scan.
	mustWriteFile(t, filepath.Join(dir, "b.txt"), "UniqueContentB")
	if err := fi.Refresh(context.TODO()); err != nil {
		t.Fatalf("incremental Refresh: %v", err)
	}

	r2, _ := fi.Search(context.TODO(), "UniqueContentB", 10)
	if len(r2) == 0 {
		t.Fatal("expected at least 1 result for new file")
	}
	if !strings.HasSuffix(r2[0].Path, "b.txt") {
		t.Fatalf("top result path=%q expected .../b.txt", r2[0].Path)
	}
}

func TestRefresh_DeletedFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	fi, err := OpenFileIndex(dir, dbPath)
	if err != nil {
		t.Fatalf("OpenFileIndex: %v", err)
	}
	defer fi.Close()

	mustWriteFile(t, filepath.Join(dir, "keep.txt"), "Keep me")
	mustWriteFile(t, filepath.Join(dir, "delete.txt"), "Delete me")
	fi.Refresh(context.TODO())

	// Delete one file.
	os.Remove(filepath.Join(dir, "delete.txt"))
	fi.Refresh(context.TODO())

	results, _ := fi.Search(context.TODO(), "", 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 file after deletion, got %d", len(results))
	}
}

func TestFileIndex_Reopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	fi, err := OpenFileIndex(dir, dbPath)
	if err != nil {
		t.Fatalf("first OpenFileIndex: %v", err)
	}
	mustWriteFile(t, filepath.Join(dir, "persist.txt"), "Persistent data")
	fi.Refresh(context.TODO())
	fi.Close()

	// Reopen the same database — data should persist.
	fi2, err := OpenFileIndex(dir, dbPath)
	if err != nil {
		t.Fatalf("second OpenFileIndex: %v", err)
	}
	defer fi2.Close()

	results, _ := fi2.Search(context.TODO(), "persistent", 10)
	if len(results) == 0 {
		t.Fatal("expected persisted data after reopen")
	}
}

func TestScoreFileName(t *testing.T) {
	tests := []struct {
		query, filename string
		minScore        float64
	}{
		{"审查意见", "OA1_审查意见.txt", 0.5},
		{"审查意见", "other.txt", 0.0},
		{"novelty", "novelty_report.pdf", 0.5},
		{"patent", "patentability_opinion.txt", 0.5},
	}
	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			score := scoreFileName(tc.query, tc.filename)
			if score < tc.minScore {
				t.Fatalf("scoreFileName(%q, %q)=%f < min %f", tc.query, tc.filename, score, tc.minScore)
			}
		})
	}
}

func TestClassifyFile(t *testing.T) {
	if cat := classifyFile("test.txt"); cat != CategoryTextDoc {
		t.Fatalf("expected text_doc, got %q", cat)
	}
	if cat := classifyFile("test.pdf"); cat != CategoryPdf {
		t.Fatalf("expected pdf, got %q", cat)
	}
	if cat := classifyFile("test.jpg"); cat != CategoryImage {
		t.Fatalf("expected image, got %q", cat)
	}
}

func TestScoreRecency(t *testing.T) {
	now := mustParseTime("2026-07-13T00:00:00Z")

	// Zero days ago = 1.0
	if s := scoreRecency(now, mustParseTime("2026-07-13T00:00:00Z")); s != 1.0 {
		t.Fatalf("expected 1.0, got %f", s)
	}

	// 30 days ago ~ 0.5
	s := scoreRecency(now, mustParseTime("2026-06-13T00:00:00Z"))
	if s < 0.4 || s > 0.6 {
		t.Fatalf("expected ~0.5 for 30 days, got %f", s)
	}
}

func TestPathSegments(t *testing.T) {
	path := "证据/审查意见/OA1.pdf"

	// Query matching a segment should score > 0.
	if s := scorePathSegments("审查意见", path); s <= 0 {
		t.Fatalf("expected >0 for '审查意见', got %f", s)
	}

	// Unrelated query should score 0.
	if s := scorePathSegments("random", path); s != 0 {
		t.Fatalf("expected 0 for 'random', got %f", s)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
