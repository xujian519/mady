package generate

import (
	"math"
	"testing"
)

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestTextWidth(t *testing.T) {
	cases := []struct {
		content string
		size    float64
		want    float64
	}{
		{"AB", 10, 15},
		{"ab", 10, 12},
		{"中文", 10, 20},
		{"R1", 10, 13.5},
	}
	for _, tc := range cases {
		if got := TextWidth(tc.content, tc.size); !almost(got, tc.want) {
			t.Errorf("TextWidth(%q, %v) = %v, want %v", tc.content, tc.size, got, tc.want)
		}
	}
}

func TestGlyphBoxFor(t *testing.T) {
	box := GlyphBoxFor("中文", [2]float64{50, 50}, 10, AnchorMiddle)
	if !almost(box.MinX, 40) || !almost(box.MaxX, 60) {
		t.Errorf("middle 宽度不对: %+v", box)
	}
	if !almost(box.MinY, 42.5) || !almost(box.MaxY, 51.2) {
		t.Errorf("高度不对: %+v", box)
	}
	start := GlyphBoxFor("中文", [2]float64{50, 50}, 10, AnchorStart)
	if !almost(start.MinX, 50) {
		t.Errorf("start 左边界 = %v", start.MinX)
	}
	end := GlyphBoxFor("中文", [2]float64{50, 50}, 10, AnchorEnd)
	if !almost(end.MaxX, 50) {
		t.Errorf("end 右边界 = %v", end.MaxX)
	}
}

func TestBoxCrossedBySegment(t *testing.T) {
	box := GlyphBox{MinX: 40, MinY: 42.5, MaxX: 60, MaxY: 51.2}
	if !BoxCrossedBySegment(box, [2]float64{20, 50}, [2]float64{80, 50}) {
		t.Error("水平穿线应判穿过")
	}
	if BoxCrossedBySegment(box, [2]float64{20, 60}, [2]float64{80, 60}) {
		t.Error("框外线条不应判穿过")
	}
	// 贴边不算：沿 y=42.5 的边共线。
	if BoxCrossedBySegment(box, [2]float64{20, 42.5}, [2]float64{80, 42.5}) {
		t.Error("沿边共线不应判穿过")
	}
	// 只在角上掠过不算。
	if BoxCrossedBySegment(box, [2]float64{30, 32.5}, [2]float64{40, 42.5}) {
		t.Error("角上掠过不应判穿过")
	}
}

func TestInflateQuadAndDistance(t *testing.T) {
	box := GlyphBox{MinX: 40, MinY: 40, MaxX: 60, MaxY: 50}
	inflated := InflateQuad(BoxQuad(box), 2)
	if !almost(inflated.Origin[0], 38) || !almost(inflated.Origin[1], 38) {
		t.Errorf("外扩原点 = %v", inflated.Origin)
	}
	if !almost(inflated.EdgeWidth[0], 24) || !almost(inflated.EdgeHeight[1], 14) {
		t.Errorf("外扩尺寸 = %v×%v", inflated.EdgeWidth, inflated.EdgeHeight)
	}
	distance := QuadSegmentDistance(BoxQuad(box), [2]float64{0, 52}, [2]float64{100, 52})
	if !almost(distance, 2) {
		t.Errorf("最短距离 = %v, want 2", distance)
	}
}

func TestLeaderEnd(t *testing.T) {
	centerY := 50 - 10*GlyphAscentRatio + 10*(GlyphAscentRatio+GlyphDescentRatio)/2
	end, ok := LeaderEnd("中文", [2]float64{50, 50}, 10, AnchorMiddle, [2]float64{0, centerY})
	if !ok {
		t.Fatal("应求得引线止点")
	}
	// 框左边界 40，自左侧引出的引线应止于 x=40。
	if !almost(end[0], 40) || !almost(end[1], centerY) {
		t.Errorf("止点 = %v, want [40 %v]", end, centerY)
	}
	if _, ok := LeaderEnd("中文", [2]float64{50, 50}, 10, AnchorMiddle, [2]float64{50, 50}); ok {
		t.Error("起点与中心重合时应返回 false")
	}
}

func TestSegmentsIntersect(t *testing.T) {
	if !SegmentsIntersect([2]float64{0, 0}, [2]float64{10, 10}, [2]float64{0, 10}, [2]float64{10, 0}) {
		t.Error("交叉线段应判相交")
	}
	if SegmentsIntersect([2]float64{0, 0}, [2]float64{10, 0}, [2]float64{0, 5}, [2]float64{10, 5}) {
		t.Error("平行线段不应判相交")
	}
}
