package chat

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

func (a *ChatApp) TerminalSize() (cols, rows int64) {
	if a.host != nil {
		return a.host.TerminalSize()
	}
	return 80, 24
}

type layoutHost interface {
	TerminalSize() (cols, rows int64)
}

// charCounter is implemented by the Editor component so the wrapping
// editorFrame can render a non-intrusive character counter in the bottom
// border without intruding into the editor's own layout budget.
type charCounter interface {
	// CharCount returns the current buffer length in runes (not bytes).
	CharCount() int64
}

// editorFrame wraps the editor with a rounded brand border so the input
// area reads as a distinct panel below the chat history. It exists so the
// editor border can participate in the declarative Flex layout.
//
// Visual style (Reasonix-inspired brand box):
//
//	╭─ 输入 (Enter 发送 · Shift+Enter 换行) ─────╮
//	│ ❯ hello world                             │
//	╰─────────────────────── N chars · ↑↓历史╯
type editorFrame struct {
	editor core.Component
}

func (f *editorFrame) Render(width int64) []string {
	pal := theme.CurrentPalette()
	bfn := pal.BorderAccent.Render
	if width < 4 {
		width = 4
	}
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	lines := f.editor.Render(inner)

	counter := ""
	if cc, ok := f.editor.(charCounter); ok {
		if n := cc.CharCount(); n > 0 {
			counter = fmt.Sprintf(" %d 字符 ", n)
			counter = pal.Dim.Render(counter)
		}
	}
	counterW := int64(0)
	if counter != "" {
		counterW = core.VisibleWidth(counter)
	}

	title := " 输入 (Enter 发送 · Shift+Enter 换行) "
	tw := core.VisibleWidth(title)
	if tw > inner {
		title = core.TruncateToWidth(title, inner, "…")
		tw = core.VisibleWidth(title)
	}
	headRune := "╭"
	diff := inner - tw
	if diff < 0 {
		diff = 0
	}
	topBody := title + strings.Repeat("─", int(diff))
	top := bfn(headRune) + topBody + bfn("╮")

	out := make([]string, 0, len(lines)+2)
	out = append(out, top)

	v := bfn("│")
	for _, ln := range lines {
		body := core.PadToWidth(ln, inner)
		out = append(out, v+body+v)
	}

	// Bottom border: counter on the far left when present, hint on the far
	// right, with ─ fill in between. If too narrow, drop hint first, then
	// counter, finally fall back to plain bar.
	hint := " ↑↓历史 · Enter发送 · Shift+Enter换行 "
	hintW := core.VisibleWidth(hint)
	dimHint := pal.Dim.Render(hint)

	var bottom string
	switch {
	case counter != "" && inner >= counterW+hintW+6:
		fill := inner - counterW - hintW
		if fill < 0 {
			fill = 0
		}
		bottom = bfn("╰") + counter + strings.Repeat("─", int(fill)) + dimHint + bfn("╯")
	case inner >= hintW+4:
		fill := inner - hintW
		if fill < 0 {
			fill = 0
		}
		bottom = bfn("╰") + strings.Repeat("─", int(fill)) + dimHint + bfn("╯")
	case counter != "" && inner >= counterW+4:
		fill := inner - counterW
		if fill < 0 {
			fill = 0
		}
		bottom = bfn("╰") + counter + strings.Repeat("─", int(fill)) + bfn("╯")
	default:
		bottom = bfn("╰") + strings.Repeat("─", int(inner)) + bfn("╯")
	}
	bottom = core.TruncateToWidth(bottom, width, "…")
	out = append(out, bottom)
	return out
}

func (f *editorFrame) Invalidate() {}

// todoBar is a compact, always-visible task anchor mounted in the layout
// flow directly above the editor frame (Reasonix todoArgs pattern). It
// surfaces the top 1-3 in-flight tasks so the user sees them while typing
// without opening the full TodoPanel overlay (Ctrl+T).
type todoBar struct {
	app *ChatApp
}

func (t *todoBar) Render(width int64) []string {
	if t.app == nil {
		return nil
	}
	items := t.app.collectTodoItems()
	var active []component.TodoItem
collectLoop:
	for _, it := range items {
		switch it.Status {
		case "pending", "in_progress":
			active = append(active, it)
			if len(active) == 3 {
				break collectLoop
			}
		}
	}
	if len(active) == 0 {
		return nil
	}
	if width < 20 {
		width = 20
	}
	const prefixW = 4 // "│ " + icon + " "
	contentW := width - prefixW - 1
	if contentW < 8 {
		contentW = 8
	}

	pal := theme.CurrentPalette()
	header := fmt.Sprintf("📋 %d 待办  Ctrl+T 全览", len(active))
	lines := make([]string, 0, len(active)+2)

	// Top border: ╭ ─ header ─ ╮
	head := pal.BorderAccent.Render("╭ ") + pal.Accent.Render(header) + " "
	headRemain := width - core.VisibleWidth(head) - 1
	if headRemain < 0 {
		headRemain = 0
	}
	head += pal.BorderAccent.Render(strings.Repeat("─", int(headRemain)) + "╮")
	lines = append(lines, core.TruncateToWidth(head, width, "…"))

	// Body rows: │ ○ content [pri]  │
	v := pal.BorderAccent.Render("│")
	for _, it := range active {
		icon := "○"
		var bodyStyle func(string) string
		switch it.Status {
		case "in_progress":
			icon = "◐"
			bodyStyle = pal.Accent.Render
		default:
			bodyStyle = pal.Assistant.Render
		}
		pri := ""
		if it.Priority != "" {
			pri = " [" + it.Priority + "]"
		}
		content := strings.ReplaceAll(it.Content, "\n", " ")
		maxCW := contentW - core.VisibleWidth(pri)
		if maxCW < 1 {
			maxCW = 1
		}
		content = core.TruncateToWidth(content, maxCW, "…")
		row := v + " " + icon + " " + bodyStyle(content) + pal.Dim.Render(pri)
		row = core.TruncateToWidth(row, width-1, "…")
		fill := width - core.VisibleWidth(row) - 1
		if fill < 0 {
			fill = 0
		}
		row = row + strings.Repeat(" ", int(fill)) + v
		lines = append(lines, row)
	}

	// Bottom border: ╰ ─ ╯
	tail := pal.BorderAccent.Render("╰" + strings.Repeat("─", int(width)-2) + "╯")
	lines = append(lines, core.TruncateToWidth(tail, width, "…"))
	return lines
}

func (t *todoBar) Invalidate() {}
