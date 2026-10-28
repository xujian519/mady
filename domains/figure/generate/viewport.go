package generate

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// SVG 根元素的视口换算：把画布尺寸、viewBox 与 preserveAspectRatio 译成
// 「用户单位 → 毫米」的映射。落版与复核共用同一映射，毫米才指同一个长度。
const (
	pxPerInch = 96.0
	mmPerInch = 25.4
	ptPerInch = 72.0
)

// MMPerUserUnit 是一个用户单位（无 viewBox 时的 CSS 像素）折算的毫米数。
const MMPerUserUnit = mmPerInch / pxPerInch

// ViewBox 是 SVG 的 viewBox（用户单位）。
type ViewBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Viewport 是根元素的视口映射：Xmm = ScaleX·(x − OriginX)。
type Viewport struct {
	// WidthMm / HeightMm 是根元素声明的画布尺寸（无法解析时 nil）。
	WidthMm  *float64 `json:"width_mm,omitempty"`
	HeightMm *float64 `json:"height_mm,omitempty"`
	// ViewportWidthMm / ViewportHeightMm 是换算用的视口尺寸。
	ViewportWidthMm  *float64 `json:"viewport_width_mm,omitempty"`
	ViewportHeightMm *float64 `json:"viewport_height_mm,omitempty"`
	// ViewBox 为根元素声明的 viewBox（不可解析时 nil）。
	ViewBox *ViewBox `json:"view_box,omitempty"`
	ScaleX  float64  `json:"scale_x"`
	ScaleY  float64  `json:"scale_y"`
	OriginX float64  `json:"origin_x"`
	OriginY float64  `json:"origin_y"`
	// Note 说明近似原因（两轴缩放不等、内容被裁等）；精确时为空。
	Note string `json:"note,omitempty"`
}

var attrPattern = func(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b` + name + `\s*=\s*"([^"]*)"`)
}

var (
	widthAttrRE   = attrPattern("width")
	heightAttrRE  = attrPattern("height")
	viewBoxAttrRE = attrPattern("viewBox")
	aspectAttrRE  = attrPattern("preserveAspectRatio")
)

// svgRootTagRE 匹配根元素开始标签；属性值内的 `>` 被引号分支吃掉，不会提前截断。
var svgRootTagRE = regexp.MustCompile(`(?is)<svg\b(?:"[^"]*"|'[^']*'|[^>"'])*>`)

// RootTag 返回 SVG 文本中的根元素开始标签（不含结尾的 `>`）。
func RootTag(svg string) string {
	match := svgRootTagRE.FindString(svg)
	if match == "" {
		return ""
	}
	return strings.TrimSuffix(match, ">")
}

func attrValue(re *regexp.Regexp, tag string) (string, bool) {
	match := re.FindStringSubmatch(tag)
	if match == nil {
		return "", false
	}
	return match[1], true
}

var lengthRE = regexp.MustCompile(`(?i)^\s*(-?\d+(?:\.\d+)?)\s*([a-z%]*)\s*$`)

// ParseLengthMm 把 SVG 长度解析为毫米；百分比等相对值返回 ok=false。
func ParseLengthMm(raw string) (float64, bool) {
	match := lengthRE.FindStringSubmatch(raw)
	if match == nil {
		return 0, false
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(match[2]) {
	case "", "px":
		return value / pxPerInch * mmPerInch, true
	case "mm":
		return value, true
	case "cm":
		return value * 10, true
	case "in":
		return value * mmPerInch, true
	case "pt":
		return value / ptPerInch * mmPerInch, true
	default:
		return 0, false
	}
}

func parseViewBox(raw string) *ViewBox {
	parts := strings.FieldsFunc(strings.TrimSpace(raw), func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n'
	})
	if len(parts) != 4 {
		return nil
	}
	values := make([]float64, 4)
	for i, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
			return nil
		}
		values[i] = value
	}
	if values[2] <= 0 || values[3] <= 0 {
		return nil
	}
	return &ViewBox{X: values[0], Y: values[1], Width: values[2], Height: values[3]}
}

type aspectRatio struct {
	kind   string // meet | slice | none | invalid
	alignX float64
	alignY float64
}

var aspectRE = regexp.MustCompile(`(?i)^(?:defer\s+)?(none|x(Min|Mid|Max)Y(Min|Mid|Max))(?:\s+(meet|slice))?$`)

func alignRatio(keyword string) float64 {
	switch strings.ToLower(keyword) {
	case "min":
		return 0
	case "mid":
		return 0.5
	default:
		return 1
	}
}

func parseAspectRatio(raw string, ok bool) aspectRatio {
	if !ok {
		return aspectRatio{kind: "meet", alignX: 0.5, alignY: 0.5}
	}
	match := aspectRE.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return aspectRatio{kind: "invalid"}
	}
	if strings.EqualFold(match[1], "none") {
		return aspectRatio{kind: "none"}
	}
	kind := "meet"
	if strings.EqualFold(match[4], "slice") {
		kind = "slice"
	}
	return aspectRatio{kind: kind, alignX: alignRatio(match[2]), alignY: alignRatio(match[3])}
}

