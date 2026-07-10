package evaluate

import (
	"context"
	"strings"
	"testing"
)

func TestEvaluateRAG(t *testing.T) {
	retrieved := []RetrievedDoc{
		{ID: "doc1", Score: 0.9},
		{ID: "doc2", Score: 0.8},
		{ID: "doc3", Score: 0.7},
		{ID: "doc4", Score: 0.6},
	}
	relevant := []string{"doc1", "doc3", "doc9"}

	ev := EvaluateRAG(retrieved, relevant, 4)

	// Precision@4: 2 hits / 4 retrieved = 0.5
	if ev.PrecisionAtK != 0.5 {
		t.Errorf("Precision@4 = %.3f want 0.5", ev.PrecisionAtK)
	}
	// Recall@4: 2 hits / 3 relevant = 0.667
	if !approxEqual(ev.RecallAtK, 2.0/3.0) {
		t.Errorf("Recall@4 = %.3f want %.3f", ev.RecallAtK, 2.0/3.0)
	}
	// MRR: first relevant at rank 1 → 1.0
	if ev.MRR != 1.0 {
		t.Errorf("MRR = %.3f want 1.0", ev.MRR)
	}
	// Hit: yes
	if !ev.HitAtK {
		t.Error("should have hit")
	}
	// NDCG: doc1 at rank1, doc3 at rank3
	// DCG = 1/log2(2) + 1/log2(4) = 1 + 0.5 = 1.5
	// IDCG (3 relevant in top 4) = 1/log2(2)+1/log2(3)+1/log2(4) = 1+0.631+0.5 = 2.131
	// NDCG = 1.5/2.131 = 0.704
	if !approxEqual(ev.NDCG, 1.5/2.131) {
		t.Errorf("NDCG = %.3f want %.3f", ev.NDCG, 1.5/2.131)
	}
}

func TestEvaluateRAG_NoHits(t *testing.T) {
	retrieved := []RetrievedDoc{{ID: "doc1"}, {ID: "doc2"}}
	relevant := []string{"doc9"}
	ev := EvaluateRAG(retrieved, relevant, 2)
	if ev.HitAtK {
		t.Error("should not hit")
	}
	if ev.MRR != 0 {
		t.Errorf("MRR = %.3f want 0", ev.MRR)
	}
	if ev.NDCG != 0 {
		t.Errorf("NDCG = %.3f want 0", ev.NDCG)
	}
}

func TestEvaluateRAG_Empty(t *testing.T) {
	ev := EvaluateRAG(nil, nil, 5)
	if ev.HitAtK {
		t.Error("empty should not hit")
	}
}

