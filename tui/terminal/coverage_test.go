package terminal

import (
	"testing"

	core "github.com/xujian519/mady/tui/core"
)

// ---------------------------------------------------------------------------
// ansi.go — ANSI escape builders (pure functions)
// ---------------------------------------------------------------------------

func TestANSIHelpers(t *testing.T) {
	if got := SetWindowTitle("mady"); got != "\x1b]0;mady\x07" {
		t.Errorf("SetWindowTitle = %q", got)
	}
	if got := ClearToEndOfLine(); got != Esc+"0K" {
		t.Errorf("ClearToEndOfLine = %q", got)
	}
}

// ---------------------------------------------------------------------------
// detect.go — TerminalBrand capability methods
// ---------------------------------------------------------------------------

func TestTerminalBrandMethods(t *testing.T) {
	if !BrandVte.IsVTEVteBased() {
		t.Error("BrandVte should be VTE-based")
	}
	if BrandKitty.IsVTEVteBased() {
		t.Error("BrandKitty must not be VTE-based")
	}

	// RequiresTrueColor is brand-level; currently false for all brands
	// (theme-level check lives in the theme package).
	if BrandKitty.RequiresTrueColor() {
		t.Error("BrandKitty.RequiresTrueColor() = true, want false")
	}

	// String() for every brand must be non-empty and unique.
	seen := map[string]bool{}
	for b := BrandUnknown; b <= BrandHyper; b++ {
		s := b.String()
		if s == "" {
			t.Errorf("Brand %d String() is empty", b)
		}
		if seen[s] {
			t.Errorf("duplicate brand name %q", s)
		}
		seen[s] = true
	}
}

// TestTerminalContextCapabilities exercises the capability gates by setting
// the private fields directly (white-box, same package).
func TestTerminalContextCapabilities(t *testing.T) {
	ctx := &TerminalContext{
		has256Color:       true,
		hasOSC8Hyperlinks: true,
	}
	if !ctx.Has256Color() {
		t.Error("Has256Color with has256Color=true = false")
	}
	if ok, reason := ctx.SupportsOSC8Hyperlinks(); !ok {
		t.Errorf("SupportsOSC8Hyperlinks = (%v, %q), want true", ok, reason)
	}
	if ctx.CtrlDotAvailable() {
		t.Error("CtrlDotAvailable default = true, want false")
	}

	ctx2 := &TerminalContext{
		hasTrueColor:     true,
		hasKittyKeyboard: true,
		ctrlDotOK:        true,
	}
	if ok, _ := ctx2.HasTrueColor(); !ok {
		t.Error("HasTrueColor with hasTrueColor=true = false")
	}
	if ok, _ := ctx2.SupportsKittyKeyboard(); !ok {
		t.Error("SupportsKittyKeyboard with hasKittyKeyboard=true = false")
	}
	if !ctx2.CtrlDotAvailable() {
		t.Error("CtrlDotAvailable with ctrlDotOK=true = false")
	}

	// Skip reason surface.
	ctx3 := &TerminalContext{kittyKeyboardSkipReason: "tmux"}
	if ctx3.KittyKeyboardSkipReason() != "tmux" {
		t.Error("KittyKeyboardSkipReason did not surface the stored reason")
	}

	// ShiftEnterAvailable defaults false.
	if ctx.ShiftEnterAvailable() {
		t.Error("ShiftEnterAvailable default = true, want false")
	}
}

// TestNerdFontStatuses covers DetectNerdFonts / NerdFontsSupported statuses.
func TestNerdFontStatuses(t *testing.T) {
	t.Setenv("NERD_FONT", "1")
	detected := DetectNerdFonts()
	if detected != NerdFontAvailable {
		t.Errorf("DetectNerdFonts with NERD_FONT=1 = %v, want available", detected)
	}
	t.Setenv("NERD_FONT", "0")
	if got := DetectNerdFonts(); got != NerdFontUnavailable {
		t.Errorf("DetectNerdFonts with NERD_FONT=0 = %v, want unavailable", got)
	}
	t.Setenv("NERD_FONT", "")

	// Supported-status surfaces the last detected value.
	_ = NerdFontsSupported()
}

// ---------------------------------------------------------------------------
// keys.go — Key helpers and KeyID matching
// ---------------------------------------------------------------------------

func TestKeyEventTypeHelpers(t *testing.T) {
	press := Key{Event: KeyPress, Rune: 'a'}
	release := Key{Event: KeyRelease, Rune: 'a'}
	repeat := Key{Event: KeyRepeat, Rune: 'a'}
	if !press.IsPrintable() {
		t.Error("KeyPress with rune should be printable")
	}
	// Non-printable: no rune (e.g. a named key).
	if (Key{Event: KeyPress}).IsPrintable() {
		t.Error("KeyPress without rune must not be printable")
	}
	// Modifier beyond Shift disqualifies printability.
	if (Key{Event: KeyPress, Rune: 'a', Mods: ModCtrl}).IsPrintable() {
		t.Error("ctrl+a must not be printable")
	}
	if !release.IsRelease() || press.IsRelease() {
		t.Error("IsRelease misbehaves")
	}
	if !repeat.IsRepeat() || press.IsRepeat() {
		t.Error("IsRepeat misbehaves")
	}
	if press.String() == "" || release.String() == "" || repeat.String() == "" {
		t.Error("Key.String() empty for event types")
	}
}

