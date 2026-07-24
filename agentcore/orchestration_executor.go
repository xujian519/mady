package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// OrchestrationExecutor runs an OrchestrationManifest by sequentially
// invoking each step's tool through the provided Agent.
//
// Usage via run_orchestration tool:
//
//	executor := NewOrchestrationExecutor(agent)
//	result, err := executor.Run(ctx, manifest, state)
//
// The executor:
//   - Evaluates Condition predicates to skip inapplicable steps.
//   - Calls Agent.InvokeTool for each step (preserving hook pipelines).
//   - Tolerates failures on Optional steps without aborting.
//   - Produces an OrchestrationResult with per-step outputs and a summary.
type OrchestrationExecutor struct {
	agent *Agent
}

// NewOrchestrationExecutor creates an executor bound to the given Agent.
// The Agent provides tool lookup and invocation.
func NewOrchestrationExecutor(agent *Agent) *OrchestrationExecutor {
	return &OrchestrationExecutor{agent: agent}
}

// Run executes the orchestration against the initial state.
//
// Steps are processed in order. For each step:
//  1. If Condition is non-nil and returns false, the step is skipped.
//  2. Tool input is read from state under the step's InputKey (if set).
//  3. The tool is invoked via Agent.InvokeTool (hooks are applied).
//  4. On success, the raw JSON output is stored in state[step.ToolName].
//  5. On failure:
//     - IsInterrupt: execution pauses, result.InterruptedStep is set, the
//     caller should present PendingReview for user confirmation, then
//     resume via Run() with the returned PartialState as initial state.
//     - Optional steps: error is recorded in result.StepErrors, execution
//     continues.
//     - Required steps: execution aborts, result.Success = false.
func (e *OrchestrationExecutor) Run(ctx context.Context, m *OrchestrationManifest, state OrchestrationState) (*OrchestrationResult, error) {
	result := &OrchestrationResult{
		OrchestrationID: m.ID,
		StepResults:     make(map[string]any),
		StepErrors:      make(map[string]string),
	}

	if state == nil {
		state = OrchestrationState{}
	}

	var summaryLines []string
	summaryLines = append(summaryLines, fmt.Sprintf("## %s\n", m.Name))

	for _, step := range m.Steps {
		// Condition check: skip if predicate says false.
		if step.Condition != nil && !step.Condition(state) {
			result.StepsSkipped++
			summaryLines = append(summaryLines,
				fmt.Sprintf("- ⏭️ **%s**: 已跳过（条件不满足）", step.Description))
			continue
		}

		// Build tool arguments from state.
		args := json.RawMessage("{}")
		if step.InputKey != "" {
			if raw, ok := state[step.InputKey]; ok {
				var err error
				args, err = json.Marshal(raw)
				if err != nil {
					slog.Warn("orchestration: failed to marshal input",
						"tool", step.ToolName, "error", err)
					args = json.RawMessage("{}")
				}
			}
		}

		// Invoke the tool through the Agent (preserves hook pipeline).
		rawOutput, err := e.agent.InvokeTool(ctx, step.ToolName, args)
		if err != nil {
			if IsInterrupt(err) {
				// Interrupt: pause execution for user confirmation.
				// Store partial state so the caller can resume later.
				result.InterruptedStep = step.ToolName
				result.PartialState = state
				result.PendingReview = fmt.Sprintf("**%s** 已完成，请确认结果后继续。\n\n%s",
					step.Description, rawOutput)

				result.StepsCompleted++
				result.StepResults[step.ToolName] = nil
				result.Success = true // not a failure, just paused

				summaryLines = append(summaryLines,
					fmt.Sprintf("- ⏸️ **%s**: 已完成，等待用户确认", step.Description))
				result.Summary = strings.Join(summaryLines, "\n")
				return result, nil // no error — caller should check InterruptedStep
			}

			if step.Optional {
				// Optional steps: record error, continue.
				result.StepErrors[step.ToolName] = err.Error()
				state[step.ToolName] = nil
				summaryLines = append(summaryLines,
					fmt.Sprintf("- ⚠️ **%s**: 失败（可选步骤，继续执行）— %v", step.Description, err))
				continue
			}
			// Required step failure: abort.
			result.Success = false
			result.StepErrors[step.ToolName] = err.Error()
			summaryLines = append(summaryLines,
				fmt.Sprintf("- ❌ **%s**: 失败 — %v", step.Description, err))
			result.Summary = strings.Join(summaryLines, "\n")
			return result, fmt.Errorf("orchestration %q step %q: %w", m.ID, step.ToolName, err)
		}

		// Store output in state for downstream steps.
		// Try to parse as JSON, fall back to raw string.
		var parsed any
		if json.Unmarshal([]byte(rawOutput), &parsed) == nil {
			state[step.ToolName] = parsed
		} else {
			state[step.ToolName] = rawOutput
		}

		result.StepsCompleted++
		result.StepResults[step.ToolName] = state[step.ToolName]
		result.FinalOutput = state[step.ToolName]

		summaryLines = append(summaryLines,
			fmt.Sprintf("- ✅ **%s**: 已完成", step.Description))
	}

	result.Success = true
	result.Summary = strings.Join(summaryLines, "\n")
	return result, nil
}
