package generate

import (
	"fmt"
	"math"
	"strings"
)

// 附图落版：把一张图形按目标法域规格放到 A4 幅面上，图号画在图形正下方，
// 页码画在版心底部。Graphviz 的 page/size 属性不产生幅面，故落版必须单独一步，
// 否则无法按法域核实字高与页边距。

// SubmissionInput 是一次落版请求。
type SubmissionInput struct {
	// DrawingSVG 是图形 SVG（Graphviz 或矢量图型输出）。
	DrawingSVG string
	Office     Office
	// FigureNumber / FigureCount 用于生成法域图号；FigureCount 为本案附图总数。
	FigureNumber int
	FigureCount  int
	// Caption 覆盖自动图号（如已按法域写好的「图1」）；为空时按法域生成。
	Caption string
	// SheetIndex / SheetTotal 用于生成页码；均为 0 时不标页码。
	SheetIndex int
	SheetTotal int
	// BodyFontSize 是图形正文的数字与字母字号（用户单位），用于核算字高。
	BodyFontSize float64
	// CaptionGapMm / CaptionFontMm / SheetFontMm 缺省分别为 3 / 4 / 3 毫米。
	CaptionGapMm  float64
	CaptionFontMm float64
	SheetFontMm   float64
	// RotateDeg 为 0/90/180/270，绕绘图区中心顺时针旋转。
	RotateDeg int
}

// SubmissionMetrics 是落版后的物理尺寸。
type SubmissionMetrics struct {
	PageScale           float64 `json:"page_scale"`
	DrawingWidthMm      float64 `json:"drawing_width_mm"`
	DrawingHeightMm     float64 `json:"drawing_height_mm"`
	PlacedWidthMm       float64 `json:"placed_width_mm"`
	PlacedHeightMm      float64 `json:"placed_height_mm"`
	CharHeightMm        float64 `json:"char_height_mm,omitempty"`
	ReducedCharHeightMm float64 `json:"reduced_char_height_mm,omitempty"`
}

// SubmissionResult 是落版结果。
type SubmissionResult struct {
	SVG         string            `json:"svg"`
	Caption     string            `json:"caption,omitempty"`
	SheetNumber string            `json:"sheet_number,omitempty"`
	Metrics     SubmissionMetrics `json:"metrics"`
	Warnings    []string          `json:"warnings,omitempty"`
}

const (
	defaultCaptionFontMm = 4.0
	defaultCaptionGapMm  = 3.0
	defaultSheetFontMm   = 3.0
	maxPageScale         = 4.0
	charHeightRatio      = 0.7
)

type drawingGeometry struct {
	widthMm, heightMm            float64
	viewBoxX, viewBoxY           float64
	userUnitToMmX, userUnitToMmY float64
	inner                        string
}

// parseDrawingGeometry 解析图形 SVG 的物理尺寸、viewBox 与内部片段。
func parseDrawingGeometry(svg string) (drawingGeometry, []string, error) {
	if err := AssertSafeSVG(svg, DefaultSVGMaxBytes); err != nil {
		return drawingGeometry{}, nil, err
	}
	rootTag := RootTag(svg)
	closeIndex := strings.LastIndex(strings.ToLower(svg), "</svg>")
	if rootTag == "" || closeIndex < 0 {
		return drawingGeometry{}, nil, newError(ErrInvalidSVG, "非 SVG 文档：缺少 <svg> 根元素")
	}
	openIndex := strings.Index(svg, rootTag)
	viewport := ResolveViewport(rootTag)
	if viewport.ViewportWidthMm == nil || viewport.ViewportHeightMm == nil {
		return drawingGeometry{}, nil, newError(ErrInvalidSVG, "SVG 缺少可解析的 width/height 或 viewBox")
	}
	var warnings []string
	if viewport.WidthMm == nil || viewport.HeightMm == nil {
		warnings = append(warnings, "图形未同时声明 width/height，缺失的一边按 viewBox 的 96 dpi 用户单位换算")
	}
	if viewport.Note != "" {
		warnings = append(warnings, viewport.Note)
	}
	innerStart := openIndex + len(rootTag) + 1
	if innerStart > closeIndex {
		return drawingGeometry{}, nil, newError(ErrInvalidSVG, "非 SVG 文档：根元素与结束标签顺序异常")
	}
	return drawingGeometry{
		widthMm:       *viewport.ViewportWidthMm,
		heightMm:      *viewport.ViewportHeightMm,
		viewBoxX:      viewport.OriginX,
		viewBoxY:      viewport.OriginY,
		userUnitToMmX: viewport.ScaleX,
		userUnitToMmY: viewport.ScaleY,
		inner:         svg[innerStart:closeIndex],
	}, warnings, nil
}

