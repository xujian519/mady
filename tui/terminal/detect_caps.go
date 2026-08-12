package terminal

import (
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Capability computation
// ---------------------------------------------------------------------------

func computeTrueColor(tc *TerminalContext, env map[string]string) {
	switch env["COLORTERM"] {
	case "truecolor", "24bit":
		tc.hasTrueColor = true
		tc.hasTrueColorReason = ""
		return
	}

	// Brand-specific heuristics.
	switch tc.Brand {
	case BrandKitty, BrandGhostty, BrandWezTerm, BrandAlacritty,
		BrandFoot, BrandRio, BrandITerm2, BrandContour, BrandHyper,
		BrandWarp, BrandOtty, BrandGrokDesktop:
		tc.hasTrueColor = true
		return
	case BrandVsCode, BrandCursor, BrandWindsurf, BrandZed:
		// VS Code integrated terminal supports truecolor.
		tc.hasTrueColor = true
		return
	case BrandWindowsTerminal:
		// Windows Terminal supports truecolor (Win10 1709+).
		tc.hasTrueColor = true
		return
	case BrandAppleTerminal:
		// Apple Terminal does NOT support truecolor (256-color max).
		tc.hasTrueColor = false
		tc.hasTrueColorReason = "apple_terminal"
		return
	case BrandVte:
		// VTE supports truecolor since 0.42 (2015). Safe to enable.
		tc.hasTrueColor = true
		return
	case BrandJetBrains:
		// JediTerm supports truecolor in the Reworked 2025 engine.
		// Assume truecolor; 256 fallback is handled downstream.
		tc.hasTrueColor = true
		return
	}

	// Fallback: check TERM for known truecolor indicators.
	term := env["TERM"]
	if strings.HasSuffix(term, "direct") || strings.Contains(term, "truecolor") {
		tc.hasTrueColor = true
		tc.hasTrueColorReason = ""
		return
	}

	// Unknown terminal — conservative: assume no truecolor.
	tc.hasTrueColor = false
	tc.hasTrueColorReason = "unknown_terminal"
}

func compute256Color(tc *TerminalContext) {
	// All recognized brands except VTE < xterm-256color support 256 colors.
	// Apple Terminal supports 256 colors. Virtually all modern terminals do.
	// Only fail in truly unknown environments.
	if tc.hasTrueColor {
		tc.has256Color = true
		return
	}
	switch tc.Brand {
	case BrandAppleTerminal, BrandVte, BrandJetBrains:
		tc.has256Color = true
	default:
		// Conservative for unknown brands.
		tc.has256Color = false
	}
}

func computeKittyKeyboard(tc *TerminalContext) {
	// Multiplexer gating comes FIRST: when running under a multiplexer,
	// the Kitty keyboard protocol is mediated by the multiplexer, not the
	// outer terminal. tmux/screen/cmux conservatively disable KKP (tmux
	// 3.3+ supports it in a limited form, but the env-based detection has
	// no version number, so skipping is the safe default). This must run
	// before the brand positive list, or a KKP-capable outer brand (e.g.
	// Kitty under tmux) would bypass the multiplexer gate entirely.
	switch tc.Multiplexer {
	case MuxScreen:
		tc.hasKittyKeyboard = false
		tc.kittyKeyboardSkipReason = "screen"
		return
	case MuxTmux:
		// tmux 3.3+ supports limited KKP; 3.4+ recommended.
		// Without a version number (env-based detection), conservative = skip.
		tc.hasKittyKeyboard = false
		tc.kittyKeyboardSkipReason = "tmux"
		return
	case MuxCmux:
		tc.hasKittyKeyboard = false
		tc.kittyKeyboardSkipReason = "cmux"
		return
	case MuxZellij:
		tc.hasKittyKeyboard = false
		tc.kittyKeyboardSkipReason = "zellij"
		return
	}

	// Positive list: brands known to implement the Kitty keyboard protocol.
	switch tc.Brand {
	case BrandKitty, BrandGhostty, BrandWezTerm, BrandAlacritty, BrandFoot,
		BrandContour, BrandRio, BrandITerm2:
		// iTerm2 supports KKP since 3.5 (2023).
		tc.hasKittyKeyboard = true
		tc.kittyKeyboardSkipReason = ""
		return
	}

	// Negative list: brands known NOT to implement the protocol, or where
	// known bugs prevent safe usage.
	switch tc.Brand {
	case BrandVsCode, BrandCursor, BrandWindsurf, BrandZed:
		tc.hasKittyKeyboard = false
		tc.kittyKeyboardSkipReason = "vscode"
		return
	case BrandAppleTerminal:
		tc.hasKittyKeyboard = false
		tc.kittyKeyboardSkipReason = "apple_terminal"
		return
	case BrandVte:
		// VTE 0.82.0+ supports KKP. Check version gate.
		if vteHasKKP(tc.VteVersion) {
			tc.hasKittyKeyboard = true
			tc.kittyKeyboardSkipReason = ""
		} else {
			tc.hasKittyKeyboard = false
			tc.kittyKeyboardSkipReason = "vte_old"
		}
		return
	case BrandWindowsTerminal:
		tc.hasKittyKeyboard = false
		tc.kittyKeyboardSkipReason = "windows_terminal"
		return
	case BrandJetBrains:
		tc.hasKittyKeyboard = false
		tc.kittyKeyboardSkipReason = "jetbrains"
		return
	}

	// Unknown brand + no multiplexer. Could be anything (SSH, embedded
	// terminal). Conservative: skip.
	if tc.Brand == BrandUnknown && tc.Multiplexer == MuxUndetected {
		tc.hasKittyKeyboard = false
		tc.kittyKeyboardSkipReason = "unknown_no_multiplexer"
		return
	}

	// Default for any unhandled combination: false with diagnostic reason.
	tc.hasKittyKeyboard = false
	tc.kittyKeyboardSkipReason = "unsupported_terminal"
}

func vteHasKKP(version string) bool {
	if version == "" {
		return false
	}
	// VTE_VERSION is a packed integer: major*10000 + minor*100 + patch.
	v, err := strconv.Atoi(version)
	if err != nil {
		return false
	}
	// 8200 = 0.82.0 (first VTE release with KKP). Note: the packed encoding
	// makes 0.82.0 == 8200, NOT 82000 (which would be 8.20.0 and is never
	// reached by any VTE release — all are on the 0.x line).
	return v >= 8200
}

func computeOSC8Hyperlinks(tc *TerminalContext, _ map[string]string) {
	switch tc.Brand {
	case BrandAppleTerminal:
		// Apple Terminal has a hostile OSC 8 parser that leaks followed content.
		tc.hasOSC8Hyperlinks = false
		tc.osc8SkipReason = "apple_terminal"
		return
	case BrandJetBrains:
		// JediTerm's OSC 8 support is unknown.
		tc.hasOSC8Hyperlinks = false
		tc.osc8SkipReason = "unsupported_terminal"
		return
	}

	// VTE < 0.50.4 does not handle OSC 8 cleanly.
	if tc.Brand == BrandVte && tc.VteVersion != "" {
		v, err := strconv.Atoi(tc.VteVersion)
		if err == nil && v < 5004 {
			tc.hasOSC8Hyperlinks = false
			tc.osc8SkipReason = "vte_old"
			return
		}
	}

	if tc.Multiplexer == MuxScreen {
		tc.hasOSC8Hyperlinks = false
		tc.osc8SkipReason = "screen"
		return
	}

	// Most modern terminals support OSC 8. Safe default for known brands.
	tc.hasOSC8Hyperlinks = true
	tc.osc8SkipReason = ""
}

func computeShiftEnterCtrlDot(tc *TerminalContext, _ map[string]string) {
	// Shift+Enter and Ctrl+. both require Kitty keyboard protocol (KKP)
	// to be distinguishable from bare Enter / '.'.
	if tc.hasKittyKeyboard {
		tc.shiftEnterAvailable = true
		tc.ctrlDotOK = true
		return
	}

	// VTE < 0.82.0: Shift+Enter unavailable.
	if tc.Brand == BrandVte && !vteHasKKP(tc.VteVersion) {
		tc.shiftEnterAvailable = false
		tc.ctrlDotOK = false
		return
	}

	// VS Code family: KKP is never negotiated, so Shift+Enter arrives as bare CR.
	if tc.Brand.IsVSCODECodeFamily() {
		tc.shiftEnterAvailable = false
		tc.ctrlDotOK = false
		return
	}

	// Unknown brand with no multiplexer: conservative.
	if tc.Brand == BrandUnknown && tc.Multiplexer == MuxUndetected {
		tc.shiftEnterAvailable = false
		tc.ctrlDotOK = false
		return
	}

	// Default for any remaining cases.
	tc.shiftEnterAvailable = false
	tc.ctrlDotOK = false
}
