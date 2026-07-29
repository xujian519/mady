package ocr

import (
	"fmt"
	"image"
	"math"
	"sort"

	ort "github.com/getcharzp/onnxruntime_purego"
)

type pxPoint struct{ X, Y int }

var (
	detMean = [3]float32{0.485, 0.456, 0.406}
	detStd  = [3]float32{0.229, 0.224, 0.225}
)

const (
	detMaxSide     = 960
	detThresh      = float32(0.3)
	detBoxThresh   = float32(0.7)
	detMinSide     = 3
	detUnclipRatio = 1.6
)

func (o *OCR) runDet(img *image.NRGBA) ([]Quad, error) {
	srcW, srcH := img.Bounds().Dx(), img.Bounds().Dy()

	scale := 1.0
	maxSide := srcW
	if srcH > maxSide {
		maxSide = srcH
	}
	if maxSide > detMaxSide {
		scale = float64(detMaxSide) / float64(maxSide)
	}
	resizedW := roundUpTo32(int(float64(srcW) * scale))
	resizedH := roundUpTo32(int(float64(srcH) * scale))
	if resizedW < 32 {
		resizedW = 32
	}
	if resizedH < 32 {
		resizedH = 32
	}
	resized := resizeBilinear(img, resizedW, resizedH)

	input := normalizeHWC2CHW(resized, detMean, detStd)

	inputTensor, err := ort.NewTensor([]int64{1, 3, int64(resizedH), int64(resizedW)}, input)
	if err != nil {
		return nil, fmt.Errorf("det: create input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	if len(o.det.InputNames) == 0 {
		return nil, fmt.Errorf("det: model has no input")
	}
	outputs, err := o.det.Run(map[string]*ort.Value{o.det.InputNames[0]: inputTensor})
	if err != nil {
		return nil, fmt.Errorf("det: run: %w", err)
	}
	for _, v := range outputs {
		defer v.Destroy()
	}

	var outVal *ort.Value
	for _, v := range outputs {
		outVal = v
		break
	}
	probData, err := ort.GetTensorData[float32](outVal)
	if err != nil {
		return nil, fmt.Errorf("det: read output: %w", err)
	}
	shape, err := outVal.GetShape()
	if err != nil {
		return nil, fmt.Errorf("det: output shape: %w", err)
	}
	if len(shape) < 4 {
		return nil, fmt.Errorf("det: unexpected output shape: %v", shape)
	}
	outH, outW := int(shape[len(shape)-2]), int(shape[len(shape)-1])

	quads := segmentationToQuads(probData, outW, outH, detThresh)

	scaleX := float64(srcW) / float64(outW)
	scaleY := float64(srcH) / float64(outH)
	var result []Quad
	for _, q := range quads {
		for i := 0; i < 4; i++ {
			q.P[i].X = int(float64(q.P[i].X) * scaleX)
			q.P[i].Y = int(float64(q.P[i].Y) * scaleY)
		}
		bb := q.BoundingBox()
		if bb[2]-bb[0] < detMinSide || bb[3]-bb[1] < detMinSide {
			continue
		}
		result = append(result, q)
	}

	sort.Slice(result, func(i, j int) bool {
		ci, cj := result[i].Center(), result[j].Center()
		hi := result[i].Height()
		hj := result[j].Height()
		hmax := hi
		if hj > hmax {
			hmax = hj
		}
		if abs(ci.Y-cj.Y) <= hmax/2 {
			return ci.X < cj.X
		}
		return ci.Y < cj.Y
	})

	return result, nil
}

func (q Quad) Center() image.Point {
	return image.Point{
		X: (q.P[0].X + q.P[1].X + q.P[2].X + q.P[3].X) / 4,
		Y: (q.P[0].Y + q.P[1].Y + q.P[2].Y + q.P[3].Y) / 4,
	}
}

func (q Quad) Height() int {
	d1, d2 := dist(q.P[0], q.P[1]), dist(q.P[2], q.P[3])
	if d1 > d2 {
		return d1
	}
	return d2
}

func dist(a, b image.Point) int {
	dx, dy := a.X-b.X, a.Y-b.Y
	return int(math.Sqrt(float64(dx*dx + dy*dy)))
}

func segmentationToQuads(prob []float32, w, h int, thresh float32) []Quad {
	if len(prob) < w*h {
		return nil
	}
	visited := make([]bool, w*h)
	var quads []Quad

	dirs := [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	queue := make([]int, 0, 1024)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			if visited[idx] || prob[idx] < thresh {
				continue
			}
			queue = queue[:0]
			queue = append(queue, idx)
			visited[idx] = true

			var pts []pxPoint
			for qi := 0; qi < len(queue); qi++ {
				i := queue[qi]
				cy, cx := i/w, i%w
				pts = append(pts, pxPoint{X: cx, Y: cy})
				for _, d := range dirs {
					nx, ny := cx+d[0], cy+d[1]
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					ni := ny*w + nx
					if visited[ni] || prob[ni] < thresh {
						continue
					}
					visited[ni] = true
					queue = append(queue, ni)
				}
			}

			if len(pts) < 3 {
				continue
			}
			q := fitQuad(pts)
			q = unclipQuad(q, detUnclipRatio)

			bb := q.BoundingBox()
			if bb[2]-bb[0] < detMinSide || bb[3]-bb[1] < detMinSide {
				continue
			}
			quads = append(quads, q)
		}
	}
	return quads
}

