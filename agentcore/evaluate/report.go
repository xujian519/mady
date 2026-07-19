package evaluate

import (
	"fmt"
	"sort"
	"strings"
)

// FormatReport renders a BatchReport as a Markdown document for human review.
func FormatReport(report *BatchReport) string {
	if report == nil {
		return "# 评估报告\n\n无数据。\n"
	}
	var b strings.Builder
	b.WriteString("# 评估报告\n\n")

	// Summary.
	b.WriteString("## 概要\n\n")
	fmt.Fprintf(&b, "- 测试用例数: %d\n", report.TotalCases)
	fmt.Fprintf(&b, "- 通过数: %d\n", report.PassedCases)
	fmt.Fprintf(&b, "- 通过率: %.1f%%\n", report.PassRate*100)
	b.WriteString("\n### 聚合指标\n\n")
	b.WriteString("| 指标 | 平均分 |\n")
	b.WriteString("|------|--------|\n")
	for name, score := range report.AggregateScores {
		fmt.Fprintf(&b, "| %s | %.3f |\n", name, score)
	}

	// Per-case details.
	if len(report.Results) > 0 {
		b.WriteString("\n## 逐条结果\n\n")
		b.WriteString("| 用例 | 平均分 | 状态 |")
		metricNames := sortedMetricNames(report.Results)
		for _, name := range metricNames {
			fmt.Fprintf(&b, " %s |", name)
		}
		b.WriteString("\n|")
		for i := 0; i < 2+len(metricNames); i++ {
			b.WriteString("------|")
		}
		b.WriteString("\n")
		for _, r := range report.Results {
			status := "✅"
			if !r.Passed {
				status = "❌"
			}
			fmt.Fprintf(&b, "| %s | %.3f | %s |", r.CaseID, r.Average, status)
			for _, name := range metricNames {
				fmt.Fprintf(&b, " %.3f |", r.Scores[name])
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// FormatRAGReport renders a RAGBatchResult as Markdown.
func FormatRAGReport(result *RAGBatchResult) string {
	if result == nil || result.Queries == 0 {
		return "# RAG 检索评估报告\n\n无数据。\n"
	}
	var b strings.Builder
	b.WriteString("# RAG 检索评估报告\n\n")
	fmt.Fprintf(&b, "- 查询数: %d\n", result.Queries)
	fmt.Fprintf(&b, "- 平均 Precision@K: %.3f\n", result.MeanPrecision)
	fmt.Fprintf(&b, "- 平均 Recall@K: %.3f\n", result.MeanRecall)
	fmt.Fprintf(&b, "- 平均 MRR: %.3f\n", result.MeanMRR)
	fmt.Fprintf(&b, "- 平均 NDCG: %.3f\n", result.MeanNDCG)
	fmt.Fprintf(&b, "- Hit Rate@K: %.1f%%\n", result.HitRate*100)

	if len(result.PerQuery) > 0 {
		b.WriteString("\n## 逐查询结果\n\n")
		b.WriteString("| 查询# | Precision@K | Recall@K | MRR | NDCG | 命中 |\n")
		b.WriteString("|-------|-------------|----------|-----|------|------|\n")
		for i, ev := range result.PerQuery {
			hit := "✅"
			if !ev.HitAtK {
				hit = "❌"
			}
			fmt.Fprintf(&b, "| %d | %.3f | %.3f | %.3f | %.3f | %s |\n",
				i+1, ev.PrecisionAtK, ev.RecallAtK, ev.MRR, ev.NDCG, hit)
		}
	}

	return b.String()
}

// sortedMetricNames collects the distinct metric names appearing across the
// given results and returns them sorted for stable report output.
func sortedMetricNames(results []CaseResult) []string {
	seen := make(map[string]bool)
	var names []string
	for _, r := range results {
		for name := range r.Scores {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}
