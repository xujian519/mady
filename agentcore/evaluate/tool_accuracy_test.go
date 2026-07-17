package evaluate

import (
	"fmt"
	"testing"
)

func TestToolAccuracy_Name(t *testing.T) {
	m := ToolAccuracy{}
	if got := m.Name(); got != "tool_accuracy" {
		t.Errorf("Name() = %q, want %q", got, "tool_accuracy")
	}
}

func TestToolAccuracy_Empty(t *testing.T) {
	m := ToolAccuracy{}
	// Both empty → score 1.
	if got := m.Compute("", ""); got != 1 {
		t.Errorf("both empty: got %v, want 1", got)
	}
	// Prediction empty, reference has tools → 0.
	if got := m.Compute("", `[{"name":"search","arguments":{}}]`); got != 0 {
		t.Errorf("pred empty: got %v, want 0", got)
	}
	// Prediction has tools, reference empty → 0.
	if got := m.Compute(`[{"name":"search","arguments":{}}]`, ""); got != 0 {
		t.Errorf("ref empty: got %v, want 0", got)
	}
}

func TestToolAccuracy_ExactMatch(t *testing.T) {
	m := ToolAccuracy{}
	pred := `[{"name":"search_patents","arguments":{"query":"AI","limit":10}}]`
	ref := `[{"name":"search_patents","arguments":{"query":"AI","limit":10}}]`
	score := m.Compute(pred, ref)
	if score != 1 {
		t.Errorf("exact match: got %v, want 1", score)
	}
}

func TestToolAccuracy_WrongName(t *testing.T) {
	m := ToolAccuracy{}
	pred := `[{"name":"wrong_tool","arguments":{"query":"AI"}}]`
	ref := `[{"name":"search_patents","arguments":{"query":"AI"}}]`
	score := m.Compute(pred, ref)
	// Tool selection: 0/1=0, arg: 0/1=0 (name mismatch → no match),
	// order: pred tool not in ref pos → skip.
	want := 0.0
	if score != want {
		t.Errorf("wrong name: got %v, want %v", score, want)
	}
}

func TestToolAccuracy_PartialArgs(t *testing.T) {
	m := ToolAccuracy{}
	// Prediction has correct tool but partial arguments.
	pred := `[{"name":"search_patents","arguments":{"query":"AI"}}]`
	ref := `[{"name":"search_patents","arguments":{"query":"AI","limit":10}}]`
	score := m.Compute(pred, ref)
	// tool: 1/1=1, arg: 1/2=0.5, order: 1/1=1 → (1+0.5+1)/3 = 0.833...
	want := (1.0 + 0.5 + 1.0) / 3.0
	if !approxEq(score, want, 0.01) {
		t.Errorf("partial args: got %v, want ~%v", score, want)
	}
}

func TestToolAccuracy_MultipleTools(t *testing.T) {
	m := ToolAccuracy{}
	pred := `[
		{"name":"search_patents","arguments":{"query":"AI"}},
		{"name":"read_document","arguments":{"path":"doc.txt"}}
	]`
	ref := `[
		{"name":"search_patents","arguments":{"query":"AI"}},
		{"name":"read_document","arguments":{"path":"doc.txt"}}
	]`
	score := m.Compute(pred, ref)
	if score != 1 {
		t.Errorf("multiple tools exact: got %v, want 1", score)
	}
}

func TestToolAccuracy_WrongOrder(t *testing.T) {
	m := ToolAccuracy{}
	pred := `[
		{"name":"read_document","arguments":{"path":"doc.txt"}},
		{"name":"search_patents","arguments":{"query":"AI"}}
	]`
	ref := `[
		{"name":"search_patents","arguments":{"query":"AI"}},
		{"name":"read_document","arguments":{"path":"doc.txt"}}
	]`
	score := m.Compute(pred, ref)
	// tool: 2/2=1, arg: 2/2=1, order: read_document at pred[1] has refPos=1 >= prevPos=0,
	// but read_document at pred[0] has refPos=1 < prevPos=-1 → 1/2? No let me recalculate.
	// pred[0]=read_document refPos[read_document]=1, prevPos=-1, 1>=-1 → ok (inOrder++)
	// pred[1]=search_patents refPos[search_patents]=0, prevPos=1, 0<1 → not ok
	// total=2, inOrder=1 → 0.5
	// (1+1+0.5)/3 = 2.5/3 = 0.833...
	want := (1.0 + 1.0 + 0.5) / 3.0
	if !approxEq(score, want, 0.01) {
		t.Errorf("wrong order: got %v, want ~%v", score, want)
	}
}

func TestToolAccuracy_IgnoreOrder(t *testing.T) {
	m := ToolAccuracy{IgnoreOrder: true}
	pred := `[
		{"name":"read_document","arguments":{"path":"doc.txt"}},
		{"name":"search_patents","arguments":{"query":"AI"}}
	]`
	ref := `[
		{"name":"search_patents","arguments":{"query":"AI"}},
		{"name":"read_document","arguments":{"path":"doc.txt"}}
	]`
	score := m.Compute(pred, ref)
	// Without order dimension: 0.5*1 + 0.5*1 = 1
	if score != 1 {
		t.Errorf("ignore order: got %v, want 1", score)
	}
}

func TestToolAccuracy_EmbeddedJSON(t *testing.T) {
	m := ToolAccuracy{}
	// Tool call JSON embedded in conversational text.
	pred := `I will search for patents. [{"name":"search_patents","arguments":{"query":"AI"}}]`
	ref := `[{"name":"search_patents","arguments":{"query":"AI"}}]`
	score := m.Compute(pred, ref)
	if score != 1 {
		t.Errorf("embedded JSON: got %v, want 1", score)
	}
}

func TestToolAccuracy_ArgStrict(t *testing.T) {
	m := ToolAccuracy{ArgStrict: true}
	pred := `[{"name":"search_patents","arguments":{"query":"AI","limit":10}}]`
	ref := `[{"name":"search_patents","arguments":{"query":"AI","limit":"10"}}]`
	score := m.Compute(pred, ref)
	// Strict mode: number vs string differ → args don't fully match.
	if score >= 1 {
		t.Errorf("arg strict: got %v, want <1 (number vs string diff)", score)
	}
}

func TestNormalizeToolCalls(t *testing.T) {
	s := `[{"name":"b","arguments":{"z":"3"}},{"name":"a","arguments":{"x":"1"}}]`
	got := NormalizeToolCalls(s)
	want := `[{"name":"a","arguments":{"x":"1"}},{"name":"b","arguments":{"z":"3"}}]`
	if got != want {
		t.Errorf("NormalizeToolCalls:\ngot  %s\nwant %s", got, want)
	}
}

func TestParseToolCalls_SingleObject(t *testing.T) {
	calls := parseToolCalls(`{"name":"test","arguments":{"key":"val"}}`)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "test" {
		t.Errorf("name = %q, want %q", calls[0].Name, "test")
	}
}

func ExampleToolAccuracy() {
	m := ToolAccuracy{}
	pred := `[{"name":"search_patents","arguments":{"query":"machine learning"}}]`
	ref := `[{"name":"search_patents","arguments":{"query":"machine learning","limit":5}}]`
	score := m.Compute(pred, ref)
	fmt.Printf("Tool accuracy: %.2f\n", score)
	// Output: Tool accuracy: 0.83
}

// approxEq checks if a and b are within tolerance.
func approxEq(a, b, tol float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tol
}
