package evaluate

import (
	"context"
)

// TestCase is a single evaluation example: an input prompt, the expected
// reference answer, and any required citations that must appear in the output.
type TestCase struct {
	ID                string
	Domain            string // patent | legal | general (用于按领域过滤 Benchmark)
	Input             string
	Expected          string
	RequiredCitations []string
}

// CaseResult holds the scored output for one TestCase.
type CaseResult struct {
	CaseID     string             `json:"case_id"`
	Passed     bool               `json:"passed"`
	Scores     map[string]float64 `json:"scores,omitempty"`
	Average    float64            `json:"average"`
	Prediction string             `json:"prediction,omitempty"`
}

// BatchReport aggregates results across many test cases.
type BatchReport struct {
	Results         []CaseResult       `json:"results,omitempty"`
	TotalCases      int                `json:"total_cases"`
	PassedCases     int                `json:"passed_cases"`
	AggregateScores map[string]float64 `json:"aggregate_scores,omitempty"`
	PassRate        float64            `json:"pass_rate"`
}

// RunFunc produces a prediction for a given input. In practice this calls the
// agent or workflow under test.
type RunFunc func(ctx context.Context, input string) (string, error)

// Evaluator runs a fixed set of metrics over test cases.
type Evaluator struct {
	metrics   []Metric
	threshold float64 // minimum average score to pass a case (default 0.7)
}

// NewEvaluator creates an Evaluator with the given metrics and a default pass
// threshold of 0.7.
func NewEvaluator(metrics ...Metric) *Evaluator {
	return &Evaluator{metrics: metrics, threshold: 0.7}
}

// WithThreshold sets the minimum average score required for a case to pass.
func (e *Evaluator) WithThreshold(t float64) *Evaluator {
	e.threshold = t
	return e
}

// Threshold returns the current pass threshold.
func (e *Evaluator) Threshold() float64 { return e.threshold }

// Metrics returns the metric set.
func (e *Evaluator) Metrics() []Metric { return e.metrics }

// Evaluate scores a single prediction against a reference.
func (e *Evaluator) Evaluate(prediction, reference string, requiredCitations []string) CaseResult {
	scores := make(map[string]float64, len(e.metrics))
	var sum float64
	for _, m := range e.metrics {
		if cam, ok := m.(CitationAwareMetric); ok && len(requiredCitations) > 0 {
			m = cam.WithCitations(requiredCitations)
		}
		score := m.Compute(prediction, reference)
		scores[m.Name()] = score
		sum += score
	}
	avg := 0.0
	if len(e.metrics) > 0 {
		avg = sum / float64(len(e.metrics))
	}
	return CaseResult{
		Scores:     scores,
		Average:    avg,
		Passed:     avg >= e.threshold,
		Prediction: prediction,
	}
}

// EvaluateBatch runs the metric set over a collection of test cases using the
// provided RunFunc to obtain predictions. A test case fails if RunFunc returns
// an error or if its average score is below the threshold.
func (e *Evaluator) EvaluateBatch(ctx context.Context, cases []TestCase, run RunFunc) (*BatchReport, error) {
	report := &BatchReport{
		AggregateScores: make(map[string]float64),
	}
	metricSums := make(map[string]float64)

	for _, tc := range cases {
		prediction, err := run(ctx, tc.Input)
		if err != nil {
			report.Results = append(report.Results, CaseResult{
				CaseID:  tc.ID,
				Passed:  false,
				Scores:  map[string]float64{},
				Average: 0,
			})
			continue
		}
		result := e.Evaluate(prediction, tc.Expected, tc.RequiredCitations)
		result.CaseID = tc.ID
		report.Results = append(report.Results, result)
		if result.Passed {
			report.PassedCases++
		}
		for name, score := range result.Scores {
			metricSums[name] += score
		}
	}

	report.TotalCases = len(cases)
	if report.TotalCases > 0 {
		report.PassRate = float64(report.PassedCases) / float64(report.TotalCases)
		for name, sum := range metricSums {
			report.AggregateScores[name] = sum / float64(report.TotalCases)
		}
	}
	return report, nil
}

// EvaluateStatic scores pre-recorded predictions without calling RunFunc.
// This is useful for evaluating logged outputs or golden-file regressions.
func (e *Evaluator) EvaluateStatic(cases []TestCase, predictions map[string]string) *BatchReport {
	report := &BatchReport{
		AggregateScores: make(map[string]float64),
	}
	metricSums := make(map[string]float64)

	for _, tc := range cases {
		prediction := predictions[tc.ID]
		result := e.Evaluate(prediction, tc.Expected, tc.RequiredCitations)
		result.CaseID = tc.ID
		report.Results = append(report.Results, result)
		if result.Passed {
			report.PassedCases++
		}
		for name, score := range result.Scores {
			metricSums[name] += score
		}
	}

	report.TotalCases = len(cases)
	if report.TotalCases > 0 {
		report.PassRate = float64(report.PassedCases) / float64(report.TotalCases)
		for name, sum := range metricSums {
			report.AggregateScores[name] = sum / float64(report.TotalCases)
		}
	}
	return report
}
