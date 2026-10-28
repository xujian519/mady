package generate

import (
	"strings"
	"testing"
)

const sampleSVG = `<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg width="210mm" height="297mm" viewBox="0 0 744.09 1052.36">
  <g>
    <text text-anchor="middle" x="100" y="100">控制器 (100)</text>
    <text x="200" y="200"><tspan>处理模块</tspan></text>
  </g>
</svg>`

func TestAssertSafeSVG(t *testing.T) {
	if err := AssertSafeSVG(sampleSVG, DefaultSVGMaxBytes); err != nil {
		t.Fatalf("标准 Graphviz SVG 应通过: %v", err)
	}
	if err := AssertSafeSVG(`<svg><!ENTITY x SYSTEM "file:///etc/passwd"></svg>`, DefaultSVGMaxBytes); errorCode(t, err) != ErrUnsafeSVG {
		t.Errorf("ENTITY 应报 unsafe_svg, got %v", err)
	}
	if err := AssertSafeSVG(`<svg><![CDATA[x]]></svg>`, DefaultSVGMaxBytes); errorCode(t, err) != ErrUnsafeSVG {
		t.Errorf("CDATA 应报 unsafe_svg, got %v", err)
	}
	if err := AssertSafeSVG(sampleSVG, 10); errorCode(t, err) != ErrTooLarge {
		t.Errorf("超限应报 too_large, got %v", err)
	}
	if err := AssertSafeSVG("not svg", DefaultSVGMaxBytes); errorCode(t, err) != ErrInvalidSVG {
		t.Errorf("非 SVG 应报 invalid_svg, got %v", err)
	}
}

func TestAnnotateSVG(t *testing.T) {
	result, err := AnnotateSVG(sampleSVG, []Reference{
		{Label: "控制器", Numeral: "20"},
		{Label: "处理模块", Numeral: "30"},
	}, DefaultSVGMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.SVG, "控制器 (100) (20)") {
		t.Errorf("未在 <text> 末尾追加标号:\n%s", result.SVG)
	}
	if !strings.Contains(result.SVG, "<tspan>处理模块 (30)</tspan>") {
		t.Errorf("未在最后一个 <tspan> 文本末尾追加标号:\n%s", result.SVG)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("不应有未命中警告: %v", result.Warnings)
	}
}

func TestAnnotateSVGWarnsUnmatched(t *testing.T) {
	result, err := AnnotateSVG(sampleSVG, []Reference{{Label: "不存在的部件", Numeral: "40"}}, DefaultSVGMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "不存在的部件" {
		t.Errorf("未命中应作为 warning 返回: %v", result.Warnings)
	}
	if result.SVG != sampleSVG {
		t.Error("未命中时 SVG 不应变化")
	}
}

func TestAnnotateSVGRejectsInvalidReference(t *testing.T) {
	if _, err := AnnotateSVG(sampleSVG, []Reference{{Label: " ", Numeral: "20"}}, DefaultSVGMaxBytes); err == nil {
		t.Error("空 label 应报错")
	}
	if _, err := AnnotateSVG(sampleSVG, []Reference{{Label: "控制器", Numeral: " "}}, DefaultSVGMaxBytes); err == nil {
		t.Error("空 numeral 应报错")
	}
}

func TestAnnotateSVGDecodesEntities(t *testing.T) {
	svg := `<svg><text>A&amp;B</text></svg>`
	result, err := AnnotateSVG(svg, []Reference{{Label: "A&B", Numeral: "10"}}, DefaultSVGMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.SVG, "A&amp;B (10)") {
		t.Errorf("实体解码/回写失败:\n%s", result.SVG)
	}
}
