package terminal

// detect.go — Terminal capability detection system.
//
// TerminalContext is the single source of truth for terminal identity and
// feature gating in a session. It is constructed once at startup via
// DetectTerminalContext() and stored in an atomic.Pointer for concurrent reads.
//
// Brand detection follows a precedence-based priority chain of env-var markers
// (see detectTerminalBrandFromEnv). Each brand resolves to a known capability
// profile, and each feature gate returns a (bool, skipReason) pair where
// skipReason is empty when the feature is supported.
//
// Unknown/unrecognized terminals default to the most conservative (fail-closed)
// capability set — no Kitty keyboard, no hyperlinks, 16-color fallback.

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Brand enumeration
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

// RequiresTrueColor returns true when the brand's theme definition looks
// incorrect when quantized to 256 colors. Only TokyoNight and RosePineMoon
// have this issue; neutral-gray themes (GrokNight, MadyDark) survive
// quantization.
func (b TerminalBrand) RequiresTrueColor() bool {
	return false // brand-level; theme-level check in theme package
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

// ---------------------------------------------------------------------------
// TerminalContext
// ---------------------------------------------------------------------------

// TerminalContext stores the detected terminal identity, capabilities, and
// feature-gating skip reasons for the current session. Construct once at
// startup via DetectTerminalContext().
type TerminalContext struct {
	Brand       TerminalBrand
	EnvBrand    TerminalBrand // raw detection before Unknown→WindowsTerminal fallback
	Multiplexer MultiplexerKind
	IsSSH       bool
	IsByobu     bool   // Byobu wrapper (tmux or screen backend)
	TermVar     string // raw $TERM value
	VteVersion  string // VTE version string (VTE-based terminals)

	// Capability gates. Each (bool, reason) pair is computed once at detection
	// time from brand+env heuristics.
	hasTrueColor            bool
	hasTrueColorReason      string
	has256Color             bool
	hasKittyKeyboard        bool
	kittyKeyboardSkipReason string
	hasOSC8Hyperlinks       bool
	osc8SkipReason          string
	hasOSC52Clipboard       bool
	shiftEnterAvailable     bool
	ctrlDotOK               bool
}

// CurrentTerminalContext returns the cached session terminal context.
func CurrentTerminalContext() *TerminalContext {
	p := terminalCtx.Load()
	if p == nil {
		// Lazily detect if never initialized.
		ctx := DetectTerminalContext()
		terminalCtx.Store(&ctx)
		return terminalCtx.Load()
	}
	return p
}

var (
	terminalCtx atomic.Pointer[TerminalContext]
	detectOnce  sync.Once
	detectedCtx TerminalContext
	detectMu    sync.Mutex
)

// DetectTerminalContext performs brand detection and capability inference from
// the current process environment. Safe to call multiple times; the result is
// cached after the first call.
func DetectTerminalContext() TerminalContext {
	detectOnce.Do(func() {
		env := collectEnv()
		ctx := buildTerminalContextFromEnv(env)
		detectedCtx = ctx
	})
	return detectedCtx
}

// ResetTerminalContext clears the cached detection result. Only for tests.
func ResetTerminalContext() {
	detectMu.Lock()
	defer detectMu.Unlock()
	detectOnce = sync.Once{}
	terminalCtx.Store(nil)
}

// ---------------------------------------------------------------------------
// Feature gates
// ---------------------------------------------------------------------------

// HasTrueColor returns whether the terminal supports 24-bit truecolor.
func (tc *TerminalContext) HasTrueColor() (bool, string) {
	return tc.hasTrueColor, tc.hasTrueColorReason
}

// Has256Color returns whether the terminal supports 256-color palette.
func (tc *TerminalContext) Has256Color() bool {
	return tc.has256Color
}

// SupportsKittyKeyboard returns whether Kitty keyboard protocol should be
// negotiated. The second return value is a non-empty skip reason when the
// protocol should NOT be enabled (for diagnostics).
func (tc *TerminalContext) SupportsKittyKeyboard() (bool, string) {
	return tc.hasKittyKeyboard, tc.kittyKeyboardSkipReason
}

// SupportsOSC8Hyperlinks returns whether OSC 8 hyperlinks are supported.
func (tc *TerminalContext) SupportsOSC8Hyperlinks() (bool, string) {
	return tc.hasOSC8Hyperlinks, tc.osc8SkipReason
}

// ShiftEnterAvailable returns whether Shift+Enter can be distinguished from
// bare Enter at the byte level. When false, the UI should advertise Alt+Enter
// for newline insertion instead.
func (tc *TerminalContext) ShiftEnterAvailable() bool {
	return tc.shiftEnterAvailable
}

// CtrlDotAvailable returns whether Ctrl+. can be used as a shortcut key.
// Without Kitty keyboard protocol, Ctrl+. is indistinguishable from '.'.
func (tc *TerminalContext) CtrlDotAvailable() bool {
	return tc.ctrlDotOK
}

// KittyKeyboardSkipReason returns the skip reason string, empty when supported.
func (tc *TerminalContext) KittyKeyboardSkipReason() string {
	return tc.kittyKeyboardSkipReason
}

// ---------------------------------------------------------------------------
// Env collection (pure, injectable)
// ---------------------------------------------------------------------------

// envProvider abstracts os.Getenv for testability.
type envProvider func(string) string

var osEnv envProvider = os.Getenv

func collectEnv() map[string]string {
	// Collect known terminal env vars into a map for pure detection.
	keys := []string{
		"TERM_PROGRAM", "TERM_PROGRAM_VERSION", "TERM",
		"COLORTERM", "KITTY_WINDOW_ID", "KITTY_PID",
		"GHOSTTY_RESOURCES_DIR", "GHOSTTY_BIN",
		"ITERM_SESSION_ID", "ITERM_PROFILE", "LC_TERMINAL",
		"TMUX", "TMUX_PANE",
		"ZELLIJ", "ZELLIJ_SESSION_NAME",
		"STY", "CMUX_SOCKET_PATH", "CMUX_PANEL_ID", "CMUX_BUNDLE_ID",
		"BYOBU_BACKEND", "BYOBU_CONFIG_DIR", "BYOBU_DISTRO",
		"SSH_CONNECTION", "SSH_TTY", "SSH_CLIENT",
		"VSCODE_GIT_ASKPASS_MAIN", "CURSOR_TRACE_ID",
		"TERMINAL_EMULATOR",
		"WT_SESSION", "ALACRITTY_SOCKET",
		"FOOT_VERSION", "VTE_VERSION",
		"WEZTERM_VERSION",
		"TERMINATOR_UUID",
		"WINDOWID", // X11 root window
	}
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		if v := osEnv(k); v != "" {
			m[k] = v
		}
	}
	return m
}

