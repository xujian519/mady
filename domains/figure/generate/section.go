package generate

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// 剖视图：被剖切零件的闭合轮廓 + 45° 剖面线（细实线），轮廓与剖切位置线用粗实线。
// 依据《专利审查指南》第一部分第一章 4.3（剖面线不得妨碍附图标记线与主线条的识别）
// 与 GB/T 4457.5（剖面线为与水平线成 45° 的细实线，同一零件间距相等、方向一致）。
// 引线标号落在轮廓之外，引线自零件上的指称点引出并止于数字外框。

// HatchSpec 是剖面线参数。
type HatchSpec struct {
	// AngleDeg 为与水平线夹角（默认 45）。
	AngleDeg float64 `json:"angle_deg,omitempty"`
	// SpacingMm 为剖面线间距（毫米，默认 3）。
	SpacingMm float64 `json:"spacing_mm,omitempty"`
	// Direction 为 forward（默认）或 backward。
	Direction string `json:"direction,omitempty"`
}

// HatchOption 是 part.hatch 的取值：省略表示默认剖面线，"none" 表示只画轮廓。
type HatchOption struct {
	Disabled bool
	Spec     HatchSpec
}

// UnmarshalJSON 接受 "none" 字符串或剖面线参数对象。
func (h *HatchOption) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == `"none"` {
		h.Disabled = true
		return nil
	}
	var spec HatchSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return err
	}
	h.Spec = spec
	return nil
}

// SectionSegment 是毫米坐标下的一条线段（视图帧：x 向右、y 向下）。
type SectionSegment struct {
	From [2]float64 `json:"from"`
	To   [2]float64 `json:"to"`
}

// SectionPart 是一个被剖切的零件。
type SectionPart struct {
	// Label 为零件名/标号；给出时落在轮廓包围盒右上方，并画引线。
	Label string `json:"label,omitempty"`
	// Outline 为闭合多边形顶点（毫米，至少 3 个）。
	Outline [][2]float64 `json:"outline"`
	// Holes 为轮廓内的孔（各至少 3 个顶点）：孔环与轮廓同线宽，孔里不打剖面线。
	// 给孔时必须同时给 HatchSegments（本模块的裁剪按单个多边形求交）。
	Holes [][][2]float64 `json:"holes,omitempty"`
	// HatchSegments 为已算好的剖面线段；给出时直接落图，不再裁剪。
	HatchSegments []SectionSegment `json:"hatch_segments,omitempty"`
	// Hatch 为剖面线参数；"none" 表示只画轮廓。
	Hatch HatchOption `json:"hatch,omitempty"`
	// StrokeWidthMm 为该零件轮廓线宽（毫米）；缺省用 SectionOptions.StrokeWidthMm。
	StrokeWidthMm float64 `json:"stroke_width_mm,omitempty"`
}

// SectionLabel 是显式指定的引线标号。
type SectionLabel struct {
	Text string      `json:"text"`
	At   [2]float64  `json:"at"`
	From *[2]float64 `json:"from,omitempty"`
}

// SectionCenterline 是细点划线（轴类零件的轴线）。
type SectionCenterline struct {
	From [2]float64 `json:"from"`
	To   [2]float64 `json:"to"`
}

// SectionOptions 是剖视图尺寸与线宽选项。
type SectionOptions struct {
	// FontSizeMm 为图面字号（毫米，默认 3.5）。
	FontSizeMm float64 `json:"font_size_mm,omitempty"`
	// StrokeWidthMm 为轮廓粗实线线宽（毫米，默认 0.5）。
	StrokeWidthMm float64 `json:"stroke_width_mm,omitempty"`
	// ThinStrokeWidthMm 为剖面线与引线的细实线线宽（毫米，默认 0.25）。
	ThinStrokeWidthMm float64 `json:"thin_stroke_width_mm,omitempty"`
	// PaddingMm 为画布留白（毫米，默认 4）。
	PaddingMm float64 `json:"padding_mm,omitempty"`
}

