package reasoning

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/pkg/csync"
)

// =============================================================================
// Workflow Checkpoint — multi-stage save / resume for FiveStepRunner
// =============================================================================

// StageCheckpoint captures the full state of a running five-step workflow
// so execution can be paused and resumed at any stage boundary.
type StageCheckpoint struct {
	CheckpointID string          `json:"checkpoint_id"`
	CaseID       string          `json:"case_id"`
	CaseType     CaseType        `json:"case_type"`
	CurrentStage int             `json:"current_stage"`
	Blackboard   *FactBlackboard `json:"blackboard"`
	Plan         *Plan           `json:"plan,omitempty"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
}

// CheckpointStore persists and retrieves workflow checkpoints.
type CheckpointStore interface {
	Save(ctx context.Context, cp *StageCheckpoint) error
	Load(ctx context.Context, checkpointID string) (*StageCheckpoint, error)
	Delete(ctx context.Context, checkpointID string) error
}

// MemoryCheckpointStore is an in-memory implementation for testing.
type MemoryCheckpointStore struct {
	checkpoints csync.Map[string, *StageCheckpoint]
}

// NewMemoryCheckpointStore creates an in-memory checkpoint store.
func NewMemoryCheckpointStore() *MemoryCheckpointStore {
	return &MemoryCheckpointStore{}
}

func (s *MemoryCheckpointStore) Save(ctx context.Context, cp *StageCheckpoint) error {
	s.checkpoints.Set(cp.CheckpointID, cp)
	return nil
}

func (s *MemoryCheckpointStore) Load(ctx context.Context, checkpointID string) (*StageCheckpoint, error) {
	cp, ok := s.checkpoints.Get(checkpointID)
	if !ok {
		return nil, fmt.Errorf("checkpoint %q not found", checkpointID)
	}
	return cp, nil
}

func (s *MemoryCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	s.checkpoints.Del(checkpointID)
	return nil
}

// SaveCheckpoint serializes the runner's current state.
func (r *FiveStepRunner) SaveCheckpoint(ctx context.Context, store CheckpointStore, checkpointID string) error {
	cp := &StageCheckpoint{
		CheckpointID: checkpointID,
		CaseID:       r.bb.CaseID,
		CaseType:     r.bb.CaseType,
		CurrentStage: r.bb.CurrentStage(),
		Blackboard:   r.bb,
		Plan:         r.bb.PlanV2(),
		Metadata: map[string]any{
			"workflow_id": r.bb.WorkflowID(),
		},
	}
	return store.Save(ctx, cp)
}

// ResumeFromCheckpoint restores a runner from a saved checkpoint.
// Returns a new FiveStepRunner ready to continue from the saved stage.
func ResumeFromCheckpoint(ctx context.Context, store CheckpointStore, checkpointID string, cfg FiveStepRunnerConfig) (*FiveStepRunner, error) {
	cp, err := store.Load(ctx, checkpointID)
	if err != nil {
		return nil, fmt.Errorf("resume: %w", err)
	}

	// Use the restored blackboard and plan.
	cfg.CaseID = cp.CaseID
	cfg.CaseType = cp.CaseType

	runner := NewFiveStepRunner(cfg)
	runner.bb = cp.Blackboard
	if cp.Plan != nil {
		runner.bb.SetPlanV2(*cp.Plan)
	}
	runner.bb.SetCurrentStage(cp.CurrentStage)

	return runner, nil
}

// ContinueFromStage resumes execution from the given stage, skipping
// earlier stages. The blackboard must contain facts and rules from
// the completed stages (loaded via ResumeFromCheckpoint or a previous run).
func (r *FiveStepRunner) ContinueFromStage(ctx context.Context, input string, fromStage int) (string, error) {
	current := r.bb.CurrentStage()
	// Allow continuation when:
	//   - current == 0 (fresh blackboard initialized but stages not yet run), or
	//   - current < fromStage (some stages already done, e.g. stage 1 done for fromStage=2)
	if current != 0 && current >= fromStage {
		return "", fmt.Errorf("cannot continue from stage %d: current stage is %d (already past or at target)", fromStage, current)
	}
	return r.runFrom(ctx, input, fromStage)
}

// runFrom executes the workflow from the given stage onward.
// Stages before startStage are skipped.
func (r *FiveStepRunner) runFrom(ctx context.Context, input string, startStage int) (string, error) {
	// Stage ①: Collect facts (skip if already done).
	if startStage <= 1 {
		r.bb.SetCurrentStage(1)
		if err := r.runStage1(ctx, input); err != nil {
			return "", fmt.Errorf("stage ① fact collection: %w", err)
		}
	}

	// Stage ②: Retrieve rules (skip if already done).
	if startStage <= 2 {
		r.bb.SetCurrentStage(2)
		if err := r.runStage2(ctx); err != nil {
			return "", fmt.Errorf("stage ② rule retrieval: %w", err)
		}
	}

	// Stage ③: Generate Plan.
	r.bb.SetCurrentStage(3)
	intent := detectPlanIntent(r.bb)
	plan, err := r.planner.GeneratePlan(ctx, r.bb, intent)
	if err != nil {
		return "", fmt.Errorf("stage ③ plan generation: %w", err)
	}

	// Stage ④: Compile and execute.
	r.bb.SetCurrentStage(4)
	pregelGraph, entryName, err := r.compiler.CompilePlanToGraph(plan, r.bb)
	if err != nil {
		return "", fmt.Errorf("stage ④ compile: %w", err)
	}

	compiled, err := pregelGraph.Compile(entryName, 50)
	if err != nil {
		return "", fmt.Errorf("stage ④ compile: %w", err)
	}

	initial := graph.PregelState{
		"input":     input,
		"bb":        r.bb,
		"plan":      plan,
		"case_type": string(r.bb.CaseType),
	}
	_, err = compiled.Run(ctx, initial)
	if err != nil {
		return "", fmt.Errorf("stage ④ execute: %w", err)
	}

	// Stage ⑤: Check.
	r.bb.SetCurrentStage(5)
	var report CheckReport
	if r.checker != nil {
		level := CheckLevel1
		if r.manifest != nil && r.manifest.Stage5.SyllogismLevel > 0 {
			level = CheckLevel(r.manifest.Stage5.SyllogismLevel)
		}
		cr, err := r.checker.Check(ctx, r.bb, plan, level)
		if err != nil {
			return "", fmt.Errorf("stage ⑤ check: %w", err)
		}
		report = *cr
	} else {
		report = r.passThroughCheck(plan)
	}
	r.bb.SetCheckReport(report)

	return r.formatResult(plan, report)
}

// =============================================================================
// JSON serialization helpers for checkpoint persistence.
// =============================================================================

// MarshalCheckpoint serializes a StageCheckpoint to JSON.
func MarshalCheckpoint(cp *StageCheckpoint) ([]byte, error) {
	if cp.Blackboard != nil {
		// Ensure the blackboard is serializable.
		data, err := json.Marshal(cp)
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return json.Marshal(cp)
}

// UnmarshalCheckpoint deserializes a StageCheckpoint from JSON.
func UnmarshalCheckpoint(data []byte) (*StageCheckpoint, error) {
	var cp StageCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}
