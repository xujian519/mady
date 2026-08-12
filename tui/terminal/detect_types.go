package terminal

// ---------------------------------------------------------------------------

// TerminalBrand identifies the terminal emulator in use.
// The detection is from environment variables; see DetectTerminalContext.
type TerminalBrand int

const (
	BrandUnknown TerminalBrand = iota
	BrandKitty
	BrandGhostty
	BrandITerm2
	BrandAppleTerminal
	BrandVsCode
	BrandCursor
	BrandWindsurf
	BrandWindowsTerminal
	BrandWezTerm
	BrandAlacritty
	BrandFoot
	BrandVte       // GNOME Terminal, kgx, Tilix, etc.
	BrandJetBrains // IntelliJ, PhpStorm, etc. (JediTerm)
	BrandWarp
	BrandRio
	BrandZed
	BrandGrokDesktop
	BrandOtty
	BrandContour
	BrandHyper
)

var brandNames = map[TerminalBrand]string{
	BrandUnknown:         "Unknown",
	BrandKitty:           "Kitty",
	BrandGhostty:         "Ghostty",
	BrandITerm2:          "iTerm2",
	BrandAppleTerminal:   "Apple Terminal",
	BrandVsCode:          "VS Code",
	BrandCursor:          "Cursor",
	BrandWindsurf:        "Windsurf",
	BrandWindowsTerminal: "Windows Terminal",
	BrandWezTerm:         "WezTerm",
	BrandAlacritty:       "Alacritty",
	BrandFoot:            "foot",
	BrandVte:             "VTE",
	BrandJetBrains:       "JetBrains",
	BrandWarp:            "Warp",
	BrandRio:             "Rio",
	BrandZed:             "Zed",
	BrandGrokDesktop:     "Grok Desktop",
	BrandOtty:            "Otty",
	BrandContour:         "Contour",
	BrandHyper:           "Hyper",
}

func (b TerminalBrand) String() string {
	if s, ok := brandNames[b]; ok {
		return s
	}
	return "Unknown"
}

// IsVSCODECodeFamily returns true for VS Code and its IDE forks (Cursor, Windsurf, Zed).
func (b TerminalBrand) IsVSCODECodeFamily() bool {
	switch b {
	case BrandVsCode, BrandCursor, BrandWindsurf, BrandZed:
		return true
	}
	return false
}

// IsVTEVteBased returns true for VTE-based terminals (GNOME Terminal, Terminator, etc.).
func (b TerminalBrand) IsVTEVteBased() bool {
	return b == BrandVte
}

// ---------------------------------------------------------------------------
// Multiplexer enumeration
// ---------------------------------------------------------------------------

// MultiplexerKind identifies the terminal multiplexer wrapping the session.
type MultiplexerKind int

const (
	MuxUndetected MultiplexerKind = iota
	MuxTmux
	MuxScreen // GNU screen
	MuxZellij
	MuxCmux // Ghostty cmux
)

var muxNames = map[MultiplexerKind]string{
	MuxUndetected: "none",
	MuxTmux:       "tmux",
	MuxScreen:     "screen",
	MuxZellij:     "zellij",
	MuxCmux:       "cmux",
}

func (m MultiplexerKind) String() string {
	if s, ok := muxNames[m]; ok {
		return s
	}
	return "none"
}

// InterceptsCSIQueries returns true when the multiplexer intercepts DA/DECRPM
// queries instead of passing them through to the outer terminal.
func (m MultiplexerKind) InterceptsCSIQueries() bool {
	switch m {
	case MuxTmux, MuxScreen, MuxZellij:
		return true
	}
	return false
}

// InterceptsOSC52 returns true when the multiplexer handles OSC 52 clipboard
// sequences itself rather than forwarding them. When intercepted, the pager
// should use the multiplexer's clipboard route.
func (m MultiplexerKind) InterceptsOSC52() bool {
	return m == MuxTmux || m == MuxScreen
}