// ---------------------------------------------------------------------------
// Detection from env map (pure, testable)
// ---------------------------------------------------------------------------

func buildTerminalContextFromEnv(env map[string]string) TerminalContext {
	brand := detectTerminalBrandFromEnv(env)
	mux := detectMultiplexerFromEnv(env)
	isByobu := detectByobuFromEnv(env)
	isSSH := env["SSH_CONNECTION"] != "" || env["SSH_TTY"] != "" || env["SSH_CLIENT"] != ""

	var vteVersion string
	if v, ok := env["VTE_VERSION"]; ok {
		vteVersion = v
	}

	envBrand := brand
	// On Windows, Unknown → WindowsTerminal (Windows Terminal is the 11+ default
	// but DefTerm handoff may not set WT_SESSION).
	if brand == BrandUnknown && isWindowsEnv() {
		brand = BrandWindowsTerminal
	}

	tc := TerminalContext{
		Brand:       brand,
		EnvBrand:    envBrand,
		Multiplexer: mux,
		IsSSH:       isSSH,
		IsByobu:     isByobu,
		TermVar:     env["TERM"],
		VteVersion:  vteVersion,
	}

	// Compute capabilities from detected identity.
	computeTrueColor(&tc, env)
	compute256Color(&tc)
	computeKittyKeyboard(&tc)
	computeOSC8Hyperlinks(&tc, env)
	computeShiftEnterCtrlDot(&tc, env)

	return tc
}