func TestMatchesKeyWithFlags(t *testing.T) {
	// "ctrl+c" matches Ctrl+C in traditional xterm encoding.
	if !MatchesKey("\x03", "ctrl+c") {
		t.Error(`MatchesKey("\x03", "ctrl+c") = false`)
	}
	// Kitty CSI u: ctrl+c = CSI 99;5u (with kitty flags set).
	kitty := "\x1b[99;5u"
	if !MatchesKeyWithFlags(kitty, "ctrl+c", 1) {
		t.Errorf("kitty ctrl+c (%q) does not match with flags=1", kitty)
	}
}

// ---------------------------------------------------------------------------
// keybindings.go — global manager accessors
// ---------------------------------------------------------------------------

func TestGlobalKeybindings(t *testing.T) {
	m := NewKeybindingsManager(map[string]KeybindingDef{})
	m.Register("global-test", KeybindingDef{DefaultKeys: []string{"alt+g"}})
	if !m.Matches("\x1bg", "global-test") {
		t.Error("registered binding alt+g does not match")
	}
	SetGlobalKeybindings(m)
	defer SetGlobalKeybindings(nil)

	if GetGlobalKeybindings() != m {
		t.Error("GetGlobalKeybindings did not return the registered manager")
	}
}

// ---------------------------------------------------------------------------
// detect.go — MultiplexerKind.String + IsCIEnvironment
// ---------------------------------------------------------------------------

func TestMultiplexerString(t *testing.T) {
	for kind, want := range map[MultiplexerKind]string{
		MuxUndetected: "none",
		MuxTmux:       "tmux",
		MuxScreen:     "screen",
		MuxZellij:     "zellij",
		MuxCmux:       "cmux",
	} {
		if got := kind.String(); got != want {
			t.Errorf("MultiplexerKind(%d).String() = %q, want %q", kind, got, want)
		}
	}
	// Unknown kinds fall back to "none".
	if got := MultiplexerKind(99).String(); got != "none" {
		t.Errorf("unknown mux String() = %q, want none", got)
	}
}

func TestIsCIEnvironment(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")
	t.Setenv("JENKINS_HOME", "")
	t.Setenv("TF_BUILD", "")
	if IsCIEnvironment() {
		t.Error("clean env reported as CI")
	}
	t.Setenv("CI", "true")
	if !IsCIEnvironment() {
		t.Error("CI=true not detected")
	}
	t.Setenv("CI", "")
	t.Setenv("JENKINS_HOME", "/jenkins")
	if !IsCIEnvironment() {
		t.Error("JENKINS_HOME set not detected")
	}
}

// ---------------------------------------------------------------------------
// keys.go — MatchesAnyKey + kitty protocol active flag
// ---------------------------------------------------------------------------

func TestMatchesAnyKey(t *testing.T) {
	if !MatchesAnyKey("\x03", "ctrl+c", "enter") {
		t.Error("MatchesAnyKey ctrl+c = false")
	}
	if MatchesAnyKey("\x03", "enter", "esc") {
		t.Error("MatchesAnyKey with wrong ids = true")
	}
	if MatchesAnyKey("\x03") {
		t.Error("MatchesAnyKey with no ids = true")
	}
}

func TestKittyProtocolActive(t *testing.T) {
	SetKittyProtocolActive(true)
	if !IsKittyProtocolActive() {
		t.Error("IsKittyProtocolActive after Set(true) = false")
	}
	SetKittyProtocolActive(false)
	if IsKittyProtocolActive() {
		t.Error("IsKittyProtocolActive after Set(false) = true")
	}
}

// ---------------------------------------------------------------------------
// keys.go — SS3 (F1-F4) and CSI special-key parsing
// ---------------------------------------------------------------------------

func TestSS3FunctionKeys(t *testing.T) {
	cases := []struct {
		seq  string
		want string
	}{
		{"\x1bOP", "f1"},
		{"\x1bOQ", "f2"},
		{"\x1bOR", "f3"},
		{"\x1bOS", "f4"},
	}
	for _, tc := range cases {
		keys := ParseKeys(tc.seq, 0)
		if len(keys) != 1 {
			t.Errorf("ParseKeys(%q) keys = %d, want 1", tc.seq, len(keys))
			continue
		}
		if keys[0].Name != tc.want {
			t.Errorf("ParseKeys(%q) name = %q, want %q", tc.seq, keys[0].Name, tc.want)
		}
	}
}

