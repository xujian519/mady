package generate

import (
	"math"
)

// 文字占位框：绘图侧（引线止点、标号落位）与复核侧（线条是否穿过文字）共用同一
// 套估算，否则一侧按较松的框躲避、另一侧按较紧的框判定，会把正确的图报成缺陷。
//
// 框以基线锚点定位：上方 AscentRatio 倍字号、下方 DescentRatio 倍字号；宽度逐码点
// 累加，全角字符按 1 个字号、西文大写按 0.75、其余按 0.6。
const (
	GlyphWidthRatio     = 0.6
	UpperWidthRatio     = 0.75
	FullWidthRatio      = 1.0
	GlyphAscentRatio    = 0.75
	GlyphDescentRatio   = 0.12
	GlyphBoxToleranceMm = 0.01
)

// 文字水平对齐方式（与 SVG text-anchor 同义）。
const (
	AnchorStart  = "start"
	AnchorMiddle = "middle"
	AnchorEnd    = "end"
)

// GlyphBox 是轴对齐的文字占位框（用户单位或毫米，随调用方坐标系）。
type GlyphBox struct {
	MinX, MinY, MaxX, MaxY float64
}

// GlyphQuad 是仿射矩形：origin 顶点与两条边向量。
type GlyphQuad struct {
	Origin     [2]float64
	EdgeWidth  [2]float64
	EdgeHeight [2]float64
}

// fullWidthRanges 是全角码点区间（Unicode East Asian Width 的 W 与 F 常用范围）。
var fullWidthRanges = [][2]rune{
	{0x1100, 0x115f}, {0x2e80, 0x303e}, {0x3041, 0x33ff}, {0x3400, 0x4dbf},
	{0x4e00, 0x9fff}, {0xa000, 0xa4cf}, {0xac00, 0xd7a3}, {0xf900, 0xfaff},
	{0xfe30, 0xfe6f}, {0xff00, 0xff60}, {0xffe0, 0xffe6}, {0x20000, 0x3fffd},
}

func isFullWidth(code rune) bool {
	for _, r := range fullWidthRanges {
		if code >= r[0] && code <= r[1] {
			return true
		}
	}
	return false
}

// TextWidth 估算文字排版宽度：全角 1 倍字号、西文大写 0.75、其余 0.6。
func TextWidth(content string, fontSize float64) float64 {
	ratio := 0.0
	for _, code := range content {
		switch {
		case isFullWidth(code):
			ratio += FullWidthRatio
		case code >= 'A' && code <= 'Z':
			ratio += UpperWidthRatio
		default:
			ratio += GlyphWidthRatio
		}
	}
	return ratio * fontSize
}

// GlyphBoxFor 按基线锚点、字号与对齐方式计算占位框。
func GlyphBoxFor(content string, baseline [2]float64, fontSize float64, anchor string) GlyphBox {
	width := TextWidth(content, fontSize)
	left := baseline[0]
	switch anchor {
	case AnchorMiddle:
		left -= width / 2
	case AnchorEnd:
		left -= width
	}
	return GlyphBox{
		MinX: left,
		MaxX: left + width,
		MinY: baseline[1] - fontSize*GlyphAscentRatio,
		MaxY: baseline[1] + fontSize*GlyphDescentRatio,
	}
}

// BoxQuad 把轴对齐框转成仿射矩形。
func BoxQuad(box GlyphBox) GlyphQuad {
	return GlyphQuad{
		Origin:     [2]float64{box.MinX, box.MinY},
		EdgeWidth:  [2]float64{box.MaxX - box.MinX, 0},
		EdgeHeight: [2]float64{0, box.MaxY - box.MinY},
	}
}

