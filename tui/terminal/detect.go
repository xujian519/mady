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
	"strings"
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Brand enumeration
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
// CI detection
// ---------------------------------------------------------------------------

// IsCIEnvironment reports whether the process is running inside a CI system.
// CI environments have no real terminal — TUI features like alternate screen,
// mouse capture, and synchronized output are either unavailable or pointless.
//
// Detection is based on well-known CI environment variables.
// See https://docs.github.com/en/actions/learn-github-actions/variables
func IsCIEnvironment() bool {
	return os.Getenv("CI") == "true" ||
		os.Getenv("GITHUB_ACTIONS") == "true" ||
		os.Getenv("GITLAB_CI") == "true" ||
		os.Getenv("JENKINS_HOME") != "" ||
		os.Getenv("TF_BUILD") == "true"
}

// ---------------------------------------------------------------------------
// Nerd Font detection
// ---------------------------------------------------------------------------

// NerdFontStatus represents the detected level of Nerd Font support.
type NerdFontStatus int

const (
	NerdFontUnknown     NerdFontStatus = iota // not yet detected
	NerdFontAvailable                         // Nerd Font symbols can be used
	NerdFontUnavailable                       // fall back to Unicode/ASCII
)

var nerdFontStatus atomic.Value // stores NerdFontStatus

func init() {
	nerdFontStatus.Store(NerdFontUnknown)
}

// DetectNerdFonts checks whether the terminal likely supports Nerd Fonts.
// Uses a combination of env-var override and terminal-program heuristics.
func DetectNerdFonts() NerdFontStatus {
	// 1. Explicit env override
	switch os.Getenv("NERD_FONT") {
	case "1", "true", "yes":
		nerdFontStatus.Store(NerdFontAvailable)
		return NerdFontAvailable
	case "0", "false", "no":
		nerdFontStatus.Store(NerdFontUnavailable)
		return NerdFontUnavailable
	}

	// 2. Known modern terminals where users commonly install Nerd Fonts
	termProg := os.Getenv("TERM_PROGRAM")
	switch termProg {
	case "iTerm.app", "WezTerm", "kitty", "ghostty", "tabby", "alacritty",
		"warp", "vscode", "Hyper":
		nerdFontStatus.Store(NerdFontAvailable)
		return NerdFontAvailable
	}

	// 3. $TERM heuristic for xterm-256color derived terminals
	term := os.Getenv("TERM")
	if strings.Contains(term, "xterm") || strings.Contains(term, "tmux") {
		nerdFontStatus.Store(NerdFontAvailable)
		return NerdFontAvailable
	}

	nerdFontStatus.Store(NerdFontUnavailable)
	return NerdFontUnavailable
}

// NerdFontsSupported returns the cached result of DetectNerdFonts.
func NerdFontsSupported() NerdFontStatus {
	v := nerdFontStatus.Load()
	if v == nil {
		return DetectNerdFonts()
	}
	status, ok := v.(NerdFontStatus)
	if !ok {
		nerdFontStatus.Store(NerdFontUnavailable)
		return NerdFontUnavailable
	}
	return status
}

// ---------------------------------------------------------------------------
// Platform helpers
// ---------------------------------------------------------------------------

// isWindowsEnv checks for Go's runtime.GOOS-like detection from env.
func isWindowsEnv() bool {
	return osEnv("OS") == "Windows_NT" || osEnv("WT_SESSION") != ""
}
