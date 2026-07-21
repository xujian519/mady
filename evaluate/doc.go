// Package evaluate provides offline evaluation infrastructure for agent
// workflows, RAG pipelines, and structured outputs.
//
// It is designed to run as a separate evaluation harness alongside production
// code: feed in test cases, obtain predictions (or use recorded outputs), and
// score them against reference answers using a pluggable set of metrics.
//
// # Metrics
//
// A [Metric] scores a single prediction against its reference. Built-in
// metrics include ExactMatch, F1Score, KeywordRecall, and
// CitationCompleteness. Custom metrics implement the interface.
//
// # RAG evaluation
//
// [EvaluateRAG] scores retrieval quality with Precision@K, Recall@K, MRR, and
// NDCG — standard information-retrieval metrics that map cleanly onto the
// retrieval package's ScoredChunk results.
//
// # Batch evaluation
//
// [Evaluator] runs a metric set over many [TestCase]s and produces a
// [BatchReport] with per-case and aggregate scores. [FormatReport] renders the
// report as Markdown for human review.
//
// # OpenTelemetry
//
// When the agent is instrumented with OpenTelemetry (see package
// tracing), evaluation runs can be wrapped in a span via
// [NewTracedEvaluator] so that evaluation latency and metric values appear in
// the same trace as the agent run under review.
package evaluate
