package tui

import (
	"time"

	core "github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// dimStyle values used by cell-level dimBackground. Palette lookups are
// function calls, so these can't be `const`.
var (
	dimTextAttr = core.AttrDim
)

// dimBgColor returns the current surface background color as a core.Color
// for overlay dimming. Derived from the theme's surface color so it follows
// theme switches (dark → ~#0c1b2a, light → ~#e0e0e0).
func dimBgColor() core.Color {
	sem := theme.CurrentPalette().Semantic
	if sem == nil {
		return core.Palette(235)
	}
	// Use the Background hex from the current semantic theme.
	bg := sem.Background
	if bg == "" {
		bg = sem.Surface
	}
	if bg == "" {
		return core.Palette(235)
	}
	return hexToCoreColor(bg)
}

// hexToCoreColor converts a "#rrggbb" hex string to a core.Color using
// the best available encoding (truecolor RGB tag).
func hexToCoreColor(hex string) core.Color {
	if len(hex) < 6 {
		return core.Palette(235)
	}
	h := hex
	if h[0] == '#' {
		h = h[1:]
	}
	if len(h) != 6 {
		return core.Palette(235)
	}
	// Parse hex to RGB.
	var r, g, b uint8
	for i := 0; i < 3; i++ {
		v := uint8(0)
		for j := 0; j < 2; j++ {
			c := h[i*2+j]
			switch {
			case c >= '0' && c <= '9':
				v = v*16 + (c - '0')
			case c >= 'a' && c <= 'f':
				v = v*16 + (c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				v = v*16 + (c - 'A' + 10)
			default:
				return core.Palette(235) // non-hex char → fallback
			}
		}
		switch i {
		case 0:
			r = v
		case 1:
			g = v
		case 2:
			b = v
		}
	}
	return core.RGB(r, g, b)
}

// ---------------------------------------------------------------------------
// OverlayCategory — classifies overlays by purpose so the TUI can apply
// consistent sizing, focus behavior, and close rules per category.
// ---------------------------------------------------------------------------

// OverlayCategory describes the purpose of an overlay panel.
type OverlayCategory int

const (
	OverlaySelection OverlayCategory = iota // 选择型—快速切换对象（会话/线程/分支）
	OverlayReview                           // 审阅型—查看依据与细节（证据/引用/键位表）
	OverlayGate                             // 复核型—结构化审阅（复核门/高风确认）
	OverlaySystem                           // 系统型—运行条件解释（降级/阻塞/日志）
)

// DefaultOverlaySize returns the default width/height percentage for a given
// overlay category. Callers may override these defaults per-instance.
func DefaultOverlaySize(cat OverlayCategory) (wPct, hPct int64) {
	switch cat {
	case OverlaySelection:
		return 40, 30
	case OverlayReview:
		return 60, 60
	case OverlayGate:
		return 70, 75
	case OverlaySystem:
		return 50, 40
	default:
		return 60, 60
	}
}

// ---------------------------------------------------------------------------
// OverlayTransition — 打开动画配置。
//
// 零值（OverlayTransitionNone）表示无动画，现有全部 overlay 行为不变。
// 动画由 TUI.PushOverlay 启动（见 tui_focus.go 的 startOverlayAnimation），
// 进度在 composeOverlays 合成时读取：
//   - SlideUp：面板从视口底部滑入（origin 行偏移插值）
//   - Fade：背景变暗强度从 0 渐变到 1（毛玻璃渐显）
// ---------------------------------------------------------------------------

// OverlayTransitionKind 描述 overlay 打开动画的类型。
type OverlayTransitionKind int

const (
	// OverlayTransitionNone 无动画（零值，默认）。
	OverlayTransitionNone OverlayTransitionKind = iota
	// OverlayTransitionSlideUp 从视口底部滑入。
	OverlayTransitionSlideUp
	// OverlayTransitionFade 背景变暗强度渐变（毛玻璃渐显）。
	OverlayTransitionFade
)

// OverlayTransition 配置一次 overlay 打开动画。Kind 为零值时忽略其余字段。
type OverlayTransition struct {
	Kind     OverlayTransitionKind
	Duration time.Duration
	// Easing 为 nil 时默认 EaseOutCubic（先快后慢）。
	Easing core.Easing
}

// ---------------------------------------------------------------------------
// Overlay — a floating panel mounted on top of the root view.
//
// Positioning supports:
//   - Anchor points (corners / center / cardinal mid-points) — OverlayAnchor*.
//   - Percentage offsets — the overlay is placed at (pctX, pctY) of the
//     viewport minus the overlay's own size × anchor ratio.
//   - Absolute rows/cols via Row/Col.
//
// Width/Height accept either fixed cells or percentages (use Percent = true).
// ---------------------------------------------------------------------------

// OverlayAnchor describes how (Row, Col) or (PercentX, PercentY) should be
// interpreted relative to the overlay itself.
type OverlayAnchor int64

const (
	AnchorTopLeft OverlayAnchor = iota
	AnchorTopCenter
	AnchorTopRight
	AnchorMiddleLeft
	AnchorCenter
	AnchorMiddleRight
	AnchorBottomLeft
	AnchorBottomCenter
	AnchorBottomRight
)

// OverlaySize specifies a dimension in cells or percent-of-viewport.
type OverlaySize struct {
	Value   int64
	Percent bool // if true, Value is 0..100 of the viewport
	Min     int64
	Max     int64
}

// resolve computes the effective cell count for a given viewport dimension.
func (s OverlaySize) resolve(viewport int64) int64 {
	v := s.Value
	if s.Percent {
		v = viewport * s.Value / 100
	}
	if s.Min > 0 && v < s.Min {
		v = s.Min
	}
	if s.Max > 0 && v > s.Max {
		v = s.Max
	}
	if v < 1 {
		v = 1
	}
	if v > viewport {
		v = viewport
	}
	return v
}

// Overlay is a floating panel positioned on top of the root view.
type Overlay struct {
	Content core.Component

	Anchor OverlayAnchor

	// Absolute positioning (takes precedence over Percent*).
	UseAbsolute bool
	Row         int64
	Col         int64

	// Percent positioning (only used if !UseAbsolute).
	PercentX int64 // 0..100
	PercentY int64 // 0..100

	Width  OverlaySize
	Height OverlaySize

	// Category classifies the overlay's purpose for default sizing and
	// behavior. Defaults to OverlaySelection (zero value) for backward
	// compatibility.
	Category OverlayCategory

	// Focus tells the TUI to push Content onto the focus stack when the
	// overlay is mounted, and pop it when removed.
	Focus bool

	// NonModal marks this overlay as transparent to input: while it is open,
	// key/mouse events still reach components behind it (the focused overlay
	// component receives input first, matching modal behavior).
	//
	// The zero value (false) keeps the overlay modal — the historical
	// default, so existing overlays behave unchanged without opting in.
	// Non-modal overlays are for auxiliary panels that must not block the
	// main workspace (e.g. a floating reference panel).
	NonModal bool

	// DimBackground applies a uniform "frosted glass" dim effect to every
	// cell outside the overlay rectangle. The dimming adds dimTextAttr and
	// a dark glass background (dimBgColor) to cells, creating visual focus
	// on the overlay content while keeping the underlying layout visible.
	//
	// No drop shadow is drawn: the shadow ring (a slightly darker bg on the
	// overlay's right column and bottom band) rendered as faint rectangular
	// "box" edges against the dim backdrop — visually indistinguishable from
	// an artifact — so the backdrop is kept perfectly uniform (see
	// dimBackgroundRows).
	DimBackground bool

	// Transition 配置 overlay 打开动画（零值 = 无动画，默认）。
	// 由 TUI.PushOverlay 启动；openAt 记录动画开始时刻。
	Transition OverlayTransition
	openAt     time.Time

	// Phase 4.2: stored render position for mouse coordinate translation.
	// Set by composeOverlays during rendering; used by TranslateMouse.
	renderedRow    int64
	renderedCol    int64
	renderedWidth  int64
	renderedHeight int64
}

// transitionProgress 返回当前帧的动画进度 p∈[0,1] 与是否已结束。
// 无动画或未开始（openAt 为零值）时返回 (1, true)。
func (o *Overlay) transitionProgress(now time.Time) (float64, bool) {
	tr := o.Transition
	if tr.Kind == OverlayTransitionNone || tr.Duration <= 0 || o.openAt.IsZero() {
		return 1, true
	}
	p := float64(now.Sub(o.openAt)) / float64(tr.Duration)
	if p >= 1 {
		return 1, true
	}
	if p < 0 {
		p = 0
	}
	e := tr.Easing
	if e == nil {
		e = core.EaseOutCubic
	}
	return e(p), false
}

// dimIntensity 返回当前帧该 overlay 的背景变暗强度。
// Fade 过渡从 0（无变暗）渐变到 1（全变暗）；其余恒为 1。
func (o *Overlay) dimIntensity(now time.Time) float64 {
	if o.Transition.Kind == OverlayTransitionFade {
		p, _ := o.transitionProgress(now)
		return p
	}
	return 1
}

// NewCenteredOverlay is a convenience constructor for a centered panel
// whose size is a percentage of the viewport.
func NewCenteredOverlay(c core.Component, widthPct, heightPct int64) *Overlay {
	return &Overlay{
		Content:  c,
		Anchor:   AnchorCenter,
		PercentX: 50, PercentY: 50,
		Width:  OverlaySize{Value: widthPct, Percent: true, Min: 10},
		Height: OverlaySize{Value: heightPct, Percent: true, Min: 3},
	}
}

// NewBottomRightOverlay is a convenience for a small toast at the bottom-right.
func NewBottomRightOverlay(c core.Component, width, height int64) *Overlay {
	return &Overlay{
		Content:  c,
		Anchor:   AnchorBottomRight,
		PercentX: 100, PercentY: 100,
		Width:  OverlaySize{Value: width},
		Height: OverlaySize{Value: height},
	}
}

type overlayOrigin struct{ row, col int64 }

// TranslateMouse adjusts absolute screen mouse coordinates to the overlay's local
// coordinate space. Returns (row - renderedRow, col - renderedCol). If the mouse
// is outside the overlay region, ok is false.
func (o *Overlay) TranslateMouse(row, col int64) (localRow, localCol int64, ok bool) {
	if o == nil {
		return 0, 0, false
	}
	localRow = row - o.renderedRow
	localCol = col - o.renderedCol
	if localRow < 0 || localCol < 0 ||
		localRow >= o.renderedHeight || localCol >= o.renderedWidth {
		return 0, 0, false
	}
	ok = true
	return
}

func resolveOverlayOrigin(ov *Overlay, cols, rows, w, h int64) overlayOrigin {
	var r, c int64
	if ov.UseAbsolute {
		r = ov.Row
		c = ov.Col
	} else {
		r = rows * ov.PercentY / 100
		c = cols * ov.PercentX / 100
	}
	// Apply anchor offsets (how much of the overlay sits to the left/above).
	switch ov.Anchor {
	case AnchorTopLeft:
		// no offset
	case AnchorTopCenter:
		c -= w / 2
	case AnchorTopRight:
		c -= w
	case AnchorMiddleLeft:
		r -= h / 2
	case AnchorCenter:
		r -= h / 2
		c -= w / 2
	case AnchorMiddleRight:
		r -= h / 2
		c -= w
	case AnchorBottomLeft:
		r -= h
	case AnchorBottomCenter:
		r -= h
		c -= w / 2
	case AnchorBottomRight:
		r -= h
		c -= w
	}
	if r < 0 {
		r = 0
	}
	if c < 0 {
		c = 0
	}
	if r+h > rows {
		r = rows - h
	}
	if c+w > cols {
		c = cols - w
	}
	if r < 0 {
		r = 0
	}
	if c < 0 {
		c = 0
	}
	return overlayOrigin{row: r, col: c}
}
