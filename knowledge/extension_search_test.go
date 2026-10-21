package knowledge

import (
	"fmt"
	"sync"
	"testing"
)

func TestQueryEmbedCache_ConcurrentAccess(t *testing.T) {
	c := newQueryEmbedCache(8)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				q := fmt.Sprintf("query-%d", i)
				c.Set(q, []float32{float32(i)})
				_ = c.Get(q)
			}
		}()
	}
	wg.Wait()
}

func TestNormalizeQuery(t *testing.T) {
	cases := map[string]string{
		"  专利   侵权 ":          "专利 侵权",
		"Claim\tConstruction": "claim construction",
		"":                    "",
	}
	for in, want := range cases {
		if got := normalizeQuery(in); got != want {
			t.Errorf("normalizeQuery(%q) = %q, want %q", in, got, want)
		}
	}
}
