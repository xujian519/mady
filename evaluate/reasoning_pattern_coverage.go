package evaluate

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/workflows/patent"
)

// ReasoningPatternCoverageReport 是推理模式规则覆盖率评估的报告结果。
type ReasoningPatternCoverageReport struct {
	// TotalPatterns 是推理模式总数（应为 18）。
	TotalPatterns int `json:"total_patterns"`
	// CoveredPatterns 是有至少一条关联规则的推理模式数。
	CoveredPatterns int `json:"covered_patterns"`
	// CoverageRate 是覆盖率（0-1）。
	CoverageRate float64 `json:"coverage_rate"`
	// TotalRules 是规则总数。
	TotalRules int `json:"total_rules"`
	// PatternsByCategory 按类别统计覆盖率。
	PatternsByCategory map[string]CategoryCoverage `json:"patterns_by_category"`
	// UncoveredPatterns 是缺少关联规则的推理模式列表。
	UncoveredPatterns []string `json:"uncovered_patterns,omitempty"`
}

// CategoryCoverage 是单个类别的覆盖率统计。
type CategoryCoverage struct {
	Total    int     `json:"total"`
	Covered  int     `json:"covered"`
	Rate     float64 `json:"rate"`
	RuleHits int     `json:"rule_hits"`
}

// ReasoningPatternCoverage 评估规则引擎对 18 种推理模式的规则覆盖率。
// 它检查 workflows/patent/reasoning_patterns.go 中定义的每种推理模式
// 是否至少对应 1 条 CheckRule，并按类别（创造性/新颖性/权利要求/其他）
// 统计覆盖率。
type ReasoningPatternCoverage struct{}

// Name 返回评估器名称。
func (r ReasoningPatternCoverage) Name() string { return "reasoning_pattern_coverage" }

// Evaluate 检查规则引擎对 18 种推理模式的覆盖情况，返回覆盖率指标。
//
// params 支持以下参数:
//   - detail (bool): 设为 true 时列出未覆盖的模式（默认 false）
func (r ReasoningPatternCoverage) Evaluate(ctx context.Context, params map[string]any) (*ReasoningPatternCoverageReport, error) {
	patterns := patent.AllPatterns()
	if len(patterns) == 0 {
		return nil, fmt.Errorf("reasoning pattern coverage: no patterns found")
	}

	report := &ReasoningPatternCoverageReport{
		TotalPatterns:      len(patterns),
		PatternsByCategory: make(map[string]CategoryCoverage),
	}

	categoryStats := make(map[string]*struct {
		total    int
		covered  int
		ruleHits int
	})

	var uncovered []string

	for _, p := range patterns {
		if _, ok := categoryStats[p.Category]; !ok {
			categoryStats[p.Category] = &struct {
				total    int
				covered  int
				ruleHits int
			}{}
		}
		categoryStats[p.Category].total++
		report.TotalRules += len(p.CheckRules)

		if len(p.CheckRules) > 0 {
			report.CoveredPatterns++
			categoryStats[p.Category].covered++
			categoryStats[p.Category].ruleHits += len(p.CheckRules)
		} else {
			uncovered = append(uncovered, fmt.Sprintf("%s (%s)", p.ID, p.Name))
		}
	}

	if report.TotalPatterns > 0 {
		report.CoverageRate = float64(report.CoveredPatterns) / float64(report.TotalPatterns)
	}

	for cat, stats := range categoryStats {
		rate := 0.0
		if stats.total > 0 {
			rate = float64(stats.covered) / float64(stats.total)
		}
		report.PatternsByCategory[cat] = CategoryCoverage{
			Total:    stats.total,
			Covered:  stats.covered,
			Rate:     rate,
			RuleHits: stats.ruleHits,
		}
	}

	showDetail := false
	if params != nil {
		if v, ok := params["detail"]; ok {
			showDetail, _ = v.(bool)
		}
	}
	if showDetail && len(uncovered) > 0 {
		report.UncoveredPatterns = uncovered
	}

	return report, nil
}

// FormatCoverageReport 将覆盖率报告格式化为可读的文本输出。
func FormatCoverageReport(report *ReasoningPatternCoverageReport) string {
	if report == nil {
		return "无覆盖率数据"
	}

	var b strings.Builder
	b.WriteString("## 推理模式规则覆盖率\n\n")
	fmt.Fprintf(&b, "总推理模式: %d\n", report.TotalPatterns)
	fmt.Fprintf(&b, "已覆盖模式: %d\n", report.CoveredPatterns)
	fmt.Fprintf(&b, "覆盖率: %.1f%%\n", report.CoverageRate*100)
	fmt.Fprintf(&b, "关联规则总数: %d\n\n", report.TotalRules)

	b.WriteString("### 分类覆盖率\n\n")
	b.WriteString("| 类别 | 模式数 | 已覆盖 | 覆盖率 | 关联规则 |\n")
	b.WriteString("|------|--------|--------|--------|----------|\n")

	// Ordered output: creativity, novelty, claims, other
	categories := []struct{ key, label string }{
		{"creativity", "创造性"},
		{"novelty", "新颖性"},
		{"claims", "权利要求/说明书"},
		{"other", "其他"},
	}
	for _, cat := range categories {
		if cc, ok := report.PatternsByCategory[cat.key]; ok {
			fmt.Fprintf(&b, "| %s | %d | %d | %.1f%% | %d |\n",
				cat.label, cc.Total, cc.Covered, cc.Rate*100, cc.RuleHits)
		}
	}

	if len(report.UncoveredPatterns) > 0 {
		b.WriteString("\n### 未覆盖的推理模式\n\n")
		for _, u := range report.UncoveredPatterns {
			fmt.Fprintf(&b, "- %s\n", u)
		}
	}

	return b.String()
}
