package generate

import (
	"math"
)

// 渲染复核：在已生成的 SVG 源上量测「只有看渲染结果才看得见」的问题。
//
// 本实现覆盖四类：文字被线条贯穿、文字与图线净距不足、字高低于内控下限、墨迹越出
// 画布；无法量测的结构统一记 not-measured，而不是略过——报告里没有 not-measured 时，
// 「未发现问题」才等于逐类量测过。点划线覆盖、剖面线取向碰撞、图元叠压等类别本轮
// 未实现（属 F4 的一致性/交付核对范围）。
//
// 量测对象是矢量源：元素按嵌套继承 transform、字号、text-anchor 与描边；曲线按
// 8 段折线采样；同一语义分组（<g class="node|edge">）内的文字与线段不互判——节点
// 标签本就排在节点框内、边标签本就贴着自己的边，那是排版而非缺陷。引线
// （data-figure-role="leader"）不参与净距判定：引线本来就止于文字外框。

// CheckKind 是复核发现的问题类别。
type CheckKind string

// Check kind constants.
const (
	CheckTextCrossed   CheckKind = "text-crossed-by-line"
	CheckTextClearance CheckKind = "text-clearance"
	CheckFontBelowMin  CheckKind = "font-below-minimum"
	CheckInkOutside    CheckKind = "ink-outside-canvas"
	CheckNotMeasured   CheckKind = "not-measured"
)

// CheckFinding 是一条复核发现。
type CheckFinding struct {
	Kind     CheckKind `json:"check"`
	Message  string    `json:"message"`
	Blocking bool      `json:"blocking"`
}

// CheckReport 是一张附图的复核报告。
type CheckReport struct {
	WidthMm   float64        `json:"width_mm,omitempty"`
	HeightMm  float64        `json:"height_mm,omitempty"`
	TextCount int            `json:"text_count"`
	MinFontMm float64        `json:"min_font_mm,omitempty"`
	Findings  []CheckFinding `json:"findings"`
}

// CheckOptions 是复核参数。
type CheckOptions struct {
	// TextClearanceMm 为文字与图线的净距下限（毫米）；0 关闭该判据。
	TextClearanceMm float64
	// MinFontMm 为字高内控下限（毫米）；0 关闭。
	MinFontMm float64
	// MaxBytes 为输入上限（字节）；0 用 DefaultSVGMaxBytes。
	MaxBytes int
	// FigureType 用于缺省净距分档（见 ClearanceMm）。
	FigureType string
}

// 按图型分档的净距下限（毫米）：取实测下界再减 0.1，只抓真正的贴靠与重叠。
var clearanceTiers = map[string]float64{
	"cross_section":       1.5,
	"section":             1.5,
	"structure":           1.5,
	"flowchart":           1.5,
	"state_diagram":       1.5,
	"state":               1.5,
	"block_diagram":       0.5,
	"block":               0.5,
	"component_hierarchy": 0.5,
	"hierarchy":           0.5,
	"template":            0.5,
	"sequence_diagram":    0.7,
	"sequence":            0.7,
}

// ClearanceMm 返回图型的净距档位；未登记图型返回 0（不判净距，仍判穿字）。
func ClearanceMm(figureType string) float64 {
	return clearanceTiers[figureType]
}

// BlockingFindings 返回阻断级发现（not-measured 不算）。
func BlockingFindings(report CheckReport) []CheckFinding {
	var out []CheckFinding
	for _, finding := range report.Findings {
		if finding.Blocking {
			out = append(out, finding)
		}
	}
	return out
}

// ---------- 扫描 ----------

// CheckFigure 量测一张 SVG 并报告渲染层面才可见的问题。
func CheckFigure(svg string, opts CheckOptions) (CheckReport, error) {
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultSVGMaxBytes
	}
	if err := AssertSafeSVG(svg, maxBytes); err != nil {
		return CheckReport{}, err
	}
	viewport := ResolveViewport(RootTag(svg))
	scaleX, scaleY := viewport.ScaleX, viewport.ScaleY
	if scaleX <= 0 {
		scaleX = MMPerUserUnit
	}
	if scaleY <= 0 {
		scaleY = MMPerUserUnit
	}
	toMM := func(p [2]float64) [2]float64 {
		return [2]float64{(p[0] - viewport.OriginX) * scaleX, (p[1] - viewport.OriginY) * scaleY}
	}

	scan := scanSVG(svg)
	report := CheckReport{TextCount: len(scan.texts)}

	if width := viewport.ViewportWidthMm; width != nil {
		report.WidthMm = *width
	}
	if height := viewport.ViewportHeightMm; height != nil {
		report.HeightMm = *height
	}

	texts := make([]scannedText, len(scan.texts))
	minFont := math.Inf(1)
	for i, text := range scan.texts {
		texts[i] = text
		texts[i].box = mmBox(text.box.MinX, text.box.MinY, text.box.MaxX, text.box.MaxY, toMM)
		texts[i].fontSize = math.Abs(text.fontSize) * math.Sqrt(scaleX*scaleY)
		height := texts[i].fontSize * (GlyphAscentRatio + GlyphDescentRatio)
		if height < minFont {
			minFont = height
		}
	}
	if !math.IsInf(minFont, 1) {
		report.MinFontMm = minFont
	}
	segments := make([]scannedSegment, len(scan.segments))
	for i, segment := range scan.segments {
		segments[i] = scannedSegment{
			from: toMM(segment.from), to: toMM(segment.to),
			groupID: segment.groupID, hasGroup: segment.hasGroup, leader: segment.leader,
		}
	}

	clearance := opts.TextClearanceMm
	if clearance == 0 && opts.FigureType != "" {
		clearance = ClearanceMm(opts.FigureType)
	}

	crossed, crossedFindings := crossedTextFindings(texts, segments)
	report.Findings = append(report.Findings, crossedFindings...)
	report.Findings = append(report.Findings, clearanceFindings(texts, segments, crossed, clearance)...)
	report.Findings = append(report.Findings, fontFindings(texts, opts.MinFontMm)...)
	report.Findings = append(report.Findings, canvasFindings(segments, report.WidthMm, report.HeightMm)...)

	for _, reason := range scan.notMeasured {
		report.Findings = append(report.Findings, CheckFinding{
			Kind: CheckNotMeasured, Blocking: false,
			Message: "未量测：" + reason,
		})
	}
	return report, nil
}

func mmBox(minX, minY, maxX, maxY float64, toMM func([2]float64) [2]float64) GlyphBox {
	cornerA := toMM([2]float64{minX, minY})
	cornerB := toMM([2]float64{maxX, maxY})
	return GlyphBox{
		MinX: math.Min(cornerA[0], cornerB[0]), MaxX: math.Max(cornerA[0], cornerB[0]),
		MinY: math.Min(cornerA[1], cornerB[1]), MaxY: math.Max(cornerA[1], cornerB[1]),
	}
}
