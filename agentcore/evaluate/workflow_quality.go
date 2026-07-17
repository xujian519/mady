package evaluate

import (
	"encoding/json"
	"strings"
)

// WorkflowQuality evaluates how faithfully an agent's workflow execution trace
// matches the expected workflow plan. It measures:
//
//  1. Step completion: did the agent execute all required steps?
//  2. Step ordering: was the execution order correct for the expected pattern?
//  3. Step correctness: did the agent avoid extraneous or wrong steps?
//
// The prediction and reference are both JSON arrays of workflow step records:
//
//	[
//	  {"step": "search_patents", "status": "completed"},
//	  {"step": "analyze_claims", "status": "completed"},
//	  {"step": "generate_report", "status": "completed"}
//	]
//
// Each dimension contributes equally to the final score. By default the metric
// expects Pipeline-style sequential execution; pass WithParallelPattern() to
// allow concurrent steps.
type WorkflowQuality struct {
	// Pattern describes the expected execution pattern: "sequential" (Pipeline),
	// "parallel" (Parallel), or "any" (Router). Default is "sequential".
	Pattern string
}

// WorkflowStepRecord is a single step in a workflow execution trace.
type WorkflowStepRecord struct {
	Step       string `json:"step"`
	Status     string `json:"status"`
	DurationMs int    `json:"duration_ms,omitempty"`
}

// WorkflowPattern constants.
const (
	WorkflowSequential = "sequential" // Pipeline: steps run in order
	WorkflowParallel   = "parallel"   // Parallel: steps may run concurrently
	WorkflowAny        = "any"        // Router: any order, any subset
)

func (m WorkflowQuality) Name() string { return "workflow_quality" }

func (m WorkflowQuality) Compute(prediction, reference string) float64 {
	predSteps := parseWorkflowSteps(prediction)
	refSteps := parseWorkflowSteps(reference)

	if len(refSteps) == 0 && len(predSteps) == 0 {
		return 1
	}
	if len(refSteps) == 0 {
		return 0 // predicted steps where none expected
	}
	if len(predSteps) == 0 {
		return 0 // expected steps but none predicted
	}

	pattern := m.Pattern
	if pattern == "" {
		pattern = WorkflowSequential
	}

	// Dimension 1: step completion (recall).
	completion := computeStepCompletion(predSteps, refSteps)

	// Dimension 2: step ordering.
	ordering := computeStepOrdering(predSteps, refSteps, pattern)

	// Dimension 3: step precision (avoiding wrong/extra steps).
	precision := computeStepPrecision(predSteps, refSteps)

	return (completion + ordering + precision) / 3.0
}

// computeStepCompletion measures the fraction of expected steps that were
// actually executed (completed or skipped, but not missing).
func computeStepCompletion(predSteps, refSteps []WorkflowStepRecord) float64 {
	predNames := make(map[string]bool, len(predSteps))
	for _, s := range predSteps {
		predNames[s.Step] = true
	}
	hit := 0
	for _, ref := range refSteps {
		if predNames[ref.Step] {
			hit++
		}
	}
	return float64(hit) / float64(len(refSteps))
}

// computeStepOrdering evaluates how well the predicted step order matches the
// expected order. For sequential, the order must match exactly. For parallel
// and any, only the relative ordering of explicitly ordered steps matters.
func computeStepOrdering(predSteps, refSteps []WorkflowStepRecord, pattern string) float64 {
	switch pattern {
	case WorkflowParallel, WorkflowAny:
		return 1 // parallel/any don't constrain order
	}

	// Build position map for reference steps.
	refPos := make(map[string]int, len(refSteps))
	for i, s := range refSteps {
		if _, ok := refPos[s.Step]; !ok {
			refPos[s.Step] = i // use first occurrence
		}
	}

	// Count how many predicted steps respect reference order.
	inOrder := 0
	total := 0
	prevPos := -1
	for _, s := range predSteps {
		pos, ok := refPos[s.Step]
		if !ok {
			continue // unknown step, skip
		}
		total++
		if pos >= prevPos {
			inOrder++
		}
		prevPos = pos
	}
	if total == 0 {
		return 0
	}
	return float64(inOrder) / float64(total)
}

// computeStepPrecision measures the fraction of predicted steps that are valid
// (appear in the reference), penalizing extraneous steps.
func computeStepPrecision(predSteps, refSteps []WorkflowStepRecord) float64 {
	refNames := make(map[string]bool, len(refSteps))
	for _, s := range refSteps {
		refNames[s.Step] = true
	}
	valid := 0
	for _, s := range predSteps {
		if refNames[s.Step] {
			valid++
		}
	}
	return float64(valid) / float64(len(predSteps))
}

// parseWorkflowSteps attempts to parse a string as a JSON array of workflow
// step records. Falls back to line-based parsing for simple step lists.
func parseWorkflowSteps(s string) []WorkflowStepRecord {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// Try JSON array parse.
	var steps []WorkflowStepRecord
	if err := json.Unmarshal([]byte(s), &steps); err == nil {
		return steps
	}

	// Try single object.
	var single WorkflowStepRecord
	if err := json.Unmarshal([]byte(s), &single); err == nil && single.Step != "" {
		return []WorkflowStepRecord{single}
	}

	// Fallback: extract JSON from text.
	if idx := strings.Index(s, "["); idx >= 0 {
		end := strings.LastIndex(s, "]")
		if end > idx {
			if err := json.Unmarshal([]byte(s[idx:end+1]), &steps); err == nil {
				return steps
			}
		}
	}

	// Fallback: line-based simple parsing.
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Try each line as JSON object.
		if err := json.Unmarshal([]byte(line), &single); err == nil && single.Step != "" {
			steps = append(steps, single)
		}
	}
	return steps
}
