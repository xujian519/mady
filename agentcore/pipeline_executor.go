package agentcore

import (
	"context"
	"errors"
	"fmt"
)

// PipelineExecutor runs a PluginManifest's pipeline by dispatching each stage
// to the registered StageHandler. Stages without a handler are skipped with
// a warning unless FailOnUnknown is set.
//
// Usage:
//
//	executor := NewPipelineExecutor(provider)
//	state, err := executor.Run(ctx, manifest, PipelineState{"input": text})
type PipelineExecutor struct {
	provider Provider

	// FailOnUnknown controls behavior for stages whose atom/tool has no
	// registered handler. When true, execution stops with an error.
	// When false, the stage is skipped with a state warning entry.
	FailOnUnknown bool
}

// ExecOption configures a PipelineExecutor.
type ExecOption func(*PipelineExecutor)

// WithFailOnUnknown configures whether unknown stages cause failure.
func WithFailOnUnknown(fail bool) ExecOption {
	return func(e *PipelineExecutor) { e.FailOnUnknown = fail }
}

// NewPipelineExecutor creates a pipeline executor with the given provider.
func NewPipelineExecutor(provider Provider, opts ...ExecOption) *PipelineExecutor {
	e := &PipelineExecutor{
		provider:      provider,
		FailOnUnknown: false, // default: skip unknown stages
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Run executes all stages in the manifest's pipeline sequentially.
// The initial state is a copy of input (safe to reuse input across calls).
//
// Returns the final PipelineState after all stages complete, or an error
// if a stage fails (unless the error is an InterruptStageError, which is
// returned as-is for the caller to handle).
func (e *PipelineExecutor) Run(ctx context.Context, manifest *PluginManifest, input PipelineState) (PipelineState, error) {
	if manifest == nil {
		return nil, fmt.Errorf("pipeline: manifest is nil")
	}
	if len(manifest.Pipeline.Stages) == 0 {
		return input, nil
	}

	// Copy input to avoid mutation of caller's map.
	state := make(PipelineState, len(input))
	for k, v := range input {
		state[k] = v
	}

	// Seed manifest metadata into state for downstream handlers.
	state["plugin_name"] = manifest.Name
	state["plugin_domain"] = manifest.Domain

	// Track stage execution metadata.
	executedStages := make([]string, 0, len(manifest.Pipeline.Stages))
	warnings := make([]string, 0)

	// finalizeState writes execution metadata before any return.
	finalizeState := func() {
		if len(warnings) > 0 {
			state["_warnings"] = warnings
		}
		state["_executed_stages"] = executedStages
	}

	for _, stage := range manifest.Pipeline.Stages {
		// 1) Atom-based stage: dispatch to registered StageHandler.
		if stage.Atom != "" {
			handler := LookupStageHandler(stage.Atom)
			if handler == nil {
				msg := fmt.Sprintf("stage %q: no handler for atom %q", stage.ID, stage.Atom)
				if e.FailOnUnknown {
					finalizeState()
					return state, &StageError{StageID: stage.ID, Atom: stage.Atom, Err: errors.New(msg)}
				}
				warnings = append(warnings, msg)
				continue
			}

			out, err := handler.Execute(ctx, state, e.provider)
			if err != nil {
				if IsInterruptStage(err) {
					// Interrupt (e.g. approval-gate) is returned as-is.
					state["_interrupted_at"] = stage.ID
					finalizeState()
					return state, err
				}
				finalizeState()
				return state, &StageError{StageID: stage.ID, Atom: stage.Atom, Err: err}
			}

			// Merge handler output into state (handler output keys
			// may shadow existing keys; this is intentional for stage
			// pipelines where later stages override earlier outputs).
			for k, v := range out {
				state[k] = v
			}
			executedStages = append(executedStages, stage.ID)
			continue
		}

		// 2) Tool-based stage: not yet implemented — skip with warning.
		if stage.Tool != "" {
			warnings = append(warnings, fmt.Sprintf(
				"stage %q: tool-based execution not yet implemented (tool=%q)", stage.ID, stage.Tool))
			continue
		}

		// 3) Neither atom nor tool — skip.
		warnings = append(warnings, fmt.Sprintf("stage %q: neither atom nor tool set", stage.ID))
	}

	finalizeState()
	return state, nil
}
