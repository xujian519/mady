package generate

import (
	"fmt"
	"math"
	"strings"
)

func sameGroup(text scannedText, segment scannedSegment) bool {
	return text.hasGroup && segment.hasGroup && text.groupID == segment.groupID
}

func stripTags(text string) string {
	var builder strings.Builder
	inTag := false
	for _, r := range text {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				builder.WriteRune(r)
			}
		}
	}
	return builder.String()
}

// crossedTextFindings 判定文字是否被线条穿过，并返回被穿过的文字下标。
func crossedTextFindings(texts []scannedText, segments []scannedSegment) ([]bool, []CheckFinding) {
	crossed := make([]bool, len(texts))
	var findings []CheckFinding
	for i := range texts {
		for _, segment := range segments {
			if segment.leader || sameGroup(texts[i], segment) {
				continue
			}
			if !BoxCrossedBySegment(texts[i].box, segment.from, segment.to) {
				continue
			}
			crossed[i] = true
			findings = append(findings, CheckFinding{
				Kind: CheckTextCrossed, Blocking: true,
				Message: fmt.Sprintf("图面文字「%s」被线条穿过：引线或轮廓线压进字身，标号读不出；把线移到文字占位框之外", texts[i].content),
			})
			break
		}
	}
	return crossed, findings
}

// clearanceFindings 判定文字与图线的净距；已被判穿字的文字不重复报。
func clearanceFindings(texts []scannedText, segments []scannedSegment, crossed []bool, clearance float64) []CheckFinding {
	if clearance <= 0 {
		return nil
	}
	var findings []CheckFinding
	for i := range texts {
		if crossed[i] {
			continue
		}
		box := BoxQuad(texts[i].box)
		inflated := InflateQuad(box, clearance)
		nearest := math.Inf(1)
		for _, segment := range segments {
			if segment.leader || sameGroup(texts[i], segment) {
				continue
			}
			if !QuadCrossedBySegment(inflated, segment.from, segment.to) {
				continue
			}
			nearest = math.Min(nearest, QuadSegmentDistance(box, segment.from, segment.to))
		}
		if math.IsInf(nearest, 1) {
			continue
		}
		findings = append(findings, CheckFinding{
			Kind: CheckTextClearance, Blocking: true,
			Message: fmt.Sprintf("图面文字「%s」距最近图线仅 %.1f 毫米，不足 %.1f 毫米净距：标号与元件名周围要留出可读的空白",
				texts[i].content, nearest, clearance),
		})
	}
	return findings
}

func fontFindings(texts []scannedText, minFontMm float64) []CheckFinding {
	if minFontMm <= 0 {
		return nil
	}
	var findings []CheckFinding
	for _, text := range texts {
		height := text.fontSize * (GlyphAscentRatio + GlyphDescentRatio)
		if height < minFontMm {
			findings = append(findings, CheckFinding{
				Kind: CheckFontBelowMin, Blocking: true,
				Message: fmt.Sprintf("图面文字「%s」字高 %.1f 毫米，低于内控下限 %.1f 毫米", text.content, height, minFontMm),
			})
		}
	}
	return findings
}

func canvasFindings(segments []scannedSegment, widthMm, heightMm float64) []CheckFinding {
	if widthMm <= 0 || heightMm <= 0 {
		return nil
	}
	const tolerance = 0.5
	outside := func(p [2]float64) bool {
		return p[0] < -tolerance || p[1] < -tolerance ||
			p[0] > widthMm+tolerance || p[1] > heightMm+tolerance
	}
	for _, segment := range segments {
		if outside(segment.from) || outside(segment.to) {
			return []CheckFinding{{
				Kind: CheckInkOutside, Blocking: true,
				Message: "图面元素越出画布：越界部分不会被渲染出来，检查画布尺寸或元素坐标",
			}}
		}
	}
	return nil
}
