package workflows

import (
	"context"
	"testing"
	"time"

	"github.com/xujian519/mady/graph"
)

// ---------------------------------------------------------------------------
// Workflow validation tests
// ---------------------------------------------------------------------------

func TestWorkflowValidate_EmptyName(t *testing.T) {
	w := &Workflow{
		Steps: []WorkflowStep{{ID: "s1", Type: StepAgent}},
	}
	err := w.Validate()
	if err == nil {
		t.Fatal("expected error for empty workflow name")
	}
}

func TestWorkflowValidate_EmptySteps(t *testing.T) {
	w := &Workflow{Name: "test"}
	err := w.Validate()
	if err == nil {
		t.Fatal("expected error for empty steps")
	}
}

func TestWorkflowValidate_StepMissingID(t *testing.T) {
	w := &Workflow{
		Name:  "test",
		Steps: []WorkflowStep{{Type: StepAgent}},
	}
	err := w.Validate()
	if err == nil {
		t.Fatal("expected error for step missing ID")
	}
}

func TestWorkflowValidate_StepDuplicateID(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "s1", Type: StepAgent},
			{ID: "s1", Type: StepTool, Tool: "search"},
		},
	}
	err := w.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate step IDs")
	}
}

func TestWorkflowValidate_ToolStepMissingTool(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "s1", Type: StepTool},
		},
	}
	err := w.Validate()
	if err == nil {
		t.Fatal("expected error for tool step missing tool name")
	}
}

func TestWorkflowValidate_UnknownDependency(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "s1", Type: StepAgent},
			{ID: "s2", Type: StepAgent, DependsOn: []string{"nonexistent"}},
		},
	}
	err := w.Validate()
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
}

func TestWorkflowValidate_SelfDependency(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "s1", Type: StepAgent, DependsOn: []string{"s1"}},
		},
	}
	err := w.Validate()
	if err == nil {
		t.Fatal("expected error for self-dependency")
	}
}

func TestWorkflowValidate_CircularDependency(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "s1", Type: StepAgent, DependsOn: []string{"s2"}},
			{ID: "s2", Type: StepAgent, DependsOn: []string{"s1"}},
		},
	}
	err := w.Validate()
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
}

func TestWorkflowValidate_OK(t *testing.T) {
	w := &Workflow{
		Name: "novelty-check",
		Steps: []WorkflowStep{
			{ID: "search", Type: StepTool, Tool: "prior_art_search"},
			{ID: "analyze", Type: StepAgent, Role: "novelty-examiner", DependsOn: []string{"search"}},
			{ID: "report", Type: StepAgent, Role: "patent-agent", DependsOn: []string{"analyze"}},
		},
	}
	if err := w.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Topological ordering tests
// ---------------------------------------------------------------------------

func TestTopologicalSteps_Sequential(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "a", Type: StepAgent},
			{ID: "b", Type: StepAgent},
			{ID: "c", Type: StepAgent},
		},
	}
	order := w.TopologicalSteps()
	if len(order) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(order))
	}
	expected := []string{"a", "b", "c"}
	for i, id := range expected {
		if order[i] != id {
			t.Errorf("position %d: expected %q, got %q", i, id, order[i])
		}
	}
}

func TestTopologicalSteps_WithDependencies(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "c", Type: StepAgent, DependsOn: []string{"a"}},
			{ID: "a", Type: StepAgent},
			{ID: "b", Type: StepAgent, DependsOn: []string{"a"}},
		},
	}
	order := w.TopologicalSteps()
	if len(order) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(order))
	}
	// "a" must come before "b" and "c".
	aIdx := indexOf(order, "a")
	bIdx := indexOf(order, "b")
	cIdx := indexOf(order, "c")
	if aIdx >= bIdx {
		t.Errorf("step 'a' (index %d) must come before 'b' (index %d)", aIdx, bIdx)
	}
	if aIdx >= cIdx {
		t.Errorf("step 'a' (index %d) must come before 'c' (index %d)", aIdx, cIdx)
	}
}

