package component

// footer.go — Footer component for the TUI.
//
// The Footer is a single-line, always-visible bar at the bottom of the
// chat layout showing the most important keyboard shortcuts. It follows
// the "progressive disclosure" pattern: the footer shows 3-5 core
// bindings; the full key reference is available via `?` (KeyHelp overlay).
//
// Layout strategy (responsive):
//   - < 60 cols: [?] help
//   - 60-79 cols: help, clipboard, commands (first 3 groups)
//   - 80-159 cols: adds quit, fold (first 5 groups)
//   - ≥ 160 cols: shows all 6 registered groups (+ theme)

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
// Order matters: Footer.Render shows groups left→right, progressively trimmed
// by available-width breakpoints (<60 / 60-80 / 80-160 / ≥160 cols).
//
// Breakpoint visibility:
//
//	<60 cols   → group 1  only                 (? help)
//	60-80 cols → groups 1-3                    (? help, ctrl+shift+c/v paste, Ctrl+P cmd)
//	80-160 cols→ groups 1-5                    (+ quit, Alt+F fold)
//	≥160 cols  → all                           (+ theme toggle)
//
// Clipboard is in the top-3 so <80-col terminals (default 80×24 on macOS
// Terminal.app is right on the breakpoint; anything smaller is common in
// splits/TMux panes) still surface copy/paste hints — critical because
// ⌘C is swallowed by the terminal itself on Apple platforms.
func defaultGroups() []FooterGroup {
	return []FooterGroup{
		{
			Label: "help",
			Items: []FooterItem{
				{Key: "?", Desc: "help"},
			},
		},
		{
			Label: "clipboard",
			Items: []FooterItem{
				{Key: "C-S-C", Desc: "copy"},
				{Key: "C-S-V", Desc: "paste"},
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
	// Deep-copy the groups under the lock: RegisterGroup/ClearGroup mutate
	// the shared slice (and replace Items) under the write lock, so the
	// render loop must not iterate the live slice after releasing RLock
	// (P1-9 — previously a shallow header copy raced with RegisterGroup's
	// in-place element writes).
	f.mu.RLock()
	src := f.leftGroups
	groups := make([]FooterGroup, len(src))
	for i, g := range src {
		groups[i] = g
		groups[i].Items = append([]FooterItem(nil), g.Items...)
	}
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
		// Fall back to minimal: just show [?] help. visible[0] may be a group
		// registered with zero items, so scan for the first non-empty group —
		// indexing Items[0] blindly panics on an empty group.
		for _, g := range visible {
			if len(g.Items) > 0 {
				item := g.Items[0]
				line = pal.Accent.Render(item.Key) + " " + pal.Muted.Render(item.Desc)
				break
			}
		}
	}

	return []string{line}
}
