package patent

import "testing"

func TestLoadPatentWorkflowManifests_AllValid(t *testing.T) {
	ms, err := LoadPatentWorkflowManifests()
	if err != nil {
		t.Fatalf("LoadPatentWorkflowManifests: %v", err)
	}
	if len(ms) != 8 {
		t.Fatalf("expected 8 manifests, got %d", len(ms))
	}
	if err := ValidateAllManifests(ms); err != nil {
		t.Fatalf("ValidateAllManifests: %v", err)
	}
}

func TestValidateManifestSchema_BadEntryPoint(t *testing.T) {
	m := &PatentWorkflowManifest{
		ID:    "x",
		Name:  "x",
		Steps: []PatentWorkflowStep{{ID: "s1", StepType: "analyze", EntryPoint: "not-real"}},
	}
	if err := ValidateManifestSchema(m); err == nil {
		t.Fatal("expected error for unknown entry_point")
	}
}

func TestValidateManifestSchema_EmptySteps(t *testing.T) {
	m := &PatentWorkflowManifest{ID: "x", Name: "x"}
	if err := ValidateManifestSchema(m); err == nil {
		t.Fatal("expected error for empty steps")
	}
}

func TestFindWorkflowManifest(t *testing.T) {
	ms, err := LoadPatentWorkflowManifests()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m := FindWorkflowManifest(ms, "novelty-analysis"); m == nil || m.Name == "" {
		t.Fatal("novelty-analysis manifest missing")
	}
	if m := FindWorkflowManifest(ms, "does-not-exist"); m != nil {
		t.Errorf("expected nil for unknown workflow, got %+v", m)
	}
}