// ---------------------------------------------------------------------------
// Brand detection (pure function)
// ---------------------------------------------------------------------------

func detectTerminalBrandFromEnv(env map[string]string) TerminalBrand {
	// 1. VS Code forks — check specific env markers before TERM_PROGRAM
	//    because some forks set TERM_PROGRAM=vscode.
	if _, ok := env["CURSOR_TRACE_ID"]; ok {
		return BrandCursor
	}
	if askpass, ok := env["VSCODE_GIT_ASKPASS_MAIN"]; ok {
		lower := strings.ToLower(askpass)
		if strings.Contains(lower, "cursor") {
			return BrandCursor
		}
		if strings.Contains(lower, "windsurf") {
			return BrandWindsurf
		}
		// Pure VS Code remote.
		return BrandVsCode
	}

	// 2. TERM_PROGRAM is the most reliable signal.
	if tp, ok := env["TERM_PROGRAM"]; ok {
		if brand := brandFromTermProgram(tp); brand != BrandUnknown {
			return brand
		}
	}

	// 3. JetBrains (TERMINAL_EMULATOR=JetBrains-JediTerm).
	if te, ok := env["TERMINAL_EMULATOR"]; ok {
		lower := strings.ToLower(te)
		if strings.Contains(lower, "jetbrains") || strings.Contains(lower, "jediterm") {
			return BrandJetBrains
		}
	}

	// 4. WezTerm.
	if _, ok := env["WEZTERM_VERSION"]; ok {
		return BrandWezTerm
	}

	// 5. iTerm2 (also survived SSH via LC_TERMINAL).
	if _, ok := env["ITERM_SESSION_ID"]; ok {
		return BrandITerm2
	}
	if _, ok := env["ITERM_PROFILE"]; ok {
		return BrandITerm2
	}
	if lc, ok := env["LC_TERMINAL"]; ok && strings.EqualFold(lc, "iterm2") {
		return BrandITerm2
	}

	// 6. Apple Terminal.
	if _, ok := env["TERM_SESSION_ID"]; ok {
		return BrandAppleTerminal
	}

	// 7. Kitty.
	if _, ok := env["KITTY_WINDOW_ID"]; ok {
		return BrandKitty
	}
	if term, ok := env["TERM"]; ok && strings.Contains(term, "kitty") {
		return BrandKitty
	}

	// 8. Ghostty (check after Kitty so Ghostty doesn't false-positive as VTE).
	if _, ok := env["GHOSTTY_RESOURCES_DIR"]; ok {
		return BrandGhostty
	}
	if _, ok := env["GHOSTTY_BIN"]; ok {
		return BrandGhostty
	}
	if tp, ok := env["TERM_PROGRAM"]; ok && strings.EqualFold(tp, "ghostty") {
		return BrandGhostty
	}

	// 9. Alacritty.
	if _, ok := env["ALACRITTY_SOCKET"]; ok {
		return BrandAlacritty
	}
	if _, ok := env["ALACRITTY_WINDOW_ID"]; ok {
		return BrandAlacritty
	}
	if term, ok := env["TERM"]; ok && term == "alacritty" {
		return BrandAlacritty
	}

	// 10. Rio.
	if term, ok := env["TERM"]; ok && term == "rio" {
		return BrandRio
	}

	// 11. foot (Wayland-native, no unique env var except TERM).
	if term, ok := env["TERM"]; ok {
		switch term {
		case "foot", "foot-extra", "foot-direct":
			return BrandFoot
		}
	}

	// 12. Terminator (Python/GTK, VTE-based).
	if _, ok := env["TERMINATOR_UUID"]; ok {
		return BrandVte
	}

	// 13. VTE-based (GNOME Terminal, kgx, Tilix).
	if _, ok := env["VTE_VERSION"]; ok {
		return BrandVte
	}

	// 14. Windows Terminal.
	if _, ok := env["WT_SESSION"]; ok {
		return BrandWindowsTerminal
	}

	// 15. Zed (TERM_PROGRAM=Zed not set in all environments).
	if tp, ok := env["TERM_PROGRAM"]; ok && tp == "Zed" {
		return BrandZed
	}

	return BrandUnknown
}

