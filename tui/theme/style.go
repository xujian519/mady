package theme

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/xujian519/mady/tui/terminal"
)

// ---------------------------------------------------------------------------
// ANSI escape codes — delegated to terminal package
// ---------------------------------------------------------------------------

// Esc and Reset re-export terminal constants for backward compatibility.
// New code should use terminal.Esc and terminal.Reset directly.
const (
	Esc   = terminal.Esc
	Reset = terminal.Reset
)

// Color represents an SGR foreground color code (30–37 normal, 90–97 bright,
// 39 = default). It maps directly to the numeric parameter in an SGR sequence.
type Color int64

// Standard and bright ANSI foreground colors. Default (39) uses the terminal's
// default foreground.
const (
	Black   Color = 30
	Red     Color = 31
	Green   Color = 32
	Yellow  Color = 33
	Blue    Color = 34
	Magenta Color = 35
	Cyan    Color = 36
	White   Color = 37
	Default Color = 39

	BrightBlack   Color = 90
	BrightRed     Color = 91
	BrightGreen   Color = 92
	BrightYellow  Color = 93
	BrightBlue    Color = 94
	BrightMagenta Color = 95
	BrightCyan    Color = 96
	BrightWhite   Color = 97
)

// Attr represents a text attribute (SGR code). Values follow the ANSI SGR
// specification (1 = bold, 2 = dim, etc.).
type Attr int64

// Text attributes usable in Style.Attrs. Values are SGR attribute codes.
const (
	Bold      Attr = 1
	Dim       Attr = 2
	Italic    Attr = 3
	Underline Attr = 4
	Blink     Attr = 5
	Reverse   Attr = 7
	Strike    Attr = 9
)

// Style describes a text style with foreground color, background color, and attributes.
type Style struct {
	fg       Color
	bg       Color
	attrs    []Attr
	fgParams string // CSI parameter segment, e.g. "38;2;r;g;b" or "38;5;n" (wins over fg)
	bgParams string // e.g. "48;2;r;g;b"
}

// NewStyle returns a Style with default foreground and background (no attrs).
func NewStyle() Style { return Style{fg: Default, bg: Default} }

// Fg sets the foreground color from the basic ANSI palette (30–37/90–97).
// For truecolor or 256-color, use WithFgParams instead.
func (s Style) Fg(c Color) Style {
	s.fg = c
	s.fgParams = ""
	return s
}

// WithFgParams sets a truecolor / 256-color foreground (SGR parameter list without ESC/[).
func (s Style) WithFgParams(csiParams string) Style {
	s.fgParams = csiParams
	s.fg = Default
	return s
}

// Bg sets the background color from the basic ANSI palette. The Color value
// is internally shifted by +10 (30→40, etc.). For truecolor/256-color, use
// WithBgParams.
func (s Style) Bg(c Color) Style {
	s.bg = c + 10
	s.bgParams = ""
	return s
}

// WithBgParams sets a truecolor / 256-color background.
func (s Style) WithBgParams(csiParams string) Style {
	s.bgParams = csiParams
	s.bg = Default
	return s
}

// Bold returns a new Style with the bold attribute set.
func (s Style) Bold() Style { s.attrs = append(s.attrs, Bold); return s }

// Dim returns a new Style with the dim attribute set.
func (s Style) Dim() Style { s.attrs = append(s.attrs, Dim); return s }

// Italic returns a new Style with the italic attribute set.
func (s Style) Italic() Style { s.attrs = append(s.attrs, Italic); return s }

// Underline returns a new Style with the underline attribute set.
func (s Style) Underline() Style { s.attrs = append(s.attrs, Underline); return s }

// Strike returns a new Style with the strikethrough attribute set.
func (s Style) Strike() Style { s.attrs = append(s.attrs, Strike); return s }

// BgStrip returns the raw background SGR escape sequence for this Style
// (e.g. "\x1b[48;2;22;76;99m"), or "" if no background is set.
// Unlike Render(), this ignores ColorEnabled() because callers need the raw
// ANSI sequence for theme string construction that is later parsed by core.
func (s Style) BgStrip() string {
	if s.bgParams != "" {
		return Esc + s.bgParams + "m"
	}
	if s.bg != Default {
		return Esc + fmt.Sprintf("%d", s.bg) + "m"
	}
	return ""
}

// Render wraps text with the SGR escape sequence for this Style and appends
// a reset. If color is disabled (NO_COLOR / dumb terminal), text is returned
// unchanged.
func (s Style) Render(text string) string {
	if !ColorEnabled() {
		return text
	}
	var parts []string
	for _, a := range s.attrs {
		parts = append(parts, fmt.Sprintf("%d", a))
	}
	if s.fgParams != "" {
		parts = append(parts, strings.Split(s.fgParams, ";")...)
	} else if s.fg != Default {
		parts = append(parts, fmt.Sprintf("%d", s.fg))
	}
	if s.bgParams != "" {
		parts = append(parts, strings.Split(s.bgParams, ";")...)
	} else if s.bg != Default {
		parts = append(parts, fmt.Sprintf("%d", s.bg))
	}
	if len(parts) == 0 {
		return text
	}
	return Esc + strings.Join(parts, ";") + "m" + text + Reset
}

// ---------------------------------------------------------------------------
// Color detection
// ---------------------------------------------------------------------------

// colorOverride is a tri-state flag: nil means auto-detect from environment,
// non-nil forces color enabled/disabled. Uses atomic.Pointer for thread-safe
// concurrent reads (ColorEnabled runs in the render loop) and writes
// (ForceColor may be called from any goroutine).
var colorOverride atomic.Pointer[bool]

