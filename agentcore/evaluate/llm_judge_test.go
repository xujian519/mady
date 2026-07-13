package evaluate

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

func approxEqualFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestParseLLMJudgeScore(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{`{"conclusion": 0.8, "reasoning": 0.6, "citation": 0.7}`, 0.7},
		{`{"conclusion":1,"reasoning":1,"citation":1}`, 1.0},
		{`{"conclusion":0,"reasoning":0,"citation":0}`, 0.0},
		{`{"conclusion": 0.9, "reasoning": 0.5, "citation": 0.3}`, 0.5666666666666667},
		{"0.8", 0.8},
		{"8/10", 0.8},
		{"85%", 0.85},
		{"最终评分为 0.75", 0.75},
		{"8", 0.8},
		{"```json\n{\"conclusion\": 0.7, \"reasoning\": 0.7, \"citation\": 0.7}\n```", 0.7},
	}

	for _, c := range cases {
		got := parseLLMJudgeScore(c.input)
		if !approxEqualFloat(got, c.expected) {
			t.Errorf("parseLLMJudgeScore(%q) = %v, want %v", c.input, got, c.expected)
		}
	}
}

func TestClampScore(t *testing.T) {
	cases := []struct {
		input, expected float64
	}{
		{-0.5, 0},
		{0.5, 0.5},
		{1.5, 1},
	}
	for _, c := range cases {
		if got := clampScore(c.input); got != c.expected {
			t.Errorf("clampScore(%v) = %v, want %v", c.input, got, c.expected)
		}
	}
}

func TestNormalizeScore(t *testing.T) {
	cases := []struct {
		input, expected float64
	}{
		{0.5, 0.5},
		{5, 0.5},
		{50, 0.5},
		{1, 1},
	}
	for _, c := range cases {
		if got := normalizeScore(c.input); got != c.expected {
			t.Errorf("normalizeScore(%v) = %v, want %v", c.input, got, c.expected)
		}
	}
}

func TestTruncateForJudgeUTF8(t *testing.T) {
	// Construct a string with >6000 runes where byte offset 3000 would land
	// in the middle of a multi-byte UTF-8 character.
	prefix := strings.Repeat("中", 999) + "AB"
	long := prefix + strings.Repeat("文", 5001)
	if runeLen(long) <= 6000 {
		t.Fatalf("test string should exceed 6000 runes, got %d", runeLen(long))
	}

	truncated := truncateForJudge(long)
	if !utf8.ValidString(truncated) {
		t.Errorf("truncateForJudge produced invalid UTF-8")
	}
	if !strings.Contains(truncated, prefix) {
		t.Errorf("truncateForJudge should preserve the start of the string")
	}
	if !strings.Contains(truncated, "...[中间内容省略]...") {
		t.Errorf("truncateForJudge should include an omission marker")
	}
}
