package util

import "testing"

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		in     string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is way too long", 7, "this is…"},
		{"", 5, ""},
		{"abc", 0, "…"},
		{"abc", -1, "…"},
		{"中文截断测试文本", 4, "中文截断…"},
	}
	for _, tc := range cases {
		if got := TruncateRunes(tc.in, tc.maxLen); got != tc.want {
			t.Errorf("TruncateRunes(%q, %d) = %q, want %q", tc.in, tc.maxLen, got, tc.want)
		}
	}
	// rune 安全：4 字节 emoji 不被劈开。
	if got := TruncateRunes("a\U0001F600bc", 2); got != "a\U0001F600…" {
		t.Errorf("multi-byte rune must not be split, got %q", got)
	}
}
