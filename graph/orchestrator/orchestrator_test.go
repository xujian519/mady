package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

// buildTestGraph creates a simple linear Pregel graph: entry → step1 → step2 → output.
func buildTestGraph(t *testing.T) *graph.CompiledPregelGraph {
	t.Helper()
	pg := graph.NewPregelGraph()

	step1 := func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		input := state.GetString("input")
		return graph.PregelState{
			"step1_output": strings.ToUpper(input),
		}, nil
	}
	step2 := func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		prev := state.GetString("step1_output")
		return graph.PregelState{
			"output": "processed: " + prev,
		}, nil
	}

	pg.AddNode("entry", step1)
	pg.AddNode("step2", step2)
	pg.AddEdge("entry", "step2")
	pg.AddEdge("step2", graph.PregelEnd)

	cpg, err := pg.Compile("entry")
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}
	return cpg
}

func TestOrchestrator_Execute(t *testing.T) {
	store := newMemoryCheckpointStore()
	cpg := buildTestGraph(t)
	orc := New(store, cpg)

	plan := &ExecutionPlan{
		ID:    "test-plan-1",
		Input: "hello world",
	}

	state, err := orc.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := state.GetString("output")
	if !strings.Contains(output, "processed: HELLO WORLD") {
		t.Errorf("expected processed output, got: %s", output)
	}
}

func TestOrchestrator_ExecuteAndResume(t *testing.T) {
	store := newMemoryCheckpointStore()
	cpg := buildTestGraph(t)
	orc := New(store, cpg)

	// First execute to completion — this saves checkpoints
	plan := &ExecutionPlan{
		ID:    "test-plan-2",
		Input: "test data",
	}
	state, err := orc.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if state.GetString("output") == "" {
		t.Error("expected output from execution")
	}

	// Resume from checkpoints — should complete (all steps already done)
	resumedState, err := orc.Resume(context.Background(), "test-plan-2")
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if resumedState.GetString("output") != state.GetString("output") {
		t.Errorf("resumed output differs: got %q, want %q",
			resumedState.GetString("output"), state.GetString("output"))
	}
}

func TestOrchestrator_ResumeNoCheckpoint(t *testing.T) {
	store := newMemoryCheckpointStore()
	cpg := buildTestGraph(t)
	orc := New(store, cpg)

	_, err := orc.Resume(context.Background(), "nonexistent-plan")
	if err == nil {
		t.Fatal("expected error for nonexistent plan")
	}
	if !strings.Contains(err.Error(), "no checkpoint") {
		t.Errorf("expected 'no checkpoint' error, got: %v", err)
	}
}

func TestOrchestrator_Cleanup(t *testing.T) {
	store := newMemoryCheckpointStore()
	cpg := buildTestGraph(t)
	orc := New(store, cpg)

	plan := &ExecutionPlan{
		ID:    "test-plan-3",
		Input: "data",
	}
	_, err := orc.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Cleanup should remove all checkpoints
	if err := orc.Cleanup(context.Background(), "test-plan-3"); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// After cleanup, resume should fail
	_, err = orc.Resume(context.Background(), "test-plan-3")
	if err == nil {
		t.Fatal("expected resume to fail after cleanup")
	}
}

func TestOrchestrator_NilPlan(t *testing.T) {
	store := newMemoryCheckpointStore()
	cpg := buildTestGraph(t)
	orc := New(store, cpg)

	_, err := orc.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil plan")
	}
}

func TestOrchestrator_EmptyPlanID(t *testing.T) {
	store := newMemoryCheckpointStore()
	cpg := buildTestGraph(t)
	orc := New(store, cpg)

	_, err := orc.Resume(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty plan ID")
	}
}
