// Package ocr implements a local OCR engine using PP-OCRv5 models on ONNX Runtime.
package ocr

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	ort "github.com/getcharzp/onnxruntime_purego"
)

type OCR struct {
	mu       sync.Mutex
	cacheDir string

	engine  *ort.Engine
	det     *ort.Session
	rec     *ort.Session
	cls     *ort.Session
	charset []string

	loaded  bool
	loadErr error
}

var (
	global     *OCR
	globalOnce sync.Once
)

func DefaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "mady", "ocr")
	}
	return filepath.Join(home, ".mady", "ocr")
}

func Global() *OCR {
	globalOnce.Do(func() {
		global = New(DefaultCacheDir())
	})
	return global
}

func New(cacheDir string) *OCR {
	return &OCR{cacheDir: cacheDir}
}

func (o *OCR) IsReady() bool    { return isReady(o.cacheDir) }
func (o *OCR) CacheDir() string { return o.cacheDir }
func (o *OCR) EnsureAssets(onProgress ProgressFunc) error {
	return EnsureAssets(o.cacheDir, onProgress)
}

type Result struct {
	Text string
	Box  [4]int
}

func (o *OCR) Recognize(imagePath string) ([]Result, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.loaded {
		o.load()
	}
	if o.loadErr != nil {
		return nil, o.loadErr
	}

	img, err := loadImage(imagePath)
	if err != nil {
		return nil, fmt.Errorf("load image: %w", err)
	}

	quads, err := o.runDet(img)
	if err != nil {
		return nil, err
	}

	out := make([]Result, 0, len(quads))
	for _, q := range quads {
		crop := extractQuad(img, q)
		text, err := o.runRec(crop)
		if err != nil {
			text = ""
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, Result{
			Text: text,
			Box:  q.BoundingBox(),
		})
	}
	out = mergeLineResults(out)
	return out, nil
}

func (o *OCR) RecognizeText(imagePath string) (string, error) {
	results, err := o.Recognize(imagePath)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", nil
	}
	var lines []string
	var currentLine []string
	prevYCenter := -1
	for _, r := range results {
		y := (r.Box[1] + r.Box[3]) / 2
		if prevYCenter < 0 || abs(y-prevYCenter) < (r.Box[3]-r.Box[1])/2+1 {
			currentLine = append(currentLine, r.Text)
		} else {
			lines = append(lines, strings.Join(currentLine, " "))
			currentLine = []string{r.Text}
		}
		prevYCenter = y
	}
	if len(currentLine) > 0 {
		lines = append(lines, strings.Join(currentLine, " "))
	}
	return strings.Join(lines, "\n"), nil
}

func (o *OCR) load() {
	o.loaded = true

	if !o.IsReady() {
		o.loadErr = fmt.Errorf("OCR assets not ready, call EnsureAssets() first: %s", o.cacheDir)
		return
	}

	libPath := filepath.Join(o.cacheDir, ortLibName)
	engine, err := ort.NewEngine(libPath)
	if err != nil {
		o.loadErr = fmt.Errorf("init onnxruntime (%s): %w", libPath, err)
		return
	}
	o.engine = engine

	opts, err := engine.NewSessionOptions()
	if err != nil {
		o.loadErr = fmt.Errorf("session options: %w", err)
		return
	}
	defer opts.Destroy()
	threads := int32(runtime.NumCPU() / 2)
	if threads < 1 {
		threads = 1
	}
	_ = opts.SetIntraOpNumThreads(threads)
	_ = opts.SetCpuMemArena(true)

	detSess, err := engine.NewSession(filepath.Join(o.cacheDir, detModelFile), opts)
	if err != nil {
		o.loadErr = fmt.Errorf("load det model: %w", err)
		return
	}
	o.det = detSess

	recSess, err := engine.NewSession(filepath.Join(o.cacheDir, recModelFile), opts)
	if err != nil {
		o.loadErr = fmt.Errorf("load rec model: %w", err)
		return
	}
	o.rec = recSess

	clsPath := filepath.Join(o.cacheDir, clsModelFile)
	if fileExists(clsPath) {
		clsSess, err := engine.NewSession(clsPath, opts)
		if err == nil {
			o.cls = clsSess
		}
	}

	charset, err := loadDict(filepath.Join(o.cacheDir, dictFile))
	if err != nil {
		o.loadErr = fmt.Errorf("load dict: %w", err)
		return
	}
	o.charset = charset
}

func (o *OCR) Close() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cls != nil {
		o.cls.Destroy()
		o.cls = nil
	}
	if o.rec != nil {
		o.rec.Destroy()
		o.rec = nil
	}
	if o.det != nil {
		o.det.Destroy()
		o.det = nil
	}
	if o.engine != nil {
		o.engine.Destroy()
		o.engine = nil
	}
	o.loaded = false
	o.loadErr = nil
}

type Quad struct {
	P [4]image.Point
}

func (q Quad) BoundingBox() [4]int {
	minX, minY := q.P[0].X, q.P[0].Y
	maxX, maxY := q.P[0].X, q.P[0].Y
	for i := 1; i < 4; i++ {
		if q.P[i].X < minX {
			minX = q.P[i].X
		}
		if q.P[i].X > maxX {
			maxX = q.P[i].X
		}
		if q.P[i].Y < minY {
			minY = q.P[i].Y
		}
		if q.P[i].Y > maxY {
			maxY = q.P[i].Y
		}
	}
	return [4]int{minX, minY, maxX, maxY}
}