func TestCSISpecialKeys(t *testing.T) {
	cases := []struct {
		seq  string
		want KeyID
	}{
		{"\x1b[5~", KeyPageUp},
		{"\x1b[6~", KeyPageDown},
		{"\x1b[7~", KeyHome},
		{"\x1b[8~", KeyEnd},
		{"\x1b[3~", KeyDelete},
		{"\x1b[2~", "insert"},
		{"\x1b[15~", "f5"},
		{"\x1b[17~", "f6"},
		{"\x1b[18~", "f7"},
		{"\x1b[19~", "f8"},
		{"\x1b[20~", "f9"},
		{"\x1b[21~", "f10"},
		{"\x1b[23~", "f11"},
		{"\x1b[24~", "f12"},
	}
	for _, tc := range cases {
		if !MatchesKey(tc.seq, tc.want) {
			t.Errorf("MatchesKey(%q, %q) = false", tc.seq, tc.want)
		}
	}
}

func TestArrowKeysWithModifiers(t *testing.T) {
	// Traditional arrows without modifiers.
	if !MatchesKey("\x1b[A", KeyUp) || !MatchesKey("\x1b[B", KeyDown) {
		t.Error("plain arrow sequences not matched")
	}
	// CSI 1;5A = ctrl+up; CSI 1;3A = alt+up.
	if !MatchesKey("\x1b[1;5A", "ctrl+up") {
		t.Error("ctrl+up not matched")
	}
	if !MatchesKey("\x1b[1;3A", "alt+up") {
		t.Error("alt+up not matched")
	}
	// SS3 arrows (application cursor mode): ESC OA = up.
	if !MatchesKey("\x1bOA", KeyUp) {
		t.Error("SS3 up not matched")
	}
}

// ---------------------------------------------------------------------------
// detect.go — Nerd Font TERM heuristic branches
// ---------------------------------------------------------------------------

func TestDetectNerdFontsTermHeuristic(t *testing.T) {
	t.Setenv("NERD_FONT", "")
	t.Setenv("TERM_PROGRAM", "")
	// xterm-derived $TERM → available.
	t.Setenv("TERM", "xterm-256color")
	if got := DetectNerdFonts(); got != NerdFontAvailable {
		t.Errorf("TERM=xterm-256color → %v, want available", got)
	}
	t.Setenv("TERM", "tmux-256color")
	if got := DetectNerdFonts(); got != NerdFontAvailable {
		t.Errorf("TERM=tmux-256color → %v, want available", got)
	}
	// Unknown TERM → unavailable.
	t.Setenv("TERM", "dumb")
	if got := DetectNerdFonts(); got != NerdFontUnavailable {
		t.Errorf("TERM=dumb → %v, want unavailable", got)
	}
}

// ---------------------------------------------------------------------------
// stdin_buffer.go — OnMouse + Close/Reset
// ---------------------------------------------------------------------------

func TestStdinBufferCallbacksAndReset(t *testing.T) {
	var gotPaste string
	b := NewStdinBuffer()
	if b == nil {
		t.Fatal("NewStdinBuffer returned nil")
	}
	b.OnPaste(func(text string) { gotPaste = text })

	// Feed bracketed paste markers.
	b.Feed([]byte("\x1b[200~hello\x1b[201~"))
	b.FlushEsc()
	if gotPaste != "hello" {
		t.Errorf("paste callback got %q, want hello", gotPaste)
	}

	b.Reset()
	b.Close() // must not panic
}

// TestStdinBufferOnMouse verifies X11 and SGR mouse sequence decoding.
func TestStdinBufferOnMouse(t *testing.T) {
	var got core.MouseMsg
	b := NewStdinBuffer()
	b.OnMouse(func(m core.MouseMsg) { got = m })

	// X11 style: ESC [ M Cb Cx Cy (button|32, col|32, row|32).
	b.Feed([]byte("\x1b[M" + string(rune(32+0)) + string(rune(32+5)) + string(rune(32+3))))
	b.FlushEsc()
	if got.Col != 4 || got.Row != 2 {
		t.Errorf("X11 mouse = col%d,row%d, want 4,2", got.Col, got.Row)
	}

	// SGR style: ESC [ < Cb ; Cx ; Cy M.
	b.Feed([]byte("\x1b[<0;10;8M"))
	b.FlushEsc()
	if got.Col != 9 || got.Row != 7 {
		t.Errorf("SGR mouse = col%d,row%d, want 9,7", got.Col, got.Row)
	}
}

// TestProcessTerminalKittyFlags covers the KittyFlags getter without
// requiring a real TTY (the constructor defaults to flag 1 = disambiguation).
func TestProcessTerminalKittyFlags(t *testing.T) {
	pt := NewProcessTerminal()
	if pt.KittyFlags() != 1 {
		t.Errorf("fresh ProcessTerminal KittyFlags = %d, want 1 (default disambiguation)", pt.KittyFlags())
	}
}
