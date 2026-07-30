package layout

// breakpoint.go — Responsive layout breakpoints for adaptive terminal layouts.
//
// Three breakpoints match common terminal widths:
//   - Compact  (< 80 cols):  single-column, all panels as overlays
//   - Standard (80–159 cols): default multi-panel layout
//   - Wide     (≥ 160 cols):  adds side panels for context

// LayoutBreakpoint identifies the terminal-width regime for responsive layout.
type LayoutBreakpoint int

const (
	LayoutCompact  LayoutBreakpoint = iota // < 80 columns — narrow terminal
	LayoutStandard                         // 80–159 columns — standard layout
	LayoutWide                             // ≥ 160 columns — extra side panels
)

// String returns a human-readable label for the breakpoint.
func (b LayoutBreakpoint) String() string {
	switch b {
	case LayoutCompact:
		return "compact"
	case LayoutStandard:
		return "standard"
	case LayoutWide:
		return "wide"
	default:
		return "unknown"
	}
}

// DetectLayoutBreakpoint returns the breakpoint for a given terminal width.
// Width is measured in visible columns (characters).
func DetectLayoutBreakpoint(cols int64) LayoutBreakpoint {
	switch {
	case cols >= 160:
		return LayoutWide
	case cols >= 80:
		return LayoutStandard
	default:
		return LayoutCompact
	}
}
