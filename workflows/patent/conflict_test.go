package patent

import "testing"

func TestResolveTrackVerdict_NoConflict(t *testing.T) {
	cases := []Verdict{VerdictPass, VerdictNeedsRevision, VerdictBlocked}
	for _, v := range cases {
		final, conflict := ResolveTrackVerdict(v, v, DefaultMergePolicy())
		if conflict.Detected {
			t.Errorf("verdict %s: expected no conflict", v)
		}
		if final != v {
			t.Errorf("verdict %s: expected final=%s, got %s", v, v, final)
		}
		if conflict.NeedsHuman {
			t.Errorf("verdict %s: unexpected human review", v)
		}
	}
}

func TestResolveTrackVerdict_Conservative(t *testing.T) {
	policy := MergePolicy{Mode: MergeConservative, MarkConflicts: true}

	// rule=pass, llm=needs_revision → stricter=needs_revision
	final, conflict := ResolveTrackVerdict(VerdictPass, VerdictNeedsRevision, policy)
	if final != VerdictNeedsRevision {
		t.Errorf("expected needs_revision, got %s", final)
	}
	if !conflict.Detected {
		t.Error("expected conflict detected")
	}
	if conflict.NeedsHuman {
		t.Error("gap 1 should not trigger human review")
	}

	// rule=pass, llm=blocked → stricter=blocked + human review (gap=2)
	final, conflict = ResolveTrackVerdict(VerdictPass, VerdictBlocked, policy)
	if final != VerdictBlocked {
		t.Errorf("expected blocked, got %s", final)
	}
	if !conflict.NeedsHuman {
		t.Error("gap 2 should trigger human review")
	}

	// rule=needs_revision, llm=pass → stricter=needs_revision
	final, _ = ResolveTrackVerdict(VerdictNeedsRevision, VerdictPass, policy)
	if final != VerdictNeedsRevision {
		t.Errorf("expected needs_revision, got %s", final)
	}
}

func TestResolveTrackVerdict_RuleAuthoritative(t *testing.T) {
	policy := MergePolicy{Mode: MergeRuleAuthoritative, MarkConflicts: true}

	// rule=pass, llm=blocked → rule wins (pass)
	final, conflict := ResolveTrackVerdict(VerdictPass, VerdictBlocked, policy)
	if final != VerdictPass {
		t.Errorf("expected pass (rule authoritative), got %s", final)
	}
	if !conflict.Detected {
		t.Error("expected conflict detected")
	}
	// LLM stricter than rule → resolution should mention human review suggestion
	if conflict.Resolution == "" {
		t.Error("expected non-empty resolution")
	}

	// rule=blocked, llm=pass → rule wins (blocked)
	final, _ = ResolveTrackVerdict(VerdictBlocked, VerdictPass, policy)
	if final != VerdictBlocked {
		t.Errorf("expected blocked, got %s", final)
	}
}

func TestResolveTrackVerdict_LLMAuthoritative(t *testing.T) {
	policy := MergePolicy{Mode: MergeLLMAuthoritative, MarkConflicts: true}

	// rule=pass, llm=needs_revision → LLM wins
	final, _ := ResolveTrackVerdict(VerdictPass, VerdictNeedsRevision, policy)
	if final != VerdictNeedsRevision {
		t.Errorf("expected needs_revision, got %s", final)
	}

	// rule=blocked, llm=pass → rule-blocked is hard constraint, stays blocked
	final, conflict := ResolveTrackVerdict(VerdictBlocked, VerdictPass, policy)
	if final != VerdictBlocked {
		t.Errorf("expected blocked (hard constraint), got %s", final)
	}
	if conflict.Resolution == "" {
		t.Error("expected non-empty resolution explaining hard constraint")
	}

	// rule=needs_revision, llm=pass → LLM wins (not blocked)
	final, _ = ResolveTrackVerdict(VerdictNeedsRevision, VerdictPass, policy)
	if final != VerdictPass {
		t.Errorf("expected pass (LLM authoritative, rule not blocked), got %s", final)
	}
}

func TestResolveTrackVerdict_NoMarkConflicts(t *testing.T) {
	policy := MergePolicy{Mode: MergeConservative, MarkConflicts: false}

	// Even with gap=2, NeedsHuman should be false.
	_, conflict := ResolveTrackVerdict(VerdictPass, VerdictBlocked, policy)
	if conflict.NeedsHuman {
		t.Error("MarkConflicts=false should never set NeedsHuman")
	}
}

func TestFormatConflict_NoConflict(t *testing.T) {
	tc := TrackConflict{Detected: false}
	if got := FormatConflict(&tc); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	// nil safety
	if got := FormatConflict(nil); got != "" {
		t.Errorf("expected empty string for nil, got %q", got)
	}
}

func TestFormatConflict_WithConflict(t *testing.T) {
	_, tc := ResolveTrackVerdict(VerdictPass, VerdictBlocked, DefaultMergePolicy())
	got := FormatConflict(&tc)
	if got == "" {
		t.Fatal("expected non-empty conflict section")
	}
	checks := []string{
		"轨间冲突",
		"规则轨判定: pass",
		"语义轨判定: blocked",
		"最终判定: blocked",
		"建议人工复核",
	}
	for _, s := range checks {
		if !containsStr(got, s) {
			t.Errorf("expected conflict output to contain %q", s)
		}
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOfStr(haystack, needle) >= 0
}

func indexOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
