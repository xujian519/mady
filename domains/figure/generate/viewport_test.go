package generate

import (
	"math"
	"strings"
	"testing"
)

func closeTo(got, want float64) bool {
	return math.Abs(got-want) < 1e-6
}

func TestParseLengthMm(t *testing.T) {
	cases := []struct {
		raw  string
		want float64
		ok   bool
	}{
		{"210mm", 210, true},
		{"21cm", 210, true},
		{"8.27in", 210.058, true},
		{"744.09", 196.8738125, true},
		{"96px", 25.4, true},
		{"72pt", 25.4, true},
		{"50%", 0, false},
		{"auto", 0, false},
	}
	for _, tc := range cases {
		got, ok := ParseLengthMm(tc.raw)
		if ok != tc.ok {
			t.Errorf("%q ok = %v, want %v", tc.raw, ok, tc.ok)
			continue
		}
		if ok && !closeTo(got, tc.want) {
			t.Errorf("%q = %.3f mm, want %.3f", tc.raw, got, tc.want)
		}
	}
}

func TestResolveViewportWithViewBox(t *testing.T) {
	tag := `<svg width="210mm" height="297mm" viewBox="0 0 744.09 1052.36" xmlns="http://www.w3.org/2000/svg">`
	viewport := ResolveViewport(tag)
	if viewport.ViewBox == nil {
		t.Fatal("viewBox 未解析")
	}
	if viewport.ViewBox.Width != 744.09 || viewport.ViewBox.Height != 1052.36 {
		t.Errorf("viewBox = %+v", viewport.ViewBox)
	}
	// A4 纵向：两边比例略有差异，meet 取较小者。
	wantScale := math.Min(210.0/744.09, 297.0/1052.36)
	if !closeTo(viewport.ScaleX, wantScale) {
		t.Errorf("ScaleX = %f, want %f", viewport.ScaleX, wantScale)
	}
	if viewport.ScaleX != viewport.ScaleY {
		t.Errorf("等比缩放应两轴相同: %f vs %f", viewport.ScaleX, viewport.ScaleY)
	}
	width, height, ok := viewport.DiagramSizeMm()
	if !ok || math.Abs(width-210) > 0.01 || math.Abs(height-297) > 0.01 {
		t.Errorf("DiagramSizeMm = %f×%f, ok=%v", width, height, ok)
	}
}

func TestResolveViewportWithoutViewBox(t *testing.T) {
	viewport := ResolveViewport(`<svg width="96px" height="192px">`)
	if viewport.ViewBox != nil {
		t.Error("无 viewBox 时不应解析出 viewBox")
	}
	if !closeTo(viewport.ScaleX, MMPerUserUnit) {
		t.Errorf("ScaleX = %f, want %f", viewport.ScaleX, MMPerUserUnit)
	}
	if width, height, ok := viewport.DiagramSizeMm(); !ok || !closeTo(width, 25.4) || !closeTo(height, 50.8) {
		t.Errorf("DiagramSizeMm = %f×%f, ok=%v", width, height, ok)
	}
}

func TestResolveViewportPreserveAspectRatio(t *testing.T) {
	// 视口 200×100、viewBox 50×100：meet 取 1（左右留白），slice 取 4（上下裁切）。
	meet := ResolveViewport(`<svg width="200mm" height="100mm" viewBox="0 0 50 100">`)
	if !closeTo(meet.ScaleX, 1) {
		t.Errorf("meet scale = %f, want 1", meet.ScaleX)
	}
	if !closeTo(meet.OriginX, -75) {
		t.Errorf("meet originX = %f, want -75", meet.OriginX)
	}

	slice := ResolveViewport(`<svg width="200mm" height="100mm" viewBox="0 0 50 100" preserveAspectRatio="xMidYMid slice">`)
	if !closeTo(slice.ScaleX, 4) {
		t.Errorf("slice scale = %f, want 4", slice.ScaleX)
	}
	if !strings.Contains(slice.Note, "裁掉") {
		t.Errorf("slice 应给出裁剪提示: %q", slice.Note)
	}

	none := ResolveViewport(`<svg width="200mm" height="100mm" viewBox="0 0 100 100" preserveAspectRatio="none">`)
	if !closeTo(none.ScaleX, 2) || !closeTo(none.ScaleY, 1) {
		t.Errorf("none 两轴缩放 = %f×%f, want 2×1", none.ScaleX, none.ScaleY)
	}
	if !strings.Contains(none.Note, "两轴缩放不等") {
		t.Errorf("none 应给出近似说明: %q", none.Note)
	}
}

func TestResolveViewportInvalidViewBox(t *testing.T) {
	viewport := ResolveViewport(`<svg width="100" height="100" viewBox="0 0 -1 50">`)
	if viewport.ViewBox != nil {
		t.Error("宽为负的 viewBox 不应被接受")
	}
	if !strings.Contains(viewport.Note, "无法解析") {
		t.Errorf("应给出解析失败说明: %q", viewport.Note)
	}
}

func TestRootTag(t *testing.T) {
	svg := "<?xml version=\"1.0\"?>\n<!DOCTYPE svg PUBLIC \"-//W3C//DTD SVG 1.1//EN\" \"x\">\n<svg width=\"10\" height=\"20\" viewBox=\"0 0 10 20\">content</svg>"
	tag := RootTag(svg)
	if !strings.Contains(tag, `viewBox="0 0 10 20"`) {
		t.Errorf("RootTag = %q", tag)
	}
	if RootTag("no svg here") != "" {
		t.Error("非 SVG 应返回空")
	}
}
