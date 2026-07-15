package fileindex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestSearchFallback_Match(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "readme.txt", "hello")
	writeFile(t, dir, "claims/独立权利要求.txt", "claim content")
	writeFile(t, dir, "docs/guide.md", "guide")
	writeFile(t, dir, "unrelated.log", "noise")

	ctx := context.Background()
	result := searchFallback(ctx, dir, "claim", 10)

	if result.Total != 1 {
		t.Fatalf("expected 1 match, got %d (%v)", result.Total, result)
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}
	if result.Files[0].Path != "claims/独立权利要求.txt" {
		t.Errorf("expected claims/独立权利要求.txt, got %s", result.Files[0].Path)
	}
	if result.Files[0].Category != CategoryTextDoc {
		t.Errorf("expected CategoryTextDoc, got %v", result.Files[0].Category)
	}
	if result.Files[0].Relevance <= 0 {
		t.Errorf("expected positive relevance, got %f", result.Files[0].Relevance)
	}
}

func TestSearchFallback_NoMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "readme.txt", "hello")

	ctx := context.Background()
	result := searchFallback(ctx, dir, "nonexistent", 10)

	if result.Total != 0 {
		t.Fatalf("expected 0 matches, got %d", result.Total)
	}
	if result.Message == "" {
		t.Fatal("expected message for no-match")
	}
}

func TestSearchFallback_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "UpperCase.TXT", "content")

	ctx := context.Background()
	result := searchFallback(ctx, dir, "upper", 10)

	if result.Total != 1 {
		t.Fatalf("expected 1 match for case-insensitive search, got %d", result.Total)
	}
}

func TestSearchFallback_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "visible.txt", "hello")
	writeFile(t, dir, ".hidden/hidden.txt", "secret")
	writeFile(t, dir, "node_modules/pkg/index.js", "module")
	writeFile(t, dir, "vendor/lib/util.go", "code")

	ctx := context.Background()
	result := searchFallback(ctx, dir, ".txt", 10)

	if result.Total != 1 {
		t.Fatalf("expected only visible.txt, got %d: %v", result.Total, result.Files)
	}
	if result.Files[0].Path != "visible.txt" {
		t.Errorf("expected visible.txt, got %s", result.Files[0].Path)
	}
}

func TestSearchFallback_MaxResults(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("file_%d.txt", i) // needs fmt import
		writeFile(t, dir, name, "data")
	}
	// Note: we avoid fmt here intentionally for this file's planned context.
	// Using manual naming:
	writeFile(t, dir, "a.txt", "a")
	writeFile(t, dir, "b.txt", "b")
	writeFile(t, dir, "c.txt", "c")
	writeFile(t, dir, "d.txt", "d")
	writeFile(t, dir, "e.txt", "e")

	ctx := context.Background()
	result := searchFallback(ctx, dir, ".txt", 3)

	if result.Total > 3 {
		t.Fatalf("expected max 3 results, got %d", result.Total)
	}
	if len(result.Files) > 3 {
		t.Fatalf("expected max 3 files, got %d", len(result.Files))
	}
}

func TestSearchFallback_RelevanceOrdering(t *testing.T) {
	dir := t.TempDir()
	// Exact match in filename
	writeFile(t, dir, "exact.txt", "exact")
	// Prefix match
	writeFile(t, dir, "prefix_suffix.txt", "prefix")
	// Contains match
	writeFile(t, dir, "some_query_here.txt", "contains")
	// Path-only match (query in directory name)
	writeFile(t, dir, "query/other.txt", "path")

	ctx := context.Background()
	result := searchFallback(ctx, dir, "query", 10)

	if result.Total < 1 {
		t.Fatal("expected matches for 'query'")
	}
	// Verify descending relevance.
	for i := 1; i < len(result.Files); i++ {
		if result.Files[i].Relevance > result.Files[i-1].Relevance {
			t.Errorf("results not sorted by relevance descending: [%d]=%f > [%d]=%f",
				i, result.Files[i].Relevance, i-1, result.Files[i-1].Relevance)
		}
	}
}

func TestSearchFallback_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	ctx := context.Background()
	result := searchFallback(ctx, dir, "anything", 10)

	if result.Total != 0 {
		t.Fatalf("expected 0 matches in empty dir, got %d", result.Total)
	}
}

func TestSearchFallback_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	// Create many files so WalkDir takes measurable time.
	for i := 0; i < 100; i++ {
		writeFile(t, dir, fmt.Sprintf("f%d.txt", i), "data")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result := searchFallback(ctx, dir, "f", 50)

	// WalkDir should have been interrupted early, possibly returning no or partial results.
	// We just verify it doesn't panic and the function returns.
	if result.Files != nil {
		t.Logf("got %d results with canceled ctx", len(result.Files))
	}
	// searchFallback should have returned (with Message or Files), not hung.
	_ = result
}

func TestSearchFallback_MissingDir(t *testing.T) {
	ctx := context.Background()
	result := searchFallback(ctx, "/nonexistent/path/12345", "query", 10)

	if result.Message == "" {
		t.Fatal("expected error message for missing directory")
	}
	if result.Total != 0 {
		t.Fatalf("expected 0 results for missing dir, got %d", result.Total)
	}
}

func TestSearchFallback_RelevanceScores(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "query.txt", "exact match")        // 1.0
	writeFile(t, dir, "query_abc.txt", "prefix match")   // 0.8
	writeFile(t, dir, "abc_query_def.txt", "contains")   // 0.6
	writeFile(t, dir, "abc/query/file.txt", "path only") // 0.3

	ctx := context.Background()
	result := searchFallback(ctx, dir, "query", 10)

	files := result.Files
	// Sort for stable comparison.
	sort.Slice(files, func(i, j int) bool {
		if files[i].Relevance != files[j].Relevance {
			return files[i].Relevance > files[j].Relevance
		}
		return files[i].Path < files[j].Path
	})

	// Check all 3 scores present.
	scores := make(map[float64]bool)
	for _, f := range files {
		scores[f.Relevance] = true
	}
	if !scores[1.0] {
		t.Error("expected exact match (1.0)")
	}
	if !scores[0.8] {
		t.Error("expected prefix match (0.8)")
	}
	if !scores[0.6] {
		t.Error("expected contains match (0.6)")
	}
	if !scores[0.3] {
		t.Error("expected path-only match (0.3)")
	}
}

// writeFile creates a file with content under dir. Parent dirs are created as needed.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
