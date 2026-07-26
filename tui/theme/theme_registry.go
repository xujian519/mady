package theme

// theme_registry.go — Named theme registry.
//
// Provides a registry of built-in themes with methods for listing
// and applying themes by name. Thread-safe for concurrent reads.

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// ThemeInfo describes a registered theme in the registry.
type ThemeInfo struct {
	Name    string // canonical short name (e.g. "mady-dark")
	Display string // human-readable name (e.g. "Mady Dark")
	Dark    bool   // true for dark-background themes
}

// ThemeFactory constructs a SemanticTheme when the theme is applied.
type ThemeFactory func() *SemanticTheme

type themeEntry struct {
	info    ThemeInfo
	factory ThemeFactory
}

var (
	registryMu    sync.RWMutex
	themeRegistry []themeEntry
)

func init() {
	registerBuiltin("mady-dark", "Mady Dark", true, DefaultMadyDark)
	registerBuiltin("mady-light", "Mady Light", false, DefaultSemanticLight)
	registerBuiltin("tokyo-night", "Tokyo Night", true, tokyoNightFactory)
	registerBuiltin("rose-pine-moon", "Rose Pine Moon", true, rosePineMoonFactory)
	registerBuiltin("grok-night", "Grok Night", true, grokNightFactory)
	registerBuiltin("high-contrast", "High Contrast", true, HighContrast)
	registerBuiltin("colorblind", "Color Blind", false, ColorBlind)
	// Auto is a meta-theme that resolves to dark or light based on system appearance.
	// Not registered via registerBuiltin because it delegates to one of the concrete themes.
	registerBuiltin("auto", "Auto (follow system)", true, autoThemeFactory)
}

func registerBuiltin(name, display string, dark bool, factory ThemeFactory) {
	registryMu.Lock()
	themeRegistry = append(themeRegistry, themeEntry{
		info:    ThemeInfo{Name: name, Display: display, Dark: dark},
		factory: factory,
	})
	registryMu.Unlock()
}

// autoThemeFactory resolves the current system appearance and returns the
// matching concrete theme.
func autoThemeFactory() *SemanticTheme {
	appearance := DetectSystemAppearance()
	if appearance == AppearanceDark {
		return DefaultMadyDark()
	}
	return DefaultSemanticLight()
}

func tokyoNightFactory() *SemanticTheme {
	return &SemanticTheme{
		Name:              "Tokyo Night",
		Accent:            "#7aa2f7",
		Border:            "#3b4261",
		BorderAccent:      "#7aa2f7",
		BorderMuted:       "#565f89",
		Success:           "#9ece6a",
		Error:             "#f7768e",
		Warning:           "#e0af68",
		Muted:             "#565f89",
		Dim:               "#3b4261",
		Text:              "#c0caf5",
		System:            "#7aa2f7",
		ThinkingText:      "#565f89",
		UserMessage:       "#7aa2f7",
		AssistantText:     "#c0caf5",
		Background:        "#1a1b26",
		Surface:           "#1f2233",
		SurfaceRaised:     "#24283b",
		MdHeading:         "#7dcfff",
		MdLink:            "#7aa2f7",
		MdCode:            "#9ece6a",
		MdCodeBlock:       "#9ece6a",
		MdCodeBlockBorder: "#3b4261",
		MdQuote:           "#565f89",
		MdQuoteBorder:     "#7aa2f7",
		MdHr:              "#3b4261",
		MdListBullet:      "#7aa2f7",
		SyntaxComment:     "#565f89",
		SyntaxKeyword:     "#bb9af7",
		SyntaxFunction:    "#7aa2f7",
		SyntaxVariable:    "#f7768e",
		SyntaxString:      "#9ece6a",
		SyntaxNumber:      "#e0af68",
		SyntaxType:        "#7dcfff",
		SyntaxOperator:    "#c0caf5",
		SyntaxPunctuation: "#565f89",
		LoaderSpinner:     "#7aa2f7",
		ProgressBar:       "#7aa2f7",
	}
}

