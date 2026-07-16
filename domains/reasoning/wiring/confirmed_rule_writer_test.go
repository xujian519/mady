package wiring

import (
	"testing"

	"github.com/xujian519/mady/domains/reasoning"
)

func TestNewConfirmedRuleWriter_NilOnEmptyDir(t *testing.T) {
	if got := NewConfirmedRuleWriter(""); got != nil {
		t.Fatalf("NewConfirmedRuleWriter(\"\") = %v, want nil", got)
	}
}

func TestConfirmedRuleWriter_WriteAndLoad(t *testing.T) {
	w := NewConfirmedRuleWriter(t.TempDir())

	rs := reasoning.ConfirmedRuleSet{
		Entries: []reasoning.ConfirmedRuleEntry{
			{Rule: reasoning.RuleConstraint{ArticleID: "NOV-001"}, Status: reasoning.RuleConfirmed},
			{Rule: reasoning.RuleConstraint{ArticleID: "NOV-002"}, Status: reasoning.RuleRejected, Feedback: "不适用"},
		},
		Locked: true,
	}

	path, err := w.Write("case_001", "novelty_search", "AI芯片", rs)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if path == "" {
		t.Fatal("Write returned empty path")
	}

	rec, err := w.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if rec.CaseID != "case_001" {
		t.Errorf("CaseID = %s, want case_001", rec.CaseID)
	}
	if rec.CaseType != "novelty_search" {
		t.Errorf("CaseType = %s", rec.CaseType)
	}
	if rec.TechField != "AI芯片" {
		t.Errorf("TechField = %s", rec.TechField)
	}
	if len(rec.RuleSet.Entries) != 2 {
		t.Errorf("RuleSet entries = %d, want 2", len(rec.RuleSet.Entries))
	}
	if rec.RuleSet.Entries[0].Rule.ArticleID != "NOV-001" {
		t.Errorf("first entry ArticleID = %s", rec.RuleSet.Entries[0].Rule.ArticleID)
	}
	if rec.RuleSet.Entries[1].Status != reasoning.RuleRejected {
		t.Errorf("second entry Status = %s, want rejected", rec.RuleSet.Entries[1].Status)
	}
}

func TestConfirmedRuleWriter_List(t *testing.T) {
	w := NewConfirmedRuleWriter(t.TempDir())
	rs := reasoning.ConfirmedRuleSet{Entries: []reasoning.ConfirmedRuleEntry{}}

	// Write two records.
	if _, err := w.Write("case_a", "novelty_search", "", rs); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write("case_b", "patentability", "", rs); err != nil {
		t.Fatal(err)
	}

	paths, err := w.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("List returned %d, want 2", len(paths))
	}
}

func TestConfirmedRuleWriter_ListEmptyDir(t *testing.T) {
	w := NewConfirmedRuleWriter(t.TempDir())
	paths, err := w.List()
	if err != nil {
		t.Fatalf("List on empty dir: %v", err)
	}
	if paths != nil && len(paths) != 0 {
		t.Errorf("empty dir List = %v, want nil/empty", paths)
	}
}

func TestConfirmedRuleWriter_ListMissingDir(t *testing.T) {
	w := NewConfirmedRuleWriter("/tmp/mady-nonexistent-confirmed-rules-xyz")
	// Missing dir should return nil, nil (not an error).
	paths, err := w.List()
	if err != nil {
		t.Errorf("missing dir should not error: %v", err)
	}
	if paths != nil {
		t.Errorf("missing dir List = %v, want nil", paths)
	}
}

func TestConfirmedRuleWriter_CaseIDSanitized(t *testing.T) {
	w := NewConfirmedRuleWriter(t.TempDir())
	// Case ID with path separators / spaces must be sanitized in filename.
	path, err := w.Write("case/with spaces:colons", "novelty_search", "", reasoning.ConfirmedRuleSet{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	// The written file should load back with the original case ID preserved.
	rec, err := w.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if rec.CaseID != "case/with spaces:colons" {
		t.Errorf("case ID not preserved in JSON: %s", rec.CaseID)
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"simple", "simple"},
		{"case/001", "case_001"},
		{"a b", "a_b"},
		{"", "case"},
	}
	for _, c := range cases {
		if got := sanitizeFilename(c.in); got != c.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
