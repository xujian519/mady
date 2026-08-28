package patent

import (
	"strings"
	"testing"
)

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

// =============================================================================
// 条件回退（retry）声明校验
// =============================================================================

func validRetryManifest() *PatentWorkflowManifest {
	return &PatentWorkflowManifest{
		ID:   "m-retry",
		Name: "回退样例",
		Steps: []PatentWorkflowStep{
			{ID: "s1-draft", StepType: "draft", EntryPoint: "report-draft"},
			{ID: "s2-check", StepType: "check", EntryPoint: "checker-review",
				Retry: &PatentWorkflowStepRetry{
					WhenOutputMatches: "需修订|不合格",
					RewindTo:          "s1-draft",
					MaxRetries:        1,
				}},
		},
	}
}

func TestValidateManifestSchema_RetryValid(t *testing.T) {
	if err := ValidateManifestSchema(validRetryManifest()); err != nil {
		t.Fatalf("valid retry manifest rejected: %v", err)
	}
}

func TestValidateManifestSchema_RewindTargetMissing(t *testing.T) {
	m := validRetryManifest()
	m.Steps[1].Retry.RewindTo = "no-such-step"
	if err := ValidateManifestSchema(m); err == nil || !strings.Contains(err.Error(), "回退目标") {
		t.Errorf("expected missing rewind target error, got %v", err)
	}
}

func TestValidateManifestSchema_RewindTargetLater(t *testing.T) {
	// 回退目标在当前步骤之后：违反"只允许向后回退"。
	m := validRetryManifest()
	m.Steps[1].Retry.RewindTo = "s3-later"
	m.Steps = append(m.Steps, PatentWorkflowStep{ID: "s3-later", StepType: "draft", EntryPoint: "report-draft"})
	// 把 retry 挂到更早的步骤上，使其目标在其之后。
	m.Steps[1].Retry = nil
	m.Steps[0].Retry = &PatentWorkflowStepRetry{
		WhenOutputMatches: "需修订",
		RewindTo:          "s2-check",
		MaxRetries:        1,
	}
	if err := ValidateManifestSchema(m); err == nil || !strings.Contains(err.Error(), "回退目标") {
		t.Errorf("expected forward-rewind rejection, got %v", err)
	}
}

func TestValidateManifestSchema_RetryBadRegex(t *testing.T) {
	m := validRetryManifest()
	m.Steps[1].Retry.WhenOutputMatches = "(未闭合"
	if err := ValidateManifestSchema(m); err == nil || !strings.Contains(err.Error(), "正则") {
		t.Errorf("expected bad-regex error, got %v", err)
	}
}

func TestValidateManifestSchema_RetryMaxRetriesOutOfRange(t *testing.T) {
	for _, n := range []int{0, 4} {
		m := validRetryManifest()
		m.Steps[1].Retry.MaxRetries = n
		if err := ValidateManifestSchema(m); err == nil || !strings.Contains(err.Error(), "max_retries") {
			t.Errorf("max_retries=%d should be rejected, got %v", n, err)
		}
	}
}

func TestValidateManifestSchema_RetryMissingPattern(t *testing.T) {
	m := validRetryManifest()
	m.Steps[1].Retry.WhenOutputMatches = ""
	if err := ValidateManifestSchema(m); err == nil || !strings.Contains(err.Error(), "when_output_matches") {
		t.Errorf("expected missing-pattern error, got %v", err)
	}
}

func TestLoadPatentWorkflowManifests_DraftingHasRetry(t *testing.T) {
	ms, err := LoadPatentWorkflowManifests()
	if err != nil {
		t.Fatal(err)
	}
	m := FindWorkflowManifest(ms, "patent-drafting")
	if m == nil {
		t.Fatal("patent-drafting manifest not found")
	}
	var found *PatentWorkflowStepRetry
	for _, s := range m.Steps {
		if s.ID == "s4-check" && s.Retry != nil {
			found = s.Retry
		}
	}
	if found == nil {
		t.Fatal("s4-check should carry a retry declaration")
	}
	if found.RewindTo != "s3-draft" || found.MaxRetries != 1 {
		t.Errorf("unexpected retry declaration: %+v", found)
	}
}
