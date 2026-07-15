package benchmark

import (
	"context"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/evaluate"
)

// DefaultEvaluator returns an Evaluator configured with the standard metric
// set used by all Golden Benchmark suites:
//   - F1Score: token overlap between prediction and reference
//   - KeywordRecall: fraction of reference keywords present in prediction
//   - CitationCompleteness: fraction of required citations present
//   - JudgeConsistency: heuristic keyword-overlap agreement (Phase 3)
//
// The pass threshold is the package default (0.7).
func DefaultEvaluator() *evaluate.Evaluator {
	return evaluate.NewEvaluator(
		evaluate.F1Score{},
		evaluate.KeywordRecall{},
		evaluate.CitationCompleteness{},
		&evaluate.JudgeConsistency{},
	)
}

// LiveEvaluator returns an Evaluator suited for live LLM evaluation of
// long-form, subjective patent exam answers. It replaces the brittle
// token-overlap metrics with an LLM rubric judge, while keeping
// CitationCompleteness for required citations. The default pass threshold is
// the package default (0.7); callers can use .WithThreshold() to adjust for
// live evaluation.
func LiveEvaluator(judge agentcore.Provider, model string) *evaluate.Evaluator {
	return evaluate.NewEvaluator(
		evaluate.CitationCompleteness{},
		evaluate.LLMJudge{Judge: judge, Model: model},
	)
}

// AllCases returns every registered benchmark case across all domains.
// New datasets should append their cases here.
//
// NOTE: AllCases still includes InvalidationDecisionCases (P2B) so that the
// static GoldenPerfect CI gate keeps verifying structural integrity of all
// registered data. For live evaluation use ValidCases instead — P2B is frozen
// because its inputs are empty shells (claim/evidence/reason all blank for
// 40/40 cases) with a degenerate conclusion distribution (34 invalidate-all /
// 5 partial / 1 maintained). See docs/evaluation-baseline-v0.6.md.
func AllCases() []evaluate.TestCase {

	var cases []evaluate.TestCase
	cases = append(cases, PatentExamCases...)
	cases = append(cases, PatentExamRealA2Cases...)
	cases = append(cases, PatentExamRealA22Cases...)
	cases = append(cases, PatentExamRealA26Cases...)
	cases = append(cases, PatentExamRealA31Cases...)
	cases = append(cases, PatentExamRealA33Cases...)
	cases = append(cases, PatentExamRealR42Cases...)
	cases = append(cases, InvalidationDecisionCases...)
	return cases
}

// ValidCases returns benchmark cases suitable for live evaluation: all
// registered cases EXCEPT the frozen P2B invalidation-decision dataset.
// Use this (not AllCases) when running live LLM evaluation so that empty-shell
// cases do not produce misleading pass-rate signals. The static CI gate
// (RunStatic) still uses AllCases to assert structural integrity of every
// registered case.
func ValidCases() []evaluate.TestCase {
	var cases []evaluate.TestCase
	cases = append(cases, PatentExamCases...)
	cases = append(cases, PatentExamRealA2Cases...)
	cases = append(cases, PatentExamRealA22Cases...)
	cases = append(cases, PatentExamRealA26Cases...)
	cases = append(cases, PatentExamRealA31Cases...)
	cases = append(cases, PatentExamRealA33Cases...)
	cases = append(cases, PatentExamRealR42Cases...)
	return cases
}

// CasesByDomain returns benchmark cases filtered by domain string.
func CasesByDomain(domain string) []evaluate.TestCase {
	var out []evaluate.TestCase
	for _, c := range AllCases() {
		if c.Domain == domain {
			out = append(out, c)
		}
	}
	return out
}

// RunSuite runs all benchmark cases through the given RunFunc (typically an
// agent or workflow under test) and returns a scored BatchReport. This is the
// primary entry point for live evaluation — e.g. "make eval-live" with a
// running LLM backend.
func RunSuite(ctx context.Context, run evaluate.RunFunc) (*evaluate.BatchReport, error) {
	return DefaultEvaluator().EvaluateBatch(ctx, AllCases(), run)
}

// RunStatic scores pre-recorded predictions without calling a live agent.
// This is the CI gate entry point: golden predictions are loaded from a file
// or generated in a prior step, and RunStatic verifies they still pass all
// metrics. A PassRate below 1.0 indicates a regression.
func RunStatic(predictions map[string]string) *evaluate.BatchReport {
	return DefaultEvaluator().EvaluateStatic(AllCases(), predictions)
}
