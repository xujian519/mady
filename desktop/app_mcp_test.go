//go:build darwin

package main

import (
	"reflect"
	"testing"
)

// TestSanitizeMcpArgs 覆盖等号内联与空格分隔两种敏感参数形式（S-7）。
func TestSanitizeMcpArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "等号内联形式掩码",
			args: []string{"--api-key=sk-xxxx", "--verbose"},
			want: []string{"--api-key=***", "--verbose"},
		},
		{
			name: "空格分隔形式掩码值参数",
			args: []string{"--api-key", "sk-xxxx", "--verbose"},
			want: []string{"--api-key", "***", "--verbose"},
		},
		{
			name: "多个敏感参数混合形式",
			args: []string{"--token=abc", "--secret", "xyz", "--password", "pwd"},
			want: []string{"--token=***", "--secret", "***", "--password", "***"},
		},
		{
			name: "末尾敏感键名无值参数不误伤",
			args: []string{"--model", "gpt-4", "--api-key"},
			want: []string{"--model", "gpt-4", "--api-key"},
		},
		{
			name: "普通参数保持原样",
			args: []string{"--model", "gpt-4", "--temperature", "0.2"},
			want: []string{"--model", "gpt-4", "--temperature", "0.2"},
		},
		{
			name: "空参数列表",
			args: []string{},
			want: []string{},
		},
		{
			name: "大小写不敏感",
			args: []string{"--API_KEY=abc"},
			want: []string{"--API_KEY=***"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeMcpArgs(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sanitizeMcpArgs(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
