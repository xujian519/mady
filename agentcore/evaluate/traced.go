package evaluate

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

// TracedEvaluator wraps an Evaluator, recording each evaluation run as a span
// when an agentcore.Tracer is configured. Metric scores are attached as span
// attributes so they appear in the same trace as the agent run under review.
//
// When the tracer is nil, evaluation runs without any tracing overhead.
type TracedEvaluator struct {
	inner  *Evaluator
	tracer agentcore.Tracer
}

// NewTracedEvaluator wraps an Evaluator with span recording. Pass nil to
// disable tracing.
func NewTracedEvaluator(inner *Evaluator, tracer agentcore.Tracer) *TracedEvaluator {
	if tracer == nil {
		tracer = agentcore.NoopTracer()
	}
	return &TracedEvaluator{inner: inner, tracer: tracer}
}

// EvaluateBatch runs the inner evaluator and records a span with aggregate
// metric attributes.
func (t *TracedEvaluator) EvaluateBatch(ctx context.Context, cases []TestCase, run RunFunc) (*BatchReport, error) {
	ctx, span := t.tracer.Start(ctx, "evaluate.EvaluateBatch",
		agentcore.Attr("evaluate.case_count", len(cases)),
	)
	defer span.End()

	report, err := t.inner.EvaluateBatch(ctx, cases, run)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	attrs := []agentcore.SpanAttribute{
		agentcore.Attr("evaluate.pass_rate", report.PassRate),
		agentcore.Attr("evaluate.passed", report.PassedCases),
		agentcore.Attr("evaluate.total", report.TotalCases),
	}
	for name, score := range report.AggregateScores {
		attrs = append(attrs, agentcore.Attr(fmt.Sprintf("evaluate.metric.%s", name), score))
	}
	span.SetAttributes(attrs...)
	return report, nil
}

// EvaluateRAGBatchTraced runs RAG evaluation with span recording.
func EvaluateRAGBatchTraced(ctx context.Context, tracer agentcore.Tracer, retrievedSet [][]RetrievedDoc, relevantIDsSet [][]string, k int) RAGBatchResult {
	if tracer == nil {
		return EvaluateRAGBatch(retrievedSet, relevantIDsSet, k)
	}
	_, span := tracer.Start(ctx, "evaluate.EvaluateRAGBatch",
		agentcore.Attr("evaluate.query_count", len(retrievedSet)),
		agentcore.Attr("evaluate.k", k),
	)
	defer span.End()

	result := EvaluateRAGBatch(retrievedSet, relevantIDsSet, k)
	span.SetAttributes(
		agentcore.Attr("evaluate.mean_precision", result.MeanPrecision),
		agentcore.Attr("evaluate.mean_recall", result.MeanRecall),
		agentcore.Attr("evaluate.mean_mrr", result.MeanMRR),
		agentcore.Attr("evaluate.mean_ndcg", result.MeanNDCG),
		agentcore.Attr("evaluate.hit_rate", result.HitRate),
	)
	return result
}
