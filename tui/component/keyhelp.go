package component

// keyhelp.go — KeyHelp overlay component.
//
// Displays keyboard shortcuts as a reference overlay. Supports both
// static groups (for documented shortcuts) and dynamic extraction from
// the keybinding manager. Triggered by pressing "?" in the footer.
//
// 输出示例（宽 50 列）：
//
//	▎ 快捷键帮助
//	────────────────────────────────────
//	 导航 (Navigation)
//	   ↑↓ / j/k — 上下滚动
//	   / — 搜索对话
//	  ...
//	────────────────────────────────────
//	↑↓ scroll · Esc 关闭

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// KeyHelpGroup is a named category of keyboard shortcuts shown in the help panel.
type KeyHelpGroup struct {
	Label string
	Items []KeyHelpItem
}

// KeyHelpItem is a single shortcut description.
type KeyHelpItem struct {
	Keys string // e.g. "↑↓ / j/k"
	Desc string // e.g. "上下滚动"
}

// KeyHelp renders a keyboard shortcut reference panel.
// It can display both static groups and bindings from the keybinding manager.
type KeyHelp struct {
	mu          sync.RWMutex
	title       string
	km          *terminal.KeybindingsManager
	groups      []KeyHelpGroup // static groups (shown before dynamic bindings)
	filter      string         // text filter
	scroll      int64
	maxRows     int64 // 0 = no limit
	onClose     func()
	onRequestFn func()
}

// NewKeyHelp creates a KeyHelp overlay from the given keybinding manager.
// Static default groups are merged with dynamic bindings from the manager.
func NewKeyHelp(km *terminal.KeybindingsManager) *KeyHelp {
	if km == nil {
		km = terminal.NewKeybindingsManager(nil)
	}
	return &KeyHelp{
		title:  "快捷键帮助 (Keybindings)",
		km:     km,
		groups: defaultKeyHelpGroups(),
	}
}

// SetTitle overrides the panel title.
func (h *KeyHelp) SetTitle(t string) {
	h.mu.Lock()
	h.title = t
	h.mu.Unlock()
}

// SetFilter narrows displayed bindings to those matching the text.
func (h *KeyHelp) SetFilter(text string) {
	h.mu.Lock()
	h.filter = text
	h.mu.Unlock()
}

// SetMaxRows clamps the visible viewport.
func (h *KeyHelp) SetMaxRows(n int64) {
	h.mu.Lock()
	h.maxRows = n
	h.mu.Unlock()
}

// OnClose sets a callback invoked when the overlay is dismissed.
func (h *KeyHelp) OnClose(fn func()) {
	h.mu.Lock()
	h.onClose = fn
	h.mu.Unlock()
}

// SetOnClose is an alias for OnClose, matching the overlay pattern.
func (h *KeyHelp) SetOnClose(fn func()) { h.OnClose(fn) }

