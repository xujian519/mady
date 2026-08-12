package terminal

import (
	"os"
	"strings"
)

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
