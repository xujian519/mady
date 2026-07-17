package evaluate

import (
	"testing"
)

// ============================================================================
// Reflection tests
// ============================================================================

func TestReflection_Name(t *testing.T) {
	m := Reflection{}
	if got := m.Name(); got != "reflection" {
		t.Errorf("Name() = %q, want %q", got, "reflection")
	}
}

func TestReflection_NoJudgeFallback(t *testing.T) {
	// Without a judge provider, Reflection falls back to F1Score.
	m := Reflection{}
	pred := "[INITIAL]\nbad answer\n[REFLECTED]\ngood answer with the right content about patents"
	ref := "good answer with the right content about patents"
	score := m.Compute(pred, ref)
	if score < 0 || score > 1 {
		t.Errorf("score out of range: %v", score)
	}
	// Reflected answer has some F1 overlap with reference, so score > 0.
	if score <= 0 {
		t.Errorf("expected positive score with F1 fallback, got %v", score)
	}
}

func TestReflection_NoDelimiter(t *testing.T) {
	m := Reflection{}
	// No delimiter → entire prediction treated as reflected, initial = empty.
	pred := "this is the final answer"
	ref := "this is the final answer"
	score := m.Compute(pred, ref)
	// Initial is "" → initialScore=0, reflectedScore=1 (F1 exact match)
	// improvement = (1-0)/1 = 1, reflectedQuality = 1
	// Final = (1+1)/2 = 1
	if score != 1 {
		t.Errorf("no delimiter: got %v, want 1", score)
	}
}

func TestReflection_Improvement(t *testing.T) {
	m := Reflection{}
	pred := "[INITIAL]\nwrong answer completely\n[REFLECTED]\ncorrect answer that matches reference"
	ref := "correct answer that matches reference"
	score := m.Compute(pred, ref)
	// Initial: F1 ~0, reflected: F1 ~1 → improvement ~1, reflectedQuality ~1
	// Score should be close to 1.
	if score < 0.9 {
		t.Errorf("expected high score for large improvement, got %v", score)
	}
}

func TestReflection_NoImprovement(t *testing.T) {
	m := Reflection{}
	pred := "[INITIAL]\nwrong answer\n[REFLECTED]\nstill wrong answer"
	ref := "correct answer that matches reference"
	score := m.Compute(pred, ref)
	// Both initial and reflected are far from reference → low score.
	if score > 0.5 {
		t.Errorf("expected low score for no improvement, got %v", score)
	}
}

func TestSplitReflection(t *testing.T) {
	tests := []struct {
		input         string
		wantInitial   string
		wantReflected string
	}{
		{
			input:         "[INITIAL]\nfirst attempt\n[REFLECTED]\nsecond attempt",
			wantInitial:   "first attempt",
			wantReflected: "second attempt",
		},
		{
			input:         "just a plain answer",
			wantInitial:   "",
			wantReflected: "just a plain answer",
		},
		{
			input:         "[INITIAL]\nno reflected section",
			wantInitial:   "no reflected section",
			wantReflected: "",
		},
	}
	for _, tt := range tests {
		initial, reflected := splitReflection(tt.input)
		if initial != tt.wantInitial {
			t.Errorf("splitReflection(%q) initial = %q, want %q", tt.input, initial, tt.wantInitial)
		}
		if reflected != tt.wantReflected {
			t.Errorf("splitReflection(%q) reflected = %q, want %q", tt.input, reflected, tt.wantReflected)
		}
	}
}

// ============================================================================
// RubricJudge tests
// ============================================================================

func TestRubricJudge_Name(t *testing.T) {
	m := RubricJudge{}
	if got := m.Name(); got != "rubric_judge" {
		t.Errorf("Name() = %q, want %q", got, "rubric_judge")
	}
}

func TestRubricJudge_NoJudge(t *testing.T) {
	m := RubricJudge{}
	score := m.Compute("any prediction", "any reference")
	if score != 0 {
		t.Errorf("no judge: got %v, want 0", score)
	}
}

func TestDefaultRubricCriteria(t *testing.T) {
	criteria := DefaultRubricCriteria()
	if len(criteria) != 3 {
		t.Fatalf("expected 3 criteria, got %d", len(criteria))
	}
	expected := []string{"conclusion", "reasoning", "citation"}
	for i, c := range criteria {
		if c.Name != expected[i] {
			t.Errorf("criteria[%d].Name = %q, want %q", i, c.Name, expected[i])
		}
		if c.Weight <= 0 {
			t.Errorf("criteria[%d].Weight = %v, want > 0", i, c.Weight)
		}
	}
}

func TestBuildRubricJSONTemplate(t *testing.T) {
	keys := []string{"a", "b"}
	tmpl := buildRubricJSONTemplate(keys)
	if tmpl != `{"a": 0.8, "b": 0.8, "overall": 0.8}` {
		t.Errorf("template = %q", tmpl)
	}
}

func TestParseRubricScore_ValidJSON(t *testing.T) {
	m := RubricJudge{}
	criteria := []RubricCriterion{
		{Name: "accuracy", Weight: 1},
		{Name: "completeness", Weight: 1},
	}
	content := `{"accuracy": 0.8, "completeness": 0.6, "overall": 0.7}`
	score := m.parseRubricScore(content, criteria, 2.0)
	// (0.8*1 + 0.6*1) / 2 = 0.7
	if score != 0.7 {
		t.Errorf("got %v, want 0.7", score)
	}
}

func TestParseRubricScore_WithOverallFallback(t *testing.T) {
	m := RubricJudge{}
	// When effective weight is 0 (no criteria matched), fall back to overall.
	content := `{"overall": 0.85}`
	criteria := []RubricCriterion{
		{Name: "nonexistent", Weight: 1},
	}
	score := m.parseRubricScore(content, criteria, 1.0)
	if score != 0.85 {
		t.Errorf("got %v, want 0.85", score)
	}
}

func TestParseRubricScore_CodeFence(t *testing.T) {
	m := RubricJudge{}
	criteria := []RubricCriterion{
		{Name: "quality", Weight: 1},
	}
	content := "```json\n{\"quality\": 0.9}\n```"
	score := m.parseRubricScore(content, criteria, 1.0)
	if score != 0.9 {
		t.Errorf("got %v, want 0.9", score)
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input interface{}
		want  float64
	}{
		{float64(0.5), 0.5},
		{int(42), 42.0},
		{int64(100), 100.0},
		{"0.75", 0.75},
	}
	for _, tt := range tests {
		got := toFloat64(tt.input)
		if got != tt.want {
			t.Errorf("toFloat64(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestExtractNumericScore(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"The score is 0.85", 0.85},
		{"8/10", 0.8},
	}
	for _, tt := range tests {
		got := extractNumericScore(tt.input)
		if got != tt.want {
			t.Errorf("extractNumericScore(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
