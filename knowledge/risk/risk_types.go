// Package risk provides active risk scanning for patent/legal analysis.
//
// The risk scanner analyzes technical feature combinations against historical
// invalidation/reexamination decisions to surface invalidation risk signals.
// It integrates as an agentcore Extension, providing a risk_scan tool and
// optional AfterModelCall lifecycle hooks.
package risk

import (
	"fmt"
	"strings"
)

// Severity rates how critical a risk signal is.
type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
)

// RiskType categorizes a risk signal by the kind of issue detected.
type RiskType string

const (
	RiskFeatureCombination RiskType = "feature_combination" // 特征组合风险
	RiskClaimScope         RiskType = "claim_scope"         // 保护范围风险
	RiskTermAmbiguity      RiskType = "term_ambiguity"      // 术语不清楚
	RiskSupport            RiskType = "support"             // 得不到说明书支持
)

// RiskSignal is a single risk finding from the scanner.
type RiskSignal struct {
	ID             string   `json:"id"`
	Type           RiskType `json:"type"`
	Severity       Severity `json:"severity"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	RelatedCases   []string `json:"related_cases,omitempty"`
	CaseCount      int      `json:"case_count"`
	InvalidRate    float64  `json:"invalid_rate"` // 0.0 ~ 1.0
	FeatureTags    []string `json:"feature_tags,omitempty"`
	Recommendation string   `json:"recommendation,omitempty"`
}

// ScanResult is the complete output of a risk scan.
type ScanResult struct {
	Signals    []RiskSignal `json:"signals"`
	TotalCases int          `json:"total_cases"`
	Confidence float64      `json:"confidence"` // 0.0 ~ 1.0
}

// HasSignals reports whether the result contains any actionable signals.
func (r *ScanResult) HasSignals() bool {
	return len(r.Signals) > 0
}

// HighCount returns the number of high-severity signals.
func (r *ScanResult) HighCount() int {
	count := 0
	for _, s := range r.Signals {
		if s.Severity == SeverityHigh {
			count++
		}
	}
	return count
}

// Summary returns a one-line summary of the scan result.
func (r *ScanResult) Summary() string {
	if !r.HasSignals() {
		return "未发现风险信号"
	}
	parts := make([]string, 0, len(r.Signals))
	for _, s := range r.Signals {
		parts = append(parts, fmt.Sprintf("[%s] %s(无效率%.0f%%)",
			string(s.Severity), s.Title, s.InvalidRate*100))
	}
	return strings.Join(parts, "; ")
}

// RenderMarkdown formats the scan result as a markdown report.
func (r *ScanResult) RenderMarkdown() string {
	if !r.HasSignals() {
		return "✅ 未发现显著风险信号。"
	}
	var b strings.Builder
	b.WriteString("## ⚠️ 风险扫描报告\n\n")
	fmt.Fprintf(&b, "> 基于 %d 篇历史文书分析  |  置信度: %.0f%%\n\n",
		r.TotalCases, r.Confidence*100)

	for i, s := range r.Signals {
		emoji := "🟢"
		switch s.Severity {
		case SeverityHigh:
			emoji = "🔴"
		case SeverityMedium:
			emoji = "🟡"
		}
		fmt.Fprintf(&b, "### %s %s\n\n", emoji, s.Title)
		fmt.Fprintf(&b, "- **严重度**: %s\n", string(s.Severity))
		fmt.Fprintf(&b, "- **风险类型**: %s\n", string(s.Type))
		fmt.Fprintf(&b, "- **历史案例**: %d 件 | **无效率**: %.0f%%\n",
			s.CaseCount, s.InvalidRate*100)
		if len(s.RelatedCases) > 0 {
			fmt.Fprintf(&b, "- **关联案号**: %s\n", strings.Join(s.RelatedCases, "、"))
		}
		if len(s.FeatureTags) > 0 {
			fmt.Fprintf(&b, "- **特征标签**: %s\n", strings.Join(s.FeatureTags, "、"))
		}
		if s.Description != "" {
			fmt.Fprintf(&b, "- **说明**: %s\n", s.Description)
		}
		if s.Recommendation != "" {
			fmt.Fprintf(&b, "- **建议**: %s\n", s.Recommendation)
		}
		if i < len(r.Signals)-1 {
			b.WriteString("\n---\n\n")
		}
	}
	return b.String()
}
