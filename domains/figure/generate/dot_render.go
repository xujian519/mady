package generate

import (
	"fmt"
	"math"
	"strings"
)

// 页面物理尺寸（英寸，纵向）。
var pageSizeInches = map[string][2]float64{
	"a4":     {8.27, 11.69},
	"letter": {8.5, 11},
}

// validatePage 校验页面参数并返回可输出的属性行。
func pageAttributeLines(page *PageOptions) ([]string, error) {
	if page == nil {
		return nil, nil
	}
	if page.DPI != 0 && page.DPI < 1 {
		return nil, newError(ErrInvalidPage, "dpi 必须为正整数：%d", page.DPI)
	}
	if page.MarginCm < 0 {
		return nil, newError(ErrInvalidPage, "页边距必须为正（厘米）：%v", page.MarginCm)
	}
	if page.Size == "" && page.Orientation == "" && page.DPI == 0 && page.MarginCm == 0 {
		return nil, nil
	}
	var lines []string
	if page.Size != "" {
		size, ok := pageSizeInches[page.Size]
		if !ok {
			return nil, newError(ErrInvalidPage, "未知页面尺寸 %q（可选 a4/letter）", page.Size)
		}
		width, height := size[0], size[1]
		if page.Orientation == "landscape" {
			width, height = height, width
		}
		lines = append(lines, fmt.Sprintf(`    page="%s,%s";`, formatInches(width), formatInches(height)))
		if page.MarginCm > 0 {
			marginIn := page.MarginCm / cmPerInch
			lines = append(lines,
				fmt.Sprintf(`    size="%s,%s";`, formatInches(width-2*marginIn), formatInches(height-2*marginIn)),
				fmt.Sprintf(`    margin="%s,%s";`, formatInches(marginIn), formatInches(marginIn)))
		}
	} else if page.MarginCm > 0 {
		marginIn := formatInches(page.MarginCm / cmPerInch)
		lines = append(lines, fmt.Sprintf(`    margin="%s,%s";`, marginIn, marginIn))
	}
	if page.DPI > 0 {
		lines = append(lines, fmt.Sprintf("    dpi=%d;", page.DPI))
	}
	if page.Orientation == "landscape" {
		lines = append(lines, "    orientation=landscape;")
	}
	return lines, nil
}

func formatInches(value float64) string {
	return fmt.Sprintf("%g", math.Round(value*100)/100)
}

// buildHeader 生成 DOT 头：图形名、方向、页面属性、node/edge 默认样式。
func buildHeader(graphName, rankdir, fontName string, filled bool, page *PageOptions, fontPt int) ([]string, error) {
	nodeFontPt := 10
	edgeFontPt := 9
	if fontPt > 0 {
		nodeFontPt, edgeFontPt = fontPt, fontPt
	}
	nodeAttrs := []string{
		fmt.Sprintf(`fontname="%s"`, EscapeLabel(fontName)),
		fmt.Sprintf("fontsize=%d", nodeFontPt),
	}
	if filled {
		nodeAttrs = append(nodeAttrs, "style=filled")
	}
	pageLines, err := pageAttributeLines(page)
	if err != nil {
		return nil, err
	}
	lines := []string{
		fmt.Sprintf("digraph %s {", graphName),
		fmt.Sprintf("    rankdir=%s;", rankdir),
	}
	lines = append(lines, pageLines...)
	lines = append(lines,
		fmt.Sprintf("    node [%s];", strings.Join(nodeAttrs, ", ")),
		fmt.Sprintf(`    edge [fontname="%s", fontsize=%d];`, EscapeLabel(fontName), edgeFontPt),
		"",
	)
	return lines, nil
}
