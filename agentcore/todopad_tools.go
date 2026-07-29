package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	TodoSetupTool = "todo_setup"
	TodoTickTool  = "todo_tick"
	TodoListTool  = "todo_list"
)

func newTodoSetupTool(pad *TodoPad) *Tool {
	return &Tool{
		Name:        TodoSetupTool,
		Description: "Set up an ordered todo checklist for the current task. Call this at the start of a multi-step task to list every step that needs to be done, in order.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"steps": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Ordered list of step descriptions, e.g. [\"Search prior art\", \"Analyze claims\", \"Write report\"]",
				},
			},
			"required":             []string{"steps"},
			"additionalProperties": false,
		},
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Steps []string `json:"steps"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if len(p.Steps) == 0 {
				return nil, fmt.Errorf("'steps' must be a non-empty array of strings")
			}
			pad.Setup(p.Steps)
			return fmt.Sprintf("Todo checklist set up with %d steps:\n%s", len(p.Steps), formatTodoSteps(p.Steps)), nil
		},
	}
}

func newTodoTickTool(pad *TodoPad) *Tool {
	return &Tool{
		Name:        TodoTickTool,
		Description: "Mark a todo step as completed. Call this after finishing each step from the todo list.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"step": map[string]any{
					"type":        "integer",
					"description": "The 1-based index of the step to mark as completed",
				},
			},
			"required":             []string{"step"},
			"additionalProperties": false,
		},
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Step int `json:"step"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if p.Step < 1 {
				return nil, fmt.Errorf("'step' must be 1-based index, got %d", p.Step)
			}
			idx := p.Step - 1
			steps := pad.List()
			if idx >= len(steps) {
				return nil, fmt.Errorf("step %d is out of range (total steps: %d)", p.Step, len(steps))
			}
			pad.Tick(idx)
			completed := pad.Completed()
			return fmt.Sprintf("Step %d (%s) marked as completed. Progress: %d/%d", p.Step, steps[idx].Text, completed, len(steps)), nil
		},
	}
}

func newTodoListTool(pad *TodoPad) *Tool {
	return &Tool{
		Name:        TodoListTool,
		Description: "View the current todo checklist and progress. Shows which steps are done and which remain.",
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		ReadOnly: true,
		Func: func(_ context.Context, _ json.RawMessage) (any, error) {
			if !pad.IsActive() {
				return "No active todo checklist. Use todo_setup to create one.", nil
			}
			steps := pad.List()
			completed := pad.Completed()
			var done, pending []string
			for _, s := range steps {
				if s.Status == TodoStatusCompleted {
					done = append(done, s.Text)
				} else {
					pending = append(pending, s.Text)
				}
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Progress: %d/%d\n\n", completed, len(steps))
			if len(done) > 0 {
				b.WriteString("Completed:\n")
				for _, d := range done {
					fmt.Fprintf(&b, "  - %s\n", d)
				}
				b.WriteString("\n")
			}
			if len(pending) > 0 {
				b.WriteString("Pending:\n")
				for _, p := range pending {
					fmt.Fprintf(&b, "  - %s\n", p)
				}
			}
			return b.String(), nil
		},
	}
}

func formatTodoSteps(steps []string) string {
	var b strings.Builder
	for _, s := range steps {
		fmt.Fprintf(&b, "  [ ] %s\n", s)
	}
	return b.String()
}
