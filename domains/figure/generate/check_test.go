package generate

import (
	"strings"
	"testing"
)

func wrapTestSVG(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" width="100mm" height="100mm" viewBox="0 0 100 100">` + body + `</svg>`
}

func findingKinds(report CheckReport) []CheckKind {
	kinds := make([]CheckKind, len(report.Findings))
	for i, finding := range report.Findings {
		kinds[i] = finding.Kind
	}
	return kinds
}

func hasKind(report CheckReport, kind CheckKind) bool {
	for _, finding := range report.Findings {
		if finding.Kind == kind {
			return true
		}
	}
	return false
}

// 门禁用例 1：线条穿过文字占位框。
func TestCheckTextCrossed(t *testing.T) {
	svg := wrapTestSVG(`
		<text x="50" y="50" text-anchor="middle" font-size="5" fill="#000000" stroke="none">标号</text>
		<line x1="20" y1="50" x2="80" y2="50" stroke="#000000" stroke-width="0.3"/>`)
	report, err := CheckFigure(svg, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(report, CheckTextCrossed) {
		t.Fatalf("应报 text-crossed-by-line, got %v", findingKinds(report))
	}
	if len(BlockingFindings(report)) == 0 {
		t.Error("穿字应为阻断级")
	}
	if !strings.Contains(report.Findings[0].Message, "标号") {
		t.Errorf("提示应包含文字内容: %s", report.Findings[0].Message)
	}
}

// 门禁用例 2：文字与图线净距不足（未穿字）。
func TestCheckTextClearance(t *testing.T) {
	svg := wrapTestSVG(`
		<text x="50" y="50" text-anchor="middle" font-size="5" fill="#000000" stroke="none">标号</text>
		<line x1="20" y1="52" x2="80" y2="52" stroke="#000000" stroke-width="0.3"/>`)
	report, err := CheckFigure(svg, CheckOptions{TextClearanceMm: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if hasKind(report, CheckTextCrossed) {
		t.Fatalf("不应判穿字: %v", findingKinds(report))
	}
	if !hasKind(report, CheckTextClearance) {
		t.Fatalf("应报 text-clearance, got %v", findingKinds(report))
	}
	if !strings.Contains(report.Findings[0].Message, "净距") {
		t.Errorf("提示应说明净距: %s", report.Findings[0].Message)
	}

	// 相同净距下，关闭判据（0）后不报。
	quiet, err := CheckFigure(svg, CheckOptions{TextClearanceMm: 0})
	if err != nil {
		t.Fatal(err)
	}
	if hasKind(quiet, CheckTextClearance) {
		t.Error("净距为 0 时应关闭该判据")
	}
}

// 净距够远时不报。
func TestCheckClearancePasses(t *testing.T) {
	svg := wrapTestSVG(`
		<text x="50" y="50" text-anchor="middle" font-size="5" fill="#000000" stroke="none">标号</text>
		<line x1="20" y1="60" x2="80" y2="60" stroke="#000000" stroke-width="0.3"/>`)
	report, err := CheckFigure(svg, CheckOptions{TextClearanceMm: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(BlockingFindings(report)) != 0 {
		t.Fatalf("不应有阻断级发现: %v", findingKinds(report))
	}
}

// 同一语义分组内的文字与线条不互判（节点标签与节点框、边标签与自己的边）。
func TestCheckSameGroupExempt(t *testing.T) {
	sameGroup := wrapTestSVG(`
		<g class="node">
			<text x="50" y="50" text-anchor="middle" font-size="5" fill="#000000" stroke="none">节点</text>
			<line x1="20" y1="50" x2="80" y2="50" stroke="#000000" stroke-width="0.3"/>
		</g>`)
	report, err := CheckFigure(sameGroup, CheckOptions{TextClearanceMm: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(BlockingFindings(report)) != 0 {
		t.Fatalf("同组文字与线条不应互判: %v", findingKinds(report))
	}

	// 同一位置的线条若属于另一分组（edge），仍判穿字。
	crossGroup := wrapTestSVG(`
		<g class="node">
			<text x="50" y="50" text-anchor="middle" font-size="5" fill="#000000" stroke="none">节点</text>
		</g>
		<g class="edge">
			<line x1="20" y1="50" x2="80" y2="50" stroke="#000000" stroke-width="0.3"/>
		</g>`)
	report, err = CheckFigure(crossGroup, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(report, CheckTextCrossed) {
		t.Fatalf("跨组压线应判穿字, got %v", findingKinds(report))
	}
}

// 引线（data-figure-role="leader"）不参与净距与穿字判定。
func TestCheckLeaderExempt(t *testing.T) {
	svg := wrapTestSVG(`
		<text x="50" y="50" text-anchor="middle" font-size="5" fill="#000000" stroke="none">标号</text>
		<line x1="20" y1="50" x2="80" y2="50" data-figure-role="leader" stroke="#000000" stroke-width="0.25"/>`)
	report, err := CheckFigure(svg, CheckOptions{TextClearanceMm: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(BlockingFindings(report)) != 0 {
		t.Fatalf("引线不应被判穿字或净距不足: %v", findingKinds(report))
	}
}

// transform 与继承字号参与量测。
func TestCheckTransformAndInheritedFont(t *testing.T) {
	svg := wrapTestSVG(`
		<g transform="translate(10,0)" font-size="5">
			<text x="40" y="50" text-anchor="middle" fill="#000000" stroke="none">标号</text>
		</g>
		<line x1="20" y1="50" x2="80" y2="50" stroke="#000000" stroke-width="0.3"/>`)
	report, err := CheckFigure(svg, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(report, CheckTextCrossed) {
		t.Fatalf("翻译后的文字位置应参与判定, got %v", findingKinds(report))
	}
}

func TestCheckFontBelowMinimum(t *testing.T) {
	svg := wrapTestSVG(`
		<text x="50" y="50" text-anchor="middle" font-size="3" fill="#000000" stroke="none">小字</text>`)
	report, err := CheckFigure(svg, CheckOptions{MinFontMm: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(report, CheckFontBelowMin) {
		t.Fatalf("应报 font-below-minimum, got %v", findingKinds(report))
	}
	if report.MinFontMm <= 0 || report.MinFontMm >= 3 {
		t.Errorf("MinFontMm = %v, want (0,3)", report.MinFontMm)
	}
}

func TestCheckInkOutsideCanvas(t *testing.T) {
	svg := wrapTestSVG(`<line x1="20" y1="50" x2="120" y2="50" stroke="#000000" stroke-width="0.3"/>`)
	report, err := CheckFigure(svg, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(report, CheckInkOutside) {
		t.Fatalf("应报 ink-outside-canvas, got %v", findingKinds(report))
	}
}

func TestCheckNotMeasured(t *testing.T) {
	svg := wrapTestSVG(`
		<use href="#symbol"/>
		<style>.a { stroke: red }</style>
		<path d="M10,10 A5,5 0 0 0 20,20" fill="none" stroke="#000000"/>`)
	report, err := CheckFigure(svg, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(report, CheckNotMeasured) {
		t.Fatalf("应记录未量测结构, got %v", findingKinds(report))
	}
	for _, finding := range report.Findings {
		if finding.Kind == CheckNotMeasured && finding.Blocking {
			t.Error("not-measured 不应为阻断级")
		}
	}
}

func TestClearanceMmTiers(t *testing.T) {
	cases := map[string]float64{
		"flowchart": 1.5, "block": 0.5, "hierarchy": 0.5,
		"sequence": 0.7, "section": 1.5, "unknown": 0,
	}
	for figureType, want := range cases {
		if got := ClearanceMm(figureType); got != want {
			t.Errorf("ClearanceMm(%q) = %v, want %v", figureType, got, want)
		}
	}
}