func indexOf(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// StepByID tests
// ---------------------------------------------------------------------------

func TestStepByID_Found(t *testing.T) {
	w := &Workflow{
		Name:  "test",
		Steps: []WorkflowStep{{ID: "s1", Type: StepAgent}},
	}
	step := w.StepByID("s1")
	if step == nil {
		t.Fatal("expected non-nil step")
	}
	if step.ID != "s1" {
		t.Errorf("expected ID 's1', got %q", step.ID)
	}
}

func TestStepByID_NotFound(t *testing.T) {
	w := &Workflow{
		Name:  "test",
		Steps: []WorkflowStep{{ID: "s1", Type: StepAgent}},
	}
	step := w.StepByID("s2")
	if step != nil {
		t.Fatalf("expected nil step, got %+v", *step)
	}
}

// ---------------------------------------------------------------------------
// Template instantiation tests
// ---------------------------------------------------------------------------

func TestTemplateInstantiate(t *testing.T) {
	tmpl := WorkflowTemplate{
		Name:        "test-template",
		Description: "A test template",
		Steps: []TemplateStep{
			{ID: "search", Type: StepTool, Tool: "prior_art_search"},
			{ID: "analyze", Type: StepAgent, Role: "novelty-examiner", DependsOn: []int{0}},
		},
	}
	params := map[string]string{
		"query": "AI-driven patent search",
	}
	w := tmpl.Instantiate(params)
	if w.Name != "test-template" {
		t.Errorf("expected name 'test-template', got %q", w.Name)
	}
	if len(w.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(w.Steps))
	}
	if w.Steps[1].DependsOn[0] != "search" {
		t.Errorf("dependency should resolve to 'search', got %q", w.Steps[1].DependsOn[0])
	}
}

// ---------------------------------------------------------------------------
// Pregel compilation tests
// ---------------------------------------------------------------------------

func TestCompileWorkflowToPregel(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "search", Type: StepTool, Tool: "prior_art_search"},
			{ID: "analyze", Type: StepAgent, Role: "novelty-examiner", DependsOn: []string{"search"}},
			{ID: "report", Type: StepAgent, Role: "patent-agent", DependsOn: []string{"analyze"}},
			{ID: "approve", Type: StepHumanApproval, HumanApprovalFor: "report", DependsOn: []string{"report"}},
		},
	}
	pg := compileWorkflowToPregel(w)
	if pg == nil {
		t.Fatal("expected non-nil PregelGraph")
	}
	compiled, err := pg.Compile("search", 40)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	result, err := compiled.Run(context.Background(), graph.PregelState{
		"input": "test input",
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	steps, _ := result["steps"].([]string)
	if len(steps) == 0 {
		t.Fatal("expected non-empty steps")
	}
	lastStep, _ := result["last_step"].(string)
	if lastStep != "approve" {
		t.Errorf("expected last step 'approve', got %q", lastStep)
	}
}

func TestCompileWorkflow_UnknownStepTypeReturnsError(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "weird", Type: StepType("unknown-type")},
		},
	}
	pg := compileWorkflowToPregel(w)
	compiled, err := pg.Compile("weird", 10)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	_, err = compiled.Run(context.Background(), graph.PregelState{})
	if err == nil {
		t.Fatal("expected error for unknown step type")
	}
}

// ---------------------------------------------------------------------------
// Dependencies tests
// ---------------------------------------------------------------------------

func TestDependencies(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "a", Type: StepAgent},
			{ID: "b", Type: StepAgent, DependsOn: []string{"a"}},
			{ID: "c", Type: StepAgent, DependsOn: []string{"a", "b"}},
		},
	}
	deps := w.Dependencies("c")
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d: %v", len(deps), deps)
	}
}

func TestDependencies_None(t *testing.T) {
	w := &Workflow{
		Name: "test",
		Steps: []WorkflowStep{
			{ID: "a", Type: StepAgent},
		},
	}
	deps := w.Dependencies("a")
	if len(deps) != 0 {
		t.Fatalf("expected 0 dependencies, got %d", len(deps))
	}
}

// ---------------------------------------------------------------------------
// Timeout and Retry tests
// ---------------------------------------------------------------------------

func TestWorkflowStep_Timeout(t *testing.T) {
	step := WorkflowStep{
		ID:      "s1",
		Type:    StepAgent,
		Timeout: 30 * time.Second,
		Retry:   3,
	}
	if step.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", step.Timeout)
	}
	if step.Retry != 3 {
		t.Errorf("expected 3 retries, got %d", step.Retry)
	}
}

// ---------------------------------------------------------------------------
// Edge case: empty workflow steps
// ---------------------------------------------------------------------------

func TestTopologicalSteps_Empty(t *testing.T) {
	w := &Workflow{Name: "empty"}
	order := w.TopologicalSteps()
	if order == nil {
		t.Fatal("expected empty slice, not nil")
	}
	if len(order) != 0 {
		t.Errorf("expected 0 steps, got %d", len(order))
	}
}

func TestStepByID_EmptyWorkflow(t *testing.T) {
	w := &Workflow{Name: "empty"}
	step := w.StepByID("anything")
	if step != nil {
		t.Fatal("expected nil for empty workflow")
	}
}