// SectionDiagramInput 是剖视图构建输入。
type SectionDiagramInput struct {
	Parts       []SectionPart       `json:"parts"`
	Labels      []SectionLabel      `json:"labels,omitempty"`
	Centerlines []SectionCenterline `json:"centerlines,omitempty"`
	Options     SectionOptions      `json:"options,omitempty"`
}

const (
	sectionDefaultFontSize   = 3.5
	sectionDefaultStroke     = 0.5
	sectionDefaultThinStroke = 0.25
	sectionDefaultPadding    = 4.0
	sectionDefaultHatchAngle = 45.0
	sectionDefaultSpacing    = 3.0
	sectionCenterlineDash    = "6 1 1 1"
)

// sectionLabelDraw 是一条待绘制的标号。
type sectionLabelDraw struct {
	content  string
	baseline [2]float64
	anchor   string
	from     *[2]float64
	box      GlyphBox
}

// BuildSectionDiagram 构造剖视图 SVG。
func BuildSectionDiagram(input SectionDiagramInput) (VectorFigure, error) {
	if len(input.Parts) == 0 {
		return VectorFigure{}, newError(ErrEmptyInput, "剖视图至少需要一个零件")
	}
	fontSize := firstPositive(input.Options.FontSizeMm, sectionDefaultFontSize)
	strokeWidth := firstPositive(input.Options.StrokeWidthMm, sectionDefaultStroke)
	thinStroke := firstPositive(input.Options.ThinStrokeWidthMm, sectionDefaultThinStroke)
	padding := firstPositive(input.Options.PaddingMm, sectionDefaultPadding)

	if err := validateSectionParts(input.Parts); err != nil {
		return VectorFigure{}, err
	}
	labels, err := collectSectionLabels(input, fontSize)
	if err != nil {
		return VectorFigure{}, err
	}

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	extend := func(p [2]float64) {
		minX = math.Min(minX, p[0])
		maxX = math.Max(maxX, p[0])
		minY = math.Min(minY, p[1])
		maxY = math.Max(maxY, p[1])
	}
	for _, part := range input.Parts {
		for _, point := range part.Outline {
			extend(point)
		}
	}
	for _, label := range labels {
		extend([2]float64{label.box.MinX, label.box.MinY})
		extend([2]float64{label.box.MaxX, label.box.MaxY})
	}
	for _, centerline := range input.Centerlines {
		extend(centerline.From)
		extend(centerline.To)
	}

	offset := [2]float64{padding - minX, padding - minY}
	body := appendSectionGeometry(input, offset, strokeWidth, thinStroke, labels, fontSize)
	widthMm := (maxX - minX) + 2*padding
	heightMm := (maxY - minY) + 2*padding
	return VectorFigure{
		WidthMm:  widthMm,
		HeightMm: heightMm,
		SVG:      wrapVectorSVG(widthMm, heightMm, strings.Join(body, "\n")),
	}, nil
}

