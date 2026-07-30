// Package orchestrator provides a Plan→Execute→Checkpoint→Recover execution
// engine built on top of Mady's Pregel graph engine.
//
// The Orchestrator wraps graph execution with:
//   - Automatic checkpointing before each node execution
//   - Resume from the latest checkpoint on restart
//   - Node-level retry with configurable limits
//   - Plan generation from intent classification results
//
// Usage:
//
//	orc := orchestrator.New(store, pregel)
//	result, err := orc.Execute(ctx, plan)
//	// On restart:
//	result, err = orc.Resume(ctx, planID)
package orchestrator

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/xujian519/mady/graph"
)

// Orchestrator executes plans with checkpoint-and-recover semantics.
type Orchestrator struct {
	store graph.CheckpointStore
	pg    *graph.PregelCheckpointer
}

// New creates an Orchestrator backed by the given checkpoint store and
// Pregel graph.
func New(store graph.CheckpointStore, cpg *graph.CompiledPregelGraph) *Orchestrator {
	return &Orchestrator{
		store: store,
		pg:    graph.NewPregelCheckpointer(cpg, store),
	}
}

// Execute runs a plan from scratch, checkpointing before each step.
// Returns the final state or an error. Checkpoints are saved with the
// plan ID as the graph ID for later Resume.
func (o *Orchestrator) Execute(ctx context.Context, plan *ExecutionPlan) (graph.PregelState, error) {
	if plan == nil {
		return nil, fmt.Errorf("orchestrator: nil plan")
	}

	slog.Info("orchestrator: starting execution", "plan_id", plan.ID)
	initial := graph.PregelState{"input": plan.Input}
	state, err := o.pg.RunWithCheckpoints(ctx, initial, plan.ID)
	if err != nil {
		return state, fmt.Errorf("orchestrator: execution failed: %w", err)
	}

	return state, nil
}

// Resume continues execution from the latest checkpoint for the given plan ID.
// Use this after a crash or intentional interrupt to pick up where execution
// left off.
func (o *Orchestrator) Resume(ctx context.Context, planID string) (graph.PregelState, error) {
	if planID == "" {
		return nil, fmt.Errorf("orchestrator: plan ID is required for resume")
	}

	slog.Info("orchestrator: resuming execution", "plan_id", planID)
	state, err := o.pg.Resume(ctx, planID)
	if err != nil {
		return state, fmt.Errorf("orchestrator: resume failed: %w", err)
	}

	return state, nil
}

// Cleanup removes all checkpoints for a completed plan.
func (o *Orchestrator) Cleanup(ctx context.Context, planID string) error {
	checkpoints, err := o.store.List(ctx, planID)
	if err != nil {
		return fmt.Errorf("orchestrator cleanup: list: %w", err)
	}
	for _, cp := range checkpoints {
		if err := o.store.Delete(ctx, cp.ID); err != nil {
			slog.Warn("orchestrator cleanup: failed to delete checkpoint",
				"checkpoint_id", cp.ID, "error", err)
		}
	}
	return nil
}
