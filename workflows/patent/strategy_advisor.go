// Package patent provides strategy advice based on statistical analysis of
// CNIPA (China National Intellectual Property Administration) invalidation
// decisions. The statistical data is sourced from published analytical reports
// on reexamination and invalidation decisions, covering novelty, inventiveness,
// disclosure, claim clarity, amendment scope, and design patents.
package patent

import (
	"fmt"
	"strings"
)

// StrategyStat holds statistical data for a single invalidation ground type or
// reasoning pattern, sourced from published CNIPA decision analysis reports.
type StrategyStat struct {
	GroundType       string  // 无效理由类型标识，如 "A22.2_novelty"
	PatternID        string  // 推理模式ID，如 "single-doc-common-knowledge"
	TotalCases       int     // 决定总数
	InvalidationRate float64 // 全部无效率（0.0 ~ 1.0）
	Frequency        float64 // 出现频率（0.0 ~ 1.0，仅推理模式有此字段）
	Description      string  // 统计描述，如 "创造性无效理由"
	Source           string  // 数据来源说明
}

// StrategyAdvice contains the generated advice for a specific invalidation
// ground, including formatted statistics, probability assessment, strategic
// recommendation, and source attribution.
type StrategyAdvice struct {
	GroundType     string // 无效理由类型
	Stat           string // 格式化统计数据
	Probability    string // 成功概率评估（使用"通常""大概率"等措辞）
	Recommendation string // 策略建议
	Source         string // 数据来源说明
}

// =============================================================================
// Statistical data — sourced from published CNIPA reexamination and
// invalidation decision analysis reports. These values represent aggregate
// statistical patterns and should not be interpreted as case-specific
// predictions.
// =============================================================================

// invalidationStats maps invalidation ground types to their aggregate
// statistical data from CNIPA published analyses.
var invalidationStats = map[InvalidationGroundType]StrategyStat{
	GroundNovelty: {
		GroundType:       string(GroundNovelty),
		TotalCases:       8410,
		InvalidationRate: 0.85,
		Description:      "新颖性无效理由",
		Source:           "基于 CNIPA 8,410 份复审无效决定统计分析",
	},
	GroundInventiveness: {
		GroundType:       string(GroundInventiveness),
		TotalCases:       12798,
		InvalidationRate: 0.989,
		Description:      "创造性无效理由",
		Source:           "基于 CNIPA 12,798 份复审无效决定统计分析",
	},
	GroundDisclosure: {
		GroundType:       string(GroundDisclosure),
		TotalCases:       4260,
		InvalidationRate: 0.72,
		Description:      "公开不充分无效理由",
		Source:           "基于 CNIPA 4,260 份说明书及程序类复审无效决定统计分析",
	},
	GroundClaimClarity: {
		GroundType:       string(GroundClaimClarity),
		TotalCases:       4260,
		InvalidationRate: 0.68,
		Description:      "权利要求不清楚/不支持无效理由",
		Source:           "基于 CNIPA 4,260 份说明书及程序类复审无效决定统计分析",
	},
	GroundAmendment: {
		GroundType:       string(GroundAmendment),
		TotalCases:       4260,
		InvalidationRate: 0.55,
		Description:      "修改超范围无效理由",
		Source:           "基于 CNIPA 4,260 份说明书及程序类复审无效决定统计分析",
	},
}

// Design invalidation statistics (separate category).
const (
	designTotalCases       = 14028
	designOverallVisualPct = 0.473
	designSource           = "基于 CNIPA 14,028 份外观设计复审无效决定统计分析"
)

// reasoningPatternStats maps reasoning pattern IDs to their statistical data
// from CNIPA inventiveness decision analyses.
var reasoningPatternStats = map[string]StrategyStat{
	"single-doc-common-knowledge": {
		PatternID:        "single-doc-common-knowledge",
		TotalCases:       5805,
		Frequency:        0.54,
		InvalidationRate: 0.959,
		Description:      "单对比文件+公知常识（最常见模式）",
		Source:           "基于 5,805 份创造性决定的统计分析",
	},
	"multi-doc-combination": {
		PatternID:        "multi-doc-combination",
		TotalCases:       1537,
		Frequency:        0.143,
		InvalidationRate: 0.737,
		Description:      "多对比文件结合",
		Source:           "基于 1,537 份创造性决定的统计分析",
	},
	"technical-motivation": {
		PatternID:        "technical-motivation",
		TotalCases:       868,
		Frequency:        0.081,
		InvalidationRate: 0.838,
		Description:      "技术启示判断",
		Source:           "基于 868 份创造性决定的统计分析",
	},
}

