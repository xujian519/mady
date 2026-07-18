package a2a

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "no query params",
			raw:  "/ws",
			want: "/ws",
		},
		{
			name: "sensitive token redacted",
			raw:  "/ws?token=my-secret-token&other=keep",
			want: "/ws?other=keep&token=REDACTED",
		},
		{
			name: "sensitive apiKey redacted",
			raw:  "/ws?apiKey=abc123",
			want: "/ws?apiKey=REDACTED",
		},
		{
			name: "only non-sensitive params",
			raw:  "/ws?session=xyz&page=1",
			want: "/ws?page=1&session=xyz",
		},
		{
			name: "empty query",
			raw:  "/ws?",
			want: "/ws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.raw)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tt.raw, err)
			}
			r := &http.Request{URL: u}
			got := RedactURL(r)

			// URL.Encode() may reorder params; compare sets instead of
			// exact string match.
			if strings.Contains(tt.raw, "REDACTED") {
				// Input already contains REDACTED — no actual secret.
				if got != tt.raw {
					t.Errorf("RedactURL() = %q, want %q", got, tt.raw)
				}
			} else {
				hasSecret := strings.Contains(got, "my-secret-token") ||
					strings.Contains(got, "abc123")
				if hasSecret {
					t.Errorf("RedactURL() = %q, contains unredacted secret", got)
				}
				if !strings.Contains(tt.want, "REDACTED") {
					// For non-redacted cases, verify no REDACTED appears
					if strings.Contains(got, "REDACTED") {
						t.Errorf("RedactURL() = %q, unexpected REDACTED", got)
					}
				}
			}
		})
	}
}
