package lawcite

import (
	"strings"
	"testing"
)

func bundled(t *testing.T, statute Statute) *Index {
	t.Helper()
	idx, err := BundledIndex(statute)
	if err != nil {
		t.Fatalf("BundledIndex(%v): %v", statute, err)
	}
	return idx
}

func TestBundledPatentLawShape(t *testing.T) {
	idx := bundled(t, StatutePatentLaw)
	if idx.Law != "专利法" {
		t.Fatalf("law = %q", idx.Law)
	}
	if idx.MaxArticle != 82 || idx.MaxVerifiedOn != "2026-09-26" {
		t.Fatalf("maxArticle=%d maxVerifiedOn=%q", idx.MaxArticle, idx.MaxVerifiedOn)
	}
	if got := len(idx.Articles()); got != 37 {
		t.Fatalf("articles = %d, want 37", got)
	}
}

// 已转录条目逐条必须是 Verified，且来源与核验日期齐备。
func TestPatentLawTranscribedEntriesVerified(t *testing.T) {
	idx := bundled(t, StatutePatentLaw)
	for _, article := range idx.Articles() {
		res := idx.Resolve(Citation{Statute: StatutePatentLaw, Article: article.Article})
		if res.Status != StatusVerified {
			t.Errorf("第%d条 status=%v reason=%s", article.Article, res.Status, res.Reason)
			continue
		}
		if article.Text == "" || article.SourceDoc == "" || article.VerifiedOn == "" {
			t.Errorf("第%d条 已判 Verified 但字段不全", article.Article)
		}
	}
}

// 专利法 1..82 中未被收录的条号必须判 ExistOnly，绝不能判 Verified。
func TestPatentLawUntranscribedAreExistOnly(t *testing.T) {
	idx := bundled(t, StatutePatentLaw)
	untranscribed := 0
	for article := 1; article <= 82; article++ {
		if _, ok := idx.Article(article); ok {
			continue
		}
		untranscribed++
		res := idx.Resolve(Citation{Statute: StatutePatentLaw, Article: article})
		if res.Status != StatusExistOnly {
			t.Errorf("第%d条 status=%v, want ExistOnly", article, res.Status)
		}
		if !strings.Contains(res.Reason, "条文尚未收录") {
			t.Errorf("第%d条 reason=%q", article, res.Reason)
		}
	}
	if untranscribed != 82-37 {
		t.Fatalf("未转录条数 = %d, want %d", untranscribed, 82-37)
	}
}

func TestPatentLawOutOfRange(t *testing.T) {
	idx := bundled(t, StatutePatentLaw)
	res := idx.Resolve(Citation{Statute: StatutePatentLaw, Article: 83})
	if res.Status != StatusOutOfRange {
		t.Fatalf("status=%v, want OutOfRange", res.Status)
	}
	if !strings.Contains(res.Reason, "共 82 条") {
		t.Fatalf("reason=%q", res.Reason)
	}
}

func TestPatentLawParagraphNarrowing(t *testing.T) {
	idx := bundled(t, StatutePatentLaw)
	// 第 22 条登记 5 款。
	if res := idx.Resolve(Citation{Statute: StatutePatentLaw, Article: 22, Paragraph: 5}); res.Status != StatusVerified {
		t.Fatalf("第22条第5款 status=%v", res.Status)
	}
	res := idx.Resolve(Citation{Statute: StatutePatentLaw, Article: 22, Paragraph: 6})
	if res.Status != StatusNarrowingMismatch {
		t.Fatalf("第22条第6款 status=%v, want NarrowingMismatch", res.Status)
	}
	if !strings.Contains(res.Reason, "共 5 款") {
		t.Fatalf("reason=%q", res.Reason)
	}
}

func TestBundledImplementingRules(t *testing.T) {
	idx := bundled(t, StatuteImplementingRules)
	if idx.MaxArticle != 149 {
		t.Fatalf("maxArticle=%d", idx.MaxArticle)
	}
	if got := len(idx.Articles()); got != 45 {
		t.Fatalf("articles = %d, want 45", got)
	}
	if res := idx.Resolve(Citation{Statute: StatuteImplementingRules, Article: 150}); res.Status != StatusOutOfRange {
		t.Fatalf("第150条 status=%v", res.Status)
	}
}

func TestBundledIndexAbsent(t *testing.T) {
	for _, statute := range []Statute{StatuteTrademarkLaw, StatuteCopyrightLaw, StatuteUnknown} {
		if _, err := BundledIndex(statute); err == nil {
			t.Fatalf("%v 不应有随包索引", statute)
		}
	}
}

func TestResolveNilAndInvalid(t *testing.T) {
	var idx *Index
	if res := idx.Resolve(Citation{Article: 22}); res.Status != StatusUnverified {
		t.Fatalf("nil index status=%v", res.Status)
	}
	patent := bundled(t, StatutePatentLaw)
	if res := patent.Resolve(Citation{Article: 0}); res.Status != StatusUnverified {
		t.Fatalf("zero article status=%v", res.Status)
	}
}

func TestStatusString(t *testing.T) {
	cases := map[Status]string{
		StatusUnverified:        "未核验",
		StatusExistOnly:         "条号存在但未转录",
		StatusVerified:          "已核验",
		StatusOutOfRange:        "条号越界",
		StatusNarrowingMismatch: "款项越界",
	}
	for status, want := range cases {
		if got := status.String(); got != want {
			t.Errorf("%d = %q, want %q", status, got, want)
		}
	}
}
