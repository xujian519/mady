package evaluate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ToolAccuracy evaluates how faithfully an agent's tool call sequence matches
// the expected tool calls. It measures three dimensions:
//
//  1. Tool selection: did the agent invoke the correct tool names?
//  2. Argument accuracy: did the agent pass the correct arguments?
//  3. Ordering: were tools called in the expected sequence?
//
// The prediction and reference are both JSON arrays of tool call objects:
//
//	[
//	  {"name": "search_patents", "arguments": {"query": "AI", "limit": 10}},
//	  {"name": "read_file", "arguments": {"path": "/tmp/doc.txt"}}
//	]
//
// Each dimension contributes equally to the final score, so a model that picks
// the right tools with wrong arguments scores ~0.33, and one that gets both
// tool + argument right but wrong order scores ~0.67.
type ToolAccuracy struct {
	// IgnoreOrder, when true, skips the ordering dimension and scores on tool +
	// argument accuracy alone. Useful for evaluating agents where call
	// sequencing is not part of the rubric.
	IgnoreOrder bool

	// ArgStrict, when true, requires exact match on all argument values.
	// When false (default), missing arguments and extra arguments are tolerated
	// as long as every expected argument (key+value) appears in the prediction.
	ArgStrict bool
}

// ToolCallJSON represents a single tool call in the prediction/reference JSON.
type ToolCallJSON struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func (m ToolAccuracy) Name() string { return "tool_accuracy" }

func (m ToolAccuracy) Compute(prediction, reference string) float64 {
	predCalls := parseToolCalls(prediction)
	refCalls := parseToolCalls(reference)

	if len(refCalls) == 0 && len(predCalls) == 0 {
		return 1 // neither side has tool calls → trivially correct
	}
	if len(refCalls) == 0 {
		return 0 // prediction added tools where none expected
	}
	if len(predCalls) == 0 {
		return 0 // expected tools but prediction had none
	}

	// Dimension 1: tool selection accuracy
	toolScore := computeToolSelection(predCalls, refCalls)

	// Dimension 2: argument accuracy
	argScore := computeArgAccuracy(predCalls, refCalls, m.ArgStrict)

	// Dimension 3: ordering accuracy (optional)
	if m.IgnoreOrder {
		return 0.5*toolScore + 0.5*argScore
	}
	orderScore := computeOrderScore(predCalls, refCalls)
	return (toolScore + argScore + orderScore) / 3.0
}

// computeToolSelection measures what fraction of expected tool names appear in
// the prediction (recall-based: missing = bad, extra = neutral).
func computeToolSelection(predCalls, refCalls []ToolCallJSON) float64 {
	predNames := make(map[string]int, len(predCalls))
	for _, c := range predCalls {
		predNames[c.Name]++
	}

	hit := 0
	for _, ref := range refCalls {
		if predNames[ref.Name] > 0 {
			hit++
			predNames[ref.Name]--
		}
	}
	return float64(hit) / float64(len(refCalls))
}

// computeArgAccuracy measures argument correctness across all matched tool
// calls. For each expected tool call, it finds the best-matching prediction
// tool call and checks argument overlap.
func computeArgAccuracy(predCalls, refCalls []ToolCallJSON, strict bool) float64 {
	// Build a pool of available prediction calls (mutable copy).
	available := make([]ToolCallJSON, len(predCalls))
	copy(available, predCalls)

	var totalScore float64
	matched := 0
	for _, ref := range refCalls {
		bestIdx := -1
		bestScore := -1.0
		for i, pred := range available {
			if pred.Name != ref.Name {
				continue
			}
			s := scoreArgs(pred.Arguments, ref.Arguments, strict)
			if s > bestScore {
				bestScore = s
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			totalScore += bestScore
			matched++
			// Remove matched call from pool (each pred call used at most once).
			available = append(available[:bestIdx], available[bestIdx+1:]...)
		}
	}
	if matched == 0 {
		return 0
	}
	return totalScore / float64(matched)
}

// scoreArgs returns the fraction of expected argument key-value pairs present
// in the prediction arguments.
func scoreArgs(pred, ref map[string]any, strict bool) float64 {
	if len(ref) == 0 && len(pred) == 0 {
		return 1
	}
	if len(ref) == 0 {
		return 1 // no ref args to match → always correct
	}

	hit := 0
	for rk, rv := range ref {
		pv, ok := pred[rk]
		if !ok {
			continue
		}
		if valuesMatch(rv, pv, strict) {
			hit++
		}
	}
	return float64(hit) / float64(len(ref))
}

// valuesMatch compares two argument values. In non-strict mode, numeric values
// are compared by their JSON-marshaled representation (so 10 == 10.0). In
// strict mode all values must be deep equal.
func valuesMatch(expected, actual any, strict bool) bool {
	if strict {
		// Include type in comparison so e.g. float64(10) != string("10").
		return fmt.Sprintf("%T(%v)", expected, expected) == fmt.Sprintf("%T(%v)", actual, actual)
	}
	// Non-strict: compare JSON representations for tolerance.
	ej, _ := json.Marshal(expected)
	aj, _ := json.Marshal(actual)
	return bytes.Equal(ej, aj)
}

// computeOrderScore measures how well the prediction call sequence matches the
// reference call sequence using relative ordering instead of exact positions.
// Returns 1 when all predicted tool calls respect the reference order.
func computeOrderScore(predCalls, refCalls []ToolCallJSON) float64 {
	// Build name→position for reference calls. When names repeat, use the
	// first occurrence's position.
	refPos := make(map[string]int, len(refCalls))
	for i, c := range refCalls {
		if _, ok := refPos[c.Name]; !ok {
			refPos[c.Name] = i
		}
	}

	// Check that the prediction's call names are in non-decreasing ref position.
	inOrder := 0
	total := 0
	prevPos := -1
	for _, c := range predCalls {
		pos, ok := refPos[c.Name]
		if !ok {
			continue
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

// parseToolCalls attempts to parse a JSON string as an array of tool calls.
// If parsing fails, it falls back to extracting tool call patterns from text.
func parseToolCalls(s string) []ToolCallJSON {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// Try direct JSON array parse.
	var calls []ToolCallJSON
	if err := json.Unmarshal([]byte(s), &calls); err == nil {
		// Only accept if we actually got results or the input was a valid array.
		return calls
	}

	// Try single tool call object.
	var single ToolCallJSON
	if err := json.Unmarshal([]byte(s), &single); err == nil && single.Name != "" {
		return []ToolCallJSON{single}
	}

	// Fallback: try to find JSON arrays embedded in text (common in LLM output).
	if idx := strings.Index(s, "["); idx >= 0 {
		end := strings.LastIndex(s, "]")
		if end > idx {
			sub := s[idx : end+1]
			if err := json.Unmarshal([]byte(sub), &calls); err == nil {
				return calls
			}
		}
	}

	return nil
}

// NormalizeToolCalls sorts a JSON array of tool calls by name then arguments.
// This helps in non-order-sensitive comparisons for debugging and display.
func NormalizeToolCalls(s string) string {
	calls := parseToolCalls(s)
	sort.Slice(calls, func(i, j int) bool {
		if calls[i].Name != calls[j].Name {
			return calls[i].Name < calls[j].Name
		}
		return fmt.Sprintf("%v", calls[i].Arguments) < fmt.Sprintf("%v", calls[j].Arguments)
	})
	data, _ := json.Marshal(calls)
	return string(data)
}
