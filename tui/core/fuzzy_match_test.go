package core

import "testing"

func TestFuzzyFilterSortsByScore(t *testing.T) {
	cands := []string{"helloWorld", "hello", "hi-world", "abc"}
	res := FuzzyFilter("hw", cands)
	if len(res) < 2 {
		t.Fatalf("expected >=2 matches, got %d", len(res))
	}
	// Top match should be one that starts with 'h' and contains 'w'.
	top := res[0].Original
	if top != "helloWorld" && top != "hi-world" {
		t.Fatalf("unexpected top match: %q (res=%v)", top, res)
	}
	// 'abc' should be filtered out.
	for _, m := range res {
		if m.Original == "abc" {
			t.Fatalf("abc shouldn't match")
		}
	}
}

func TestFuzzyFilterEmptyQuery(t *testing.T) {
	cands := []string{"a", "b"}
	res := FuzzyFilter("", cands)
	if len(res) != 2 {
		t.Fatalf("empty query should keep all, got %d", len(res))
	}
}

func TestFindExactMatch(t *testing.T) {
	start, end, ok := Find("hello world", "world")
	if !ok || start != 6 || end != 11 {
		t.Fatalf("expected (6, 11, true), got (%d, %d, %v)", start, end, ok)
	}
}

func TestFindNormalizedMatch(t *testing.T) {
	// Smart quotes should match after normalization.
	// "他说“你好”": bytes 6-9=“, 9-12=你, 12-15=好, 15-18=”.
	start, end, ok := Find("他说“你好”", "“你好”")
	if !ok || start != 6 || end != 18 {
		t.Fatalf("expected (6, 18, true), got (%d, %d, %v)", start, end, ok)
	}
}

func TestFindNoMatch(t *testing.T) {
	_, _, ok := Find("hello world", "xyz")
	if ok {
		t.Fatal("expected no match")
	}
}

func TestFindEmptySearch(t *testing.T) {
	// strings.Index finds empty string at position 0, so Find returns true.
	start, end, ok := Find("hello", "")
	if !ok || start != 0 || end != 0 {
		t.Fatalf("empty search: expected (0, 0, true), got (%d, %d, %v)", start, end, ok)
	}
}

func TestNormalizeForMatchTrimsTrailingSpace(t *testing.T) {
	// NormalizeForMatch trims trailing whitespace per line, preserving leading space.
	got := NormalizeForMatch("  hello  \n  world  ")
	want := "  hello\n  world"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeForMatchSmartPunctuation(t *testing.T) {
	got := NormalizeForMatch("‘hello’ “world”")
	want := "'hello' \"world\""
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeForMatchCarriageReturn(t *testing.T) {
	got := NormalizeForMatch("hello\r\nworld")
	want := "hello\nworld"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeForMatchEmDash(t *testing.T) {
	got := NormalizeForMatch("hello—world")
	want := "hello-world"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
