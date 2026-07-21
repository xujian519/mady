package evaluate

import (
	"testing"
)

func TestWorkflowQuality_Name(t *testing.T) {
	m := WorkflowQuality{}
	if got := m.Name(); got != "workflow_quality" {
		t.Errorf("Name() = %q, want %q", got, "workflow_quality")
	}
}

func TestWorkflowQuality_Empty(t *testing.T) {
	m := WorkflowQuality{}
	if got := m.Compute("", ""); got != 1 {
		t.Errorf("both empty: got %v, want 1", got)
	}
	if got := m.Compute("", `[{"step":"search","status":"completed"}]`); got != 0 {
		t.Errorf("pred empty: got %v, want 0", got)
	}
	if got := m.Compute(`[{"step":"search","status":"completed"}]`, ""); got != 0 {
		t.Errorf("ref empty: got %v, want 0", got)
	}
}

func TestWorkflowQuality_ExactMatch(t *testing.T) {
	m := WorkflowQuality{}
	pred := `[
		{"step":"search_patents","status":"completed"},
		{"step":"analyze_claims","status":"completed"},
		{"step":"generate_report","status":"completed"}
	]`
	ref := `[
		{"step":"search_patents","status":"completed"},
		{"step":"analyze_claims","status":"completed"},
		{"step":"generate_report","status":"completed"}
	]`
	score := m.Compute(pred, ref)
	if score != 1 {
		t.Errorf("exact match: got %v, want 1", score)
	}
}

func TestWorkflowQuality_MissingStep(t *testing.T) {
	m := WorkflowQuality{}
	pred := `[
		{"step":"search_patents","status":"completed"},
		{"step":"generate_report","status":"completed"}
	]`
	ref := `[
		{"step":"search_patents","status":"completed"},
		{"step":"analyze_claims","status":"completed"},
		{"step":"generate_report","status":"completed"}
	]`
	score := m.Compute(pred, ref)
	// completion: 2/3=0.667, ordering: 2/2=1, precision: 2/2=1 → (0.667+1+1)/3 = 0.889
	want := (2.0/3.0 + 1.0 + 1.0) / 3.0
	if !approxEq(score, want, 0.01) {
		t.Errorf("missing step: got %v, want ~%v", score, want)
	}
}

func TestWorkflowQuality_ExtraStep(t *testing.T) {
	m := WorkflowQuality{}
	pred := `[
		{"step":"search_patents","status":"completed"},
		{"step":"analyze_claims","status":"completed"},
		{"step":"extra_step","status":"completed"},
		{"step":"generate_report","status":"completed"}
	]`
	ref := `[
		{"step":"search_patents","status":"completed"},
		{"step":"analyze_claims","status":"completed"},
		{"step":"generate_report","status":"completed"}
	]`
	score := m.Compute(pred, ref)
	// completion: 3/3=1, ordering: 3/3=1, precision: 3/4=0.75 → (1+1+0.75)/3 = 0.917
	want := (1.0 + 1.0 + 0.75) / 3.0
	if !approxEq(score, want, 0.01) {
		t.Errorf("extra step: got %v, want ~%v", score, want)
	}
}

func TestWorkflowQuality_WrongOrder(t *testing.T) {
	m := WorkflowQuality{}
	pred := `[
		{"step":"generate_report","status":"completed"},
		{"step":"search_patents","status":"completed"},
		{"step":"analyze_claims","status":"completed"}
	]`
	ref := `[
		{"step":"search_patents","status":"completed"},
		{"step":"analyze_claims","status":"completed"},
		{"step":"generate_report","status":"completed"}
	]`
	score := m.Compute(pred, ref)
	// completion: 3/3=1, ordering: 2/3=0.667, precision: 3/3=1 → (1+0.667+1)/3 = 0.889
	want := (1.0 + 2.0/3.0 + 1.0) / 3.0
	if !approxEq(score, want, 0.01) {
		t.Errorf("wrong order: got %v, want ~%v", score, want)
	}
}

func TestWorkflowQuality_ParallelPattern(t *testing.T) {
	m := WorkflowQuality{Pattern: WorkflowParallel}
	pred := `[
		{"step":"search_patents","status":"completed"},
		{"step":"generate_report","status":"completed"},
		{"step":"analyze_claims","status":"completed"}
	]`
	ref := `[
		{"step":"search_patents","status":"completed"},
		{"step":"analyze_claims","status":"completed"},
		{"step":"generate_report","status":"completed"}
	]`
	score := m.Compute(pred, ref)
	// Parallel: completion=3/3=1, ordering=1 (parallel), precision=3/3=1 → (1+1+1)/3 = 1
	if score != 1 {
		t.Errorf("parallel pattern: got %v, want 1", score)
	}
}

func TestWorkflowQuality_EmbeddedJSON(t *testing.T) {
	m := WorkflowQuality{}
	pred := `The workflow executed the following steps:
		[{"step":"search_patents","status":"completed"}]
		Then moved to analysis.`
	ref := `[{"step":"search_patents","status":"completed"}]`
	score := m.Compute(pred, ref)
	if score != 1 {
		t.Errorf("embedded JSON: got %v, want 1", score)
	}
}

func TestWorkflowQuality_StatusIgnored(t *testing.T) {
	// Status ("completed", "skipped", "failed") should not affect the scores
	// since we evaluate based on step names only.
	m := WorkflowQuality{}
	pred := `[{"step":"search","status":"failed"}]`
	ref := `[{"step":"search","status":"completed"}]`
	score := m.Compute(pred, ref)
	if score != 1 {
		t.Errorf("status ignored: got %v, want 1", score)
	}
}

func TestParseWorkflowSteps_Single(t *testing.T) {
	steps := parseWorkflowSteps(`{"step":"test","status":"completed"}`)
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Step != "test" {
		t.Errorf("step name = %q, want %q", steps[0].Step, "test")
	}
}

func TestParseWorkflowSteps_LineByLine(t *testing.T) {
	input := `{"step":"a","status":"completed"}
{"step":"b","status":"completed"}`
	steps := parseWorkflowSteps(input)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
}