// BuildSubmissionPage 把图形落版到目标法域的 A4 附图页。
func BuildSubmissionPage(input SubmissionInput) (SubmissionResult, error) {
	profile, err := OfficeProfileFor(input.Office)
	if err != nil {
		return SubmissionResult{}, err
	}
	rotate := input.RotateDeg
	switch rotate {
	case 0, 90, 180, 270:
	default:
		return SubmissionResult{}, newError(ErrInvalidPage, "旋转角只支持 0/90/180/270：%d", rotate)
	}
	captionFont := positiveOr(input.CaptionFontMm, defaultCaptionFontMm)
	captionGap := positiveOr(input.CaptionGapMm, defaultCaptionGapMm)
	sheetFont := positiveOr(input.SheetFontMm, defaultSheetFontMm)

	caption := strings.TrimSpace(input.Caption)
	if caption == "" && input.FigureNumber > 0 {
		figureCount := input.FigureCount
		if figureCount == 0 {
			// 未给总数时按单幅处理：与「仅一幅不编号」的通行规则一致。
			figureCount = input.FigureNumber
		}
		if caption, err = FigureCaption(input.Office, input.FigureNumber, figureCount); err != nil {
			return SubmissionResult{}, err
		}
	}
	sheetNumber := ""
	if input.SheetIndex > 0 || input.SheetTotal > 0 {
		if sheetNumber, err = SheetNumberText(input.Office, input.SheetIndex, input.SheetTotal); err != nil {
			return SubmissionResult{}, err
		}
	}

	geometry, warnings, err := parseDrawingGeometry(input.DrawingSVG)
	if err != nil {
		return SubmissionResult{}, err
	}

	areaWidth := profile.Paper.WidthMm - profile.Margins.LeftMm - profile.Margins.RightMm
	areaHeight := profile.Paper.HeightMm - profile.Margins.TopMm - profile.Margins.BottomMm
	if areaWidth <= 0 || areaHeight <= 0 {
		return SubmissionResult{}, newError(ErrInvalidPage, "页边距之和超过纸张尺寸")
	}
	captionBlock := 0.0
	if caption != "" {
		captionBlock = captionGap + captionFont
	}
	figureAreaHeight := areaHeight - captionBlock

	quarterTurn := rotate == 90 || rotate == 270
	boundingWidth, boundingHeight := geometry.widthMm, geometry.heightMm
	if quarterTurn {
		boundingWidth, boundingHeight = geometry.heightMm, geometry.widthMm
	}
	rawScale := math.Min(areaWidth/boundingWidth, figureAreaHeight/boundingHeight)
	pageScale := math.Min(rawScale, maxPageScale)
	if rawScale > maxPageScale {
		warnings = append(warnings,
			fmt.Sprintf("图形相对幅面过小，落版放大被限制在 %v 倍；请核对图面细节是否仍可辨", maxPageScale))
	}
	scaledWidth := geometry.widthMm * pageScale
	scaledHeight := geometry.heightMm * pageScale
	placedWidth, placedHeight := scaledWidth, scaledHeight
	if quarterTurn {
		placedWidth, placedHeight = scaledHeight, scaledWidth
	}

	placeX := profile.Margins.LeftMm + (areaWidth-scaledWidth)/2
	placeY := profile.Margins.TopMm + (figureAreaHeight-scaledHeight)/2
	centerX := profile.Margins.LeftMm + areaWidth/2
	centerY := profile.Margins.TopMm + figureAreaHeight/2

	paper := profile.Paper
	var lines []string
	lines = append(lines,
		fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%smm" height="%smm" viewBox="0 0 %s %s">`,
			fmtMm(paper.WidthMm), fmtMm(paper.HeightMm), fmtMm(paper.WidthMm), fmtMm(paper.HeightMm)),
		"  <!-- 附图落版页：幅面与页边距按目标法域规格 -->",
	)
	if rotate != 0 {
		lines = append(lines, fmt.Sprintf(
			`  <g transform="translate(%s,%s) rotate(%d) translate(%s,%s)">`,
			fmtMm(centerX), fmtMm(centerY), rotate, fmtMm(-centerX), fmtMm(-centerY)))
	}
	pad := "  "
	if rotate != 0 {
		pad = "    "
	}
	lines = append(lines,
		fmt.Sprintf(`%s<g transform="translate(%s,%s) scale(%s)">`,
			pad, fmtMm(placeX), fmtMm(placeY), fmtMm(pageScale)),
		fmt.Sprintf(`%s  <g transform="translate(%s,%s) scale(%s,%s)">`,
			pad, fmtMm(-geometry.viewBoxX), fmtMm(-geometry.viewBoxY),
			fmtMm(geometry.userUnitToMmX), fmtMm(geometry.userUnitToMmY)),
		indentBlock(geometry.inner, len(pad)+4),
		pad+"  </g>",
		pad+"</g>",
	)
	if rotate != 0 {
		lines = append(lines, "  </g>")
	}
	var captionY float64
	if caption != "" {
		captionY = profile.Margins.TopMm + (figureAreaHeight-placedHeight)/2 + placedHeight + captionGap + captionFont*0.8
		lines = append(lines, submissionText(centerX, captionY, captionFont, caption))
	}
	if sheetNumber != "" {
		sheetY := profile.Margins.TopMm + areaHeight - sheetFont*0.3
		lines = append(lines, submissionText(centerX, sheetY, sheetFont, sheetNumber))
	}
	lines = append(lines, "</svg>", "")

	metrics := SubmissionMetrics{
		PageScale: pageScale, DrawingWidthMm: geometry.widthMm, DrawingHeightMm: geometry.heightMm,
		PlacedWidthMm: placedWidth, PlacedHeightMm: placedHeight,
	}
	if input.BodyFontSize > 0 {
		metrics.CharHeightMm = input.BodyFontSize * geometry.userUnitToMmY * charHeightRatio * pageScale
		metrics.ReducedCharHeightMm = metrics.CharHeightMm * profile.ReductionRatio
	}
	return SubmissionResult{
		SVG: strings.Join(lines, "\n"), Caption: caption, SheetNumber: sheetNumber,
		Metrics: metrics, Warnings: warnings,
	}, nil
}

func positiveOr(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

// indentBlock 逐行缩进片段（空行保持空）。
func indentBlock(body string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = prefix + strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

// submissionText 生成页面上的一行居中文本（图号或页码）。
func submissionText(centerX, baselineY, fontMm float64, text string) string {
	return fmt.Sprintf(
		`  <text x="%s" y="%s" text-anchor="middle" font-family="sans-serif" font-size="%s" fill="#000000" stroke="none">%s</text>`,
		fmtMm(centerX), fmtMm(baselineY), fmtMm(fontMm), escapeXMLText(text))
}
