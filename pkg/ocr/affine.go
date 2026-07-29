package ocr

import (
	"image"
	"image/color"
	"math"
)

func extractQuad(src *image.NRGBA, q Quad) *image.NRGBA {
	tl, tr, br, bl := sortQuadPoints(q)

	dx1 := tr.X - tl.X
	dy1 := tr.Y - tl.Y

	if dx1 == 0 && dy1 == 0 {
		w := dist(tl, tr)
		if w < 2 {
			w = 2
		}
		h := dist(tl, bl)
		if h < 2 {
			h = 2
		}
		return cropRect(src, image.Rect(tl.X, tl.Y, tl.X+w, tl.Y+h))
	}

	angle := math.Atan2(float64(dy1), float64(dx1))
	if absFloat(angle) > 0.05 {
		return rotateCrop(src, tl, tr, br, bl)
	}

	r := image.Rect(tl.X, tl.Y, br.X, br.Y).Intersect(src.Bounds())
	if r.Empty() {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	return cropRect(src, r)
}

func sortQuadPoints(q Quad) (tl, tr, br, bl image.Point) {
	pts := q.P[:]

	var topLeft, topRight, bottomLeft, bottomRight image.Point
	topLeft = pts[0]

	topIdx := 0
	for i, p := range pts {
		if p.Y < topLeft.Y || (p.Y == topLeft.Y && p.X < topLeft.X) {
			topLeft = p
			topIdx = i
		}
	}

	rightMost := pts[(topIdx+1)%4]
	leftMost := pts[(topIdx+3)%4]

	if dist(topLeft, rightMost) > dist(topLeft, leftMost) {
		topRight = rightMost
		bottomLeft = leftMost
	} else {
		topRight = leftMost
		bottomLeft = rightMost
	}

	used := map[image.Point]bool{topLeft: true, topRight: true, bottomLeft: true}
	for _, p := range pts {
		if !used[p] {
			bottomRight = p
			break
		}
	}

	return topLeft, topRight, bottomRight, bottomLeft
}

func rotateCrop(src *image.NRGBA, tl, tr, br, bl image.Point) *image.NRGBA {
	topW := float64(dist(tl, tr))
	bottomW := float64(dist(bl, br))
	dstW := int(math.Ceil((topW + bottomW) / 2.0))
	if dstW < 2 {
		dstW = 2
	}

	leftH := float64(dist(tl, bl))
	rightH := float64(dist(tr, br))
	dstH := int(math.Ceil((leftH + rightH) / 2.0))
	if dstH < 2 {
		dstH = 2
	}

	srcPts := [4][2]float64{
		{float64(tl.X), float64(tl.Y)},
		{float64(tr.X), float64(tr.Y)},
		{float64(br.X), float64(br.Y)},
		{float64(bl.X), float64(bl.Y)},
	}
	dstPts := [4][2]float64{
		{0, 0},
		{float64(dstW - 1), 0},
		{float64(dstW - 1), float64(dstH - 1)},
		{0, float64(dstH - 1)},
	}

	matrix := getPerspectiveTransform(srcPts, dstPts)

	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	bounds := src.Bounds()
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			sx := matrix[0]*float64(x) + matrix[1]*float64(y) + matrix[2]
			sy := matrix[3]*float64(x) + matrix[4]*float64(y) + matrix[5]

			ix, iy := int(math.Floor(sx)), int(math.Floor(sy))
			if ix < 0 || iy < 0 || ix+1 >= bounds.Max.X || iy+1 >= bounds.Max.Y {
				continue
			}
			fx, fy := sx-float64(ix), sy-float64(iy)
			c00 := src.NRGBAAt(ix, iy)
			c10 := src.NRGBAAt(ix+1, iy)
			c01 := src.NRGBAAt(ix, iy+1)
			c11 := src.NRGBAAt(ix+1, iy+1)

			r := uint8((1-fx)*(1-fy)*float64(c00.R) + fx*(1-fy)*float64(c10.R) + (1-fx)*fy*float64(c01.R) + fx*fy*float64(c11.R))
			g := uint8((1-fx)*(1-fy)*float64(c00.G) + fx*(1-fy)*float64(c10.G) + (1-fx)*fy*float64(c01.G) + fx*fy*float64(c11.G))
			b := uint8((1-fx)*(1-fy)*float64(c00.B) + fx*(1-fy)*float64(c10.B) + (1-fx)*fy*float64(c01.B) + fx*fy*float64(c11.B))
			dst.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return dst
}

func getPerspectiveTransform(src, dst [4][2]float64) [8]float64 {
	var matrix [8]float64
	n := 8
	A := make([]float64, n*n)
	B := make([]float64, n)

	for i := 0; i < 4; i++ {
		sx, sy := src[i][0], src[i][1]
		dx, dy := dst[i][0], dst[i][1]

		rowA := i * 2
		A[rowA*n+0] = sx
		A[rowA*n+1] = sy
		A[rowA*n+2] = 1
		A[rowA*n+3] = 0
		A[rowA*n+4] = 0
		A[rowA*n+5] = 0
		A[rowA*n+6] = -sx * dx
		A[rowA*n+7] = -sy * dx
		B[rowA] = dx

		rowB := rowA + 1
		A[rowB*n+0] = 0
		A[rowB*n+1] = 0
		A[rowB*n+2] = 0
		A[rowB*n+3] = sx
		A[rowB*n+4] = sy
		A[rowB*n+5] = 1
		A[rowB*n+6] = -sx * dy
		A[rowB*n+7] = -sy * dy
		B[rowB] = dy
	}

	_ = gaussianElim(A, B, &matrix, n)
	return matrix
}

func gaussianElim(A []float64, B []float64, result *[8]float64, n int) bool {
	for i := 0; i < n; i++ {
		maxIdx := i
		for j := i + 1; j < n; j++ {
			if absFloat(A[j*n+i]) > absFloat(A[maxIdx*n+i]) {
				maxIdx = j
			}
		}

		for j := i; j < n; j++ {
			A[i*n+j], A[maxIdx*n+j] = A[maxIdx*n+j], A[i*n+j]
		}
		B[i], B[maxIdx] = B[maxIdx], B[i]

		pivot := A[i*n+i]
		if absFloat(pivot) < 1e-12 {
			continue
		}

		for j := i + 1; j < n; j++ {
			factor := A[j*n+i] / pivot
			for k := i; k < n; k++ {
				A[j*n+k] -= factor * A[i*n+k]
			}
			B[j] -= factor * B[i]
		}
	}

	for i := n - 1; i >= 0; i-- {
		sum := B[i]
		for j := i + 1; j < n; j++ {
			sum -= A[i*n+j] * result[j]
		}
		if absFloat(A[i*n+i]) > 1e-12 {
			result[i] = sum / A[i*n+i]
		}
	}
	return true
}
