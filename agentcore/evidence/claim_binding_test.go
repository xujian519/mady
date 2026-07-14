package evidence

import (
	"testing"
)

func TestClaimBinding_RegisterAndGet(t *testing.T) {
	cb := NewClaimBinding()

	span := EvidenceSpan{
		ID:        "ev_001",
		Snippet:   "CN12345678A 公开了特征X",
		Direction: DirectionSupporting,
		ClaimRefs: []string{"claim_feature_X", "claim_prior_art"},
	}
	cb.RegisterSpan(span)

	if cb.SpanCount() != 1 {
		t.Fatalf("SpanCount()=%d want 1", cb.SpanCount())
	}

	ev := cb.GetEvidence("claim_feature_X")
	if len(ev) != 1 {
		t.Fatalf("GetEvidence(claim_feature_X)=%d items, want 1", len(ev))
	}
	if ev[0].ID != "ev_001" {
		t.Errorf("evidence ID=%q want ev_001", ev[0].ID)
	}

	// Claim not in binding.
	ev2 := cb.GetEvidence("nonexistent")
	if len(ev2) != 0 {
		t.Errorf("GetEvidence(nonexistent)=%d items, want 0", len(ev2))
	}
}

func TestClaimBinding_LinkEvidence(t *testing.T) {
	cb := NewClaimBinding()

	span := EvidenceSpan{ID: "ev_002", Direction: DirectionContradicting}
	cb.RegisterSpan(span)

	// Link to a new claim.
	if err := cb.LinkEvidence("claim_new", "ev_002"); err != nil {
		t.Fatalf("LinkEvidence: %v", err)
	}
	ev := cb.GetEvidence("claim_new")
	if len(ev) != 1 {
		t.Fatalf("GetEvidence()=%d items, want 1", len(ev))
	}

	// Link to non-existent span.
	if err := cb.LinkEvidence("claim_x", "ev_missing"); err == nil {
		t.Error("expected error for missing span")
	}
}

func TestClaimBinding_UnbackedClaims(t *testing.T) {
	cb := NewClaimBinding()

	span := EvidenceSpan{
		ID:        "ev_003",
		Direction: DirectionContradicting, // only contradicting, no supporting
		ClaimRefs: []string{"claim_only_contra"},
	}
	cb.RegisterSpan(span)
	// Register a supporting one.
	span2 := EvidenceSpan{
		ID:        "ev_004",
		Direction: DirectionSupporting,
		ClaimRefs: []string{"claim_supported"},
	}
	cb.RegisterSpan(span2)

	unbacked := cb.UnbackedClaims()
	// claim_only_contra has no supporting evidence -> unbacked.
	// claim_supported has supporting evidence -> not unbacked.
	found := false
	for _, c := range unbacked {
		if c == "claim_only_contra" {
			found = true
		}
		if c == "claim_supported" {
			t.Error("claim_supported should NOT be in unbacked claims")
		}
	}
	if !found {
		t.Error("claim_only_contra should be in unbacked claims")
	}
}

func TestClaimBinding_Clear(t *testing.T) {
	cb := NewClaimBinding()
	span := EvidenceSpan{ID: "ev_005", Direction: DirectionSupporting, ClaimRefs: []string{"c1"}}
	cb.RegisterSpan(span)
	cb.Clear()
	if cb.SpanCount() != 0 {
		t.Errorf("SpanCount after Clear=%d, want 0", cb.SpanCount())
	}
	if len(cb.GetEvidence("c1")) != 0 {
		t.Error("expected no evidence after Clear")
	}
}

func TestClaimBinding_GetClaims(t *testing.T) {
	cb := NewClaimBinding()
	span := EvidenceSpan{
		ID: "ev_006", Direction: DirectionSupporting,
		ClaimRefs: []string{"c_a", "c_b"},
	}
	cb.RegisterSpan(span)

	claims := cb.GetClaims()
	if len(claims) != 2 {
		t.Fatalf("GetClaims()=%d, want 2", len(claims))
	}
	seen := make(map[string]bool)
	for _, c := range claims {
		seen[c] = true
	}
	if !seen["c_a"] || !seen["c_b"] {
		t.Errorf("expected c_a and c_b, got %v", claims)
	}
}
