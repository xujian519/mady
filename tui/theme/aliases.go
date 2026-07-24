package theme

// aliases.go — Package-level Style globals synced from the active palette.
//
// These are convenience aliases for the most commonly used styles.
// They are updated by SyncPaletteGlobals whenever the theme changes.
// Consumers can use theme.UserStyle, theme.DimStyle etc. instead of
// theme.CurrentPalette().User etc.

import "sync/atomic"

var (
	// UserStyle is the style for user messages.
	UserStyle atomic.Pointer[Style]
	// DimStyle for dimmed/secondary text.
	DimStyle atomic.Pointer[Style]
	// SystemStyle for system/info messages.
	SystemStyle atomic.Pointer[Style]
	// ToolStyle for tool call names.
	ToolStyle atomic.Pointer[Style]
	// ToolBorder for tool card borders.
	ToolBorder atomic.Pointer[Style]
	// SuccessStyle for success indicators.
	SuccessStyle atomic.Pointer[Style]
	// ErrorStyle for error indicators.
	ErrorStyle atomic.Pointer[Style]
	// ThinkingStyle for reasoning/thinking text.
	ThinkingStyle atomic.Pointer[Style]
)

// syncAliases updates the package-level style aliases from the given palette.
// Called by SyncPaletteGlobals.
func syncAliases(p *Palette) {
	copyStyle := func(dst *atomic.Pointer[Style], src Style) {
		dst.Store(&src)
	}
	copyStyle(&UserStyle, p.User)
	copyStyle(&DimStyle, p.Dim)
	copyStyle(&SystemStyle, p.System)
	copyStyle(&ToolStyle, p.Tool)
	copyStyle(&ToolBorder, p.BorderMuted)
	copyStyle(&SuccessStyle, p.Success)
	copyStyle(&ErrorStyle, p.Error)
	copyStyle(&ThinkingStyle, p.Thinking)
}

// init is deliberately NOT placed here — palette.go:init() calls SyncPaletteGlobals
// which already calls syncAliases via BuildPalette.