// ForceColor overrides color detection: enabled=true forces colors on,
// enabled=false forces colors off. Subsequent ColorEnabled calls honor
// the override until the next ForceColor call.
func ForceColor(enabled bool) {
	e := enabled
	colorOverride.Store(&e)
}

// ColorEnabled reports whether ANSI color output should be emitted. When
// ForceColor has been called, it returns the forced value; otherwise it
// auto-detects from NO_COLOR / FORCE_COLOR / TERM / stdout TTY status.
func ColorEnabled() bool {
	if v := colorOverride.Load(); v != nil {
		return *v
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	if term := os.Getenv("TERM"); term == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ---------------------------------------------------------------------------
// Unicode symbols
// ---------------------------------------------------------------------------

const (
	SymbolCheck    = "✓"
	SymbolCross    = "✗"
	SymbolArrow    = "→"
	SymbolBullet   = "•"
	SymbolDot      = "·"
	SymbolStar     = "★"
	SymbolWarning  = "⚠"
	SymbolInfo     = "ℹ"
	SymbolThinking = "◐"
	SymbolRight    = "▸"
	SymbolDown     = "▾"

	// Box-drawing primitives (Box Light / Heavy families). 所有原语
	// 统一为 1-cell 宽，便于 VisibleWidth / 版面栅格计算。
	SymbolBarLightV      = "│"       // 竖线：时间线/色带连接
	SymbolBarHeavyV      = "┃"       // 粗竖线：强调左栏
	SymbolBarQuote       = "▌"       // 引用块左边框：实心半格
	SymbolBarAccentThin  = "▏"       // 跨角色色带：1/8 细竖条
	SymbolRuleLightH     = "─"       // 分隔细线
	SymbolRuleHeavyH     = "━"       // 分隔粗线
	SymbolRuleEighthDots = SymbolDot // 同角色紧凑分隔符（eighth-width dot series）

	// Glyphs 语义层：给调用方使用命名字面量，而不是硬编码 ▸/▾
	ChevronCollapsed = SymbolRight // 折叠态提示
	ChevronExpanded  = SymbolDown  // 展开态提示
	StreamCursor     = "▊"         // 流式输出末尾光标
	ScrollUpHint     = "▲"
	ScrollDownHint   = "↓"
)

// Spacing constants —— 对应竞品分析的 8px baseline 栅格（在 TUI
// 中用"行数/空格数"近似）。
const (
	Spacer0        = 0
	Spacer1        = 1 // ~8px 栅格
	Spacer2        = 2 // ~16px
	Spacer3        = 3 // ~24px
	IndentEdge     = 0 // 气泡对 viewport 的默认缩进
	IndentQuote    = 2 // 引用块/列表项的内层缩进
	IndentTimeline = 1 // 时间线/工具组色带与正文之间的 gutter

	// Bubble sizing（同 chat_history_render_message.messageBubbleWidth
	// 语义一致，提取到 tokens 后供所有组件复用）。
	BubbleMaxRatio = 0.85 // ≥60cols 终端时最大气泡占比
	NarrowCols     = 60   // <此阈值为窄屏，气泡放宽至 100%
)

// RoleTransitionBandRule returns the horizontal rule length for the
// cross-role accent band (see renderRoleTransitionBand). 所有调用方
// 都走这个函数而不是各自算一个 magic number。
func RoleTransitionBandRule(bubbleWidth int64) int {
	if bubbleWidth < 4 {
		bubbleWidth = 4
	}
	return int(bubbleWidth) - 1
}

// ---------------------------------------------------------------------------
// Multi-level icon with Nerd Font / Unicode / ASCII fallback
// ---------------------------------------------------------------------------

// Icon represents a single symbol with three fallback levels:
//   - NerdFont: PUA-encoded glyph (requires a Nerd Font installed).
//   - Unicode:  standard Unicode codepoint.
//   - ASCII:    plain ASCII fallback (always safe).
type Icon struct {
	NerdFont string
	Unicode  string
	ASCII    string
}

// ResolveIcon returns the best available representation of the icon based on
// the terminal's detected Nerd Font support.
func ResolveIcon(ic Icon) string {
	switch terminal.NerdFontsSupported() {
	case terminal.NerdFontAvailable:
		if ic.NerdFont != "" {
			return ic.NerdFont
		}
		fallthrough
	default:
		if ic.Unicode != "" {
			return ic.Unicode
		}
		return ic.ASCII
	}
}

// Common icons used throughout the TUI.
var (
	IconFolder = Icon{NerdFont: "\uf07b", Unicode: "📁", ASCII: "[D]"}
	IconFile   = Icon{NerdFont: "\uf15b", Unicode: "📄", ASCII: "[F]"}
	IconSearch = Icon{NerdFont: "\uf002", Unicode: "🔍", ASCII: "[S]"}
	IconGear   = Icon{NerdFont: "\uf013", Unicode: "⚙", ASCII: "[C]"}
	IconUser   = Icon{NerdFont: "\uf007", Unicode: "👤", ASCII: "[U]"}
	IconCheck  = Icon{NerdFont: "\uf00c", Unicode: SymbolCheck, ASCII: "[Y]"}
	IconX      = Icon{NerdFont: "\uf00d", Unicode: SymbolCross, ASCII: "[N]"}
	IconTime   = Icon{NerdFont: "\uf017", Unicode: "🕐", ASCII: "[T]"}
	IconBranch = Icon{NerdFont: "\uf1d3", Unicode: "⑂", ASCII: "[B]"}
	IconLock   = Icon{NerdFont: "\uf023", Unicode: "🔒", ASCII: "[L]"}
)