func fitQuad(pts []pxPoint) Quad {
	cx, cy := 0.0, 0.0
	for _, p := range pts {
		cx += float64(p.X)
		cy += float64(p.Y)
	}
	cx /= float64(len(pts))
	cy /= float64(len(pts))

	angle, cov := 0.0, 0.0
	for _, p := range pts {
		dx := float64(p.X) - cx
		dy := float64(p.Y) - cy
		cov += dx * dy
	}
	if len(pts) > 1 {
		cov /= float64(len(pts))
	}

	if absFloat(cov) < 1e-6 {
		angle = 0
	} else {
		angle = math.Atan2(cov, 1.0)
	}

	cosA, sinA := math.Cos(angle), math.Sin(angle)
	minP, maxP := 1e9, -1e9
	minR, maxR := 1e9, -1e9
	for _, p := range pts {
		px := float64(p.X)*cosA + float64(p.Y)*sinA
		py := -float64(p.X)*sinA + float64(p.Y)*cosA
		if px < minP {
			minP = px
		}
		if px > maxP {
			maxP = px
		}
		if py < minR {
			minR = py
		}
		if py > maxR {
			maxR = py
		}
	}

	tl := image.Point{
		X: int(minP*cosA - minR*sinA),
		Y: int(minP*sinA + minR*cosA),
	}
	tr := image.Point{
		X: int(maxP*cosA - minR*sinA),
		Y: int(maxP*sinA + minR*cosA),
	}
	br := image.Point{
		X: int(maxP*cosA - maxR*sinA),
		Y: int(maxP*sinA + maxR*cosA),
	}
	bl := image.Point{
		X: int(minP*cosA - maxR*sinA),
		Y: int(minP*sinA + maxR*cosA),
	}
	return Quad{P: [4]image.Point{tl, tr, br, bl}}
}

func unclipQuad(q Quad, ratio float64) Quad {
	area := polygonArea(q)
	peri := polygonPerimeter(q)
	if peri < 1e-6 {
		return q
	}
	dist := area * ratio / peri
	if dist < 0.5 {
		dist = 0.5
	}
	return offsetPolygon(q, dist)
}

func polygonArea(q Quad) float64 {
	n := 4
	area := 0.0
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		area += float64(q.P[i].X) * float64(q.P[j].Y)
		area -= float64(q.P[j].X) * float64(q.P[i].Y)
	}
	return absFloat(area) / 2.0
}

func polygonPerimeter(q Quad) float64 {
	peri := 0.0
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		dx := float64(q.P[i].X - q.P[j].X)
		dy := float64(q.P[i].Y - q.P[j].Y)
		peri += math.Sqrt(dx*dx + dy*dy)
	}
	return peri
}

func offsetPolygon(q Quad, d float64) Quad {
	var out Quad
	for i := 0; i < 4; i++ {
		prev := (i + 3) % 4
		next := (i + 1) % 4

		dx1 := float64(q.P[i].X - q.P[prev].X)
		dy1 := float64(q.P[i].Y - q.P[prev].Y)
		len1 := math.Sqrt(dx1*dx1 + dy1*dy1)

		dx2 := float64(q.P[next].X - q.P[i].X)
		dy2 := float64(q.P[next].Y - q.P[i].Y)
		len2 := math.Sqrt(dx2*dx2 + dy2*dy2)

		n1x, n1y := -dy1/len1, dx1/len1
		n2x, n2y := -dy2/len2, dx2/len2

		midX, midY := (n1x+n2x)/2, (n1y+n2y)/2
		midLen := math.Sqrt(midX*midX + midY*midY)
		if midLen < 1e-6 {
			out.P[i] = q.P[i]
			continue
		}
		out.P[i].X = q.P[i].X + int(midX/midLen*d)
		out.P[i].Y = q.P[i].Y + int(midY/midLen*d)
	}
	return out
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