func firstPositive(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

// validateSectionParts 校验轮廓、孔与带孔零件的剖面线前提。
func validateSectionParts(parts []SectionPart) error {
	for index, part := range parts {
		if err := validatePolygon(part.Outline, fmt.Sprintf("第 %d 个零件轮廓", index+1)); err != nil {
			return err
		}
		for holeIndex, hole := range part.Holes {
			if err := validatePolygon(hole, fmt.Sprintf("第 %d 个零件的第 %d 个孔", index+1, holeIndex+1)); err != nil {
				return err
			}
			if len(part.HatchSegments) == 0 && !part.Hatch.Disabled {
				return newError(ErrInvalidLabel,
					"第 %d 个零件带孔时必须用 hatch_segments 给出剖面线段（本模块的裁剪按单个多边形求交，会把孔里也打上剖面线）", index+1)
			}
		}
	}
	return nil
}

// collectSectionLabels 汇总零件自动标号与显式标号。
func collectSectionLabels(input SectionDiagramInput, fontSize float64) ([]sectionLabelDraw, error) {
	var labels []sectionLabelDraw
	for _, part := range input.Parts {
		content := strings.TrimSpace(part.Label)
		if content == "" {
			continue
		}
		box := polygonBounds(part.Outline)
		baseline := [2]float64{box.MaxX + 3, box.MinY - 1}
		from := polygonCentroid(part.Outline)
		labels = append(labels, sectionLabelDraw{
			content: content, baseline: baseline, anchor: AnchorStart, from: &from,
			box: GlyphBoxFor(content, baseline, fontSize, AnchorStart),
		})
	}
	for index, label := range input.Labels {
		content := strings.TrimSpace(label.Text)
		if content == "" {
			return nil, newError(ErrInvalidLabel, "第 %d 个标号的文本不能为空", index+1)
		}
		if err := validatePoint(label.At, "标号落点"); err != nil {
			return nil, err
		}
		if label.From != nil {
			if err := validatePoint(*label.From, "标号引线起点"); err != nil {
				return nil, err
			}
		}
		baseline := [2]float64{label.At[0], label.At[1] + VectorTextMidRatio*fontSize}
		labels = append(labels, sectionLabelDraw{
			content: content, baseline: baseline, anchor: AnchorMiddle, from: label.From,
			box: GlyphBoxFor(content, baseline, fontSize, AnchorMiddle),
		})
	}
	return labels, nil
}

// appendSectionGeometry 依次绘制中心线、零件轮廓与剖面线，最后绘制标号与引线。
func appendSectionGeometry(input SectionDiagramInput, offset [2]float64,
	strokeWidth, thinStroke float64, labels []sectionLabelDraw, fontSize float64) []string {
	translate := func(p [2]float64) [2]float64 {
		return [2]float64{p[0] + offset[0], p[1] + offset[1]}
	}
	var body []string
	for _, centerline := range input.Centerlines {
		body = append(body, vectorLine(translate(centerline.From), translate(centerline.To), thinStroke, sectionCenterlineDash, ""))
	}
	for _, part := range input.Parts {
		partStroke := part.StrokeWidthMm
		if partStroke <= 0 {
			partStroke = strokeWidth
		}
		body = append(body, polygonElement(part.Outline, offset, partStroke))
		for _, hole := range part.Holes {
			body = append(body, polygonElement(hole, offset, partStroke))
		}
		hatch := part.HatchSegments
		if len(hatch) == 0 && !part.Hatch.Disabled {
			hatch = clipHatchSegments(part.Outline, resolveHatch(part.Hatch.Spec))
		}
		for _, segment := range hatch {
			body = append(body, vectorLine(translate(segment.From), translate(segment.To), thinStroke, "", ""))
		}
	}
	for _, label := range labels {
		baseline := translate(label.baseline)
		if label.from != nil {
			from := translate(*label.from)
			if end, ok := LeaderEnd(label.content, baseline, fontSize, label.anchor, from); ok {
				body = append(body, vectorLine(from, end, thinStroke, "", "leader"))
			}
		}
		body = append(body, vectorText(baseline[0], baseline[1], label.anchor, fontSize, label.content, ""))
	}
	return body
}

// resolveHatch 填入剖面线默认值。
func resolveHatch(spec HatchSpec) HatchSpec {
	if spec.AngleDeg == 0 {
		spec.AngleDeg = sectionDefaultHatchAngle
	}
	if spec.SpacingMm <= 0 {
		spec.SpacingMm = sectionDefaultSpacing
	}
	if strings.EqualFold(spec.Direction, "backward") {
		spec.AngleDeg = -spec.AngleDeg
	}
	return spec
}

// clipHatchSegments 用 even-odd 规则把 45° 平行线族裁剪到多边形内部。
func clipHatchSegments(points [][2]float64, spec HatchSpec) []SectionSegment {
	radians := spec.AngleDeg * math.Pi / 180
	direction := [2]float64{math.Cos(radians), math.Sin(radians)}
	normal := [2]float64{-direction[1], direction[0]}
	dot := func(a, b [2]float64) float64 { return a[0]*b[0] + a[1]*b[1] }

	minC, maxC := math.Inf(1), math.Inf(-1)
	for _, point := range points {
		c := dot(point, normal)
		minC = math.Min(minC, c)
		maxC = math.Max(maxC, c)
	}

	var segments []SectionSegment
	for c := minC + spec.SpacingMm/2; c < maxC; c += spec.SpacingMm {
		var projections []float64
		for i := range points {
			a := points[i]
			b := points[(i+1)%len(points)]
			fa := dot(a, normal) - c
			fb := dot(b, normal) - c
			if (fa <= 0 && fb > 0) || (fa > 0 && fb <= 0) {
				t := fa / (fa - fb)
				intersection := [2]float64{a[0] + t*(b[0]-a[0]), a[1] + t*(b[1]-a[1])}
				projections = append(projections, dot(intersection, direction))
			}
		}
		sort.Float64s(projections)
		projections = dedupeProjections(projections)
		if len(projections) < 2 {
			continue
		}
		for i := 0; i+1 < len(projections); i += 2 {
			from := [2]float64{
				direction[0]*projections[i] + normal[0]*c,
				direction[1]*projections[i] + normal[1]*c,
			}
			to := [2]float64{
				direction[0]*projections[i+1] + normal[0]*c,
				direction[1]*projections[i+1] + normal[1]*c,
			}
			segments = append(segments, SectionSegment{From: from, To: to})
		}
	}
	return segments
}

// dedupeProjections 合并同一交点沿扫描线方向的重复投影：恰落在扫描线上的
// 顶点会被相邻两条边各计一次，不去重会配出零长度段并错位后续配对。
func dedupeProjections(values []float64) []float64 {
	if len(values) == 0 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if math.Abs(value-out[len(out)-1]) < 1e-9 {
			continue
		}
		out = append(out, value)
	}
	return out
}

