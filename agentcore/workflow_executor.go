// Package agentcore provides the unified Workflow abstraction —
// a common interface for all workflow definition formats and execution engines.
//
// WorkflowManifest is implemented by both PluginManifest (pluginsys) and
// OrchestrationManifest (orchestration.go). WorkflowExecutor dispatches to
// the appropriate engine (PipelineExecutor or OrchestrationExecutor) based
// on the manifest type at runtime.
//
// This is a Facade pattern: the two existing executors continue to work
// independently. New code should prefer the WorkflowExecutor interface for
// type safety and future compatibility.
package agentcore

import (
	"context"
	"fmt"
)

// WorkflowManifest is the common interface for all workflow definition formats.
// Both PluginManifest and OrchestrationManifest implement this interface.
type WorkflowManifest interface {
	// ID returns the unique workflow identifier (e.g., "novelty-analysis", "oa_response").
	ID() string

	// Name returns the human-readable label.
	Name() string

	// Description explains what this workflow does.
	Description() string

	// Domain returns the functional domain (patent, legal, design, general).
	Domain() string
}

// WorkflowResult is the common result type for all workflow executions.
type WorkflowResult struct {
	// Success indicates whether the workflow completed without fatal errors.
	Success bool

	// Output is the final workflow output (type depends on workflow format).
	Output any

	// Summary is a human-readable execution summary.
	Summary string

	// StepCount is the number of steps that actually executed.
	StepCount int

	// Interrupted indicates the workflow paused for user confirmation.
	Interrupted bool

	// PartialState is the state at interrupt time (for resume support).
	PartialState map[string]any
}

// WorkflowExecutor is the common interface for workflow execution engines.
// Both PipelineExecutor and OrchestrationExecutor conform to this interface.
type WorkflowExecutor interface {
	// Name returns a human-readable engine identifier (e.g., "pipeline", "orchestration").
	Name() string

	// CanExecute returns true if this executor can handle the given manifest type.
	CanExecute(m WorkflowManifest) bool

	// Execute runs the workflow and returns the result.
	Execute(ctx context.Context, m WorkflowManifest, input map[string]any) (*WorkflowResult, error)
}

// =============================================================================
// WorkflowManifest adapters
// =============================================================================

// pluginManifestAdapter adapts PluginManifest to the WorkflowManifest interface.
type pluginManifestAdapter struct {
	manifest PluginManifest
}

func (a *pluginManifestAdapter) ID() string          { return a.manifest.Name }
func (a *pluginManifestAdapter) Name() string        { return a.manifest.Name }
func (a *pluginManifestAdapter) Description() string { return a.manifest.Description }
func (a *pluginManifestAdapter) Domain() string      { return a.manifest.Domain }

// AsWorkflowManifest wraps a PluginManifest as a WorkflowManifest.
func AsWorkflowManifest(p PluginManifest) WorkflowManifest {
	return &pluginManifestAdapter{manifest: p}
}

// orchestrationManifestAdapter adapts OrchestrationManifest to the WorkflowManifest interface.
type orchestrationManifestAdapter struct {
	manifest OrchestrationManifest
}

func (a *orchestrationManifestAdapter) ID() string          { return a.manifest.ID }
func (a *orchestrationManifestAdapter) Name() string        { return a.manifest.Name }
func (a *orchestrationManifestAdapter) Description() string { return a.manifest.Description }
func (a *orchestrationManifestAdapter) Domain() string      { return "patent" } // orchestrations are patent-domain

// AsOrchestrationManifest wraps an OrchestrationManifest as a WorkflowManifest.
func AsOrchestrationManifest(m OrchestrationManifest) WorkflowManifest {
	return &orchestrationManifestAdapter{manifest: m}
}

// =============================================================================
// Executor adaptation
// =============================================================================

// PipelineExecutorAdapter adapts PipelineExecutor to the WorkflowExecutor interface.
type PipelineExecutorAdapter struct {
	inner *PipelineExecutor
}

