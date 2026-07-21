package benchmark

import (
	"context"
	"os"
	"strconv"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/evaluate"
)

// DefaultEvaluator returns an Evaluator configured with the standard metric
// set used by all Golden Benchmark suites:
//   - F1Score: token overlap between prediction and reference
//   - KeywordRecall: fraction of reference keywords present in prediction
//   - CitationCompleteness: fraction of required citations present
//   - JudgeConsistency: heuristic keyword-overlap agreement (Phase 3)
//   - ToolAccuracy: tool call name, argument, and ordering correctness
//   - WorkflowQuality: workflow step completion, ordering, and precision
//
// The pass threshold is the package default (0.7).
func DefaultEvaluator() *evaluate.Evaluator {
	return evaluate.NewEvaluator(
		evaluate.F1Score{},
		evaluate.KeywordRecall{},
		evaluate.CitationCompleteness{},
		&evaluate.JudgeConsistency{},
		evaluate.ToolAccuracy{},
		evaluate.WorkflowQuality{},
	)
}

// LiveEvaluator returns an Evaluator suited for live LLM evaluation of
// long-form, subjective patent exam answers. It replaces the brittle
// token-overlap metrics with an LLM rubric judge, while keeping
// CitationCompleteness for required citations. The default pass threshold is
// the package default (0.7); callers can use .WithThreshold() to adjust for
// live evaluation.
//
// The LLMJudge uses 3-sample median scoring by default (configurable via
// MADY_JUDGE_SAMPLES env var) to suppress the high variance observed in
// single-shot judging (empirically up to 0.71 spread across runs of the same
// case). Set MADY_JUDGE_SAMPLES=1 to restore single-shot behavior.
func LiveEvaluator(judge agentcore.Provider, model string) *evaluate.Evaluator {
	samples := 3
	if v := os.Getenv("MADY_JUDGE_SAMPLES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			samples = n
		}
	}
	return evaluate.NewEvaluator(
		evaluate.CitationCompleteness{},
		evaluate.LLMJudge{Judge: judge, Model: model, Samples: samples},
	)
}

// registeredCases 汇总所有已注册的 benchmark 数据集。
// 新数据集应在此追加。
func registeredCases() []evaluate.TestCase {
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

// AllCases 返回跨领域的全部已注册 benchmark 案例。
func AllCases() []evaluate.TestCase {
	return registeredCases()
}

// ValidCases 返回适用于 live evaluation 的 benchmark 案例。
//
// P2B（InvalidationDecisionCases）曾因空壳输入（40/40 案例的
// claim/evidence/reason 全空）于 2026-07-15 冻结，此后已基于宝宸知识库_Raw
// 数据集（31562 件真实无效决定书 MD 文件）重建，字段提取正确（权利要求 1、
// 证据列表、无效理由），且结论分布均衡（全部无效/维持有效/部分无效）。
// P2B 现已重新启用 live evaluation，当前与 AllCases 一致。
// 见 scripts/extract_invalidation_cases.py。
func ValidCases() []evaluate.TestCase {
	return registeredCases()
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