// ResolveViewport 解析根元素开始标签的视口映射。
//
// 没有 viewBox 时 1 用户单位按 96 dpi 像素换算；有 viewBox 时按声明的画布尺寸
// （缺失一边按同一 96 dpi 折算）与 preserveAspectRatio 决定等比缩放与对齐。
func ResolveViewport(openTag string) Viewport {
	rawWidth, hasWidth := attrValue(widthAttrRE, openTag)
	rawHeight, hasHeight := attrValue(heightAttrRE, openTag)
	var widthMm, heightMm *float64
	if hasWidth {
		if value, ok := ParseLengthMm(rawWidth); ok {
			widthMm = &value
		}
	}
	if hasHeight {
		if value, ok := ParseLengthMm(rawHeight); ok {
			heightMm = &value
		}
	}

	rawViewBox, hasViewBox := attrValue(viewBoxAttrRE, openTag)
	viewBox := (*ViewBox)(nil)
	if hasViewBox {
		viewBox = parseViewBox(rawViewBox)
	}
	if viewBox == nil {
		viewport := Viewport{
			WidthMm: widthMm, HeightMm: heightMm,
			ScaleX: MMPerUserUnit, ScaleY: MMPerUserUnit,
		}
		if widthMm != nil {
			viewport.ViewportWidthMm = widthMm
		}
		if heightMm != nil {
			viewport.ViewportHeightMm = heightMm
		}
		if hasViewBox {
			viewport.Note = "根元素的 viewBox 无法解析，用户单位按 96 dpi 像素换算"
		}
		return viewport
	}

	viewportWidthMm := viewBox.Width * MMPerUserUnit
	if widthMm != nil {
		viewportWidthMm = *widthMm
	}
	viewportHeightMm := viewBox.Height * MMPerUserUnit
	if heightMm != nil {
		viewportHeightMm = *heightMm
	}
	viewport := Viewport{
		WidthMm: widthMm, HeightMm: heightMm,
		ViewportWidthMm: &viewportWidthMm, ViewportHeightMm: &viewportHeightMm,
		ViewBox: viewBox,
	}

	rawAspect, hasAspect := attrValue(aspectAttrRE, openTag)
	aspect := parseAspectRatio(rawAspect, hasAspect)
	if aspect.kind == "none" {
		viewport.ScaleX = viewportWidthMm / viewBox.Width
		viewport.ScaleY = viewportHeightMm / viewBox.Height
		viewport.OriginX = viewBox.X
		viewport.OriginY = viewBox.Y
		viewport.Note = `preserveAspectRatio="none"：两轴缩放不等，线宽按两轴缩放的几何平均换算`
		return viewport
	}

	scale := math.Min(viewportWidthMm/viewBox.Width, viewportHeightMm/viewBox.Height)
	if aspect.kind == "slice" {
		scale = math.Max(viewportWidthMm/viewBox.Width, viewportHeightMm/viewBox.Height)
	}
	alignX, alignY := aspect.alignX, aspect.alignY
	if aspect.kind == "invalid" {
		alignX, alignY = 0.5, 0.5
	}
	offsetX := (viewportWidthMm - viewBox.Width*scale) * alignX
	offsetY := (viewportHeightMm - viewBox.Height*scale) * alignY
	viewport.ScaleX = scale
	viewport.ScaleY = scale
	viewport.OriginX = viewBox.X - offsetX/scale
	viewport.OriginY = viewBox.Y - offsetY/scale

	var notes []string
	if (widthMm == nil) != (heightMm == nil) {
		notes = append(notes, "根元素只声明了一边的画布尺寸，对齐偏移按 96 dpi 假设的视口计")
	}
	if aspect.kind == "invalid" {
		notes = append(notes, "根元素的 preserveAspectRatio 无法解析，按默认 xMidYMid meet 换算")
	}
	if aspect.kind == "slice" {
		notes = append(notes, `preserveAspectRatio="… slice"：超出视口的内容被裁掉，报告按越出画布处理`)
	}
	viewport.Note = strings.Join(notes, "；")
	return viewport
}

// DiagramSizeMm 返回图面的物理尺寸（毫米），供落版与检查使用。
// 优先使用 viewBox 与缩放比；缺失时退回声明的画布尺寸。
func (v Viewport) DiagramSizeMm() (width, height float64, ok bool) {
	if v.ViewBox != nil && v.ScaleX > 0 && v.ScaleY > 0 {
		return v.ViewBox.Width * v.ScaleX, v.ViewBox.Height * v.ScaleY, true
	}
	if v.WidthMm != nil && v.HeightMm != nil {
		return *v.WidthMm, *v.HeightMm, true
	}
	return 0, 0, false
}

// String 返回便于日志与错误提示的尺寸摘要。
func (v Viewport) String() string {
	width, height, ok := v.DiagramSizeMm()
	if !ok {
		return "视口尺寸未知"
	}
	return fmt.Sprintf("%.1f×%.1f mm", width, height)
}
