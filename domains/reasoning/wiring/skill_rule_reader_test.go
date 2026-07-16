package wiring

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/domains/reasoning"
)

func TestNewSkillRuleReader_NilOnEmptyRoot(t *testing.T) {
	if got := NewSkillRuleReader(""); got != nil {
		t.Fatalf("NewSkillRuleReader(\"\") = %v, want nil", got)
	}
}

func TestSkillRuleReader_ReadRules(t *testing.T) {
	r := NewSkillRuleReader("testdata")
	rules, err := r.ReadRules(context.Background(), "")
	if err != nil {
		t.Fatalf("ReadRules: %v", err)
	}
	// 2 .md cards; the README.txt must be skipped.
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2 (md cards only)", len(rules))
	}

	// Find the 三步法 card and verify field mapping.
	var threeStep *reasoning.RetrievedRule
	var common *reasoning.RetrievedRule
	for i := range rules {
		switch rules[i].Rule.ArticleID {
		case "三步法":
			threeStep = &rules[i]
		case "公知常识":
			common = &rules[i]
		}
	}
	if threeStep == nil {
		t.Fatal("missing 三步法 card in results")
	}

	// Authority/Source/Priority per design-doc wiki tier.
	if threeStep.Source != reasoning.RuleSourceSkill {
		t.Errorf("Source = %q, want %q", threeStep.Source, reasoning.RuleSourceSkill)
	}
	if threeStep.Priority != 3 {
		t.Errorf("Priority = %d, want 3", threeStep.Priority)
	}
	if threeStep.AuthorityScore != 0.4 {
		t.Errorf("AuthorityScore = %v, want 0.4", threeStep.AuthorityScore)
	}
	if threeStep.Confidence != 0.95 {
		t.Errorf("Confidence (quality) = %v, want 0.95", threeStep.Confidence)
	}
	if threeStep.Rule.ArticleName != `专利创造性判断的"三步法"框架是什么？` {
		t.Errorf("ArticleName = %q", threeStep.Rule.ArticleName)
	}
	if threeStep.Rule.Requirement != reasoning.ReqNote {
		t.Errorf("Requirement = %q, want ReqNote", threeStep.Rule.Requirement)
	}
	if threeStep.Baggage == "" {
		t.Error("Baggage should hold card body")
	}

	// 公知常识 card has no 质量分 → default 0.5.
	if common == nil {
		t.Fatal("missing 公知常识 card in results")
	}
	if common.Confidence != 0.5 {
		t.Errorf("default Confidence = %v, want 0.5 for card without 质量分", common.Confidence)
	}
}

func TestSkillRuleReader_DomainFilter(t *testing.T) {
	r := NewSkillRuleReader("testdata")

	// "专利授权" matches the 三步法 card only; 公知常识 is 未分类 and passes.
	rules, err := r.ReadRules(context.Background(), "专利授权")
	if err != nil {
		t.Fatalf("ReadRules: %v", err)
	}
	if len(rules) != 2 {
		// Both pass: 三步法 matches directly, 公知常识 has unset domain so it bypasses the filter.
		t.Fatalf("domain=专利授权 got %d rules, want 2", len(rules))
	}

	// An unrelated concrete domain excludes the tagged card but keeps 未分类.
	rules, err = r.ReadRules(context.Background(), "侵权")
	if err != nil {
		t.Fatalf("ReadRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("domain=侵权 got %d rules, want 1 (only 未分类 card passes)", len(rules))
	}
	if rules[0].Rule.ArticleID != "公知常识" {
		t.Errorf("expected 公知常识 to survive, got %q", rules[0].Rule.ArticleID)
	}
}

func TestSkillRuleReader_MissingDirReturnsNil(t *testing.T) {
	r := &SkillRuleReader{cardDir: filepath.Join("testdata", "nonexistent")}
	rules, err := r.ReadRules(context.Background(), "")
	if err != nil {
		t.Fatalf("missing dir should not error, got %v", err)
	}
	if rules != nil && len(rules) != 0 {
		t.Fatalf("missing dir got %v, want empty", rules)
	}
}

func TestSkillRuleReader_NilReceiverSafe(t *testing.T) {
	var r *SkillRuleReader
	rules, err := r.ReadRules(context.Background(), "")
	if err != nil {
		t.Fatalf("nil receiver: %v", err)
	}
	if rules != nil {
		t.Fatalf("nil receiver got %v, want nil", rules)
	}
}

func TestParseCard_Structure(t *testing.T) {
	// Write a temp card to verify parseCard in isolation.
	dir := t.TempDir()
	content := "# 测试标题\n\n- 概念: 测试概念\n- 领域: 专利授权\n- 质量分: 0.88\n\n## 卡片内容\n\n正文第一行。\n正文第二行。\n"
	path := filepath.Join(dir, "card.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	card, err := parseCard(path)
	if err != nil {
		t.Fatalf("parseCard: %v", err)
	}
	if card.Title != "测试标题" {
		t.Errorf("Title = %q", card.Title)
	}
	if card.Concept != "测试概念" {
		t.Errorf("Concept = %q", card.Concept)
	}
	if card.Domain != "专利授权" {
		t.Errorf("Domain = %q", card.Domain)
	}
	if card.Quality != 0.88 {
		t.Errorf("Quality = %v", card.Quality)
	}
	wantBody := "正文第一行。\n正文第二行。"
	if card.Body != wantBody {
		t.Errorf("Body = %q, want %q", card.Body, wantBody)
	}
}

func TestDomainMatches(t *testing.T) {
	cases := []struct {
		card, req string
		want      bool
	}{
		{"专利授权", "专利授权", true},
		{"专利授权", "授权", true}, // substring either direction
		{"侵权", "专利侵权", true}, // request superset
		{"专利授权", "侵权", false},
	}
	for _, c := range cases {
		if got := domainMatches(c.card, c.req); got != c.want {
			t.Errorf("domainMatches(%q,%q) = %v, want %v", c.card, c.req, got, c.want)
		}
	}
	// Note: domainMatches is never called with an empty card domain in
	// practice — ReadRules guards `card.Domain != ""` before calling.
}