// QuadCorners 返回仿射矩形的四个顶点（origin、+宽、+宽+高、+高）。
func QuadCorners(quad GlyphQuad) [4][2]float64 {
	p0 := quad.Origin
	p1 := [2]float64{p0[0] + quad.EdgeWidth[0], p0[1] + quad.EdgeWidth[1]}
	p2 := [2]float64{p1[0] + quad.EdgeHeight[0], p1[1] + quad.EdgeHeight[1]}
	p3 := [2]float64{p0[0] + quad.EdgeHeight[0], p0[1] + quad.EdgeHeight[1]}
	return [4][2]float64{p0, p1, p2, p3}
}

func unitVector(v [2]float64) [2]float64 {
	length := math.Hypot(v[0], v[1])
	if length == 0 {
		return [2]float64{0, 0}
	}
	return [2]float64{v[0] / length, v[1] / length}
}

// InflateQuad 沿两条边的方向各向外扩 clearance，等价于四周留出净距。
func InflateQuad(quad GlyphQuad, clearance float64) GlyphQuad {
	widthUnit := unitVector(quad.EdgeWidth)
	heightUnit := unitVector(quad.EdgeHeight)
	return GlyphQuad{
		Origin: [2]float64{
			quad.Origin[0] - clearance*(widthUnit[0]+heightUnit[0]),
			quad.Origin[1] - clearance*(widthUnit[1]+heightUnit[1]),
		},
		EdgeWidth: [2]float64{
			quad.EdgeWidth[0] + 2*clearance*widthUnit[0],
			quad.EdgeWidth[1] + 2*clearance*widthUnit[1],
		},
		EdgeHeight: [2]float64{
			quad.EdgeHeight[0] + 2*clearance*heightUnit[0],
			quad.EdgeHeight[1] + 2*clearance*heightUnit[1],
		},
	}
}

// clipInterval 把参数区间收窄到 value0+t·(value1−value0) 落在 (low,high) 内的部分。
func clipInterval(interval [2]float64, value0, value1, low, high float64) ([2]float64, bool) {
	if value0 == value1 {
		if value0 > low && value0 < high {
			return interval, true
		}
		return [2]float64{}, false
	}
	atLow := (low - value0) / (value1 - value0)
	atHigh := (high - value0) / (value1 - value0)
	lower := math.Max(interval[0], math.Min(atLow, atHigh))
	upper := math.Min(interval[1], math.Max(atLow, atHigh))
	if lower < upper {
		return [2]float64{lower, upper}, true
	}
	return [2]float64{}, false
}

// QuadCrossedBySegment 报告线段是否真正进入仿射矩形内部（离每条边至少
// GlyphBoxToleranceMm）。贴边、沿边共线、只在角上掠过都不算。
func QuadCrossedBySegment(quad GlyphQuad, from, to [2]float64) bool {
	width, height := quad.EdgeWidth, quad.EdgeHeight
	determinant := width[0]*height[1] - width[1]*height[0]
	if determinant == 0 {
		return false
	}
	coordinate := func(point [2]float64) [2]float64 {
		dx, dy := point[0]-quad.Origin[0], point[1]-quad.Origin[1]
		return [2]float64{
			(dx*height[1] - dy*height[0]) / determinant,
			(width[0]*dy - width[1]*dx) / determinant,
		}
	}
	fromCoord := coordinate(from)
	toCoord := coordinate(to)
	marginAlpha := GlyphBoxToleranceMm / math.Hypot(width[0], width[1])
	marginBeta := GlyphBoxToleranceMm / math.Hypot(height[0], height[1])
	interval := [2]float64{0, 1}
	for _, axis := range [][3]float64{
		{fromCoord[0], toCoord[0], marginAlpha},
		{fromCoord[1], toCoord[1], marginBeta},
	} {
		clipped, ok := clipInterval(interval, axis[0], axis[1], axis[2], 1-axis[2])
		if !ok {
			return false
		}
		interval = clipped
	}
	return true
}

// BoxCrossedBySegment 是 QuadCrossedBySegment 的轴对齐形式。
func BoxCrossedBySegment(box GlyphBox, from, to [2]float64) bool {
	return QuadCrossedBySegment(BoxQuad(box), from, to)
}

