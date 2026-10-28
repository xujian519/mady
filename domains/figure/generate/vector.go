package generate

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// VectorFigure 是直接绘制 SVG 的图型产物（毫米画布 + 完整 SVG 文档）。
//
// 与 Graphviz 通路并列：Graphviz 画不出电气符号、剖面线与时序生命线，这些图型
// 各自产出毫米坐标的黑色描边片段，再封装为独立 SVG。
type VectorFigure struct {
	WidthMm  float64  `json:"width_mm"`
	HeightMm float64  `json:"height_mm"`
	SVG      string   `json:"svg"`
	Labels   []string `json:"labels,omitempty"`
}

// 矢量图型默认线宽（毫米）：GB/T 4457.4 线宽系列的 0.35。
const (
	VectorStrokeMm     = 0.35
	VectorThinStrokeMm = 0.18
	VectorFontSizeMm   = 3.5
	VectorTextMidRatio = 0.35
)

// fmtMm 格式化毫米坐标：保留至多 3 位小数并去掉尾随零。
func fmtMm(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "0"
	}
	rounded := math.Round(value*1000) / 1000
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

// wrapVectorSVG 把片段封装为独立 SVG 文档（毫米画布）。
func wrapVectorSVG(widthMm, heightMm float64, body string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%smm" height="%smm" viewBox="0 0 %s %s">`,
		fmtMm(widthMm), fmtMm(heightMm), fmtMm(widthMm), fmtMm(heightMm))
	builder.WriteString("\n")
	builder.WriteString(body)
	builder.WriteString("\n</svg>\n")
	return builder.String()
}

// vectorText 生成不自继承描边的文本元素（fill 黑、stroke none）。
func vectorText(x, y float64, anchor string, fontSizeMm float64, content string, extraAttrs string) string {
	return fmt.Sprintf(
		`<text x="%s" y="%s" text-anchor="%s" fill="#000000" stroke="none" font-size="%s"%s>%s</text>`,
		fmtMm(x), fmtMm(y), anchor, fmtMm(fontSizeMm), extraAttrs, escapeXMLText(content))
}

// vectorLine 生成一条线段。
func vectorLine(from, to [2]float64, strokeMm float64, dash string, role string) string {
	dashAttr := ""
	if dash != "" {
		dashAttr = fmt.Sprintf(` stroke-dasharray="%s"`, dash)
	}
	roleAttr := ""
	if role != "" {
		roleAttr = fmt.Sprintf(` data-figure-role="%s"`, role)
	}
	return fmt.Sprintf(
		`<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="#000000" stroke-width="%s"%s%s/>`,
		fmtMm(from[0]), fmtMm(from[1]), fmtMm(to[0]), fmtMm(to[1]), fmtMm(strokeMm), dashAttr, roleAttr)
}
