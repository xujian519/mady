package component

// footer.go — Footer component for the TUI.
//
// The Footer is a single-line, always-visible bar at the bottom of the
// chat layout showing the most important keyboard shortcuts. It follows
// the "progressive disclosure" pattern: the footer shows 3-5 core
// bindings; the full key reference is available via `?` (KeyHelp overlay).
//
// Layout strategy (responsive):
//   - < 80 cols: [?] help · [Ctrl+P] cmd · [Ctrl+C] quit
//   - 80-159 cols: adds [/] search · [Tab] focus
//   - ≥ 160 cols: shows all 5-7 registered groups

import (
	"strings"
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// FooterItem represents a single keyboard shortcut hint.
type FooterItem struct {
	Key  string // key name, e.g. "?"
	Desc string // description, e.g. "help"
}

// FooterGroup is a named group of related shortcuts.
type FooterGroup struct {
	Label string // group label (optional, e.g. "search")
	Items []FooterItem
}

// Footer is a single-line shortcut hint bar rendered at the bottom of the
// chat layout. It implements core.Component.
type Footer struct {
	mu         sync.RWMutex
	leftGroups []FooterGroup // left-aligned groups
	compact    bool          // true = < 80 cols
}

// NewFooter creates a Footer with the default shortcut groups.
func NewFooter() *Footer {
	return &Footer{
		leftGroups: defaultGroups(),
	}
}

// defaultGroups returns the standard shortcut groups.
func defaultGroups() []FooterGroup {
	return []FooterGroup{
		{
			Label: "help",
			Items: []FooterItem{
				{Key: "?", Desc: "help"},
			},
		},
		{
			Label: "fold",
			Items: []FooterItem{
				{Key: "Alt+F", Desc: "fold"},
			},
		},
		{
			Label: "theme",
			Items: []FooterItem{
				{Key: "Ctrl+Alt+T", Desc: "theme"},
			},
		},
		{
			Label: "commands",
			Items: []FooterItem{
				{Key: "Ctrl+P", Desc: "cmd"},
			},
		},
		{
			Label: "quit",
			Items: []FooterItem{
				{Key: "Ctrl+C", Desc: "quit"},
			},
		},
	}
}

// RegisterGroup adds or replaces a named group. Items are shown in order.
func (f *Footer) RegisterGroup(name string, items ...FooterItem) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, g := range f.leftGroups {
		if g.Label == name {
			f.leftGroups[i].Items = items
			return
		}
	}
	f.leftGroups = append(f.leftGroups, FooterGroup{Label: name, Items: items})
}

// ClearGroup removes a named group.
func (f *Footer) ClearGroup(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, g := range f.leftGroups {
		if g.Label == name {
			f.leftGroups = append(f.leftGroups[:i], f.leftGroups[i+1:]...)
			return
		}
	}
}

// SetCompact controls whether the footer uses the compact (<80 cols) layout.
func (f *Footer) SetCompact(v bool) {
	f.mu.Lock()
	f.compact = v
	f.mu.Unlock()
}

func (f *Footer) Invalidate() {}

// Render produces a single-line footer with keyboard shortcut hints.
func (f *Footer) Render(width int64) []string {
	f.mu.RLock()
	groups := f.leftGroups
	compact := f.compact
	f.mu.RUnlock()

	if len(groups) == 0 {
		return []string{""}
	}

	pal := theme.CurrentPalette()

	// Select groups based on available width.
	var visible []FooterGroup
	if compact || width < 80 {
		if width < 60 {
			// Ultra-narrow: only the first group's first item (? help).
			if len(groups) > 0 {
				visible = groups[:1]
			}
		} else {
			// Compact: only first 3 groups (help, cmd, quit).
			// width < 80 is a safety net in case SetCompact hasn't been called yet.
			if len(groups) > 3 {
				visible = groups[:3]
			} else {
				visible = groups
			}
		}
	} else if width < 160 {
		// Standard: show up to 5 groups.
		if len(groups) > 5 {
			visible = groups[:5]
		} else {
			visible = groups
		}
	} else {
		visible = groups
	}

	// Render groups into a line.
	var parts []string
	for _, g := range visible {
		for _, item := range g.Items {
			keyStr := pal.Accent.Render(item.Key)
			descStr := pal.Muted.Render(item.Desc)
			parts = append(parts, keyStr+" "+descStr)
		}
	}

	line := strings.Join(parts, " · ")

	// If the rendered line exceeds width, trim from the right.
	if core.VisibleWidth(line) > width {
		// Fall back to minimal: just show [?] help
		if len(visible) > 1 {
			item := visible[0].Items[0]
			line = pal.Accent.Render(item.Key) + " " + pal.Muted.Render(item.Desc)
		}
	}

	return []string{line}
}
