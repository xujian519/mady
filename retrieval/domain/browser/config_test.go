package browser

import "testing"

// TestRetrieversEnabled 验证 MADY_BROWSER_RETRIEVERS 门控的取值族：
// 空（未设置）与任意非禁用值为启用；off/false/0/disabled/no（大小写不敏感）
// 为禁用。
func TestRetrieversEnabled(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"", true},         // 未设置 → 启用（默认行为）
		{"off", false},     // 既有约定
		{"OFF", false},     // 大小写不敏感
		{" false ", false}, // 容忍空白
		{"false", false},
		{"0", false},
		{"disabled", false},
		{"no", false},
		{"1", true},    // 非禁用值
		{"yes", true},  // 非禁用值
		{"auto", true}, // 非禁用值
	}
	for _, tc := range cases {
		t.Setenv("MADY_BROWSER_RETRIEVERS", tc.val)
		if got := RetrieversEnabled(); got != tc.want {
			t.Errorf("RetrieversEnabled(%q) = %v, want %v", tc.val, got, tc.want)
		}
	}
}