// NewPipelineExecutorAdapter wraps a PipelineExecutor for use as a WorkflowExecutor.
func NewPipelineExecutorAdapter(inner *PipelineExecutor) *PipelineExecutorAdapter {
	return &PipelineExecutorAdapter{inner: inner}
}

func (a *PipelineExecutorAdapter) Name() string { return "pipeline" }

func (a *PipelineExecutorAdapter) CanExecute(m WorkflowManifest) bool {
	_, ok := m.(*pluginManifestAdapter)
	return ok
}

func (a *PipelineExecutorAdapter) Execute(ctx context.Context, m WorkflowManifest, input map[string]any) (*WorkflowResult, error) {
	adapter, ok := m.(*pluginManifestAdapter)
	if !ok {
		return nil, fmt.Errorf("pipeline executor: expected PluginManifest, got %T", m)
	}
	state := PipelineState(input)
	result, err := a.inner.Run(ctx, &adapter.manifest, state)
	if err != nil {
		if IsInterruptStage(err) {
			return &WorkflowResult{
				Success:      true,
				Interrupted:  true,
				PartialState: result,
				Summary:      "Pipeline paused for user confirmation.",
			}, nil
		}
		return nil, err
	}
	return &WorkflowResult{
		Success:   true,
		Output:    result,
		StepCount: len(adapter.manifest.Pipeline.Stages),
	}, nil
}

// OrchestrationExecutorAdapter adapts OrchestrationExecutor to the WorkflowExecutor interface.
type OrchestrationExecutorAdapter struct {
	inner *OrchestrationExecutor
}

// NewOrchestrationExecutorAdapter wraps an OrchestrationExecutor for use as a WorkflowExecutor.
func NewOrchestrationExecutorAdapter(inner *OrchestrationExecutor) *OrchestrationExecutorAdapter {
	return &OrchestrationExecutorAdapter{inner: inner}
}

func (a *OrchestrationExecutorAdapter) Name() string { return "orchestration" }

func (a *OrchestrationExecutorAdapter) CanExecute(m WorkflowManifest) bool {
	_, ok := m.(*orchestrationManifestAdapter)
	return ok
}

func (a *OrchestrationExecutorAdapter) Execute(ctx context.Context, m WorkflowManifest, input map[string]any) (*WorkflowResult, error) {
	adapter, ok := m.(*orchestrationManifestAdapter)
	if !ok {
		return nil, fmt.Errorf("orchestration executor: expected OrchestrationManifest, got %T", m)
	}
	state := OrchestrationState(input)
	result, err := a.inner.Run(ctx, &adapter.manifest, state)
	if err != nil {
		return nil, err
	}
	return &WorkflowResult{
		Success:      result.Success,
		Output:       result.FinalOutput,
		Summary:      result.Summary,
		StepCount:    result.StepsCompleted,
		Interrupted:  result.InterruptedStep != "",
		PartialState: result.PartialState,
	}, nil
}

// ComposeExecutors creates a single WorkflowExecutor that delegates to the
// first matching executor. Executors are tried in order — the first one
// whose CanExecute returns true handles the manifest.
func ComposeExecutors(executors ...WorkflowExecutor) WorkflowExecutor {
	return &compositeExecutor{executors: executors}
}

type compositeExecutor struct {
	executors []WorkflowExecutor
}

func (c *compositeExecutor) Name() string { return "composite" }

func (c *compositeExecutor) CanExecute(m WorkflowManifest) bool {
	for _, ex := range c.executors {
		if ex.CanExecute(m) {
			return true
		}
	}
	return false
}

func (c *compositeExecutor) Execute(ctx context.Context, m WorkflowManifest, input map[string]any) (*WorkflowResult, error) {
	for _, ex := range c.executors {
		if ex.CanExecute(m) {
			return ex.Execute(ctx, m, input)
		}
	}
	return nil, fmt.Errorf("no executor found for workflow %q (type: %T)", m.ID(), m)
}
