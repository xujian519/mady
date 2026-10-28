package generate

import (
	"math"
	"strings"
	"testing"
)

func squarePart(size float64) SectionPart {
	return SectionPart{
		Label: "1",
		Outline: [][2]float64{
			{0, 0}, {size, 0}, {size, size}, {0, size},
		},
	}
}

func TestBuildSectionDiagram(t *testing.T) {
	figure, err := BuildSectionDiagram(SectionDiagramInput{
		Parts: []SectionPart{squarePart(40)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if figure.WidthMm <= 40 || figure.HeightMm <= 40 {
		t.Errorf("画布应包含轮廓与留白: %v×%v", figure.WidthMm, figure.HeightMm)
	}
	if !strings.Contains(figure.SVG, "<polygon") {
		t.Error("缺少轮廓")
	}
	if !strings.Contains(figure.SVG, ">1<") {
		t.Error("缺少标号")
	}
	if !strings.Contains(figure.SVG, `data-figure-role="leader"`) {
		t.Error("缺少引线（应标注 leader 角色）")
	}
	// 默认 45° 剖面线：应有若干细实线段。
	hatchCount := strings.Count(figure.SVG, `stroke-width="0.25"`)
	if hatchCount < 5 {
		t.Errorf("剖面线数量过少: %d", hatchCount)
	}
}

// 剖面线应完全落在轮廓内。
func TestClipHatchSegmentsInsidePolygon(t *testing.T) {
	points := [][2]float64{{0, 0}, {40, 0}, {40, 40}, {0, 40}}
	segments := clipHatchSegments(points, resolveHatch(HatchSpec{}))
	if len(segments) == 0 {
		t.Fatal("未生成剖面线")
	}
	for _, segment := range segments {
		for _, point := range [][2]float64{segment.From, segment.To} {
			if point[0] < -1e-6 || point[0] > 40+1e-6 || point[1] < -1e-6 || point[1] > 40+1e-6 {
				t.Errorf("剖面线越出轮廓: %v", point)
			}
		}
	}
}

func TestSectionHatchDisabled(t *testing.T) {
	part := squarePart(40)
	part.Label = ""
	part.Hatch = HatchOption{Disabled: true}
	figure, err := BuildSectionDiagram(SectionDiagramInput{Parts: []SectionPart{part}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(figure.SVG, `<line`) != 0 {
		t.Errorf("关闭剖面线后不应有线条:\n%s", figure.SVG)
	}
}

func TestSectionHolesRequireSegments(t *testing.T) {
	part := squarePart(40)
	part.Label = ""
	part.Holes = [][][2]float64{{{10, 10}, {20, 10}, {20, 20}, {10, 20}}}
	if _, err := BuildSectionDiagram(SectionDiagramInput{Parts: []SectionPart{part}}); err == nil ||
		!strings.Contains(err.Error(), "hatch_segments") {
		t.Fatalf("带孔未给剖面线段应报错, got %v", err)
	}

	part.HatchSegments = []SectionSegment{{From: [2]float64{0, 5}, To: [2]float64{40, 5}}}
	if _, err := BuildSectionDiagram(SectionDiagramInput{Parts: []SectionPart{part}}); err != nil {
		t.Fatalf("给出剖面线段后应可用: %v", err)
	}
}

func TestSectionRejectsBadPolygon(t *testing.T) {
	part := SectionPart{Outline: [][2]float64{{0, 0}, {10, 0}}}
	if _, err := BuildSectionDiagram(SectionDiagramInput{Parts: []SectionPart{part}}); err == nil {
		t.Error("少于 3 个顶点应报错")
	}
	bad := SectionPart{Outline: [][2]float64{{0, 0}, {math.Inf(1), 0}, {10, 10}}}
	if _, err := BuildSectionDiagram(SectionDiagramInput{Parts: []SectionPart{bad}}); err == nil {
		t.Error("非有限坐标应报错")
	}
}

// 剖视图应通过渲染复核（净距档位 1.5 毫米）。
func TestSectionPassesRenderCheck(t *testing.T) {
	figure, err := BuildSectionDiagram(SectionDiagramInput{
		Parts: []SectionPart{squarePart(40)},
		Centerlines: []SectionCenterline{
			{From: [2]float64{-5, 20}, To: [2]float64{45, 20}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := CheckFigure(figure.SVG, CheckOptions{FigureType: "section"})
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range BlockingFindings(report) {
		if finding.Kind != CheckTextClearance && finding.Kind != CheckTextCrossed {
			t.Errorf("意外发现: %+v", finding)
		}
	}
	// 中心线穿过轮廓内部（有意为之），但不应穿过标号。
	for _, finding := range report.Findings {
		if finding.Kind == CheckTextCrossed {
			t.Errorf("标号不应被线条穿过: %s", finding.Message)
		}
	}
}
