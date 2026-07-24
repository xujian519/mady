// Package agentcore provides the Orchestration abstraction — a named,
// executable workflow composed of ordered tool-calling steps.
//
// Orchestration bridges the gap between YAML-defined workflow plans
// (domains/rules/data/orchestrations/) and runtime tool execution.
// Instead of requiring the LLM to interpret workflow descriptions and
// manually chain tool calls, an OrchestrationManifest provides a
// machine-executable recipe that OrchestrationExecutor follows.
//
// Design inspired by:
//   - Eino's Chain pattern (sequential tool composition)
//   - Semantic Kernel's Planner (task → plan decomposition)
//   - LangGraph's conditional edges (dynamic step routing)
//
// Key concepts:
//   - OrchestrationManifest: a named workflow with ordered steps.
//   - OrchestrationStep: one step in the workflow — tool name, description,
//     optional condition, optional flag, and input routing.
//   - ConditionFunc: a predicate evaluated against accumulated step outputs
//     that determines whether a conditional step should execute.
//   - InputMapper: transforms accumulated outputs into a tool's input arguments.
package agentcore

import (
	"strings"
)

// OrchestrationManifest defines a complete executable workflow —
// a named sequence of tool-calling steps with conditional branching.
//
// It is the runtime counterpart of YAML orchestration files
// (e.g., domains/rules/data/orchestrations/oa-response.yaml).
// The YAML compiler lives in domains/orchestration_bridge.go.
type OrchestrationManifest struct {
	// ID is the unique identifier (e.g., "oa_response", "re_examination").
	ID string

	// Name is the human-readable label (e.g., "审查意见答复").
	Name string

	// Description explains what this orchestration does and when to use it.
	Description string

	// Steps is the ordered list of tool-calling steps.
	// The executor runs them sequentially unless a Condition blocks a step.
	Steps []OrchestrationStep
}

// OrchestrationStep is a single tool-invocation step within an orchestration.
//
// Each step maps to one tool call. Conditional steps (Condition != nil)
// are skipped when the predicate returns false. Optional steps (Optional=true)
// are skipped without failing the orchestration when the tool call errors.
type OrchestrationStep struct {
	// ToolName is the registered tool to call (e.g., "parse_office_action").
	ToolName string

	// Description is a human-readable label for this step
	// (shown in execution logs and result summaries).
	Description string

	// InputKey is the key under which the tool's input arguments are
	// stored in the orchestration state. When empty, the tool receives
	// an empty argument map (useful for no-input tools).
	// The caller populates this key before invoking the executor.
	InputKey string

	// Condition is an optional predicate. When non-nil, the step is
	// executed only when Condition(state) returns true.
	// Use this for conditional branching: e.g., only call
	// analyze_enablement when the OA mentions 26.3.
	Condition ConditionFunc

	// Optional marks a step whose failure should not abort the
	// orchestration. Optional steps that error are logged and
	// produce a nil output, and execution continues with the next step.
	Optional bool
}

// ConditionFunc is a predicate over the accumulated orchestration state.
// It receives a map of step-name → step-output and returns whether the
// associated step should execute.
type ConditionFunc func(state map[string]any) bool

// OrchestrationState is the key-value store carried through an
// orchestration execution. Each step writes its output under the
// step's ToolName key (or a custom key set by the step's handler).
//
// Type-safe access methods (GetMap, GetString, GetArray) reduce
// the boilerplate of repeated type assertions in condition functions
// and callers.
type OrchestrationState map[string]any

// GetMap returns the map value for key, or nil if missing/unexpected type.
func (s OrchestrationState) GetMap(key string) map[string]any {
	if v, ok := s[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return nil
}

// GetString returns the string value for key, or "" if missing/unexpected type.
func (s OrchestrationState) GetString(key string) string {
	if v, ok := s[key]; ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

// GetArray returns the []any value for key, or nil if missing/unexpected type.
func (s OrchestrationState) GetArray(key string) []any {
	if v, ok := s[key]; ok {
		if a, ok := v.([]any); ok {
			return a
		}
	}
	return nil
}

// StepOutputKey returns the state key under which a step's output is stored.
// By convention, each step's output is stored at state[step.ToolName].
// Use this instead of hardcoding magic strings like state["parse_office_action"].
func StepOutputKey(toolName string) string {
	return toolName
}

// HasRejectionKeywords checks whether the given state key's string value
// contains any of the standard patent rejection keywords.
// This is a convenience for condition functions that inspect parsed OA output.
func (s OrchestrationState) HasRejectionKeywords(key string) bool {
	text := s.GetString(key)
	if text != "" {
		return containsRejectionKeyword(text)
	}
	// Also check nested fields in a map value.
	if m := s.GetMap(key); m != nil {
		for _, nested := range []string{"result", "summary", "text", "grounds"} {
			if s, ok := m[nested].(string); ok {
				if containsRejectionKeyword(s) {
					return true
				}
			}
		}
	}
	return false
}

// containsRejectionKeyword is a local keyword matcher that avoids a
// circular dependency on pkg/util from the agentcore layer.
func containsRejectionKeyword(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "创造性") ||
		strings.Contains(lower, "新颖性") ||
		strings.Contains(lower, "充分公开") ||
		strings.Contains(lower, "不清楚") ||
		strings.Contains(lower, "不支持") ||
		strings.Contains(lower, "22.2") ||
		strings.Contains(lower, "22.3") ||
		strings.Contains(lower, "26.3") ||
		strings.Contains(lower, "26.4") ||
		strings.Contains(lower, "33条") ||
		strings.Contains(lower, "a33")
}

// OrchestrationResult summarizes the outcome of an orchestration run.
type OrchestrationResult struct {
	// OrchestrationID identifies which orchestration was executed.
	OrchestrationID string

	// Success indicates whether the orchestration completed without
	// fatal errors. Optional-step failures do not affect Success.
	// Interrupted (paused for user confirmation) is also Success=true.
	Success bool

	// StepsCompleted is the number of steps that executed (including
	// optional steps that failed).
	StepsCompleted int

	// StepsSkipped counts conditional steps whose Condition returned false.
	StepsSkipped int

	// StepResults maps step ToolName to its output. Failed optional
	// steps store the error message as the value.
	StepResults map[string]any

	// StepErrors maps step ToolName to error messages (only non-empty
	// for optional-step failures).
	StepErrors map[string]string

	// Summary is a human-readable Markdown summary of the orchestration
	// execution, suitable for display to the user.
	Summary string

	// FinalOutput is the output of the last executed (non-optional,
	// non-skipped) step. This is the primary result the caller cares about.
	FinalOutput any

	// --- Interrupt fields ---
	// When a step interrupts (returns IsInterrupt), execution pauses.
	// The caller should present PendingReview to the user, let them
	// confirm or request changes, then resume via Run() with restart=true.

	// InterruptedStep is set to the step's ToolName when execution paused
	// at that step for user confirmation. Empty = no interrupt.
	InterruptedStep string

	// PartialState captures the orchestration state at interrupt time.
	// Pass this back as initial state when resuming.
	PartialState OrchestrationState

	// PendingReview is a human-readable summary of what needs user
	// confirmation: the step's output and a prompt for the user.
	PendingReview string
}
