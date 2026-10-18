package domains

// audit_extension_test.go 覆盖审计辅助函数的边界用例：参数扫描对未闭合
// JSON 的越界保护，以及详情截断不得产出非法 UTF-8。

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExtractArgNames(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"null", "null", nil},
		{"empty object", "{}", nil},
		{"single key", `{"a":1}`, []string{"a"}},
		{"nested and escaped", `{"a":{"b":"x\"y"},"c":2}`, []string{"a", "b", "c"}},
		{"non-json", `[1,2]`, []string{"<non-json>"}},
		// 未闭合引号曾触发 raw[j+1:] 切片越界 panic。
		{"unterminated string", `{"a":"x`, []string{"a"}},
		{"unterminated key", `{"abc`, nil},
		{"only quote", `{"`, nil},
		{"trailing backslash", `{"a":"x\`, []string{"a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractArgNames(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("extractArgNames(%q) = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("extractArgNames(%q) = %v, want %v", c.in, got, c.want)
				}
			}
		})
	}
}

func TestTruncateAuditDetail(t *testing.T) {
	short := "短详情"
	if got := truncateAuditDetail(short); got != short {
		t.Errorf("short detail changed: %q", got)
	}

	// 中文重复填充，确保截断点落在多字节字符中间时被回退到字符边界。
	long := strings.Repeat("审", 400)
	got := truncateAuditDetail(long)
	if len(got) > 500 {
		t.Fatalf("truncated length = %d, want <= 500", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncated detail is not valid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncated detail should end with ellipsis: %q", got)
	}
}

// TestHandleListCases_InvalidArgs 验证非法参数返回错误结果，而非按零值
// 查询返回全部案件（旧实现忽略 Unmarshal 错误，模型会把全量结果当成筛选命中）。
func TestHandleListCases_InvalidArgs(t *testing.T) {
	e := &CaseExtension{}
	got, err := e.handleListCases(context.Background(), []byte(`{"year":"2024"}`))
	if err != nil {
		t.Fatalf("handleListCases returned error: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", got)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Fatalf("result = %v, want error entry", m)
	}
}
