package ocr

import (
	"bufio"
	"fmt"
	"image"
	"os"
	"strings"

	ort "github.com/getcharzp/onnxruntime_purego"
)

var (
	recMean = [3]float32{0.5, 0.5, 0.5}
	recStd  = [3]float32{0.5, 0.5, 0.5}
)

const (
	recHeight = 48
	recMaxW   = 1280
	recMinW   = 32
)

func loadDict(path string) ([]string, error) {
	f, err := os.Open(path) //nolint:gosec // app-controlled cache path
	if err != nil {
		return nil, fmt.Errorf("open dict: %w", err)
	}
	defer func() { _ = f.Close() }()

	chars := []string{""}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		chars = append(chars, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	chars = append(chars, " ")
	return chars, nil
}

func (o *OCR) runRec(crop *image.NRGBA) (string, error) {
	srcW, srcH := crop.Bounds().Dx(), crop.Bounds().Dy()
	if srcW < 2 || srcH < 2 {
		return "", nil
	}

	ratio := float64(srcW) / float64(srcH)
	dstW := int(float64(recHeight) * ratio)
	if dstW < recMinW {
		dstW = recMinW
	}
	if dstW > recMaxW {
		dstW = recMaxW
	}
	resized := resizeBilinear(crop, dstW, recHeight)

	input := normalizeHWC2CHW(resized, recMean, recStd)

	inputTensor, err := ort.NewTensor([]int64{1, 3, int64(recHeight), int64(dstW)}, input)
	if err != nil {
		return "", fmt.Errorf("rec: create tensor: %w", err)
	}
	defer inputTensor.Destroy()

	if len(o.rec.InputNames) == 0 {
		return "", fmt.Errorf("rec: model has no input")
	}
	outputs, err := o.rec.Run(map[string]*ort.Value{o.rec.InputNames[0]: inputTensor})
	if err != nil {
		return "", fmt.Errorf("rec: run: %w", err)
	}
	for _, v := range outputs {
		defer v.Destroy()
	}

	var outVal *ort.Value
	for _, v := range outputs {
		outVal = v
		break
	}
	probs, err := ort.GetTensorData[float32](outVal)
	if err != nil {
		return "", fmt.Errorf("rec: read output: %w", err)
	}
	shape, err := outVal.GetShape()
	if err != nil {
		return "", fmt.Errorf("rec: output shape: %w", err)
	}
	if len(shape) != 3 {
		return "", fmt.Errorf("rec: unexpected output shape: %v", shape)
	}
	T := int(shape[1])
	C := int(shape[2])

	return ctcGreedyDecode(probs, T, C, o.charset), nil
}

func ctcGreedyDecode(probs []float32, T, C int, charset []string) string {
	if T == 0 || C == 0 {
		return ""
	}
	var b strings.Builder
	prev := -1
	for t := 0; t < T; t++ {
		best := 0
		maxP := probs[t*C]
		for c := 1; c < C; c++ {
			p := probs[t*C+c]
			if p > maxP {
				maxP = p
				best = c
			}
		}
		if best != 0 && best != prev && best < len(charset) {
			b.WriteString(charset[best])
		}
		prev = best
	}
	return b.String()
}
