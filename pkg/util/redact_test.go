package util

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "openai style key",
			in:   "token sk-abcdef1234567890 leaked",
			want: "token [REDACTED] leaked",
		},
		{
			name: "masked api key from upstream 401",
			in:   "Authentication Fails, Your api key: ****7c08 is invalid",
			want: "Authentication Fails, Your api key: [REDACTED] is invalid",
		},
		{
			name: "authorization bearer",
			in:   "Authorization: Bearer abcdef123456",
			want: "Authorization: [REDACTED] [REDACTED]",
		},
		{
			name: "plain text untouched",
			in:   "provider_call failed: provider_stream (no credentials here)",
			want: "provider_call failed: provider_stream (no credentials here)",
		},
		{
			name: "short sk prefix is not a key",
			in:   "task sk-run finished",
			want: "task sk-run finished",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RedactSecrets(c.in); got != c.want {
				t.Fatalf("RedactSecrets(%q) = %q, want %q", c.in, got, c.want)
			}
			if strings.Contains(RedactSecrets(c.in), "abcdef1234567890") ||
				strings.Contains(RedactSecrets(c.in), "7c08") ||
				strings.Contains(RedactSecrets(c.in), "abcdef123456") {
				t.Fatalf("secret fragment survived redaction: %q", c.in)
			}
		})
	}
}
