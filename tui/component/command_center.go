package component

// command_center.go implements a Ctrl+P command palette overlay that provides
// fuzzy search across all registered slash commands with category grouping,
// status indicators, and keyboard-driven execution.

import (
	"fmt"
	"strings"
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// CommandItem describes one entry in the command center.
type CommandItem struct {
	Name        string // e.g. "plan"
	Label       string // e.g. "/plan [on|off]"
	Category    string // grouping key
	Description string
	Status      string // current state, e.g. "开启" / "关闭" / "" (hidden)
	Available   bool
	Reason      string // why unavailable, shown dimmed
}

// CommandCenter is a full-screen overlay that lets users search and execute commands.
type CommandCenter struct {
	mu       sync.RWMutex
	items    []CommandItem
	filtered []CommandItem
	filter   string
	cursor   int
	km       *terminal.KeybindingsManager
	focused  bool

	onExecute func(item CommandItem)
	onClose   func()
}

// NewCommandCenter creates a command center with the given items.
func NewCommandCenter(items []CommandItem) *CommandCenter {
	return &CommandCenter{
		items:    items,
		filtered: items,
		km:       terminal.GetGlobalKeybindings(),
	}
}

// SetItems replaces the command list.
func (c *CommandCenter) SetItems(items []CommandItem) {
	c.mu.Lock()
	c.items = items
	c.applyFilterLocked()
	c.clampCursorLocked()
	c.mu.Unlock()
}

// OnExecute sets the callback when a command is selected (Enter).
func (c *CommandCenter) OnExecute(fn func(CommandItem)) {
	c.mu.Lock()
	c.onExecute = fn
	c.mu.Unlock()
}

// OnClose sets the callback when dismissed (Esc).
func (c *CommandCenter) OnClose(fn func()) {
	c.mu.Lock()
	c.onClose = fn
	c.mu.Unlock()
}

// SetFocused / IsFocused implement Focusable.
func (c *CommandCenter) SetFocused(on bool) { c.mu.Lock(); c.focused = on; c.mu.Unlock() }
func (c *CommandCenter) IsFocused() bool    { c.mu.RLock(); defer c.mu.RUnlock(); return c.focused }

// Invalidate is a no-op.
func (c *CommandCenter) Invalidate() {}

// Render draws the command palette: a search bar + scrollable filtered list.
func (c *CommandCenter) Render(width int64) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	p := theme.CurrentPalette()
	if width < 1 {
		width = 1
	}
	maxVisible := int64(12)

	var out []string

	// Title bar
	title := p.Accent.Render("▎ 命令中心") + p.Dim.Render("  —  输入关键词筛选，Enter 执行，Esc 关闭")
	out = append(out, core.PadToWidth(title, width))

	// Search bar
	searchText := c.filter
	if searchText == "" {
		searchText = p.Dim.Render("输入搜索…")
	} else {
		searchText = p.Assistant.Render(searchText)
	}
	out = append(out, core.PadToWidth("  > "+searchText, width))
	out = append(out, p.Dim.Render(strings.Repeat("─", int(width))))

	// Items with group headers
	if len(c.filtered) == 0 {
		out = append(out, "", core.PadToWidth(p.Dim.Render("  无匹配命令"), width))
		return out
	}

	start := int64(c.cursor) - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > int64(len(c.filtered)) {
		end = int64(len(c.filtered))
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	lastGroup := ""
	for i := start; i < end; i++ {
		item := c.filtered[i]
		// Group header
		if item.Category != "" && item.Category != lastGroup {
			lastGroup = item.Category
			out = append(out, p.Dim.Render("  ▾ "+lastGroup))
		}
		// Item line
		prefix := "  "
		style := func(s string) string { return s }
		if i == int64(c.cursor) {
			prefix = "▸ "
			style = p.SelectHighlight.Render
		}
		line := prefix + style(item.Label)
		if item.Description != "" {
			line += "  " + p.Dim.Render(item.Description)
		}
		if item.Status != "" {
			line += "  " + p.Accent.Render(item.Status)
		}
		if !item.Available && item.Reason != "" {
			line += "  " + p.Dim.Render("("+item.Reason+")")
		}
		out = append(out, core.PadToWidth(core.TruncateToWidth(line, width, "…"), width))
	}

	// Footer
	info := fmt.Sprintf("  [%d/%d]", c.cursor+1, len(c.filtered))
	if c.filter != "" {
		info += fmt.Sprintf("  匹配: %d", len(c.filtered))
	}
	out = append(out, p.Dim.Render(info))

	return out
}

// Update processes key input for the command center.
func (c *CommandCenter) Update(msg core.Msg) core.Cmd {
	switch m := msg.(type) {
	case core.KeyMsg:
		c.processKey(m.Data)
	}
	return nil
}

func (c *CommandCenter) processKey(data string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch {
	case c.km.Matches(data, "tui.editor.cursorUp") || c.km.Matches(data, "tui.select.up"):
		if c.cursor > 0 {
			c.cursor--
		}
	case c.km.Matches(data, "tui.editor.cursorDown") || c.km.Matches(data, "tui.select.down"):
		if c.cursor < len(c.filtered)-1 {
			c.cursor++
		}
	case c.km.Matches(data, "tui.select.confirm"):
		if c.cursor >= 0 && c.cursor < len(c.filtered) {
			item := c.filtered[c.cursor]
			if c.onExecute != nil {
				c.mu.Unlock()
				c.onExecute(item)
				c.mu.Lock()
			}
		}
	case c.km.Matches(data, "tui.editor.cancel"):
		if c.onClose != nil {
			c.mu.Unlock()
			c.onClose()
			c.mu.Lock()
		}
	case len(data) == 1 && data[0] >= 32 && data[0] < 127:
		// Printable character: append to filter
		c.filter += data
		c.applyFilterLocked()
		c.cursor = 0
	case c.km.Matches(data, "tui.editor.backspace"):
		if len(c.filter) > 0 {
			c.filter = c.filter[:len(c.filter)-1]
			c.applyFilterLocked()
			c.cursor = 0
		}
	}
}

func (c *CommandCenter) applyFilterLocked() {
	if c.filter == "" {
		c.filtered = c.items
		return
	}
	lower := strings.ToLower(c.filter)
	var out []CommandItem
	for _, it := range c.items {
		if strings.Contains(strings.ToLower(it.Name), lower) ||
			strings.Contains(strings.ToLower(it.Description), lower) ||
			strings.Contains(strings.ToLower(it.Label), lower) {
			out = append(out, it)
		}
	}
	c.filtered = out
}

func (c *CommandCenter) clampCursorLocked() {
	if c.cursor < 0 {
		c.cursor = 0
	}
	if n := len(c.filtered); c.cursor >= n {
		if n == 0 {
			c.cursor = 0
		} else {
			c.cursor = n - 1
		}
	}
}
