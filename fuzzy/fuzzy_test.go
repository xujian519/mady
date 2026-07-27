package fuzzy

import "testing"

func TestNormalizeForMatch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"  spaced  ", "  spaced"},
		{"smart'quote", "smart'quote"},
		{"‘curly’", "'curly'"},
		{"“double”", `"double"`},
		{"–dash—", "-dash-"},
		{"line1\nline2  ", "line1\nline2"},
		{" nbsp", " nbsp"},
		{"\rskip", "skip"},
	}
	for _, tc := range tests {
		got := NormalizeForMatch(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeForMatch(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestFind(t *testing.T) {
	content := "The quick brown fox"

	// exact match
	start, end, found := Find(content, "brown")
	if !found || content[start:end] != "brown" {
		t.Errorf("Find exact: got %d,%d,%v", start, end, found)
	}

	// no match
	if _, _, found := Find(content, "purple"); found {
		t.Error("Find: purple should not match")
	}

	// normalized match: curly quotes
	curlyContent := "The “quick” brown fox"
	start, end, found = Find(curlyContent, "\"quick\"")
	if !found || curlyContent[start:end] != "“quick”" {
		t.Errorf("Find normalized: got %d,%d,%v", start, end, found)
	}

	// empty search: Index returns 0 (empty string matches at position 0)
	start, end, found = Find(content, "")
	if !found {
		t.Error("Find: empty search should match at position 0")
	}
	if start != 0 || end != 0 {
		t.Errorf("Find empty: got %d,%d", start, end)
	}

	// case sensitive
	if _, _, found := Find(content, "BROWN"); found {
		t.Error("Find: case sensitive match should not find uppercase")
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int64
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"sitting", "kitten", 3},
		{"abc", "def", 3},
	}
	for _, tc := range tests {
		got := LevenshteinDistance(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestMapNormalizedOffset(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		normalized string
		normOffset int
		want       int
	}{
		{"simple", "hello", "hello", 2, 2},
		{"skip CR", "a\rb", "ab", 1, 1},      // '\r' stripped before 'b' is reached; offset maps to 'a'+'\r' position
		{"curly quote", "“hi", "\"hi", 1, 3}, // curly quote folded to 1-byte '"', original is 3 bytes
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mapNormalizedOffset(tc.original, tc.normalized, tc.normOffset)
			if got != tc.want {
				t.Errorf("mapNormalizedOffset(%q, %q, %d) = %d, want %d",
					tc.original, tc.normalized, tc.normOffset, got, tc.want)
			}
		})
	}
}
