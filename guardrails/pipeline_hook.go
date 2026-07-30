package guardrails

import (
	"context"

	iface "github.com/xujian519/mady/agentcore/iface"
)

// PipelineHook adapts a RulePipeline into a LifecycleHook that runs
// after each model call. It applies the pipeline rules and updates
// the model call context with any modifications.
type PipelineHook struct {
	iface.BaseLifecycleHook
	Pipeline *RulePipeline
	// MetadataFn provides additional metadata for rule checks.
	// If nil, no metadata is passed.
	MetadataFn func() map[string]any
}

// NewPipelineHook creates a LifecycleHook from a RulePipeline.
func NewPipelineHook(pipeline *RulePipeline) *PipelineHook {
	return &PipelineHook{Pipeline: pipeline}
}

// AfterModelCall applies all pipeline rules to the model output.
func (h *PipelineHook) AfterModelCall(_ context.Context, _ *iface.AgentRunContext, mcc *iface.ModelCallContext) {
	if h.Pipeline == nil || mcc == nil {
		return
	}

	var metadata map[string]any
	if h.MetadataFn != nil {
		metadata = h.MetadataFn()
	}

	content, results := h.Pipeline.Apply(mcc.Content, metadata)
	mcc.Content = content

	// If any blocking rule fired, mark the content as blocked.
	if HasBlocking(results) {
		mcc.Blocked = true
	}
}
