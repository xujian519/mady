package domains_test

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/prompt"
)

func TestResolveSystemPrompt(t *testing.T) {
	store, err := prompt.NewPromptStore()
	if err != nil {
		t.Fatalf("NewPromptStore: %v", err)
	}
	domains.SetupPromptStore(store)

	tests := []struct {
		name        string
		raw         string
		want        string
		checkPrefix bool
	}{
		{
			name: "inline unchanged",
			raw:  "you are helpful",
			want: "you are helpful",
		},
		{
			name:        "template reference",
			raw:         "prompt://claim-drafting",
			want:        "你是资深专利代理师，请根据技术交底书撰写符合中国专利法要求的权利要求书。",
			checkPrefix: true,
		},
		{
			name: "missing template fallback",
			raw:  "prompt://missing-template",
			want: "prompt://missing-template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domains.ResolveSystemPrompt(tt.raw)
			if tt.checkPrefix {
				if !strings.HasPrefix(got, tt.want) {
					t.Fatalf("got %q, want prefix %q", got, tt.want)
				}
			} else {
				if got != tt.want {
					t.Fatalf("got %q, want %q", got, tt.want)
				}
			}
		})
	}
}
