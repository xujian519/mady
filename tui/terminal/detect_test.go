package terminal

import (
	"testing"
)

// envMap is a helper to construct an env map for tests.
func envMap(pairs ...string) map[string]string {
	m := make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return m
}

func TestDetectBrand_Unknown(t *testing.T) {
	env := envMap()
	brand := detectTerminalBrandFromEnv(env)
	if brand != BrandUnknown {
		t.Errorf("empty env: got %v, want Unknown", brand)
	}
}

func TestDetectBrand_TermProgram(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want TerminalBrand
	}{
		{"iTerm2", envMap("TERM_PROGRAM", "iTerm2"), BrandITerm2},
		{"iTerm2_v2", envMap("TERM_PROGRAM", "iTerm.app"), BrandITerm2},
		{"WezTerm", envMap("TERM_PROGRAM", "WezTerm"), BrandWezTerm},
		{"Warp", envMap("TERM_PROGRAM", "Warp"), BrandWarp},
		{"Kitty", envMap("TERM_PROGRAM", "kitty"), BrandKitty},
		{"Alacritty", envMap("TERM_PROGRAM", "Alacritty"), BrandAlacritty},
		{"Rio", envMap("TERM_PROGRAM", "Rio"), BrandRio},
		{"Zed", envMap("TERM_PROGRAM", "Zed"), BrandZed},
		{"Ghostty", envMap("TERM_PROGRAM", "ghostty"), BrandGhostty},
		{"Grok Desktop", envMap("TERM_PROGRAM", "grok-desktop"), BrandGrokDesktop},
		{"Windows Terminal", envMap("TERM_PROGRAM", "WindowsTerminal"), BrandWindowsTerminal},
		{"Contour", envMap("TERM_PROGRAM", "contour"), BrandContour},
		{"Hyper", envMap("TERM_PROGRAM", "hyper"), BrandHyper},
		{"foot", envMap("TERM_PROGRAM", "foot"), BrandFoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTerminalBrandFromEnv(tt.env)
			if got != tt.want {
				t.Errorf("detectTerminalBrandFromEnv(%v) = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

func TestDetectBrand_VSCodeForks(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want TerminalBrand
	}{
		{"Cursor via CURSOR_TRACE_ID", envMap("CURSOR_TRACE_ID", "abc123"), BrandCursor},
		{"Cursor via askpass", envMap("VSCODE_GIT_ASKPASS_MAIN", "/usr/bin/cursor"), BrandCursor},
		{"Windsurf via askpass", envMap("VSCODE_GIT_ASKPASS_MAIN", "/usr/bin/windsurf"), BrandWindsurf},
		{"VS Code", envMap("VSCODE_GIT_ASKPASS_MAIN", "/usr/bin/vscode"), BrandVsCode},
		{"VS Code via TERM_PROGRAM", envMap("TERM_PROGRAM", "vscode"), BrandVsCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTerminalBrandFromEnv(tt.env)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectBrand_ByEnvVar(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want TerminalBrand
	}{
		{"Kitty KITTY_WINDOW_ID", envMap("KITTY_WINDOW_ID", "1"), BrandKitty},
		{"Kitty TERM", envMap("TERM", "xterm-kitty"), BrandKitty},
		{"Ghostty resources", envMap("GHOSTTY_RESOURCES_DIR", "/tmp/ghostty"), BrandGhostty},
		{"Alacritty socket", envMap("ALACRITTY_SOCKET", "/tmp/ala"), BrandAlacritty},
		{"Alacritty TERM", envMap("TERM", "alacritty"), BrandAlacritty},
		{"Rio TERM", envMap("TERM", "rio"), BrandRio},
		{"iTerm2 ITERM_SESSION_ID", envMap("ITERM_SESSION_ID", "abc"), BrandITerm2},
		{"iTerm2 ITERM_PROFILE", envMap("ITERM_PROFILE", "Default"), BrandITerm2},
		{"iTerm2 via LC_TERMINAL", envMap("LC_TERMINAL", "iTerm2"), BrandITerm2},
		{"Apple Terminal via TERM_SESSION_ID", envMap("TERM_SESSION_ID", "ABC"), BrandAppleTerminal},
		{"WezTerm via WEZTERM_VERSION", envMap("WEZTERM_VERSION", "20240101"), BrandWezTerm},
		{"Windows Terminal via WT_SESSION", envMap("WT_SESSION", "guid"), BrandWindowsTerminal},
		{"VTE via VTE_VERSION", envMap("VTE_VERSION", "6800"), BrandVte},
		{"foot via TERM", envMap("TERM", "foot"), BrandFoot},
		{"foot-direct via TERM", envMap("TERM", "foot-direct"), BrandFoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTerminalBrandFromEnv(tt.env)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectBrand_JetBrains(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want TerminalBrand
	}{
		{"JetBrains JediTerm", envMap("TERMINAL_EMULATOR", "JetBrains-JediTerm"), BrandJetBrains},
		{"JetBrains lowercase", envMap("TERMINAL_EMULATOR", "jetbrains-jediterm"), BrandJetBrains},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTerminalBrandFromEnv(tt.env)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectBrand_TerminatorOverridesVTE(t *testing.T) {
	// Terminator sets both TERMINATOR_UUID and VTE_VERSION.
	// Detection must prefer Terminator (not generic VTE).
	env := envMap("TERMINATOR_UUID", "abc", "VTE_VERSION", "6800")
	brand := detectTerminalBrandFromEnv(env)
	if brand != BrandVte {
		t.Errorf("terminator: got %v, want Vte", brand)
	}
}

func TestDetectMultiplexer(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want MultiplexerKind
	}{
		{"none", envMap(), MuxUndetected},
		{"tmux", envMap("TMUX", "/tmp/tmux-501/default,12345,0"), MuxTmux},
		{"zellij", envMap("ZELLIJ", "1"), MuxZellij},
		{"zellij session name", envMap("ZELLIJ_SESSION_NAME", "my-session"), MuxZellij},
		{"screen", envMap("STY", "12345.pts-0.host"), MuxScreen},
		{"cmux socket", envMap("CMUX_SOCKET_PATH", "/tmp/cmux.sock"), MuxCmux},
		{"cmux panel", envMap("CMUX_PANEL_ID", "1"), MuxCmux},
		{"cmux bundle", envMap("CMUX_BUNDLE_ID", "bundle"), MuxCmux},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectMultiplexerFromEnv(tt.env)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectByobu(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"none", envMap(), false},
		{"byobu backend", envMap("BYOBU_BACKEND", "tmux"), true},
		{"byobu config dir", envMap("BYOBU_CONFIG_DIR", "/tmp/byobu"), true},
		{"byobu distro", envMap("BYOBU_DISTRO", "ubuntu"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectByobuFromEnv(tt.env)
			if got != tt.want {
				t.Errorf("detectByobuFromEnv(%v) = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

func TestKittyKeyboardGate(t *testing.T) {
	tests := []struct {
		name     string
		brand    TerminalBrand
		mux      MultiplexerKind
		vteVer   string
		wantOK   bool
		wantSkip string // non-empty means skip
	}{
		{"Kitty — supported", BrandKitty, MuxUndetected, "", true, ""},
		{"Ghostty — supported", BrandGhostty, MuxUndetected, "", true, ""},
		{"WezTerm — supported", BrandWezTerm, MuxUndetected, "", true, ""},
		{"Alacritty — supported", BrandAlacritty, MuxUndetected, "", true, ""},
		{"foot — supported", BrandFoot, MuxUndetected, "", true, ""},
		{"VS Code — skip vscode", BrandVsCode, MuxUndetected, "", false, "vscode"},
		{"Cursor — skip vscode", BrandCursor, MuxUndetected, "", false, "vscode"},
		{"Windsurf — skip vscode", BrandWindsurf, MuxUndetected, "", false, "vscode"},
		{"Apple Terminal — skip apple_terminal", BrandAppleTerminal, MuxUndetected, "", false, "apple_terminal"},
		{"VTE >= 0.82.0 — supported", BrandVte, MuxUndetected, "8300", true, ""},
		{"VTE < 0.82.0 — skip vte_old", BrandVte, MuxUndetected, "7400", false, "vte_old"},
		{"VTE no version — skip vte_old", BrandVte, MuxUndetected, "", false, "vte_old"},
		{"Windows Terminal — skip windows_terminal", BrandWindowsTerminal, MuxUndetected, "", false, "windows_terminal"},
		{"JetBrains — skip jetbrains", BrandJetBrains, MuxUndetected, "", false, "jetbrains"},
		{"screen — skip screen", BrandUnknown, MuxScreen, "", false, "screen"},
		{"tmux — skip tmux", BrandUnknown, MuxTmux, "", false, "tmux"},
		{"unknown — skip unknown_no_multiplexer", BrandUnknown, MuxUndetected, "", false, "unknown_no_multiplexer"},
		{"Unknown+ssH — skip unknown_no_multiplexer", BrandUnknown, MuxUndetected, "", false, "unknown_no_multiplexer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TerminalContext{
				Brand:       tt.brand,
				Multiplexer: tt.mux,
				VteVersion:  tt.vteVer,
			}
			computeKittyKeyboard(&tc)
			if tc.hasKittyKeyboard != tt.wantOK {
				t.Errorf("hasKittyKeyboard = %v, want %v; skipReason=%q", tc.hasKittyKeyboard, tt.wantOK, tc.kittyKeyboardSkipReason)
			}
			if tt.wantSkip != "" && tc.kittyKeyboardSkipReason != tt.wantSkip {
				t.Errorf("skipReason = %q, want %q", tc.kittyKeyboardSkipReason, tt.wantSkip)
			}
		})
	}
}

func TestShiftEnterGate(t *testing.T) {
	tests := []struct {
		name   string
		brand  TerminalBrand
		mux    MultiplexerKind
		kkp    bool
		vteVer string
		want   bool
	}{
		{"KKP enabled — works", BrandKitty, MuxUndetected, true, "", true},
		{"VS Code — not available", BrandVsCode, MuxUndetected, false, "", false},
		{"VTE >= 0.82.0 — works", BrandVte, MuxUndetected, true, "83000", true},
		{"VTE < 0.82.0 — not available", BrandVte, MuxUndetected, false, "74000", false},
		{"Apple Terminal — not available", BrandAppleTerminal, MuxUndetected, false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TerminalContext{
				Brand:            tt.brand,
				Multiplexer:      tt.mux,
				hasKittyKeyboard: tt.kkp,
				kittyKeyboardSkipReason: func() string {
					if tt.kkp {
						return ""
					}
					return "test"
				}(),
			}
			if tt.vteVer != "" {
				tc.VteVersion = tt.vteVer
			}
			// computeShiftEnterCtrlDot needs KKP state already set.
			computeShiftEnterCtrlDot(&tc, envMap())
			if tc.shiftEnterAvailable != tt.want {
				t.Errorf("shiftEnterAvailable = %v, want %v", tc.shiftEnterAvailable, tt.want)
			}
		})
	}
}

func TestTrueColorGate(t *testing.T) {
	tests := []struct {
		name   string
		brand  TerminalBrand
		cterm  string
		want   bool
		reason string
	}{
		{"Kitty — truecolor", BrandKitty, "", true, ""},
		{"Apple Terminal — no truecolor", BrandAppleTerminal, "", false, "apple_terminal"},
		{"VS Code — truecolor", BrandVsCode, "", true, ""},
		{"COLORTERM=truecolor — truecolor", BrandUnknown, "truecolor", true, ""},
		{"Unknown + no env — no truecolor", BrandUnknown, "", false, "unknown_terminal"},
		{"Windows Terminal — truecolor", BrandWindowsTerminal, "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := envMap()
			if tt.cterm != "" {
				env["COLORTERM"] = tt.cterm
			}
			tc := TerminalContext{Brand: tt.brand}
			computeTrueColor(&tc, env)
			if tc.hasTrueColor != tt.want {
				t.Errorf("hasTrueColor = %v, want %v", tc.hasTrueColor, tt.want)
			}
			if tt.reason != "" && tc.hasTrueColorReason != tt.reason {
				t.Errorf("reason = %q, want %q", tc.hasTrueColorReason, tt.reason)
			}
		})
	}
}

func TestOSC8HyperlinkGate(t *testing.T) {
	tests := []struct {
		name   string
		brand  TerminalBrand
		mux    MultiplexerKind
		vteVer string
		want   bool
		reason string
	}{
		{"Kitty — supported", BrandKitty, MuxUndetected, "", true, ""},
		{"Apple Terminal — skip", BrandAppleTerminal, MuxUndetected, "", false, "apple_terminal"},
		{"JetBrains — skip", BrandJetBrains, MuxUndetected, "", false, "unsupported_terminal"},
		{"screen — skip", BrandUnknown, MuxScreen, "", false, "screen"},
		{"VTE >= 0.50.4 — supported", BrandVte, MuxUndetected, "7000", true, ""},
		{"VTE < 0.50.4 — skip", BrandVte, MuxUndetected, "5000", false, "vte_old"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := envMap()
			if tt.vteVer != "" {
				env["VTE_VERSION"] = tt.vteVer
			}
			tc := TerminalContext{
				Brand:       tt.brand,
				Multiplexer: tt.mux,
				VteVersion:  env["VTE_VERSION"],
			}
			computeOSC8Hyperlinks(&tc, env)
			if tc.hasOSC8Hyperlinks != tt.want {
				t.Errorf("hasOSC8Hyperlinks = %v, want %v", tc.hasOSC8Hyperlinks, tt.want)
			}
			if tt.reason != "" && tc.osc8SkipReason != tt.reason {
				t.Errorf("reason = %q, want %q", tc.osc8SkipReason, tt.reason)
			}
		})
	}
}

func TestDetectTerminalContext_Known(t *testing.T) {
	// Simulate a Kitty environment.
	env := envMap(
		"KITTY_WINDOW_ID", "1",
		"COLORTERM", "truecolor",
		"TERM", "xterm-kitty",
	)
	tc := buildTerminalContextFromEnv(env)
	if tc.Brand != BrandKitty {
		t.Errorf("Brand = %v, want Kitty", tc.Brand)
	}
	if !tc.hasTrueColor {
		t.Errorf("hasTrueColor should be true")
	}
	if ok, _ := tc.SupportsKittyKeyboard(); !ok {
		t.Errorf("SupportsKittyKeyboard should be true")
	}
	if !tc.ShiftEnterAvailable() {
		t.Errorf("ShiftEnterAvailable should be true")
	}
}

func TestDetectTerminalContext_VSCode(t *testing.T) {
	env := envMap(
		"TERM_PROGRAM", "vscode",
		"TERM_PROGRAM_VERSION", "1.96.0",
	)
	tc := buildTerminalContextFromEnv(env)
	if tc.Brand != BrandVsCode {
		t.Errorf("Brand = %v, want VsCode", tc.Brand)
	}
	if ok, reason := tc.SupportsKittyKeyboard(); ok {
		t.Errorf("SupportsKittyKeyboard should be false, got reason=%q", reason)
	}
	if tc.ShiftEnterAvailable() {
		t.Errorf("ShiftEnterAvailable should be false")
	}
}

func TestDetectTerminalContext_tmux(t *testing.T) {
	env := envMap(
		"TMUX", "/tmp/tmux-501/default,12345,0",
		"TERM", "screen-256color",
	)
	tc := buildTerminalContextFromEnv(env)
	if tc.Multiplexer != MuxTmux {
		t.Errorf("Multiplexer = %v, want Tmux", tc.Multiplexer)
	}
}

func TestDetectTerminalContext_SSH(t *testing.T) {
	ResetTerminalContext()
	env := envMap(
		"SSH_CONNECTION", "10.0.0.1 22 10.0.0.2 22",
		"TERM", "xterm-256color",
	)
	tc := buildTerminalContextFromEnv(env)
	if !tc.IsSSH {
		t.Errorf("IsSSH should be true, got env=%v", env)
	}
}

func TestDetectTerminalContext_AppleTerminal(t *testing.T) {
	ResetTerminalContext()
	env := envMap(
		"TERM_SESSION_ID", "ABC123",
		"TERM_PROGRAM", "Apple_Terminal",
		"TERM", "xterm-256color",
	)
	tc := buildTerminalContextFromEnv(env)
	if tc.Brand != BrandAppleTerminal {
		t.Errorf("Brand = %v, want AppleTerminal", tc.Brand)
	}
	if ok, _ := tc.HasTrueColor(); ok {
		t.Errorf("hasTrueColor should be false on Apple Terminal, got true")
	}
}

func TestDetectTerminalContext_ByobuOnTmux(t *testing.T) {
	ResetTerminalContext()
	env := envMap(
		"BYOBU_BACKEND", "tmux",
		"TMUX", "/tmp/tmux-501/default",
	)
	tc := buildTerminalContextFromEnv(env)
	if !tc.IsByobu {
		t.Errorf("IsByobu should be true")
	}
	if tc.Multiplexer != MuxTmux {
		t.Errorf("Multiplexer = %v, want Tmux", tc.Multiplexer)
	}
}

func TestDetectTerminalContext_Cached(t *testing.T) {
	ResetTerminalContext()

	// First call populates the cache.
	tc1 := DetectTerminalContext()

	// Second call should return the same (cached) context.
	tc2 := DetectTerminalContext()
	if tc1.Brand != tc2.Brand || tc1.Multiplexer != tc2.Multiplexer {
		t.Error("DetectTerminalContext returned different values on second call (cache miss)")
	}

	// Reset and detect again to verify reset clears the cache.
	ResetTerminalContext()
	tc3 := DetectTerminalContext()
	_ = tc3
}

func TestResetTerminalContext(t *testing.T) {
	ResetTerminalContext()
	tc := DetectTerminalContext()
	tc2 := DetectTerminalContext()
	// After reset, both calls should succeed (no panic) and return cached result.
	if tc.Brand != tc2.Brand {
		t.Error("DetectTerminalContext returned inconsistent brands after reset")
	}
}

func TestBrandString(t *testing.T) {
	if BrandUnknown.String() != "Unknown" {
		t.Errorf("BrandUnknown.String() = %q, want Unknown", BrandUnknown.String())
	}
	if BrandKitty.String() != "Kitty" {
		t.Errorf("BrandKitty.String() = %q, want Kitty", BrandKitty.String())
	}
}

func TestVteHasKKP(t *testing.T) {
	tests := []struct {
		ver  string
		want bool
	}{
		{"", false},
		{"7400", false}, // 0.74.0 — pre-KKP
		{"8100", false}, // 0.81.0 — pre-KKP
		{"8200", true},  // 0.82.0 — first VTE release with KKP
		{"8300", true},
		{"9000", true},
	}
	for _, tt := range tests {
		got := vteHasKKP(tt.ver)
		if got != tt.want {
			t.Errorf("vteHasKKP(%q) = %v, want %v", tt.ver, got, tt.want)
		}
	}
}

func TestBrandIsVSCODEFamily(t *testing.T) {
	if !BrandVsCode.IsVSCODECodeFamily() {
		t.Error("BrandVsCode should be VS Code family")
	}
	if !BrandCursor.IsVSCODECodeFamily() {
		t.Error("BrandCursor should be VS Code family")
	}
	if !BrandWindsurf.IsVSCODECodeFamily() {
		t.Error("BrandWindsurf should be VS Code family")
	}
	if !BrandZed.IsVSCODECodeFamily() {
		t.Error("BrandZed should be VS Code family")
	}
	if BrandKitty.IsVSCODECodeFamily() {
		t.Error("BrandKitty should NOT be VS Code family")
	}
}

func TestMultiplexerIntercepts(t *testing.T) {
	if !MuxTmux.InterceptsCSIQueries() {
		t.Error("MuxTmux should intercept CSI queries")
	}

	if MuxUndetected.InterceptsCSIQueries() {
		t.Error("MuxUndetected should NOT intercept CSI queries")
	}
	if !MuxTmux.InterceptsOSC52() {
		t.Error("MuxTmux should intercept OSC 52")
	}
	if MuxCmux.InterceptsOSC52() {
		t.Error("MuxCmux should NOT intercept OSC 52")
	}
}

func TestVteHasKKP_VersionCheck(t *testing.T) {
	tests := []struct {
		ver  string
		want bool
	}{
		{"", false},
		{"7400", false},
		{"8100", false},
		{"8200", true},
		{"8300", true},
		{"9000", true},
	}
	for _, tt := range tests {
		got := vteHasKKP(tt.ver)
		if got != tt.want {
			t.Errorf("vteHasKKP(%q) = %v, want %v", tt.ver, got, tt.want)
		}
	}
}