// =============================================================================
// Public API
// =============================================================================

// AllInvalidationStats returns all invalidation ground statistics for the
// defined ground types. Each entry includes total cases, invalidation rate,
// description, and data source attribution.
func AllInvalidationStats() []StrategyStat {
	stats := make([]StrategyStat, 0, len(invalidationStats))
	for _, s := range invalidationStats {
		stats = append(stats, s)
	}
	return stats
}

// AllReasoningPatternStats returns all reasoning pattern statistics for the
// defined patterns. Each entry includes total cases, frequency, invalidation
// rate, description, and data source attribution.
func AllReasoningPatternStats() []StrategyStat {
	stats := make([]StrategyStat, 0, len(reasoningPatternStats))
	for _, s := range reasoningPatternStats {
		stats = append(stats, s)
	}
	return stats
}

// GetStrategyAdvice generates strategy advice for the specified invalidation
// ground type and (optionally) reasoning pattern. The advice includes formatted
// statistics, a probability assessment, a strategic recommendation, and the
// data source. All probability assessments use non-absolute phrasing
// ("通常", "大概率") in compliance with project tone guidelines.
//
// groundType is the invalidation ground type (e.g. "A22.2_novelty").
// patternID is optional — pass "" to skip pattern-specific advice.
func GetStrategyAdvice(groundType string, patternID string) StrategyAdvice {
	advice := StrategyAdvice{
		GroundType: groundType,
	}

	// Look up ground-level statistics.
	gt := InvalidationGroundType(groundType)
	stat, ok := invalidationStats[gt]
	if !ok {
		advice.Stat = "暂无该无效理由类型的统计数据"
		advice.Probability = "无法评估"
		advice.Recommendation = "建议参考类似案例或咨询专利代理师"
		advice.Source = ""
		return advice
	}

	advice.Source = stat.Source

	// Format statistics and generate probability assessment.
	ratePct := stat.InvalidationRate * 100
	advice.Stat = fmt.Sprintf("%s：共分析 %d 份决定，全部无效率 %.1f%%",
		stat.Description, stat.TotalCases, ratePct)

	switch {
	case stat.InvalidationRate >= 0.90:
		advice.Probability = "成功率通常较高（基于统计）"
	case stat.InvalidationRate >= 0.70:
		advice.Probability = "成功率大概率处于中等偏高水平（基于统计）"
	case stat.InvalidationRate >= 0.50:
		advice.Probability = "成功率大致处于中等水平（基于统计）"
	default:
		advice.Probability = "成功率通常偏低（基于统计）"
	}

	// Generate recommendation based on ground type.
	switch gt {
	case GroundNovelty:
		advice.Recommendation = "新颖性无效理由在实务中成功率通常较高，但需注意单独对比原则的要求。建议优先检索一份能够公开权利要求全部技术特征的对比文件，避免多份文件结合。"
	case GroundInventiveness:
		advice.Recommendation = "创造性无效理由是最常用的无效手段，全部无效率极高（约98.9%）。建议优先采用单对比文件+公知常识的论证模式，其次考虑多份对比文件结合并充分论证组合动机。"
	case GroundDisclosure:
		advice.Recommendation = "公开不充分理由的成功率大致处于中等水平。建议重点关注说明书中是否缺少必要技术手段、技术方案是否仅公开效果而未公开具体实现方式。"
	case GroundClaimClarity:
		advice.Recommendation = "权利要求不清楚/不支持理由的成功率大致处于中等水平。建议重点审查权利要求中使用的不确定术语、功能性限定的范围是否合理、以及是否得到说明书支持。"
	case GroundAmendment:
		advice.Recommendation = "修改超范围理由的成功率通常偏低。建议重点对比修改前后的文本，审查修改内容是否能够从原申请文件中直接且毫无疑义地确定。"
	}

	// Supplement with pattern-specific advice if available.
	if patternID != "" {
		if pStat, ok := reasoningPatternStats[patternID]; ok {
			pFreq := pStat.Frequency * 100
			pRate := pStat.InvalidationRate * 100
			patternAdvice := fmt.Sprintf("\n\n**推理模式参考**：%s（出现频率 %.1f%%，全部无效率 %.1f%%）",
				pStat.Description, pFreq, pRate)
			advice.Stat += patternAdvice
			advice.Source += "；" + pStat.Source
		}
	}

	return advice
}

