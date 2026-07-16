package reasoning

import (
	"testing"
)

func TestConfirmedRuleSet_ActiveConstraints(t *testing.T) {
	original := RuleConstraint{ArticleID: "NOV-001", ArticleName: "新颖性", Requirement: ReqMust}
	modified := RuleConstraint{ArticleID: "NOV-002", ArticleName: "创造性(修订)", Requirement: ReqShould}

	rs := ConfirmedRuleSet{
		Entries: []ConfirmedRuleEntry{
			{Rule: original, Status: RuleConfirmed},
			{Rule: RuleConstraint{ArticleID: "NOV-002", ArticleName: "创造性"}, Status: RuleModified, Modified: &modified},
			{Rule: RuleConstraint{ArticleID: "REJ-001", ArticleName: "被驳回"}, Status: RuleRejected},
		},
	}

	active := rs.ActiveConstraints()
	if len(active) != 2 {
		t.Fatalf("got %d active constraints, want 2 (confirmed + modified, rejected excluded)", len(active))
	}

	// First: confirmed original.
	if active[0].ArticleID != "NOV-001" || active[0].ArticleName != "新颖性" {
		t.Errorf("confirmed entry = %+v, want original NOV-001", active[0])
	}
	// Second: modified version (not the original).
	if active[1].ArticleID != "NOV-002" || active[1].ArticleName != "创造性(修订)" {
		t.Errorf("modified entry = %+v, want modified version", active[1])
	}
}

func TestConfirmedRuleSet_ModifiedWithoutRevision(t *testing.T) {
	// Status=modified but Modified=nil → falls back to original Rule.
	rs := ConfirmedRuleSet{
		Entries: []ConfirmedRuleEntry{
			{Rule: RuleConstraint{ArticleID: "X"}, Status: RuleModified, Modified: nil},
		},
	}
	active := rs.ActiveConstraints()
	if len(active) != 1 || active[0].ArticleID != "X" {
		t.Errorf("modified-without-revision should fall back to original, got %+v", active)
	}
}

func TestConfirmedRuleSet_AllRejected(t *testing.T) {
	rs := ConfirmedRuleSet{
		Entries: []ConfirmedRuleEntry{
			{Rule: RuleConstraint{ArticleID: "R1"}, Status: RuleRejected},
			{Rule: RuleConstraint{ArticleID: "R2"}, Status: RuleRejected},
		},
	}
	if got := rs.ActiveConstraints(); len(got) != 0 {
		t.Errorf("all-rejected should yield 0 active, got %d", len(got))
	}
}

func TestConfirmedRuleSet_NilSafe(t *testing.T) {
	var rs *ConfirmedRuleSet
	if got := rs.ActiveConstraints(); got != nil {
		t.Errorf("nil ActiveConstraints = %v, want nil", got)
	}
}

func TestFactBlackboard_ConfirmedRuleConstraints_Fallback(t *testing.T) {
	// No confirmation set → ConfirmedRuleConstraints falls back to raw RuleConstraints.
	bb := NewFactBlackboard("case1", CaseNoveltySearch, "AI")
	bb.AddRuleConstraint(RuleConstraint{ArticleID: "RAW-001"})

	if got := bb.ConfirmedRuleConstraints(); len(got) != 1 || got[0].ArticleID != "RAW-001" {
		t.Errorf("fallback should return raw constraints, got %+v", got)
	}
	if bb.ConfirmedRules() != nil {
		t.Error("ConfirmedRules should be nil before SetConfirmedRules")
	}
}

func TestFactBlackboard_ConfirmedRuleConstraints_UsesConfirmation(t *testing.T) {
	bb := NewFactBlackboard("case1", CaseNoveltySearch, "AI")
	// Raw constraints include 3 rules.
	bb.AddRuleConstraint(RuleConstraint{ArticleID: "RAW-A"})
	bb.AddRuleConstraint(RuleConstraint{ArticleID: "RAW-B"})
	bb.AddRuleConstraint(RuleConstraint{ArticleID: "RAW-C"})

	// Human confirms only A (confirmed) and B (modified), rejects C.
	bb.SetConfirmedRules(ConfirmedRuleSet{
		Entries: []ConfirmedRuleEntry{
			{Rule: RuleConstraint{ArticleID: "RAW-A"}, Status: RuleConfirmed},
			{Rule: RuleConstraint{ArticleID: "RAW-B"}, Status: RuleModified,
				Modified: &RuleConstraint{ArticleID: "RAW-B", ArticleName: "修订版"}},
			{Rule: RuleConstraint{ArticleID: "RAW-C"}, Status: RuleRejected},
		},
		Locked: true,
	})

	active := bb.ConfirmedRuleConstraints()
	if len(active) != 2 {
		t.Fatalf("after confirmation got %d, want 2 (A confirmed + B modified, C rejected)", len(active))
	}
	if active[0].ArticleID != "RAW-A" {
		t.Errorf("first = %s, want RAW-A", active[0].ArticleID)
	}
	if active[1].ArticleName != "修订版" {
		t.Errorf("second should use modified version, got ArticleName=%q", active[1].ArticleName)
	}
}

func TestFactBlackboard_ConfirmedRules_JSONRoundTrip(t *testing.T) {
	bb := NewFactBlackboard("case1", CaseNoveltySearch, "AI")
	bb.SetConfirmedRules(ConfirmedRuleSet{
		Entries: []ConfirmedRuleEntry{
			{Rule: RuleConstraint{ArticleID: "X"}, Status: RuleConfirmed},
		},
	})

	data, err := bb.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	restored := NewFactBlackboard("", "", "")
	if err := restored.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	cr := restored.ConfirmedRules()
	if cr == nil || len(cr.Entries) != 1 || cr.Entries[0].Rule.ArticleID != "X" {
		t.Errorf("confirmed rules not restored: %+v", cr)
	}
}
