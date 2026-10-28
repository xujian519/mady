package guardrails

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/pkg/lawcite"
)

func newIndexSource(t *testing.T) *IndexCitationSource {
	t.Helper()
	src, err := NewIndexCitationSource(nil)
	if err != nil {
		t.Fatalf("NewIndexCitationSource: %v", err)
	}
	if len(src.Indexed()) != 2 {
		t.Fatalf("indexed = %v", src.Indexed())
	}
	return src
}

func TestIndexSourceResolveVerified(t *testing.T) {
	src := newIndexSource(t)
	res := src.Resolve(lawcite.Citation{Statute: lawcite.StatutePatentLaw, Article: 22})
	if res.Status != lawcite.StatusVerified {
		t.Fatalf("status=%v reason=%s", res.Status, res.Reason)
	}
	if res.Article == nil || !strings.Contains(res.Article.SourceDoc, "专利法") {
		t.Fatalf("article=%+v", res.Article)
	}
}

func TestIndexSourceResolveUntranscribed(t *testing.T) {
	src := newIndexSource(t)
	// 第 1 条在 82 条范围内但未被索引收录。
	res := src.Resolve(lawcite.Citation{Statute: lawcite.StatutePatentLaw, Article: 1})
	if res.Status != lawcite.StatusExistOnly {
		t.Fatalf("status=%v, want ExistOnly", res.Status)
	}
}

func TestIndexSourceResolveWithoutIndex(t *testing.T) {
	src := newIndexSource(t)
	res := src.Resolve(lawcite.Citation{Statute: lawcite.StatuteTrademarkLaw, Article: 57})
	if res.Status != lawcite.StatusUnverified {
		t.Fatalf("status=%v, want Unverified", res.Status)
	}
	if !strings.Contains(res.Reason, "尚无随包条文索引") {
		t.Fatalf("reason=%q", res.Reason)
	}
}

func TestIndexSourceTopicsPreferIndex(t *testing.T) {
	src := newIndexSource(t)
	topics, ok := src.Topics(lawcite.StatutePatentLaw, 22)
	if !ok || len(topics) == 0 {
		t.Fatalf("topics=%v ok=%v", topics, ok)
	}
	if !strings.Contains(strings.Join(topics, "、"), "创造性") {
		t.Fatalf("topics=%v", topics)
	}
}

func TestIndexSourceTopicsFallbackToStaticTable(t *testing.T) {
	src := newIndexSource(t)
	// 商标法没有随包索引，主题词应来自静态表。
	if _, ok := src.Topics(lawcite.StatuteTrademarkLaw, 57); !ok {
		t.Fatal("静态表应覆盖商标法第57条")
	}
}

func TestIndexSourceMaxArticle(t *testing.T) {
	src := newIndexSource(t)
	if got := src.MaxArticle(lawcite.StatutePatentLaw); got != 82 {
		t.Fatalf("patent max = %d", got)
	}
	if got := src.MaxArticle(lawcite.StatuteImplementingRules); got != 149 {
		t.Fatalf("rules max = %d", got)
	}
}
