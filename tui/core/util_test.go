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

// TestSerializeRows verifies CRLF joining, empty-input short-circuit, and
// round-trip through ParseLine.
func TestSerializeRows(t *testing.T) {
	if got := SerializeRows(nil); got != "" {
		t.Fatalf("SerializeRows(nil) = %q, want empty", got)
	}

	rows := []Row{
		ParseLine("\x1b[31mred\x1b[0m"),
		ParseLine("plain"),
	}
	out := SerializeRows(rows)
	if !strings.Contains(out, "\r\n") {
		t.Fatalf("SerializeRows missing CRLF: %q", out)
	}

	// Round-trip: re-parsing the serialized output yields the same visible text.
	reparsed := strings.Split(out, "\r\n")
	if len(reparsed) != 2 {
		t.Fatalf("re-parsed lines = %d, want 2", len(reparsed))
	}
	if v := VisibleWidth(reparsed[0]); v != 3 {
		t.Fatalf("first line visible width = %d, want 3 (red)", v)
	}
	if v := VisibleWidth(reparsed[1]); v != 5 {
		t.Fatalf("second line visible width = %d, want 5 (plain)", v)
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