// LeaderEnd 求引线在文字占位框上的止点：自 from 指向文字中心，止于框边界。
// 起点与中心重合或已在框内时 ok=false。
func LeaderEnd(content string, baseline [2]float64, fontSize float64, anchor string, from [2]float64) ([2]float64, bool) {
	box := GlyphBoxFor(content, baseline, fontSize, anchor)
	center := [2]float64{(box.MinX + box.MaxX) / 2, (box.MinY + box.MaxY) / 2}
	dx, dy := center[0]-from[0], center[1]-from[1]
	distance := math.Hypot(dx, dy)
	if distance == 0 {
		return [2]float64{}, false
	}
	unit := [2]float64{dx / distance, dy / distance}
	stepX := math.Inf(1)
	if unit[0] != 0 {
		if unit[0] > 0 {
			stepX = (center[0] - box.MinX) / math.Abs(unit[0])
		} else {
			stepX = (box.MaxX - center[0]) / math.Abs(unit[0])
		}
	}
	stepY := math.Inf(1)
	if unit[1] != 0 {
		if unit[1] > 0 {
			stepY = (center[1] - box.MinY) / math.Abs(unit[1])
		} else {
			stepY = (box.MaxY - center[1]) / math.Abs(unit[1])
		}
	}
	back := math.Min(stepX, stepY)
	if !(back < distance) {
		return [2]float64{}, false
	}
	return [2]float64{center[0] - back*unit[0], center[1] - back*unit[1]}, true
}

// SegmentDistance 返回两条线段的最短距离；相交为 0。
func SegmentDistance(a1, a2, b1, b2 [2]float64) float64 {
	if SegmentsIntersect(a1, a2, b1, b2) {
		return 0
	}
	return math.Min(
		math.Min(PointSegmentDistance(a1, b1, b2), PointSegmentDistance(a2, b1, b2)),
		math.Min(PointSegmentDistance(b1, a1, a2), PointSegmentDistance(b2, a1, a2)),
	)
}

// SegmentsIntersect 报告两条线段是否相交（含端点接触与共线重叠）。
func SegmentsIntersect(a1, a2, b1, b2 [2]float64) bool {
	d1 := sideOf(a1, a2, b1)
	d2 := sideOf(a1, a2, b2)
	d3 := sideOf(b1, b2, a1)
	d4 := sideOf(b1, b2, a2)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	return onSegment(b1, a1, a2) || onSegment(b2, a1, a2) ||
		onSegment(a1, b1, b2) || onSegment(a2, b1, b2)
}

func sideOf(a, b, p [2]float64) float64 {
	return (b[0]-a[0])*(p[1]-a[1]) - (b[1]-a[1])*(p[0]-a[0])
}

func onSegment(p, a, b [2]float64) bool {
	if math.Abs(sideOf(a, b, p)) > 1e-9 {
		return false
	}
	return p[0] >= math.Min(a[0], b[0])-1e-9 && p[0] <= math.Max(a[0], b[0])+1e-9 &&
		p[1] >= math.Min(a[1], b[1])-1e-9 && p[1] <= math.Max(a[1], b[1])+1e-9
}

// PointSegmentDistance 返回点到线段的最短距离。
func PointSegmentDistance(p, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	lengthSquared := dx*dx + dy*dy
	if lengthSquared == 0 {
		return math.Hypot(p[0]-a[0], p[1]-a[1])
	}
	t := ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / lengthSquared
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(p[0]-(a[0]+t*dx), p[1]-(a[1]+t*dy))
}

// QuadSegmentDistance 返回仿射矩形四条边到线段的最短距离。
func QuadSegmentDistance(quad GlyphQuad, from, to [2]float64) float64 {
	corners := QuadCorners(quad)
	best := math.Inf(1)
	for i := 0; i < 4; i++ {
		next := corners[(i+1)%4]
		best = math.Min(best, SegmentDistance(corners[i], next, from, to))
	}
	return best
}
