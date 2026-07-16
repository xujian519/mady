// computer_use_som.go：SOM（Set-of-Mark）标注渲染与纯解析助手，平台无关。
// 职责：AX 树文本解析为元素列表、截图编号叠加层绘制（basicfont 标注）、
// 截图按窗口边界裁剪、数字字符串解析。

package tools

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"regexp"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type somElement struct {
	ID    int
	Label string
	X, Y  int
	W, H  int
}

var somColors = []color.RGBA{
	{255, 50, 50, 200},   // red
	{50, 150, 255, 200},  // blue
	{50, 200, 50, 200},   // green
	{255, 200, 0, 200},   // yellow
	{200, 50, 200, 200},  // purple
	{255, 100, 0, 200},   // orange
	{0, 200, 200, 200},   // cyan
	{200, 100, 50, 200},  // brown
	{100, 200, 100, 200}, // light green
	{200, 150, 200, 200}, // pink
}

func renderSOMOverlay(jpegBase64, axTree string) (string, []somElement, error) {
	raw, err := base64.StdEncoding.DecodeString(jpegBase64)
	if err != nil {
		return "", nil, fmt.Errorf("decode screenshot: %w", err)
	}
	elements := parseAXElements(axTree)
	annotated, err := renderSOMBody(raw, elements)
	if err != nil {
		return "", nil, err
	}
	return annotated, elements, nil
}

func renderSOMOverlayFromB64(jpegBase64 string, elements []somElement) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(jpegBase64)
	if err != nil {
		return "", fmt.Errorf("decode screenshot: %w", err)
	}
	return renderSOMBody(raw, elements)
}

func renderSOMBody(raw []byte, elements []somElement) (string, error) {
	src, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		// fallback: try other formats
		src2, _, err2 := image.Decode(bytes.NewReader(raw))
		if err2 != nil {
			return "", fmt.Errorf("decode image: jpeg: %w, other: %w", err, err2)
		}
		src = src2
	}

	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, image.Point{}, draw.Src)

	for i, el := range elements {
		c := somColors[i%len(somColors)]
		r := image.Rect(el.X, el.Y, el.X+el.W, el.Y+el.H)
		r = r.Intersect(bounds)
		if r.Empty() {
			continue
		}

		overlay := image.NewRGBA(r)
		draw.Draw(overlay, overlay.Bounds(), &image.Uniform{color.RGBA{c.R, c.G, c.B, 80}}, image.Point{}, draw.Src)
		draw.Draw(dst, r, overlay, image.Point{}, draw.Over)

		for dx := 0; dx < r.Dx(); dx++ {
			if dx < 2 || dx > r.Dx()-3 {
				for dy := 0; dy < r.Dy(); dy++ {
					dst.Set(r.Min.X+dx, r.Min.Y+dy, c)
				}
			}
		}
		for dy := 0; dy < r.Dy(); dy++ {
			if dy < 2 || dy > r.Dy()-3 {
				for dx := 0; dx < r.Dx(); dx++ {
					dst.Set(r.Min.X+dx, r.Min.Y+dy, c)
				}
			}
		}

		numStr := fmt.Sprintf("%d", el.ID)
		labelX := r.Min.X + 4
		labelY := r.Min.Y + 14
		if labelX < 0 {
			labelX = 0
		}
		if labelY < 0 {
			labelY = 14
		}
		d := font.Drawer{
			Dst:  dst,
			Src:  image.NewUniform(color.RGBA{c.R, c.G, c.B, 255}),
			Face: basicfont.Face7x13,
			Dot:  fixed.P(labelX-1, labelY-1),
		}
		d.DrawString(numStr)
		d.Src = image.NewUniform(color.White)
		d.Dot = fixed.P(labelX, labelY)
		d.DrawString(numStr)

		if el.Label != "" {
			maxLen := (r.Dx() - 8) / 7
			if maxLen > 0 && len(el.Label) > maxLen {
				el.Label = el.Label[:maxLen]
			}
			if maxLen > 0 {
				d.Dot = fixed.P(labelX, labelY+14)
				d.DrawString(el.Label)
			}
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return "", fmt.Errorf("encode annotated jpeg: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func parseAXElements(axTree string) []somElement {
	var elements []somElement
	seen := make(map[int]bool)

	re := regexp.MustCompile(`ax_id[=:](\d+)`)
	posRe := regexp.MustCompile(`(?:pos|position)\s*[=:]\s*\(?\s*(\d+)\s*,?\s*(\d+)`)
	sizeRe := regexp.MustCompile(`size\s*[=:]\s*\(?\s*(\d+)\s*,?\s*(\d+)`)
	labelRe := regexp.MustCompile(`"([^"]{1,40})"`)

	lines := strings.Split(axTree, "\n")
	for _, line := range lines {
		idMatch := re.FindStringSubmatch(line)
		if idMatch == nil {
			continue
		}
		id := parseInt(idMatch[1])
		if id <= 0 || seen[id] {
			continue
		}

		posMatch := posRe.FindStringSubmatch(line)
		if posMatch == nil {
			continue
		}
		x, y := parseInt(posMatch[1]), parseInt(posMatch[2])

		var w, h int
		if sizeMatch := sizeRe.FindStringSubmatch(line); sizeMatch != nil {
			w, h = parseInt(sizeMatch[1]), parseInt(sizeMatch[2])
		}
		if w <= 0 || h <= 0 {
			w, h = 20, 20
		}

		var label string
		if labelMatch := labelRe.FindStringSubmatch(line); labelMatch != nil {
			label = labelMatch[1]
		}

		seen[id] = true
		elements = append(elements, somElement{
			ID:    id,
			Label: label,
			X:     x,
			Y:     y,
			W:     w,
			H:     h,
		})
	}
	return elements
}

func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

func cropImageToBounds(data []byte, x, y, w, h int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, nil
	}
	bounds := img.Bounds()
	r := image.Rect(x, y, x+w, y+h).Intersect(bounds)
	if r.Empty() {
		return data, nil
	}
	dst := image.NewRGBA(r)
	draw.Draw(dst, dst.Bounds(), img, r.Min, draw.Src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return data, nil
	}
	return buf.Bytes(), nil
}
