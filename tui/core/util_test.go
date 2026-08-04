package core

import (
	"strings"
	"testing"
)

// TestHighlightMatches verifies matched-index wrapping and nil/empty guards.
func TestHighlightMatches(t *testing.T) {
	if got := HighlightMatches("abc", nil, nil); got != "abc" {
		t.Fatalf("HighlightMatches(nil markFn) = %q, want unchanged", got)
	}
	if got := HighlightMatches("abc", nil, func(s string) string { return "[" + s + "]" }); got != "abc" {
		t.Fatalf("HighlightMatches(empty indexes) = %q, want unchanged", got)
	}

	// Byte-offset semantics: indexes are byte positions in the candidate
	// ("你" occupies bytes 1-3, so "b" is at byte 4).
	got := HighlightMatches("a你b", []int64{0, 4}, func(s string) string { return "{" + s + "}" })
	want := "{a}你{b}"
	if got != want {
		t.Fatalf("HighlightMatches = %q, want %q", got, want)
	}
}

// TestNormalizeForMatch covers quote/dash/space normalization and CR stripping.
func TestNormalizeForMatch(t *testing.T) {
	in := "He said “hi”\r\n— fine. done"
	got := NormalizeForMatch(in)
	if strings.Contains(got, "“") || strings.Contains(got, "”") {
		t.Errorf("NormalizeForMatch left smart quotes: %q", got)
	}
	if strings.Contains(got, "\r") {
		t.Errorf("NormalizeForMatch left CR: %q", got)
	}
	if !strings.Contains(got, `"hi"`) {
		t.Errorf("NormalizeForMatch did not normalize quotes: %q", got)
	}
	if !strings.Contains(got, "- fine.") {
		t.Errorf("NormalizeForMatch did not normalize em-dash: %q", got)
	}
}

// TestFindNormalized verifies the normalized fallback maps byte offsets back
// to the original content (smart quotes → ASCII search hits).
func TestFindNormalized(t *testing.T) {
	content := "原告主张“新颖性”成立"
	search := `"新颖性"`
	start, end, found := Find(content, search)
	if !found {
		t.Fatal("Find with normalized search = not found, want found")
	}
	if start < 0 || end <= start || end > int64(len(content)) {
		t.Fatalf("Find offsets out of range: [%d,%d) len=%d", start, end, len(content))
	}
	// The matched slice must be the original quoted text.
	if got := content[start:end]; got != "“新颖性”" {
		t.Fatalf("Find slice = %q, want %q", got, "“新颖性”")
	}

	// Empty normalized search is not found.
	if _, _, found := Find("x", "   "); found {
		t.Fatal("Find(blank) = found, want not found")
	}
}

// TestPutDiffCellsPooling verifies pool reuse for short slices and pass-through
// for oversized ones (no panic, no corruption).
func TestPutDiffCells(t *testing.T) {
	small := getDiffCells(8)
	if cap(small) < 8 {
		t.Fatalf("getDiffCells(8) cap = %d, want >= 8", cap(small))
	}
	small[0] = Cell{Rune: 'x'}
	PutDiffCells(small)

	large := getDiffCells(64) // > 32 → not pooled
	if len(large) != 64 {
		t.Fatalf("getDiffCells(64) len = %d, want 64", len(large))
	}
	PutDiffCells(large) // must not panic

	// A re-borrowed small slice must be reusable.
	again := getDiffCells(4)
	PutDiffCells(again)
}