// FormatStrategySection generates a Markdown-formatted strategy advice section
// based on the identified invalidation grounds. The section includes an
// introductory note on statistical limitations, per-ground strategy analysis,
// and reasoning pattern statistics.
//
// The section is formatted as a level-2 heading section suitable for inclusion
// in the invalidation analysis report, before the disclaimer.
func FormatStrategySection(grounds []InvGround) string {
	if len(grounds) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("## 统计数据策略建议\n\n")
	b.WriteString("> **说明**：以下统计数据来源于 CNIPA 已公开复审无效决定的分析报告，反映的是整体统计趋势，")
	b.WriteString("不作为个案的胜诉保证。实际案件结果取决于具体事实、证据和法律适用。\n\n")

	b.WriteString("### 各无效理由策略分析\n\n")

	for i, g := range grounds {
		advice := GetStrategyAdvice(string(g.Type), "")
		fmt.Fprintf(&b, "**%d. %s**\n\n", i+1, g.Description)
		fmt.Fprintf(&b, "- 统计：%s\n", advice.Stat)
		fmt.Fprintf(&b, "- 概率：%s\n", advice.Probability)
		fmt.Fprintf(&b, "- 建议：%s\n", advice.Recommendation)
		fmt.Fprintf(&b, "- 来源：%s\n\n", advice.Source)
	}

	// Append reasoning pattern statistics for inventiveness grounds.
	hasInventiveness := false
	for _, g := range grounds {
		if g.Type == GroundInventiveness {
			hasInventiveness = true
			break
		}
	}
	if hasInventiveness {
		b.WriteString("### 创造性推理模式统计参考\n\n")
		b.WriteString("| 推理模式 | 出现频率 | 全部无效率 | 案件数量 |\n")
		b.WriteString("|----------|----------|------------|----------|\n")
		for _, pid := range []string{"single-doc-common-knowledge", "multi-doc-combination", "technical-motivation"} {
			if ps, ok := reasoningPatternStats[pid]; ok {
				freqPct := ps.Frequency * 100
				ratePct := ps.InvalidationRate * 100
				fmt.Fprintf(&b, "| %s | %.1f%% | %.1f%% | %d 件 |\n",
					ps.Description, freqPct, ratePct, ps.TotalCases)
			}
		}
		b.WriteString("\n")
		b.WriteString("以上数据源自 CNIPA 复审无效决定统计分析。建议优先采用出现频率高且全部无效率高的推理模式（如单对比文件+公知常识），可参考已有决定的说理思路。\n\n")
	}

	b.WriteString("### 数据来源\n\n")
	b.WriteString("本策略建议模块的统计数据来源于以下分析报告：\n")
	b.WriteString("- 创造性无效理由：12,798 份复审无效决定分析报告\n")
	b.WriteString("- 新颖性无效理由：8,410 份复审无效决定分析报告\n")
	b.WriteString("- 说明书及程序类无效理由：4,260 份复审无效决定分析报告\n")
	b.WriteString("- 外观设计无效理由：14,028 份复审无效决定分析报告\n\n")

	return b.String()
}

// GetDesignStat returns the formatted design patent invalidation statistic.
func GetDesignStat() string {
	pct := designOverallVisualPct * 100
	return fmt.Sprintf("外观设计无效理由：共分析 %d 份决定，%.1f%% 的决定涉及「整体视觉效果」的判断。",
		designTotalCases, pct)
}
