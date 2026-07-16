package wiring

import (
	"context"
	"errors"
	"testing"

	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/retrieval"
)

// fakeFTS implements FTSSearcher for testing without a real database.
type fakeFTS struct {
	chunks []retrieval.ScoredChunk
	err    error
	calls  int
	lastQ  string
	lastK  int
}

func (f *fakeFTS) FTSSearch(query string, topK int) ([]retrieval.ScoredChunk, error) {
	f.calls++
	f.lastQ = query
	f.lastK = topK
	if f.err != nil {
		return nil, f.err
	}
	return f.chunks, nil
}

func TestNewVectorRuleStore_NilInput(t *testing.T) {
	if got := NewVectorRuleStore(nil); got != nil {
		t.Fatalf("NewVectorRuleStore(nil) = %v, want nil", got)
	}
}

func TestVectorRuleStore_SearchRules_Mapping(t *testing.T) {
	fts := &fakeFTS{chunks: []retrieval.ScoredChunk{
		{
			Chunk: retrieval.Chunk{
				ID:      "c1",
				DocID:   "guide-p2-c3",
				Content: "单独对比原则：每篇对比文件单独与申请比较\n第二行",
			},
			Score: 0.82,
		},
		{
			Chunk: retrieval.Chunk{
				ID:      "c2",
				DocID:   "CN110xxxxx",
				Content: "一种电路拓扑结构……",
			},
			Score: 0.65,
		},
	}}
	v := NewVectorRuleStore(fts)

	rules, err := v.SearchRules(context.Background(), "新颖性 单独对比", 5)
	if err != nil {
		t.Fatalf("SearchRules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	if fts.calls != 1 || fts.lastQ != "新颖性 单独对比" || fts.lastK != 5 {
		t.Fatalf("FTSSearch call args = (%q,%d), want (新颖性 单独对比,5)", fts.lastQ, fts.lastK)
	}

	r0 := rules[0]
	if r0.Source != reasoning.RuleSourceVector {
		t.Errorf("Source = %q, want %q", r0.Source, reasoning.RuleSourceVector)
	}
	if r0.Priority != 2 {
		t.Errorf("Priority = %d, want 2", r0.Priority)
	}
	if r0.AuthorityScore != 0.7 {
		t.Errorf("AuthorityScore = %v, want 0.7", r0.AuthorityScore)
	}
	if r0.Confidence != 0.82 {
		t.Errorf("Confidence = %v, want 0.82", r0.Confidence)
	}
	if r0.Rule.ArticleID != "guide-p2-c3" {
		t.Errorf("ArticleID = %q, want guide-p2-c3", r0.Rule.ArticleID)
	}
	if r0.Rule.ArticleName != "单独对比原则：每篇对比文件单独与申请比较" {
		t.Errorf("ArticleName = %q, want first non-empty line", r0.Rule.ArticleName)
	}
	if r0.Rule.Requirement != reasoning.ReqNote {
		t.Errorf("Requirement = %q, want ReqNote", r0.Rule.Requirement)
	}
	if r0.Baggage == "" {
		t.Error("Baggage should preserve raw content")
	}
}

func TestVectorRuleStore_SearchRules_PropagatesError(t *testing.T) {
	fts := &fakeFTS{err: errors.New("db locked")}
	v := NewVectorRuleStore(fts)

	if _, err := v.SearchRules(context.Background(), "q", 3); err == nil {
		t.Fatal("expected error propagation, got nil")
	}
}

func TestVectorRuleStore_SearchRules_EmptyResults(t *testing.T) {
	v := NewVectorRuleStore(&fakeFTS{})
	rules, err := v.SearchRules(context.Background(), "无匹配", 5)
	if err != nil {
		t.Fatalf("SearchRules: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("got %d rules, want empty", len(rules))
	}
}

func TestVectorRuleStore_NilReceiverSafe(t *testing.T) {
	var v *VectorRuleStore
	rules, err := v.SearchRules(context.Background(), "q", 1)
	if err != nil {
		t.Fatalf("nil receiver SearchRules: %v", err)
	}
	if rules != nil {
		t.Fatalf("nil receiver got %v, want nil", rules)
	}
}

func TestFirstLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"\n\n  \n实际第一行", "实际第一行"},
		{"第一行\n第二行", "第一行"},
	}
	for _, c := range cases {
		if got := firstLine(c.in); got != c.want {
			t.Errorf("firstLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// long line truncation
	long := string(make([]rune, 200))
	if got := firstLine(long); len([]rune(got)) > 81 {
		t.Errorf("firstLine not truncated: len=%d", len([]rune(got)))
	}
}
