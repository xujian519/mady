package evidence

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEvidenceSpan_Defaults(t *testing.T) {
	s := EvidenceSpan{
		ID:        "ev_001",
		Snippet:   "CN12345678A 公开了一种...",
		Direction: DirectionSupporting,
	}
	if !s.Direction.Valid() {
		t.Error("expected DirectionSupporting to be valid")
	}
	if s.RetrievalAt != (time.Time{}) {
		t.Error("expected zero time")
	}
}

func TestEvidenceDirection_Valid(t *testing.T) {
	tests := []struct {
		d  EvidenceDirection
		ok bool
	}{
		{DirectionSupporting, true},
		{DirectionContradicting, true},
		{DirectionNeutral, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.d.Valid(); got != tt.ok {
			t.Errorf("Valid(%q) = %v, want %v", tt.d, got, tt.ok)
		}
	}
}

func TestEvidenceSpan_JSONRoundTrip(t *testing.T) {
	s := EvidenceSpan{
		ID:         "ev_001",
		TurnID:     "turn_1",
		DocVersion: "v1.0",
		PageRange:  "第3页第15-20行",
		Snippet:    "本发明提供了一种...",
		SourceURI:  "file:///case/CN12345678A/document.pdf",
		Direction:  DirectionSupporting,
		ClaimRefs:  []string{"claim_feature_1"},
	}
	// JSON serialization should not panic and should preserve fields.
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got EvidenceSpan
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ID != s.ID || got.Direction != s.Direction {
		t.Errorf("round-trip: got %+v, want %+v", got, s)
	}
}
