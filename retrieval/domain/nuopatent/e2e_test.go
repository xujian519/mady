package nuopatent

import (
	"context"
	"os"
	"testing"

	"github.com/xujian519/mady/retrieval/domain"
)

// TestE2ESearch 仅在 MADY_E2E=1 且本机存在 nuo-patent 时运行真实 CLI；CI 默认跳过。
func TestE2ESearch(t *testing.T) {
	if os.Getenv("MADY_E2E") != "1" {
		t.Skip("e2e tests disabled: set MADY_E2E=1 to run real nuo-patent CLI")
	}
	r, err := NewNuoPatentRetriever(Config{})
	if err != nil {
		t.Fatalf("NewNuoPatentRetriever: %v", err)
	}
	if r == nil {
		t.Skip("nuo-patent CLI not available")
	}
	res, err := r.Search(context.Background(), domain.DomainQuery{Text: "sensor", MaxResults: 3})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	t.Logf("e2e results: %d documents", len(res.Documents))
}
