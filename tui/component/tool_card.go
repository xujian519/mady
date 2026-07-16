package component

// ToolCard renders a single tool-call result as a left-bar + title + status
// block, with optional collapsible diff content. It factors out the rendering
// ChatHistory previously inlined for RoleTool / diff messages, so tool
// results, diffs, and (future) reasoning blocks share one visual treatment
// and one click-to-toggle contract.
//
// ToolCard is a pure renderer: it produces []string and owns no mutable
// state. Collapsed/expanded state is driven by the caller (ChatHistory stores
// it on the ChatMessage), so mouse hit-testing continues to work via
// ChatHistory's cachedMsgRanges exactly as before.

import (
	"fmt"
	"strings"
	"time"

	"github.com/xujian519/mady/tui/core"
)

// ToolCardTheme carries the string-styling functions ToolCard needs. It is
// intentionally a flat bag of funcs (not the full ChatHistoryTheme) so this
// file stays in package component and does not depend on package chat.
// Callers construct it from their ChatHistoryTheme.
type ToolCardTheme struct {
	// Border styles the left bar when the status is neither success nor error.
	Border func(string) string
	// Success styles the left bar when the status indicates success.
	Success func(string) string
	// Error styles the left bar when the status indicates failure.
	Error func(string) string
	// Title styles the tool name.
	Title func(string) string
	// Dim styles secondary text (summary, duration).
	Dim func(string) string
	// MarkdownTheme styles the diff block body. Required when DiffText != "".
	MarkdownTheme MarkdownTheme
}

// ToolCardConfig describes one tool-call card to render.
type ToolCardConfig struct {
	// Name is the tool name shown after the bar (e.g. "edit_block").
	Name string
	// Status is the result text (e.g. "✓ done" or "✗ failed: ..."). It drives
	// the bar color via the theme: "done"/"✓" → Success, "failed"/"✗" →
	// Error, otherwise Border.
	Status string
	// Duration, when > 0, is shown as "(1.2s)" after the status.
	Duration time.Duration
	// DiffText, when non-empty, is rendered as a fenced ```diff block beneath
	// the header (line numbers + +/- coloring via Markdown).
	DiffText string
	// Collapsed controls whether the body is shown. When true, only a
	// one-line summary "[+] <name> <status>" is rendered.
	Collapsed bool
}

// RenderToolCard renders cfg to width using theme, returning the lines.
// The output matches what ChatHistory previously produced inline for RoleTool
// messages, so callers can adopt it without visual change.
func RenderToolCard(cfg ToolCardConfig, theme ToolCardTheme, width int64) []string {
	meta := ""
	if cfg.Duration > 0 {
		meta = " " + theme.Dim(fmt.Sprintf("(%s)", cfg.Duration.Round(time.Millisecond)))
	}

	barColor := theme.Border
	if strings.Contains(cfg.Status, "done") || strings.Contains(cfg.Status, "✓") {
		barColor = theme.Success
	} else if strings.Contains(cfg.Status, "failed") || strings.Contains(cfg.Status, "✗") {
		barColor = theme.Error
	}
	bar := barColor("▌")

	if cfg.Collapsed {
		summary := cfg.Status
		if len(summary) > 120 {
			summary = summary[:117] + "..."
		}
		head := bar + " [+] " + theme.Title(cfg.Name) + " " + theme.Dim(summary)
		return core.WrapAnsi(head, width)
	}

	head := bar + " " + theme.Title(cfg.Name) + " " + cfg.Status + meta
	lines := core.WrapAnsi(head, width)

	if cfg.DiffText != "" {
		diffSrc := "```diff\n" + cfg.DiffText + "\n```"
		md := NewMarkdown(diffSrc)
		md.SetTheme(theme.MarkdownTheme)
		lines = append(lines, md.Render(width)...)
	}
	return lines
}
