package evaluate

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// EnhancedReport extends the standard BatchReport with additional analysis:
//   - Per-metric pass/fail breakdown
//   - Worst/best performing cases
//   - Score distribution (percentiles)
//   - Trend comparison against a baseline report (optional)
type EnhancedReport struct {
	*BatchReport

	// MetricBreakdown maps metric name → pass count / total count.
	MetricBreakdown map[string]MetricBreakdown

	// WorstCases lists the bottom-N cases by average score.
	WorstCases []CaseResult

	// BestCases lists the top-N cases by average score.
	BestCases []CaseResult

	// Percentiles gives score thresholds at P10, P25, P50, P75, P90.
	Percentiles ScorePercentiles

	// Trend, when compared against a prior report, shows improvement/regression.
	Trend *TrendReport
}

// MetricBreakdown shows pass/fail statistics for a single metric.
type MetricBreakdown struct {
	Name      string  `json:"name"`
	Mean      float64 `json:"mean"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	StdDev    float64 `json:"std_dev"`
	PassCount int     `json:"pass_count"` // score >= 0.7
	FailCount int     `json:"fail_count"` // score < 0.7
}

// ScorePercentiles shows score distribution thresholds.
type ScorePercentiles struct {
	P10 float64 `json:"p10"`
	P25 float64 `json:"p25"`
	P50 float64 `json:"p50"` // median
	P75 float64 `json:"p75"`
	P90 float64 `json:"p90"`
}

// TrendReport compares two evaluation runs.
type TrendReport struct {
	// PreviousPassRate is the pass rate from the prior run.
	PreviousPassRate float64 `json:"previous_pass_rate"`
	// CurrentPassRate is the pass rate of the current run.
	CurrentPassRate float64 `json:"current_pass_rate"`
	// Delta is the change in pass rate (positive = improvement).
	Delta float64 `json:"delta"`
	// RegressedCases lists cases that passed before but fail now.
	RegressedCases []string `json:"regressed_cases,omitempty"`
	// ImprovedCases lists cases that failed before but pass now.
	ImprovedCases []string `json:"improved_cases,omitempty"`
}

// FormatEnhancedReport renders an EnhancedReport as Markdown.
func FormatEnhancedReport(report *EnhancedReport) string {
	if report == nil || report.BatchReport == nil {
		return "# 增强评估报告\n\n无数据。\n"
	}

	var b strings.Builder
	b.WriteString("# 增强评估报告\n\n")

	writeSummarySection(&b, report)
	writeTrendSection(&b, report)
	writeMetricBreakdownSection(&b, report)
	writePercentileSection(&b, report)
	writeWorstCasesSection(&b, report)
	writeBestCasesSection(&b, report)

	return b.String()
}

func writeSummarySection(b *strings.Builder, report *EnhancedReport) {
	b.WriteString("## 概要\n\n")
	fmt.Fprintf(b, "- 测试用例数: %d\n", report.TotalCases)
	fmt.Fprintf(b, "- 通过数: %d\n", report.PassedCases)
	fmt.Fprintf(b, "- 通过率: **%.1f%%**\n", report.PassRate*100)

	if len(report.AggregateScores) > 0 {
		b.WriteString("\n| 指标 | 平均分 |\n|------|--------|\n")
		names := sortedStringKeys(report.AggregateScores)
		for _, name := range names {
			fmt.Fprintf(b, "| %s | %.3f |\n", name, report.AggregateScores[name])
		}
	}
}

func writeTrendSection(b *strings.Builder, report *EnhancedReport) {
	if report.Trend == nil {
		return
	}
	b.WriteString("\n## 趋势对比\n\n")
	arrow := "🟢"
	if report.Trend.Delta < -0.05 {
		arrow = "🔴"
	} else if report.Trend.Delta < 0 {
		arrow = "🟡"
	}
	fmt.Fprintf(b, "- 前次通过率: %.1f%%\n", report.Trend.PreviousPassRate*100)
	fmt.Fprintf(b, "- 本次通过率: %.1f%%\n", report.Trend.CurrentPassRate*100)
	fmt.Fprintf(b, "- 变化: %s %+.1f%%\n", arrow, report.Trend.Delta*100)

	if len(report.Trend.RegressedCases) > 0 {
		b.WriteString("\n### 退化用例\n\n")
		for _, id := range report.Trend.RegressedCases {
			fmt.Fprintf(b, "- ❌ %s\n", id)
		}
	}
	if len(report.Trend.ImprovedCases) > 0 {
		b.WriteString("\n### 改善用例\n\n")
		for _, id := range report.Trend.ImprovedCases {
			fmt.Fprintf(b, "- 🟢 %s\n", id)
		}
	}
}

func writeMetricBreakdownSection(b *strings.Builder, report *EnhancedReport) {
	if len(report.MetricBreakdown) == 0 {
		return
	}
	b.WriteString("\n## 指标分解\n\n")
	b.WriteString("| 指标 | 平均分 | 最小值 | 最大值 | 标准差 | 通过 | 未通过 |\n")
	b.WriteString("|------|--------|--------|--------|--------|------|--------|\n")
	names := sortedStringKeysStr(report.MetricBreakdown)
	for _, name := range names {
		mb := report.MetricBreakdown[name]
		fmt.Fprintf(b, "| %s | %.3f | %.3f | %.3f | %.3f | %d | %d |\n",
			name, mb.Mean, mb.Min, mb.Max, mb.StdDev, mb.PassCount, mb.FailCount)
	}
}

func writePercentileSection(b *strings.Builder, report *EnhancedReport) {
	p := report.Percentiles
	b.WriteString("\n## 分数分布\n\n")
	b.WriteString("| 百分位 | 分数 |\n|--------|------|\n")
	fmt.Fprintf(b, "| P10 | %.3f |\n", p.P10)
	fmt.Fprintf(b, "| P25 | %.3f |\n", p.P25)
	fmt.Fprintf(b, "| P50 (中位数) | %.3f |\n", p.P50)
	fmt.Fprintf(b, "| P75 | %.3f |\n", p.P75)
	fmt.Fprintf(b, "| P90 | %.3f |\n", p.P90)
}

func writeWorstCasesSection(b *strings.Builder, report *EnhancedReport) {
	if len(report.WorstCases) == 0 {
		return
	}
	b.WriteString("\n## 最差用例 (Bottom-5)\n\n")
	b.WriteString("| 用例 | 平均分 |\n|------|--------|\n")
	for _, c := range report.WorstCases {
		fmt.Fprintf(b, "| ❌ %s | %.3f |\n", c.CaseID, c.Average)
	}
}

func writeBestCasesSection(b *strings.Builder, report *EnhancedReport) {
	if len(report.BestCases) == 0 {
		return
	}
	b.WriteString("\n## 最佳用例 (Top-5)\n\n")
	b.WriteString("| 用例 | 平均分 |\n|------|--------|\n")
	for _, c := range report.BestCases {
		fmt.Fprintf(b, "| 🟢 %s | %.3f |\n", c.CaseID, c.Average)
	}
}

// BuildEnhancedReport wraps a BatchReport with enhanced analysis.
// If previousResults is non-nil, it also computes a trend comparison.
func BuildEnhancedReport(report *BatchReport, previousReport *BatchReport) *EnhancedReport {
	if report == nil {
		return nil
	}

	enh := &EnhancedReport{
		BatchReport:     report,
		MetricBreakdown: computeMetricBreakdown(report),
		Percentiles:     computePercentiles(report),
	}

	// Top/bottom 5 cases.
	sorted := sortedByAverage(report.Results)
	n := len(sorted)
	if n > 5 {
		enh.WorstCases = sorted[:5]
		enh.BestCases = sorted[n-5:]
	} else {
		enh.WorstCases = sorted
		enh.BestCases = sorted
	}

	// Trend comparison.
	if previousReport != nil {
		enh.Trend = computeTrend(report, previousReport)
	}

	return enh
}

// computeMetricBreakdown computes per-metric statistics across all cases.
func computeMetricBreakdown(report *BatchReport) map[string]MetricBreakdown {
	// Collect scores per metric.
	type scoreCollector struct {
		scores []float64
	}
	collector := make(map[string]*scoreCollector)

	// Track per-case pass/fail per metric.
	type metricPassFail struct {
		pass int
		fail int
	}
	passFail := make(map[string]*metricPassFail)

	for _, r := range report.Results {
		for name, score := range r.Scores {
			if collector[name] == nil {
				collector[name] = &scoreCollector{}
				passFail[name] = &metricPassFail{}
			}
			collector[name].scores = append(collector[name].scores, score)
			if score >= 0.7 {
				passFail[name].pass++
			} else {
				passFail[name].fail++
			}
		}
	}

	breakdown := make(map[string]MetricBreakdown, len(collector))
	for name, col := range collector {
		scores := col.scores
		if len(scores) == 0 {
			continue
		}
		mean := meanFloat64(scores)
		breakdown[name] = MetricBreakdown{
			Name:      name,
			Mean:      mean,
			Min:       minFloat64(scores),
			Max:       maxFloat64(scores),
			StdDev:    stdDevFloat64(scores, mean),
			PassCount: passFail[name].pass,
			FailCount: passFail[name].fail,
		}
	}
	return breakdown
}

// computePercentiles calculates score distribution percentiles.
func computePercentiles(report *BatchReport) ScorePercentiles {
	avgs := make([]float64, len(report.Results))
	for i, r := range report.Results {
		avgs[i] = r.Average
	}
	sort.Float64s(avgs)
	n := len(avgs)
	if n == 0 {
		return ScorePercentiles{}
	}
	return ScorePercentiles{
		P10: percentile(avgs, 0.10),
		P25: percentile(avgs, 0.25),
		P50: percentile(avgs, 0.50),
		P75: percentile(avgs, 0.75),
		P90: percentile(avgs, 0.90),
	}
}

// computeTrend compares two evaluation runs.
func computeTrend(current, previous *BatchReport) *TrendReport {
	trend := &TrendReport{
		PreviousPassRate: previous.PassRate,
		CurrentPassRate:  current.PassRate,
		Delta:            current.PassRate - previous.PassRate,
	}

	// Build pass/fail maps.
	prevPassed := make(map[string]bool)
	for _, r := range previous.Results {
		if r.Passed {
			prevPassed[r.CaseID] = true
		}
	}
	currPassed := make(map[string]bool)
	for _, r := range current.Results {
		if r.Passed {
			currPassed[r.CaseID] = true
		}
	}

	// Find regressed cases (passed before, fail now).
	for id := range prevPassed {
		if !currPassed[id] {
			trend.RegressedCases = append(trend.RegressedCases, id)
		}
	}
	// Find improved cases (failed before, pass now).
	for id := range currPassed {
		if !prevPassed[id] {
			trend.ImprovedCases = append(trend.ImprovedCases, id)
		}
	}
	sort.Strings(trend.RegressedCases)
	sort.Strings(trend.ImprovedCases)

	return trend
}

// sortedByAverage returns results sorted ascending by average score.
func sortedByAverage(results []CaseResult) []CaseResult {
	sorted := make([]CaseResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Average < sorted[j].Average
	})
	return sorted
}

// percentile returns the value at the given percentile from a sorted slice
// using linear interpolation between adjacent values.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}

	// Calculate the exact position as a float.
	pos := p * float64(n-1)
	idx := int(pos)
	frac := pos - float64(idx)

	if idx >= n-1 {
		return sorted[n-1]
	}
	return sorted[idx]*(1-frac) + sorted[idx+1]*frac
}

// -- float64 slice helpers --

func meanFloat64(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func minFloat64(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	m := v[0]
	for _, x := range v[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maxFloat64(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	m := v[0]
	for _, x := range v[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

func stdDevFloat64(v []float64, mean float64) float64 {
	if len(v) < 2 {
		return 0
	}
	sumSq := 0.0
	for _, x := range v {
		d := x - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(v)-1))
}

// sortedStringKeys returns sorted keys from a map[string]float64.
func sortedStringKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedStringKeysStr returns sorted keys from a map[string]MetricBreakdown.
func sortedStringKeysStr(m map[string]MetricBreakdown) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