// Close dismisses the overlay.
func (h *KeyHelp) Close() {
	h.mu.RLock()
	fn := h.onClose
	h.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// ScrollBy adjusts the scroll offset.
func (h *KeyHelp) ScrollBy(n int64) {
	h.mu.Lock()
	h.scroll += n
	if h.scroll < 0 {
		h.scroll = 0
	}
	h.mu.Unlock()
	h.requestRender()
}

// SetOnRequestRender wires a render request callback.
func (h *KeyHelp) SetOnRequestRender(fn func()) {
	h.mu.Lock()
	h.onRequestFn = fn
	h.mu.Unlock()
}

func (h *KeyHelp) requestRender() {
	h.mu.RLock()
	fn := h.onRequestFn
	h.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (h *KeyHelp) Invalidate() {}

func (h *KeyHelp) Render(width int64) []string {
	h.mu.RLock()
	title := h.title
	groups := h.groups
	filter := h.filter
	scroll := h.scroll
	maxRows := h.maxRows
	h.mu.RUnlock()

	if width < 10 {
		width = 10
	}

	pal := theme.CurrentPalette()

	// Collect lines
	var lines []string

	// Title
	lines = append(lines, core.PadToWidth(pal.Accent.Render("▎ "+title), width))
	lines = append(lines, pal.BorderMuted.Render(strings.Repeat("─", int(width))))

	// Static groups
	for _, g := range groups {
		shown := false
		if filter != "" {
			// Filter mode: only show items matching filter
			for _, item := range g.Items {
				if !strings.Contains(strings.ToLower(item.Keys+item.Desc), strings.ToLower(filter)) {
					continue
				}
				if !shown {
					lines = append(lines, "  "+pal.Dim.Render(g.Label))
					shown = true
				}
				itemLine := "   " + pal.Accent.Render(item.Keys) + " — " + pal.Muted.Render(item.Desc)
				itemLine = core.TruncateToWidth(itemLine, width-2, "…")
				lines = append(lines, core.PadToWidth(itemLine, width))
			}
		} else {
			lines = append(lines, "  "+pal.Dim.Render(g.Label))
			for _, item := range g.Items {
				itemLine := "   " + pal.Accent.Render(item.Keys) + " — " + pal.Muted.Render(item.Desc)
				itemLine = core.TruncateToWidth(itemLine, width-2, "…")
				lines = append(lines, core.PadToWidth(itemLine, width))
			}
		}
		lines = append(lines, pal.BorderMuted.Render(strings.Repeat("─", int(width))))
	}

	// Dynamic bindings from keybinding manager
	allDefIDs := h.km.All()
	if len(allDefIDs) > 0 && filter == "" {
		// Sort by ID
		ids := make([]string, 0, len(allDefIDs))
		for id := range allDefIDs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		// Only show first 30 to avoid overwhelming the overlay
		maxShown := 30
		if len(ids) > maxShown {
			ids = ids[:maxShown]
		}
		lines = append(lines, "  "+pal.Dim.Render(fmt.Sprintf("其他绑定 (%d)", len(allDefIDs))))
		for _, id := range ids {
			keys := allDefIDs[id]
			def := h.km.Definition(id)
			desc := def.Description
			if desc == "" {
				desc = id
			}
			keysStr := joinKeyIDs(keys)
			itemLine := "   " + pal.Accent.Render(keysStr) + " — " + pal.Muted.Render(desc)
			itemLine = core.TruncateToWidth(itemLine, width-2, "…")
			lines = append(lines, core.PadToWidth(itemLine, width))
		}
		lines = append(lines, pal.BorderMuted.Render(strings.Repeat("─", int(width))))
	} else if filter != "" && len(allDefIDs) > 0 {
		// Show matching dynamic bindings
		for id, keys := range allDefIDs {
			def := h.km.Definition(id)
			desc := def.Description
			if desc == "" {
				desc = id
			}
			if !strings.Contains(strings.ToLower(id+desc), strings.ToLower(filter)) {
				continue
			}
			keysStr := joinKeyIDs(keys)
			itemLine := "   " + pal.Accent.Render(keysStr) + " — " + pal.Muted.Render(desc)
			itemLine = core.TruncateToWidth(itemLine, width-2, "…")
			lines = append(lines, core.PadToWidth(itemLine, width))
		}
	}

	// Footer hint
	if filter != "" {
		lines = append(lines, core.PadToWidth(pal.Dim.Render(fmt.Sprintf("  过滤: %s  ↑↓ 翻页  Esc 关闭", filter)), width))
	} else {
		lines = append(lines, core.PadToWidth(pal.Dim.Render("  ↑↓ 翻页  Esc 关闭"), width))
	}

	// Apply scroll offset BEFORE viewport clipping so scroll effectively
	// pans through the full content, then clamps to maxRows.
	if scroll > 0 {
		if scroll >= int64(len(lines)) {
			scroll = int64(len(lines)) - 1
		}
		if scroll < 0 {
			scroll = 0
		}
		lines = lines[scroll:]
	}

	// Viewport clipping: after scroll, clamp to maxRows.
	if maxRows > 0 && int64(len(lines)) > maxRows {
		lines = lines[:maxRows]
	}

	return lines
}

func (h *KeyHelp) Update(msg core.Msg) core.Cmd {
	switch m := msg.(type) {
	case core.KeyMsg:
		for _, k := range terminal.ParseKeys(m.Data, m.KittyFlags) {
			name := strings.ToLower(k.Name)
			switch name {
			case "escape":
				h.Close()
				return nil
			case "up", "k":
				h.ScrollBy(-1)
			case "down", "j":
				h.ScrollBy(1)
			case "pageup":
				h.ScrollBy(-10)
			case "pagedown":
				h.ScrollBy(10)
			case "home":
				h.mu.Lock()
				h.scroll = 0
				h.mu.Unlock()
				h.requestRender()
			case "end":
				h.mu.Lock()
				h.scroll = 1 << 30
				h.mu.Unlock()
				h.requestRender()
			}
		}
	}
	return nil
}

// defaultKeyHelpGroups returns the complete set of keyboard shortcuts
// organized by category for the chat TUI.
func defaultKeyHelpGroups() []KeyHelpGroup {
	return []KeyHelpGroup{
		{
			Label: "导航 (Navigation)",
			Items: []KeyHelpItem{
				{Keys: "↑↓ / j/k", Desc: "上下滚动"},
				{Keys: "PgUp/PgDn", Desc: "翻页"},
				{Keys: "Home/End", Desc: "顶部/尾部"},
				{Keys: "Alt+↑/↓", Desc: "逐行滚动"},
				{Keys: "/", Desc: "搜索对话"},
				{Keys: "Tab", Desc: "切换焦点"},
			},
		},
		{
			Label: "编辑 (Editor)",
			Items: []KeyHelpItem{
				{Keys: "Enter", Desc: "发送消息"},
				{Keys: "Ctrl+A/E", Desc: "行首/行尾"},
				{Keys: "Ctrl+B/F", Desc: "左移/右移"},
				{Keys: "Ctrl+U/K", Desc: "删至行首/尾"},
				{Keys: "Ctrl+Y", Desc: "召回上条输入"},
				{Keys: "Ctrl+W", Desc: "删除前一词"},
				{Keys: "Esc", Desc: "弹出自动补全"},
			},
		},
		{
			Label: "折叠 (Collapse)",
			Items: []KeyHelpItem{
				{Keys: "Alt+F", Desc: "切换折叠/展开"},
				{Keys: "鼠标点击", Desc: "切换工具组/思维段"},
			},
		},
		{
			Label: "系统 (System)",
			Items: []KeyHelpItem{
				{Keys: "Ctrl+P", Desc: "命令面板"},
				{Keys: "Ctrl+C / Ctrl+Q", Desc: "退出"},
				{Keys: "Ctrl+T", Desc: "任务面板"},
				{Keys: "Ctrl+Shift+D", Desc: "调试面板"},
				{Keys: "Ctrl+Alt+T", Desc: "切换主题"},
				{Keys: "F2", Desc: "鼠标穿透模式"},
			},
		},
		{
			Label: "专业操作 (Domain)",
			Items: []KeyHelpItem{
				{Keys: "s (展开时)", Desc: "系统状态"},
				{Keys: "e (展开时)", Desc: "证据浮层"},
				{Keys: "p/b/f", Desc: "审阅门控操作"},
			},
		},
	}
}

// Ensure KeyHelp implements core.Component.
var _ core.Component = (*KeyHelp)(nil)

// joinKeyIDs joins KeyID values into a readable display string.
func joinKeyIDs(keys []terminal.KeyID) string {
	strs := make([]string, len(keys))
	copy(strs, keys)
	return strings.Join(strs, " / ")
}
