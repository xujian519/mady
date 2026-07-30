package main

import (
	"testing"

	"github.com/xujian519/mady/evaluate"
	"github.com/xujian519/mady/guardrails"
)

func TestAdapter_Empty(t *testing.T) {
	r := guardrails.CitationReport{}
	adapted := adapter(r)
	if adapted.Total != 0 {
		t.Errorf("expected Total=0, got %d", adapted.Total)
	}
}

func TestAdapter_MapsAllFields(t *testing.T) {
	r := guardrails.CitationReport{
		Total:        10,
		Valid:        5,
		Unknown:      2,
		Unverifiable: 1,
		Suspect:      1,
		Invalid:      1,
	}
	adapted := adapter(r)
	if adapted != (evaluate.CitationValidityReport{
		Total: 10, Valid: 5, Unknown: 2,
		Unverifiable: 1, Suspect: 1, Invalid: 1,
	}) {
		t.Errorf("unexpected mapping: %+v", adapted)
	}
}

func TestAdapter_Zero(t *testing.T) {
	r := guardrails.CitationReport{
		Total: 0, Valid: 0, Unknown: 0,
		Unverifiable: 0, Suspect: 0, Invalid: 0,
	}
	adapted := adapter(r)
	if adapted.Total != 0 || adapted.Valid != 0 {
		t.Errorf("expected all zero, got %+v", adapted)
	}
}
