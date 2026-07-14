package evidence

import (
	"testing"
)

func TestConflictDetector_Direction(t *testing.T) {
	cb := NewClaimBinding()

	// Two evidence spans, same claim, opposite directions.
	cb.RegisterSpan(EvidenceSpan{
		ID:        "ev_support",
		Direction: DirectionSupporting,
		Snippet:   "CN12345678A 公开了特征X",
		ClaimRefs: []string{"claim_feature_X"},
	})
	cb.RegisterSpan(EvidenceSpan{
		ID:        "ev_contra",
		Direction: DirectionContradicting,
		Snippet:   "CN12345678A 未公开特征X",
		ClaimRefs: []string{"claim_feature_X"},
	})

	cd := NewConflictDetector(cb)
	conflicts := cd.Detect()

	found := false
	for _, c := range conflicts {
		if c.Type == ConflictDirection {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected direction conflict, got none")
	}
}

func TestConflictDetector_Source(t *testing.T) {
	cb := NewClaimBinding()

	// Two evidence spans, same source, different claims, opposite directions.
	cb.RegisterSpan(EvidenceSpan{
		ID:        "ev_s1",
		Direction: DirectionSupporting,
		SourceURI: "patent:CN12345678A",
		Snippet:   "从CN12345678A 的实施例1可知...",
		ClaimRefs: []string{"claim_A"},
	})
	cb.RegisterSpan(EvidenceSpan{
		ID:        "ev_c1",
		Direction: DirectionContradicting,
		SourceURI: "patent:CN12345678A",
		Snippet:   "CN12345678A 的实施例1 并未...",
		ClaimRefs: []string{"claim_B"},
	})

	cd := NewConflictDetector(cb)
	conflicts := cd.Detect()

	found := false
	for _, c := range conflicts {
		if c.Type == ConflictSource {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected source conflict, got none")
	}
}

func TestConflictDetector_NoConflict(t *testing.T) {
	cb := NewClaimBinding()

	// All evidence supporting, no conflict.
	cb.RegisterSpan(EvidenceSpan{
		ID:        "ev_1",
		Direction: DirectionSupporting,
		SourceURI: "patent:CN12345678A",
		ClaimRefs: []string{"claim_A"},
	})
	cb.RegisterSpan(EvidenceSpan{
		ID:        "ev_2",
		Direction: DirectionSupporting,
		SourceURI: "patent:CN87654321B",
		ClaimRefs: []string{"claim_A"},
	})

	cd := NewConflictDetector(cb)
	conflicts := cd.Detect()
	if len(conflicts) > 0 {
		t.Errorf("expected 0 conflicts, got %d: %v", len(conflicts), conflicts)
	}
}

func TestConflictDetector_NilBinding(t *testing.T) {
	cd := NewConflictDetector(nil)
	conflicts := cd.Detect()
	if len(conflicts) != 0 {
		t.Errorf("nil binding should produce 0 conflicts, got %d", len(conflicts))
	}
}
