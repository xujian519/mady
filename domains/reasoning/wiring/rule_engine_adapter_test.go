package wiring

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/domains/rules"
)

// newTestEngine writes minimal rule YAMLs to a temp dir and loads them,
// producing a real Engine with indexed rulesByDomain (LoadFromDir runs
// indexRules internally).
func newTestEngine(t *testing.T) *rules.Engine {
	t.Helper()
	dir := t.TempDir()
	// Two rule domains so we can test cross-domain caseType mapping + dedup.
	if err := os.WriteFile(filepath.Join(dir, "novelty.yaml"), []byte(`
rules:
  - ruleId: NOV-001
    name: 独立权利要求新颖性
    description: 独立权利要求是否包含区别于所有现有技术的技术特征
    legalBasis: 专利法第22条第2款
    domain: patent_novelty
    severity: critical
    action: block
    check:
      type: feature_comparison
      scope: ["novelty_check"]
  - ruleId: NOV-002
    name: 抵触申请检查
    description: 是否存在申请日在前的相同主题申请
    legalBasis: 专利法第22条第2款
    domain: patent_novelty
    severity: critical
    action: block
    check:
      type: conflicting_application
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "inventiveness.yaml"), []byte(`
rules:
  - ruleId: INV-001
    name: 三步法判断创造性
    description: 确定最接近现有技术、区别特征、技术启示
    legalBasis: 专利法第22条第3款
    domain: patent_inventiveness
    severity: critical
    action: block
    check:
      type: three_step
`), 0o644); err != nil {
		t.Fatal(err)
	}
	rs, err := rules.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	return rules.NewEngine(rs)
}

func TestNewRuleEngineAdapter_NilOnNilEngine(t *testing.T) {
	if got := NewRuleEngineAdapter(nil); got != nil {
		t.Fatalf("NewRuleEngineAdapter(nil) = %v, want nil", got)
	}
}

func TestRuleEngineAdapter_MatchRules_Patentability(t *testing.T) {
	a := NewRuleEngineAdapter(newTestEngine(t))

	// patentability maps to patent_novelty + patent_inventiveness.
	rules, err := a.MatchRules(context.Background(), "patentability", map[string]string{})
	if err != nil {
		t.Fatalf("MatchRules: %v", err)
	}
	// 2 novelty + 1 inventiveness = 3 distinct rules.
	if len(rules) != 3 {
		t.Fatalf("got %d rules, want 3", len(rules))
	}

	// All must carry the deterministic-rules source + highest authority.
	for _, r := range rules {
		if r.Source != reasoning.RuleSourceRules {
			t.Errorf("Source = %q, want %q", r.Source, reasoning.RuleSourceRules)
		}
		if r.AuthorityScore != 0.95 {
			t.Errorf("AuthorityScore = %v, want 0.95", r.AuthorityScore)
		}
		if r.Confidence != 1.0 {
			t.Errorf("Confidence = %v, want 1.0 (deterministic)", r.Confidence)
		}
	}
}

func TestRuleEngineAdapter_MatchRules_NoveltyOnly(t *testing.T) {
	a := NewRuleEngineAdapter(newTestEngine(t))

	rules, err := a.MatchRules(context.Background(), "novelty_search", map[string]string{})
	if err != nil {
		t.Fatalf("MatchRules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("novelty_search got %d rules, want 2 (NOV-001, NOV-002)", len(rules))
	}
	// Verify both are critical → Priority 1 (ReqMust).
	for _, r := range rules {
		if r.Priority != 1 {
			t.Errorf("critical rule Priority = %d, want 1", r.Priority)
		}
	}
}

func TestRuleEngineAdapter_MatchRules_DedupAcrossDomains(t *testing.T) {
	// patentability queries both patent_novelty and patent_inventiveness.
	// If a rule ID appeared in both domains it must be deduped. Our test data
	// has distinct IDs, so this verifies the dedup path doesn't drop valid rules.
	a := NewRuleEngineAdapter(newTestEngine(t))
	rules, _ := a.MatchRules(context.Background(), "patentability", nil)

	seen := make(map[string]int)
	for _, r := range rules {
		seen[r.Rule.ArticleID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("rule %s appeared %d times (not deduped)", id, count)
		}
	}
}

func TestRuleEngineAdapter_MatchRules_KeywordFallback(t *testing.T) {
	a := NewRuleEngineAdapter(newTestEngine(t))

	// Unknown caseType → keyword search fallback. "新颖性" matches NOV-001's name.
	rules, err := a.MatchRules(context.Background(), "unknown_casetype", map[string]string{
		"keywords": "新颖性",
	})
	if err != nil {
		t.Fatalf("MatchRules fallback: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("keyword fallback returned 0 rules for '新颖性'")
	}
}

func TestRuleEngineAdapter_MatchRules_EmptyKeywordFallback(t *testing.T) {
	a := NewRuleEngineAdapter(newTestEngine(t))
	rules, err := a.MatchRules(context.Background(), "unknown", map[string]string{})
	if err != nil {
		t.Fatalf("MatchRules: %v", err)
	}
	if rules != nil && len(rules) != 0 {
		t.Errorf("empty keyword + unknown caseType got %d rules, want 0", len(rules))
	}
}

func TestRuleEngineAdapter_NilReceiverSafe(t *testing.T) {
	var a *RuleEngineAdapter
	rules, err := a.MatchRules(context.Background(), "patentability", nil)
	if err != nil {
		t.Fatalf("nil receiver: %v", err)
	}
	if rules != nil {
		t.Fatalf("nil receiver got %v, want nil", rules)
	}
}
