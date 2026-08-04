package component

// ToolCard renders a single tool-call result as a compact status line,
// with optional collapsible diff content. It factors out the rendering
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
	// Border styles the status text when the status is neither success nor error.
	Border func(string) string
	// Success styles the status text when the status indicates success.
	Success func(string) string
	// Error styles the status text when the status indicates failure.
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
	// Seq is the tool call sequence number. 0 means no number displayed.
	Seq int
}

// RenderToolCard renders cfg to width using theme, returning the lines.
// The output uses a compact single-line header by default; the diff body is
// appended only when DiffText is provided.
func RenderToolCard(cfg ToolCardConfig, theme ToolCardTheme, width int64) []string {
	meta := ""
	if cfg.Duration > 0 {
		meta = " " + theme.Dim(fmt.Sprintf("(%s)", cfg.Duration.Round(time.Millisecond)))
	}
	seqStr := ""
	if cfg.Seq > 0 {
		seqStr = fmt.Sprintf("[%d] ", cfg.Seq)
	}

	statusStyle := theme.Border
	if strings.Contains(cfg.Status, "done") || strings.Contains(cfg.Status, "✓") {
		statusStyle = theme.Success
	} else if strings.Contains(cfg.Status, "failed") || strings.Contains(cfg.Status, "✗") {
		statusStyle = theme.Error
	}
	styledStatus := statusStyle(cfg.Status)

	if cfg.Collapsed {
		return []string{renderToolCardHeader(seqStr, "[+] ", cfg.Name, cfg.Status, meta, theme.Title, theme.Dim, width)}
	}

	head := renderToolCardHeader(seqStr, "", cfg.Name, styledStatus, meta, theme.Title, nil, width)
	lines := []string{head}

	if cfg.DiffText != "" {
		diffSrc := "```diff\n" + cfg.DiffText + "\n```"
		md := NewMarkdown(diffSrc)
		md.SetTheme(theme.MarkdownTheme)
		lines = append(lines, md.Render(width)...)
	}
	return lines
}

// renderToolCardHeader composes a single-line tool card header that never
// wraps: the status is truncated so the name, status, and optional meta all
// fit within width. titleFn styles the tool name; dimFn styles the status in
// collapsed mode (pass nil to keep the already-styled status).
func renderToolCardHeader(seq, marker, name, status, meta string, titleFn, dimFn func(string) string, width int64) string {
	if dimFn != nil {
		status = dimFn(status)
	}
	prefix := seq + marker + titleFn(name) + " "
	metaWidth := int64(0)
	if meta != "" {
		metaWidth = core.VisibleWidth(meta)
	}
	// 至少给 status 保留 1 列省略号空间：极窄窗口下状态也不应完全消失。
	available := width - core.VisibleWidth(prefix) - metaWidth
	if available < 1 {
		available = 1
	}
	status = core.TruncateToWidth(status, available, "…")
	line := prefix + status + meta
	// 兜底：极端窄窗下 prefix+meta 本身就可能超宽，整体截断到 width，
	// 保证返回行永不超宽，不依赖引擎层 normalizeLine 的二次兜底。
	if core.VisibleWidth(line) > width {
		line = core.TruncateToWidth(line, width, "…")
	}
	return core.PadToWidth(line, width)
}