func rosePineMoonFactory() *SemanticTheme {
	return &SemanticTheme{
		Name:              "Rose Pine Moon",
		Accent:            "#c4a7e7",
		Border:            "#1f1d30",
		BorderAccent:      "#c4a7e7",
		BorderMuted:       "#403d52",
		Success:           "#3e8fb0",
		Error:             "#eb6f92",
		Warning:           "#f6c177",
		Muted:             "#403d52",
		Dim:               "#2a273f",
		Text:              "#e0def4",
		System:            "#3e8fb0",
		ThinkingText:      "#403d52",
		UserMessage:       "#c4a7e7",
		AssistantText:     "#e0def4",
		Background:        "#232136",
		Surface:           "#2a273f",
		SurfaceRaised:     "#393552",
		MdHeading:         "#3e8fb0",
		MdLink:            "#c4a7e7",
		MdCode:            "#ea9a97",
		MdCodeBlock:       "#ea9a97",
		MdCodeBlockBorder: "#403d52",
		MdQuote:           "#403d52",
		MdQuoteBorder:     "#c4a7e7",
		MdHr:              "#403d52",
		MdListBullet:      "#c4a7e7",
		SyntaxComment:     "#403d52",
		SyntaxKeyword:     "#3e8fb0",
		SyntaxFunction:    "#c4a7e7",
		SyntaxVariable:    "#eb6f92",
		SyntaxString:      "#ea9a97",
		SyntaxNumber:      "#f6c177",
		SyntaxType:        "#3e8fb0",
		SyntaxOperator:    "#e0def4",
		SyntaxPunctuation: "#403d52",
		LoaderSpinner:     "#c4a7e7",
		ProgressBar:       "#3e8fb0",
	}
}

func grokNightFactory() *SemanticTheme {
	return &SemanticTheme{
		Name:              "Grok Night",
		Accent:            "#e1e1e1",
		Border:            "#2c2c2c",
		BorderAccent:      "#e1e1e1",
		BorderMuted:       "#414141",
		Success:           "#9ece6a",
		Error:             "#f7768e",
		Warning:           "#e0af68",
		Muted:             "#6c6c6c",
		Dim:               "#5a5a5a",
		Text:              "#e1e1e1",
		System:            "#7aa2f7",
		ThinkingText:      "#6c6c6c",
		UserMessage:       "#e1e1e1",
		AssistantText:     "#e1e1e1",
		Background:        "#0a0a0a",
		Surface:           "#141414",
		SurfaceRaised:     "#242424",
		MdHeading:         "#73daca",
		MdLink:            "#7aa2f7",
		MdCode:            "#3a95ab",
		MdCodeBlock:       "#3a95ab",
		MdCodeBlockBorder: "#414141",
		MdQuote:           "#6c6c6c",
		MdQuoteBorder:     "#e1e1e1",
		MdHr:              "#414141",
		MdListBullet:      "#e1e1e1",
		SyntaxComment:     "#6c6c6c",
		SyntaxKeyword:     "#bb9af7",
		SyntaxFunction:    "#7aa2f7",
		SyntaxVariable:    "#f7768e",
		SyntaxString:      "#9ece6a",
		SyntaxNumber:      "#e0af68",
		SyntaxType:        "#7dcfff",
		SyntaxOperator:    "#e1e1e1",
		SyntaxPunctuation: "#6c6c6c",
		LoaderSpinner:     "#e1e1e1",
		ProgressBar:       "#e1e1e1",
	}
}

// ThemeNames returns a sorted list of all registered theme names.
func ThemeNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(themeRegistry))
	for _, e := range themeRegistry {
		names = append(names, e.info.Name)
	}
	sort.Strings(names)
	return names
}

// ThemeInfoByName returns ThemeInfo for a theme name, or nil if not found.
func ThemeInfoByName(name string) *ThemeInfo {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, e := range themeRegistry {
		if e.info.Name == name {
			return &e.info
		}
	}
	return nil
}

// ApplyThemeByName applies the named theme and returns its SemanticTheme.
// Returns nil if the name is not found.
func ApplyThemeByName(name string) *SemanticTheme {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, e := range themeRegistry {
		if e.info.Name == name {
			sem := e.factory()
			SetSemanticTheme(sem, ColorModeFromEnv())
			return sem
		}
	}
	return nil
}

// SetThemeByName applies a theme by name and returns an error if not found.
func SetThemeByName(name string) error {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, e := range themeRegistry {
		if e.info.Name == name {
			sem := e.factory()
			SetSemanticTheme(sem, ColorModeFromEnv())
			return nil
		}
	}
	return fmt.Errorf("theme %q not found", name)
}

// StartAutoThemeWatcher starts a system appearance watcher that switches
// between MadyDark and MadyLight when the OS theme changes and the current
// active theme is "auto". Returns a cancel function.
func StartAutoThemeWatcher() func() {
	return WatchSystemAppearance(context.TODO(), 0, func(a SystemAppearance) {
		_ = a
		ApplyThemeByName("auto")
	})
}

// RegisterTheme registers a custom theme in the registry.
// If a theme with the same name already exists, it is replaced.
func RegisterTheme(info ThemeInfo, factory ThemeFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	for i, e := range themeRegistry {
		if e.info.Name == info.Name {
			themeRegistry[i] = themeEntry{info: info, factory: factory}
			return
		}
	}
	themeRegistry = append(themeRegistry, themeEntry{info: info, factory: factory})
}
