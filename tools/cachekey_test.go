package tools

import (
	"testing"

	"github.com/xujian519/mady/retrieval/domain"
)

func TestSearchCacheKeyOrderInsensitive(t *testing.T) {
	left := searchCacheKey(domain.DomainQuery{
		Text: "散热壳体", Keywords: []string{"散热", "壳体"}, MaxResults: 5,
		Filters: map[string]string{"ipc": "F28", "applicant": "某公司"},
	})
	right := searchCacheKey(domain.DomainQuery{
		Text: "散热壳体", Keywords: []string{"壳体", "散热"}, MaxResults: 5,
		Filters: map[string]string{"applicant": "某公司", "ipc": "F28"},
	})
	if left != right {
		t.Errorf("同一语义的查询应得到同一键：%q != %q", left, right)
	}
}

func TestSearchCacheKeyDistinguishesContent(t *testing.T) {
	cases := [][2]domain.DomainQuery{
		{
			{Text: "x", Keywords: []string{"a,b"}},
			{Text: "x", Keywords: []string{"a", "b"}},
		},
		{
			{Text: "x", MaxResults: 5},
			{Text: "x", MaxResults: 10},
		},
		{
			{Text: "x", Filters: map[string]string{"applicant": "AT&T"}},
			{Text: "x", Filters: map[string]string{"applicant": "AT", "T": ""}},
		},
	}
	for i, pair := range cases {
		if searchCacheKey(pair[0]) == searchCacheKey(pair[1]) {
			t.Errorf("case %d：不同查询不应共用缓存键", i)
		}
	}
}