func TestEvaluateRAGBatch(t *testing.T) {
	set := [][]RetrievedDoc{
		{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		{{ID: "x"}, {ID: "y"}},
	}
	relevant := [][]string{
		{"a", "c"},
		{"z"},
	}
	result := EvaluateRAGBatch(set, relevant, 3)
	if result.Queries != 2 {
		t.Errorf("queries = %d want 2", result.Queries)
	}
	// Q1: precision=2/3, recall=2/2=1, MRR=1, hit=yes
	// Q2: precision=0, recall=0, MRR=0, hit=no
	if !approxEqual(result.MeanPrecision, (2.0/3.0+0)/2) {
		t.Errorf("mean precision = %.3f", result.MeanPrecision)
	}
	if !approxEqual(result.HitRate, 0.5) {
		t.Errorf("hit rate = %.3f want 0.5", result.HitRate)
	}
}

func TestEvaluatorEvaluate(t *testing.T) {
	e := NewEvaluator(ExactMatch{}, F1Score{})
	result := e.Evaluate("hello", "hello", nil)
	if !result.Passed {
		t.Error("perfect match should pass")
	}
	if result.Average != 1.0 {
		t.Errorf("average = %.3f want 1.0", result.Average)
	}
}

func TestEvaluatorEvaluateBatch(t *testing.T) {
	e := NewEvaluator(ExactMatch{}).WithThreshold(0.5)
	cases := []TestCase{
		{ID: "t1", Input: "ping", Expected: "pong"},
		{ID: "t2", Input: "hello", Expected: "hello"},
	}
	run := func(_ context.Context, input string) (string, error) {
		if input == "hello" {
			return "hello", nil
		}
		return "wrong", nil
	}
	report, err := e.EvaluateBatch(context.Background(), cases, run)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalCases != 2 {
		t.Errorf("total = %d want 2", report.TotalCases)
	}
	if report.PassedCases != 1 {
		t.Errorf("passed = %d want 1", report.PassedCases)
	}
	if !approxEqual(report.PassRate, 0.5) {
		t.Errorf("pass rate = %.3f want 0.5", report.PassRate)
	}
}

func TestEvaluatorEvaluateStatic(t *testing.T) {
	e := NewEvaluator(ExactMatch{})
	cases := []TestCase{
		{ID: "t1", Expected: "yes"},
		{ID: "t2", Expected: "no"},
	}
	predictions := map[string]string{"t1": "yes", "t2": "wrong"}
	report := e.EvaluateStatic(cases, predictions)
	if report.PassedCases != 1 {
		t.Errorf("passed = %d want 1", report.PassedCases)
	}
}

func TestEvaluatorEvaluateBatch_Error(t *testing.T) {
	e := NewEvaluator(ExactMatch{})
	cases := []TestCase{
		{ID: "t1", Input: "x", Expected: "y"},
	}
	run := func(_ context.Context, _ string) (string, error) {
		return "", context.DeadlineExceeded
	}
	report, err := e.EvaluateBatch(context.Background(), cases, run)
	if err != nil {
		t.Fatal(err)
	}
	if report.PassedCases != 0 {
		t.Error("error case should not pass")
	}
}

func TestEvaluatorWithCitations(t *testing.T) {
	e := NewEvaluator(CitationCompleteness{})
	result := e.Evaluate("引用CN123", "", []string{"CN123", "CN456"})
	if !approxEqual(result.Scores["citation_completeness"], 0.5) {
		t.Errorf("citation score = %.3f want 0.5", result.Scores["citation_completeness"])
	}
}

func TestFormatReport(t *testing.T) {
	e := NewEvaluator(ExactMatch{}).WithThreshold(0.5)
	cases := []TestCase{{ID: "t1", Expected: "a"}, {ID: "t2", Expected: "b"}}
	predictions := map[string]string{"t1": "a", "t2": "x"}
	report := e.EvaluateStatic(cases, predictions)
	md := FormatReport(report)
	if !strings.Contains(md, "评估报告") {
		t.Error("report should contain title")
	}
	if !strings.Contains(md, "通过率") {
		t.Error("report should contain pass rate")
	}
}

func TestFormatRAGReport(t *testing.T) {
	result := EvaluateRAGBatch(
		[][]RetrievedDoc{{{ID: "a"}, {ID: "b"}}},
		[][]string{{"a"}},
		2,
	)
	md := FormatRAGReport(&result)
	if !strings.Contains(md, "RAG") {
		t.Error("should contain RAG")
	}
}

func TestTracedEvaluator(t *testing.T) {
	te := NewTracedEvaluator(NewEvaluator(ExactMatch{}), nil) // noop tracer
	cases := []TestCase{{ID: "t1", Input: "hi", Expected: "hi"}}
	run := func(_ context.Context, _ string) (string, error) { return "hi", nil }
	report, err := te.EvaluateBatch(context.Background(), cases, run)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalCases != 1 {
		t.Errorf("total = %d want 1", report.TotalCases)
	}
}

func TestEvaluateRAGBatchTraced(t *testing.T) {
	result := EvaluateRAGBatchTraced(context.Background(), nil,
		[][]RetrievedDoc{{{ID: "a"}}},
		[][]string{{"a"}},
		1,
	)
	if result.Queries != 1 {
		t.Errorf("queries = %d want 1", result.Queries)
	}
}

func TestMetricFunc(t *testing.T) {
	m := MetricFunc{MetricName: "custom", Run: func(p, r string) float64 {
		if p == r {
			return 1
		}
		return 0
	}}
	if m.Name() != "custom" {
		t.Error("name")
	}
	if m.Compute("a", "a") != 1 {
		t.Error("compute")
	}
}
