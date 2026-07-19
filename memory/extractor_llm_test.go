package memory

import (
	"testing"
)

func TestParseFactsFromResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int // expected number of facts
	}{
		{
			name:    "standard json",
			content: `{"facts": ["用户偏好使用表格", "用户从事专利代理工作"]}`,
			want:    2,
		},
		{
			name:    "json with markdown fences",
			content: "```json\n{\"facts\": [\"用户偏好使用表格\"]}\n```",
			want:    1,
		},
		{
			name:    "empty facts array",
			content: `{"facts": []}`,
			want:    0,
		},
		{
			name:    "filtered empty strings",
			content: `{"facts": ["用户偏好使用表格", "", "   ", "无"]}`,
			want:    1,
		},
		{
			name:    "invalid json",
			content: `not json at all`,
			want:    -1, // error expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := parseFactsFromResponse(tt.content)
			if tt.want == -1 {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(facts) != tt.want {
				t.Errorf("got %d facts, want %d", len(facts), tt.want)
			}
		})
	}
}

func TestParseFactsFromText(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{
			name:    "plain lines",
			content: "用户偏好使用表格\n用户从事专利代理工作\n助手建议三步法",
			want:    3,
		},
		{
			name:    "numbered lines",
			content: "1. 用户偏好使用表格\n2. 用户从事专利代理工作",
			want:    2,
		},
		{
			name:    "dash prefixed",
			content: "- 用户偏好使用表格\n- 用户从事专利代理工作",
			want:    2,
		},
		{
			name:    "empty and noise",
			content: "\n\n无\n用户偏好使用表格\n",
			want:    1,
		},
		{
			name:    "only noise",
			content: "无",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := parseFactsFromText(tt.content)
			if len(facts) != tt.want {
				t.Errorf("got %d facts, want %d: %v", len(facts), tt.want, facts)
			}
		})
	}
}

func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"```json\n{\"a\":1}\n```", "{\"a\":1}"},
		{"```\ntext\n```", "text"},
		{"plain text", "plain text"},
		{"```json\n{\"a\":1}", "{\"a\":1}"},
	}

	for _, tt := range tests {
		t.Run(tt.input[:min(20, len(tt.input))], func(t *testing.T) {
			got := stripMarkdownFences(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewProviderExtractor(t *testing.T) {
	// nil provider is acceptable — just won't be usable, but shouldn't panic.
	e := NewProviderExtractor(nil, "test-model")
	if e == nil {
		t.Fatal("expected non-nil extractor")
	}
	if e.model != "test-model" {
		t.Errorf("model = %q, want %q", e.model, "test-model")
	}

	// Empty model defaults to "default".
	e2 := NewProviderExtractor(nil, "")
	if e2.model != "default" {
		t.Errorf("model = %q, want %q", e2.model, "default")
	}
}