func polygonElement(points [][2]float64, offset [2]float64, strokeMm float64) string {
	parts := make([]string, len(points))
	for i, point := range points {
		parts[i] = fmtMm(point[0]+offset[0]) + "," + fmtMm(point[1]+offset[1])
	}
	return fmt.Sprintf(`<polygon points="%s" fill="none" stroke="#000000" stroke-width="%s"/>`,
		strings.Join(parts, " "), fmtMm(strokeMm))
}

func polygonBounds(points [][2]float64) GlyphBox {
	box := GlyphBox{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
	for _, point := range points {
		box.MinX = math.Min(box.MinX, point[0])
		box.MaxX = math.Max(box.MaxX, point[0])
		box.MinY = math.Min(box.MinY, point[1])
		box.MaxY = math.Max(box.MaxY, point[1])
	}
	return box
}

func polygonCentroid(points [][2]float64) [2]float64 {
	var sumX, sumY float64
	for _, point := range points {
		sumX += point[0]
		sumY += point[1]
	}
	return [2]float64{sumX / float64(len(points)), sumY / float64(len(points))}
}

func validatePolygon(points [][2]float64, subject string) error {
	if len(points) < 3 {
		return newError(ErrInvalidLabel, "%s 至少需要 3 个顶点", subject)
	}
	for _, point := range points {
		if err := validatePoint(point, subject); err != nil {
			return err
		}
	}
	return nil
}

func validatePoint(point [2]float64, subject string) error {
	if math.IsNaN(point[0]) || math.IsInf(point[0], 0) || math.IsNaN(point[1]) || math.IsInf(point[1], 0) {
		return newError(ErrInvalidLabel, "%s 的坐标必须是有限数：%v", subject, point)
	}
	return nil
}