func brandFromTermProgram(value string) TerminalBrand {
	// Normalize: lowercase, strip spaces/dashes/underscores/dots.
	norm := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '_' || r == '.' {
			return -1
		}
		return r
	}, strings.ToLower(value))

	switch norm {
	case "appleterminal":
		return BrandAppleTerminal
	case "ghostty":
		return BrandGhostty
	case "iterm", "iterm2", "itermapp":
		return BrandITerm2
	case "warp", "warpterminal":
		return BrandWarp
	case "vscode":
		return BrandVsCode
	case "cursor":
		return BrandCursor
	case "windsurf":
		return BrandWindsurf
	case "wezterm":
		return BrandWezTerm
	case "kitty":
		return BrandKitty
	case "alacritty":
		return BrandAlacritty
	case "rio":
		return BrandRio
	case "zed":
		return BrandZed
	case "grokdesktop":
		return BrandGrokDesktop
	case "windowsterminal":
		return BrandWindowsTerminal
	case "otty":
		return BrandOtty
	case "contour":
		return BrandContour
	case "hyper":
		return BrandHyper
	case "foot":
		return BrandFoot
	}
	return BrandUnknown
}

// ---------------------------------------------------------------------------
// Multiplexer detection
// ---------------------------------------------------------------------------

func detectMultiplexerFromEnv(env map[string]string) MultiplexerKind {
	if _, ok := env["TMUX"]; ok {
		return MuxTmux
	}
	if _, ok := env["ZELLIJ"]; ok {
		return MuxZellij
	}
	if _, ok := env["ZELLIJ_SESSION_NAME"]; ok {
		return MuxZellij
	}
	if _, ok := env["STY"]; ok {
		return MuxScreen
	}
	if _, ok := env["CMUX_SOCKET_PATH"]; ok {
		return MuxCmux
	}
	if _, ok := env["CMUX_PANEL_ID"]; ok {
		return MuxCmux
	}
	if _, ok := env["CMUX_BUNDLE_ID"]; ok {
		return MuxCmux
	}
	return MuxUndetected
}

func detectByobuFromEnv(env map[string]string) bool {
	_, hasBackend := env["BYOBU_BACKEND"]
	_, hasConfig := env["BYOBU_CONFIG_DIR"]
	_, hasDistro := env["BYOBU_DISTRO"]
	return hasBackend || hasConfig || hasDistro
}

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
		BrandFoot, BrandRio, BrandITerm2, BrandContour, BrandHyper:
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
	// Positive list: brands known to implement the Kitty keyboard protocol.
	switch tc.Brand {
	case BrandKitty, BrandGhostty, BrandWezTerm, BrandAlacritty, BrandFoot:
		tc.hasKittyKeyboard = true
		tc.kittyKeyboardSkipReason = ""
		return
	case BrandContour, BrandRio:
		tc.hasKittyKeyboard = true
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

	// Multiplexer gating.
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
	// 82000 = 0.82.0 (first VTE release with KKP).
	return v >= 82000
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

	switch tc.Multiplexer {
	case MuxScreen:
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

// ---------------------------------------------------------------------------
// Platform helpers
// ---------------------------------------------------------------------------

// isWindowsEnv checks for Go's runtime.GOOS-like detection from env.
func isWindowsEnv() bool {
	return osEnv("OS") == "Windows_NT" || osEnv("WT_SESSION") != ""
}
