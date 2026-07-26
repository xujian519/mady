package component

// confidence_bar.go — shared confidence bar renderer.
//
// Prior to this file, three nearly identical confidence bar implementations
// existed in conclusion_card.go, review_gate.go, and approval_card.go. This
// shared function replaces all three, with callers providing only the color
// functions and label preferences.

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/tui/core"
)

// ConfidenceBarColors provides optional color functions for the confidence bar.
// All fields may be nil (default: no coloring).
type ConfidenceBarColors struct {
	High   func(string) string // confidence ≥ 67%
	Medium func(string) string // confidence 34–66%
	Low    func(string) string // confidence < 34%
}

// RenderConfidenceBar draws a 10-cell confidence bar with optional coloring
// and optional level label.
//
// The bar is formatted as:
//
//	"  置信度: ████░░░░░░ 50%"          (no coloring, no level)
//	"  置信度: ████████░░ 80% (高)"     (colored, with level)
func RenderConfidenceBar(conf float64, colors *ConfidenceBarColors, width int64, showLevel bool) string {
	const cells = 10
	pct := int(conf * 100)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := (pct * cells) / 100

	bar := strings.Repeat("█", filled) + strings.Repeat("░", cells-filled)

	if colors != nil {
		var colorize func(string) string
		switch {
		case pct >= 67:
			colorize = colors.High
		case pct >= 34:
			colorize = colors.Medium
		default:
			colorize = colors.Low
		}
		if colorize != nil {
			bar = colorize(bar)
		}
	}

	result := "  置信度: " + bar
	if showLevel {
		var level string
		switch {
		case pct >= 67:
			level = "高"
		case pct >= 34:
			level = "中"
		default:
			level = "低"
		}
		result += fmt.Sprintf(" %d%% (%s)", pct, level)
	} else {
		result += fmt.Sprintf(" %d%%", pct)
	}

	return core.PadToWidth(result, width)
}
