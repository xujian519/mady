package checker

import (
	"fmt"
	"strings"
)

// VerdictStatus is the conclusion of a single checker review.
type VerdictStatus string

const (
	StatusPass          VerdictStatus = "pass"           // 通过，无问题
	StatusNeedsRevision VerdictStatus = "needs_revision" // 需修订，有轻微问题
	StatusBlocked       VerdictStatus = "blocked"        // 阻塞，严重问题需人工介入
)

// CheckerVerdict is the structured result of a single checker's review.
type CheckerVerdict struct {
	RoleID      string        `json:"role_id"`
	Status      VerdictStatus `json:"status"`
	Score       float64       `json:"score,omitempty"` // 0.0 - 1.0
	Summary     string        `json:"summary,omitempty"`
	Issues      []Issue       `json:"issues,omitempty"`
	Suggestions []string      `json:"suggestions,omitempty"`
}

// Issue records a single problem found during review.
type Issue struct {
	Severity    IssueSeverity `json:"severity"`
	Location    string        `json:"location,omitempty"` // 问题位置（段落号/行号/章节名）
	Description string        `json:"description"`
	Suggestion  string        `json:"suggestion,omitempty"` // 修改建议
}

// IssueSeverity rates a single issue.
type IssueSeverity string

const (
	SeverityError   IssueSeverity = "error"
	SeverityWarning IssueSeverity = "warning"
	SeverityInfo    IssueSeverity = "info"
)

// VerdictAggregator merges multiple CheckerVerdicts into a single composite result.
type VerdictAggregator struct {
	verdicts []CheckerVerdict
}

// NewVerdictAggregator creates an aggregator from one or more verdicts.
func NewVerdictAggregator(verdicts []CheckerVerdict) *VerdictAggregator {
	return &VerdictAggregator{verdicts: verdicts}
}

// Aggregate merges all verdicts. The composite status is:
//   - blocked if ANY verdict is blocked
//   - needs_revision if ANY verdict is needs_revision (and none blocked)
//   - pass otherwise
//
// The composite score is the average of all verdict scores.
func (va *VerdictAggregator) Aggregate() CheckerVerdict {
	if len(va.verdicts) == 0 {
		return CheckerVerdict{
			Status:  StatusPass,
			Score:   1.0,
			Summary: "未运行任何检查器，默认通过",
		}
	}

	composite := CheckerVerdict{
		Status:      StatusPass,
		Issues:      make([]Issue, 0),
		Suggestions: make([]string, 0),
	}

	var scoreSum float64
	var worstStatus = StatusPass
	seenIssues := make(map[string]bool)

	for _, v := range va.verdicts {
		scoreSum += v.Score

		// Track worst status
		if v.Status == StatusBlocked {
			worstStatus = StatusBlocked
		} else if v.Status == StatusNeedsRevision && worstStatus != StatusBlocked {
			worstStatus = StatusNeedsRevision
		}

		// Merge issues (deduplicate by description)
		for _, iss := range v.Issues {
			key := string(iss.Severity) + ":" + iss.Description
			if !seenIssues[key] {
				seenIssues[key] = true
				composite.Issues = append(composite.Issues, iss)
			}
		}

		// Merge suggestions
		composite.Suggestions = append(composite.Suggestions, v.Suggestions...)
	}

	composite.Status = worstStatus
	composite.Score = scoreSum / float64(len(va.verdicts))
	composite.RoleID = "aggregated"

	// Build summary
	var parts []string
	parts = append(parts, fmt.Sprintf("综合判定: %s", verdictLabel(composite.Status)))
	parts = append(parts, fmt.Sprintf("综合评分: %.2f", composite.Score))
	if len(composite.Issues) > 0 {
		parts = append(parts, fmt.Sprintf("发现问题: %d 项", len(composite.Issues)))
	}
	composite.Summary = strings.Join(parts, " | ")

	return composite
}

// verdictLabel returns a human-readable label for a VerdictStatus.
func verdictLabel(s VerdictStatus) string {
	switch s {
	case StatusPass:
		return "✅ 通过"
	case StatusNeedsRevision:
		return "⚠️ 需修订"
	case StatusBlocked:
		return "❌ 阻塞（需人工介入）"
	default:
		return string(s)
	}
}

// FormatVerdict renders a CheckerVerdict as a Markdown string.
func FormatVerdict(v CheckerVerdict) string {
	var b strings.Builder
	b.WriteString("## Checker 复核结果\n\n")
	fmt.Fprintf(&b, "**角色**: %s\n", v.RoleID)
	fmt.Fprintf(&b, "**状态**: %s\n", verdictLabel(v.Status))
	if v.Score > 0 {
		fmt.Fprintf(&b, "**评分**: %.2f/1.0\n", v.Score)
	}
	if v.Summary != "" {
		fmt.Fprintf(&b, "**摘要**: %s\n", v.Summary)
	}

	if len(v.Issues) > 0 {
		b.WriteString("\n### 发现的问题\n\n")
		for i, iss := range v.Issues {
			sevLabel := "🔴"
			switch iss.Severity {
			case SeverityWarning:
				sevLabel = "🟡"
			case SeverityInfo:
				sevLabel = "🔵"
			}
			fmt.Fprintf(&b, "%d. %s **%s**", i+1, sevLabel, iss.Description)
			if iss.Location != "" {
				fmt.Fprintf(&b, " (位于: %s)", iss.Location)
			}
			b.WriteString("\n")
			if iss.Suggestion != "" {
				fmt.Fprintf(&b, "   - 建议: %s\n", iss.Suggestion)
			}
		}
	}

	if len(v.Suggestions) > 0 {
		b.WriteString("\n### 改进建议\n\n")
		for _, s := range v.Suggestions {
			fmt.Fprintf(&b, "- %s\n", s)
		}
	}

	return b.String()
}

// ParseVerdict attempts to parse a Markdown or JSON string into a CheckerVerdict.
// For now this returns a simple heuristic-based verdict; a full LLM-based
// parser can be added when the Checker sub-agent is implemented.
func ParseVerdict(text string) CheckerVerdict {
	lower := strings.ToLower(text)

	v := CheckerVerdict{
		Status:      StatusNeedsRevision,
		Score:       0.5,
		Issues:      make([]Issue, 0),
		Suggestions: make([]string, 0),
	}

	// Simple heuristic status detection
	if strings.Contains(lower, "status: pass") ||
		strings.Contains(lower, "✅ 通过") ||
		strings.Contains(lower, "全部通过") {
		v.Status = StatusPass
		v.Score = 0.85
	} else if strings.Contains(lower, "status: blocked") ||
		strings.Contains(lower, "❌ 阻塞") ||
		strings.Contains(lower, "严重问题") {
		v.Status = StatusBlocked
		v.Score = 0.2
	}

	return v
}
