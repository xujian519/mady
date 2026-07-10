// Track-conflict resolution for the dual-track checker.
//
// When the deterministic rule track and the semantic LLM track disagree on a
// verdict, a conflict arises. This package-level logic decides the final
// verdict, records the conflict, and optionally flags the result for human
// review.
//
// Three merge modes are supported:
//
//   - Conservative (default): take the stricter verdict (max severity).
//   - RuleAuthoritative:      the rule verdict wins; LLM is advisory only.
//   - LLMAuthoritative:       the LLM verdict wins, except that a rule-blocked
//                             result is a hard constraint that cannot be
//                             overridden.
//
// Conflict severity ranking (loose → strict):
//
//	pass < needs_revision < blocked
package patent

import "strings"

// MergeMode controls how the two tracks are reconciled when they disagree.
type MergeMode int

const (
	// MergeConservative takes the stricter of the two verdicts. This is the
	// safest default: if either track finds a problem, the problem stands.
	MergeConservative MergeMode = iota

	// MergeRuleAuthoritative treats the rule verdict as binding. The LLM
	// verdict is recorded as advisory context but does not change the final
	// outcome. Use this when the rules encode hard legal constraints.
	MergeRuleAuthoritative

	// MergeLLMAuthoritative lets the LLM verdict decide, except that a
	// rule-blocked result cannot be overridden (rules remain authoritative
	// on hard constraints). Use this when the LLM provides nuanced semantic
	// judgment that should supersede coarse keyword rules.
	MergeLLMAuthoritative
)

// MergePolicy configures track-conflict resolution.
type MergePolicy struct {
	Mode MergeMode

	// MarkConflicts, when true, sets TrackConflict.NeedsHuman for any
	// disagreement between the two tracks whose severity gap is at least
	// HumanReviewGap. This surfaces significant conflicts for manual review.
	MarkConflicts bool
}

// HumanReviewGap is the minimum severity gap (in ordinal steps) that triggers
// human review when MarkConflicts is enabled. A gap of 2 means pass vs blocked
// (0 vs 2) requires review, but pass vs needs_revision (0 vs 1) does not.
const HumanReviewGap = 2

// DefaultMergePolicy returns the conservative policy with conflict marking.
func DefaultMergePolicy() MergePolicy {
	return MergePolicy{Mode: MergeConservative, MarkConflicts: true}
}

// TrackConflict records a disagreement between the two tracks and how it was
// resolved. Detected is false when both tracks agree.
type TrackConflict struct {
	Detected    bool
	RuleVerdict Verdict
	LLMVerdict  Verdict
	FinalVerdict Verdict
	// Resolution is a human-readable explanation of how the conflict was
	// settled (empty when no conflict).
	Resolution string
	// NeedsHuman is true when the conflict is significant enough to warrant
	// manual review.
	NeedsHuman bool
}

// verdictSeverity ranks a verdict on a 0-2 ordinal scale (loose → strict).
func verdictSeverity(v Verdict) int {
	switch v {
	case VerdictPass:
		return 0
	case VerdictNeedsRevision:
		return 1
	case VerdictBlocked:
		return 2
	default:
		return 0
	}
}

// stricterVerdict returns the verdict with the higher severity.
func stricterVerdict(a, b Verdict) Verdict {
	if verdictSeverity(a) >= verdictSeverity(b) {
		return a
	}
	return b
}

// ResolveTrackVerdict reconciles the rule-track and LLM-track verdicts
// according to the given policy, returning the final verdict and conflict info.
func ResolveTrackVerdict(ruleVerdict, llmVerdict Verdict, policy MergePolicy) (Verdict, TrackConflict) {
	conflict := TrackConflict{
		RuleVerdict:  ruleVerdict,
		LLMVerdict:   llmVerdict,
		Detected:     ruleVerdict != llmVerdict,
		FinalVerdict: ruleVerdict, // overwritten below
	}

	switch policy.Mode {
	case MergeConservative:
		final := stricterVerdict(ruleVerdict, llmVerdict)
		conflict.FinalVerdict = final
		if conflict.Detected {
			conflict.Resolution = "取严归并：采用较严判定 " + string(final)
		}

	case MergeRuleAuthoritative:
		// Rule verdict is binding; LLM is advisory.
		conflict.FinalVerdict = ruleVerdict
		if conflict.Detected && verdictSeverity(llmVerdict) > verdictSeverity(ruleVerdict) {
			conflict.Resolution = "规则优先：LLM 判定较严（" + string(llmVerdict) + "）但规则为准（" + string(ruleVerdict) + "），建议人工复核"
		} else if conflict.Detected {
			conflict.Resolution = "规则优先：以规则判定 " + string(ruleVerdict) + " 为准"
		}

	case MergeLLMAuthoritative:
		// LLM wins, but rule-blocked is a hard constraint.
		if ruleVerdict == VerdictBlocked {
			conflict.FinalVerdict = VerdictBlocked
			if conflict.Detected {
				conflict.Resolution = "LLM优先但规则硬约束：规则判定 blocked 不可覆盖"
			}
		} else {
			conflict.FinalVerdict = llmVerdict
			if conflict.Detected {
				conflict.Resolution = "LLM优先：采用 LLM 判定 " + string(llmVerdict)
			}
		}

	default:
		conflict.FinalVerdict = stricterVerdict(ruleVerdict, llmVerdict)
	}

	// Human review flag: significant severity gap.
	if policy.MarkConflicts && conflict.Detected {
		gap := verdictSeverity(ruleVerdict) - verdictSeverity(llmVerdict)
		if gap < 0 {
			gap = -gap
		}
		if gap >= HumanReviewGap {
			conflict.NeedsHuman = true
		}
	}

	return conflict.FinalVerdict, conflict
}

// FormatConflict renders a TrackConflict as a Markdown section. Returns an
// empty string when no conflict was detected.
func FormatConflict(tc *TrackConflict) string {
	if tc == nil || !tc.Detected {
		return ""
	}
	var b strings.Builder
	b.WriteString("### 轨间冲突\n\n")
	b.WriteString("- 规则轨判定: ")
	b.WriteString(string(tc.RuleVerdict))
	b.WriteString("\n- 语义轨判定: ")
	b.WriteString(string(tc.LLMVerdict))
	b.WriteString("\n- 最终判定: ")
	b.WriteString(string(tc.FinalVerdict))
	b.WriteString("\n- 解决方式: ")
	b.WriteString(tc.Resolution)
	if tc.NeedsHuman {
		b.WriteString("\n- ⚠️ **建议人工复核**：两轨判定差异显著")
	}
	b.WriteString("\n")
	return b.String()
}
