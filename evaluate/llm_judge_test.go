package evaluate

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/xujian519/mady/agentcore"
)

func approxEqualFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestParseLLMJudgeScore(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{`{"conclusion": 0.8, "reasoning": 0.6, "citation": 0.7}`, 0.7},
		{`{"conclusion":1,"reasoning":1,"citation":1}`, 1.0},
		{`{"conclusion":0,"reasoning":0,"citation":0}`, 0.0},
		{`{"conclusion": 0.9, "reasoning": 0.5, "citation": 0.3}`, 0.5666666666666667},
		{"0.8", 0.8},
		{"8/10", 0.8},
		{"85%", 0.85},
		{"最终评分为 0.75", 0.75},
		{"8", 0.8},
		{"```json\n{\"conclusion\": 0.7, \"reasoning\": 0.7, \"citation\": 0.7}\n```", 0.7},
	}

	for _, c := range cases {
		got := parseLLMJudgeScore(c.input)
		if !approxEqualFloat(got, c.expected) {
			t.Errorf("parseLLMJudgeScore(%q) = %v, want %v", c.input, got, c.expected)
		}
	}
}

func TestClampScore(t *testing.T) {
	cases := []struct {
		input, expected float64
	}{
		{-0.5, 0},
		{0.5, 0.5},
		{1.5, 1},
	}
	for _, c := range cases {
		if got := clampScore(c.input); got != c.expected {
			t.Errorf("clampScore(%v) = %v, want %v", c.input, got, c.expected)
		}
	}
}

func TestNormalizeScore(t *testing.T) {
	cases := []struct {
		input, expected float64
	}{
		{0.5, 0.5},
		{5, 0.5},
		{50, 0.5},
		{1, 1},
	}
	for _, c := range cases {
		if got := normalizeScore(c.input); got != c.expected {
			t.Errorf("normalizeScore(%v) = %v, want %v", c.input, got, c.expected)
		}
	}
}

func TestTruncateForJudgeUTF8(t *testing.T) {
	// Construct a string with >6000 runes where byte offset 3000 would land
	// in the middle of a multi-byte UTF-8 character.
	prefix := strings.Repeat("中", 999) + "AB"
	long := prefix + strings.Repeat("文", 5001)
	if runeLen(long) <= 6000 {
		t.Fatalf("test string should exceed 6000 runes, got %d", runeLen(long))
	}

	truncated := truncateForJudge(long)
	if !utf8.ValidString(truncated) {
		t.Errorf("truncateForJudge produced invalid UTF-8")
	}
	if !strings.Contains(truncated, prefix) {
		t.Errorf("truncateForJudge should preserve the start of the string")
	}
	if !strings.Contains(truncated, "...[中间内容省略]...") {
		t.Errorf("truncateForJudge should include an omission marker")
	}
}

func TestMedian(t *testing.T) {
	cases := []struct {
		name   string
		scores []float64
		want   float64
	}{
		{"empty", []float64{}, 0},
		{"single", []float64{0.5}, 0.5},
		{"odd", []float64{0.3, 0.9, 0.5}, 0.5},                 // sorted: 0.3,0.5,0.9 → middle
		{"even", []float64{0.1, 0.4, 0.6, 0.9}, 0.6},           // sorted → index len/2=2 → 0.6
		{"outlier_robust", []float64{0.7, 0.7, 0.7, 0.0}, 0.7}, // one parse-failure 0.0 ignored
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := median(c.scores)
			if !approxEqualFloat(got, c.want) {
				// Recompute expected for even case: sorted [0.1,0.4,0.6,0.9], len/2=2, sorted[2]=0.6
				t.Errorf("median(%v) = %v, want %v", c.scores, got, c.want)
			}
		})
	}
}

// stubJudgeProvider returns a scripted sequence of responses to simulate
// multi-sample judging without network calls. It is safe for concurrent use
// because LLMJudge.Compute fans out samples in parallel.
type stubJudgeProvider struct {
	mu        sync.Mutex
	responses []string
	calls     int
}

func (s *stubJudgeProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	s.mu.Lock()
	resp := "0.5"
	if s.calls < len(s.responses) {
		resp = s.responses[s.calls]
	}
	s.calls++
	s.mu.Unlock()
	return &agentcore.ProviderResponse{Content: resp}, nil
}

func (s *stubJudgeProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

func TestLLMJudge_SamplesTakesMedian(t *testing.T) {
	// Three samples: 0.8, 0.2, 0.6 → median 0.6 (robust to the 0.2 outlier).
	p := &stubJudgeProvider{responses: []string{"0.8", "0.2", "0.6"}}
	j := LLMJudge{Judge: p, Model: "stub", Samples: 3}
	score := j.Compute("pred", "ref")
	if !approxEqualFloat(score, 0.6) {
		t.Errorf("3-sample median: got %v, want 0.6 (calls=%d)", score, p.calls)
	}
	if p.calls != 3 {
		t.Errorf("expected 3 judge calls, got %d", p.calls)
	}
}

func TestLLMJudge_SamplesDefaultSingleShot(t *testing.T) {
	// Samples=0 defaults to 1 → single call.
	p := &stubJudgeProvider{responses: []string{"0.7"}}
	j := LLMJudge{Judge: p, Model: "stub"}
	score := j.Compute("pred", "ref")
	if !approxEqualFloat(score, 0.7) {
		t.Errorf("single-shot: got %v, want 0.7", score)
	}
	if p.calls != 1 {
		t.Errorf("expected 1 judge call, got %d", p.calls)
	}
}
