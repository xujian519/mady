package ocr

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// abs
// ---------------------------------------------------------------------------

func TestAbs(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 0},
		{5, 5},
		{-5, 5},
		{-1, 1},
		{100, 100},
	}
	for _, tt := range tests {
		got := abs(tt.input)
		if got != tt.want {
			t.Errorf("abs(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// absFloat
// ---------------------------------------------------------------------------

func TestAbsFloat(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{0, 0},
		{3.14, 3.14},
		{-3.14, 3.14},
		{-0.001, 0.001},
	}
	for _, tt := range tests {
		got := absFloat(tt.input)
		if got != tt.want {
			t.Errorf("absFloat(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Quad
// ---------------------------------------------------------------------------

func TestQuad_BoundingBox(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 10, Y: 20},
			{X: 100, Y: 20},
			{X: 100, Y: 80},
			{X: 10, Y: 80},
		},
	}
	bb := q.BoundingBox()
	want := [4]int{10, 20, 100, 80}
	if bb != want {
		t.Errorf("BoundingBox() = %v, want %v", bb, want)
	}
}

func TestQuad_BoundingBox_NonAxisAligned(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 15, Y: 10},
			{X: 110, Y: 25},
			{X: 100, Y: 90},
			{X: 5, Y: 75},
		},
	}
	bb := q.BoundingBox()
	want := [4]int{5, 10, 110, 90}
	if bb != want {
		t.Errorf("BoundingBox() = %v, want %v", bb, want)
	}
}

func TestQuad_Center_Square(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 0, Y: 0},
			{X: 100, Y: 0},
			{X: 100, Y: 100},
			{X: 0, Y: 100},
		},
	}
	c := q.Center()
	want := image.Point{X: 50, Y: 50}
	if c != want {
		t.Errorf("Center() = %v, want %v", c, want)
	}
}

func TestQuad_Center_NonCentered(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 10, Y: 20},
			{X: 30, Y: 20},
			{X: 30, Y: 40},
			{X: 10, Y: 40},
		},
	}
	c := q.Center()
	want := image.Point{X: 20, Y: 30}
	if c != want {
		t.Errorf("Center() = %v, want %v", c, want)
	}
}

func TestQuad_Height_ReturnsMaxEdge(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 0, Y: 0},
			{X: 100, Y: 0},
			{X: 100, Y: 50},
			{X: 0, Y: 50},
		},
	}
	h := q.Height()
	// Height returns max(dist(P[0],P[1]), dist(P[2],P[3])) which
	// is the longer horizontal edge (width-like), not vertical span.
	if h != 100 {
		t.Errorf("Height() = %d, want 100 (max edge length)", h)
	}
}

// ---------------------------------------------------------------------------
// dist
// ---------------------------------------------------------------------------

func TestDist(t *testing.T) {
	a, b := image.Point{X: 0, Y: 0}, image.Point{X: 3, Y: 4}
	d := dist(a, b)
	if d != 5 {
		t.Errorf("dist((0,0)-(3,4)) = %d, want 5", d)
	}
}

func TestDist_Zero(t *testing.T) {
	p := image.Point{X: 10, Y: 20}
	d := dist(p, p)
	if d != 0 {
		t.Errorf("dist(self) = %d, want 0", d)
	}
}

// ---------------------------------------------------------------------------
// polygonArea
// ---------------------------------------------------------------------------

func TestPolygonArea_Square(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 0, Y: 0},
			{X: 10, Y: 0},
			{X: 10, Y: 10},
			{X: 0, Y: 10},
		},
	}
	area := polygonArea(q)
	if area != 100 {
		t.Errorf("polygonArea(square) = %v, want 100", area)
	}
}

func TestPolygonArea_Rectangle(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 0, Y: 0},
			{X: 20, Y: 0},
			{X: 20, Y: 5},
			{X: 0, Y: 5},
		},
	}
	area := polygonArea(q)
	if area != 100 {
		t.Errorf("polygonArea(20x5) = %v, want 100", area)
	}
}

func TestPolygonArea_Degenerate(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 0, Y: 0},
			{X: 0, Y: 0},
			{X: 0, Y: 0},
			{X: 0, Y: 0},
		},
	}
	area := polygonArea(q)
	if area != 0 {
		t.Errorf("polygonArea(degenerate) = %v, want 0", area)
	}
}

// ---------------------------------------------------------------------------
// polygonPerimeter
// ---------------------------------------------------------------------------

func TestPolygonPerimeter_Square(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 0, Y: 0},
			{X: 10, Y: 0},
			{X: 10, Y: 10},
			{X: 0, Y: 10},
		},
	}
	peri := polygonPerimeter(q)
	if math.Abs(peri-40) > 1e-6 {
		t.Errorf("polygonPerimeter(square) = %v, want 40", peri)
	}
}

func TestPolygonPerimeter_Unit(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 0, Y: 0},
			{X: 1, Y: 0},
			{X: 1, Y: 1},
			{X: 0, Y: 1},
		},
	}
	peri := polygonPerimeter(q)
	if math.Abs(peri-4) > 1e-6 {
		t.Errorf("polygonPerimeter(unit) = %v, want 4", peri)
	}
}

// ---------------------------------------------------------------------------
// offsetPolygon
// ---------------------------------------------------------------------------

func TestOffsetPolygon_NonDegenerate(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 0, Y: 0},
			{X: 500, Y: 0},
			{X: 500, Y: 200},
			{X: 0, Y: 200},
		},
	}
	offset := offsetPolygon(q, 2)
	// Offset polygon should still be a valid quad with positive area.
	offsetArea := polygonArea(offset)
	if offsetArea <= 0 {
		t.Errorf("offset area should be positive, got %v", offsetArea)
	}
	// All vertices should be within reasonable bounds
	for _, p := range offset.P {
		if p.X < -10 || p.X > 510 || p.Y < -10 || p.Y > 210 {
			t.Errorf("offset vertex out of bounds: %v", p)
		}
	}
}

// ---------------------------------------------------------------------------
// unclipQuad
// ---------------------------------------------------------------------------

func TestUnclipQuad_NonDegenerate(t *testing.T) {
	q := Quad{
		P: [4]image.Point{
			{X: 0, Y: 0},
			{X: 500, Y: 0},
			{X: 500, Y: 200},
			{X: 0, Y: 200},
		},
	}
	u := unclipQuad(q, 0)
	area := polygonArea(u)
	if area <= 0 {
		t.Errorf("unclip quad area should be positive, got %v", area)
	}
	// The default unclip ratio=0 yields dist=0.5, which should produce
	// a valid quad slightly offset from the original.
	for _, p := range u.P {
		if p.X < -10 || p.X > 510 || p.Y < -10 || p.Y > 210 {
			t.Errorf("vertex out of bounds: %v", p)
		}
	}
}

// ---------------------------------------------------------------------------
// roundUpTo32
// ---------------------------------------------------------------------------

func TestRoundUpTo32(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 0},
		{1, 32},
		{32, 32},
		{33, 64},
		{64, 64},
		{100, 128},
		{31, 32},
	}
	for _, tt := range tests {
		got := roundUpTo32(tt.input)
		if got != tt.want {
			t.Errorf("roundUpTo32(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// segmentationToQuads
// ---------------------------------------------------------------------------

func TestSegmentationToQuads_EmptyProb(t *testing.T) {
	quads := segmentationToQuads(nil, 10, 10, 0.5)
	if len(quads) != 0 {
		t.Errorf("expected 0 quads for nil prob, got %d", len(quads))
	}
}

func TestSegmentationToQuads_NoAboveThreshold(t *testing.T) {
	prob := make([]float32, 100)
	quads := segmentationToQuads(prob, 10, 10, 0.5)
	if len(quads) != 0 {
		t.Errorf("expected 0 quads when all below threshold, got %d", len(quads))
	}
}

func TestSegmentationToQuads_AllAboveThreshold(t *testing.T) {
	prob := make([]float32, 100)
	for i := range prob {
		prob[i] = 0.9
	}
	quads := segmentationToQuads(prob, 10, 10, 0.5)
	if len(quads) == 0 {
		t.Fatal("expected at least one quad from full activation")
	}
}

// ---------------------------------------------------------------------------
// mergeLineResults
// ---------------------------------------------------------------------------

func TestMergeLineResults_Nil(t *testing.T) {
	merged := mergeLineResults(nil)
	if merged != nil {
		t.Errorf("expected nil for nil input, got %v", merged)
	}
}

func TestMergeLineResults_Single(t *testing.T) {
	r := []Result{{Text: "hello", Box: [4]int{0, 0, 50, 20}}}
	merged := mergeLineResults(r)
	if len(merged) != 1 {
		t.Errorf("expected 1 result for single input, got %d", len(merged))
	}
}

func TestMergeLineResults_SameLine(t *testing.T) {
	results := []Result{
		{Text: "Hello", Box: [4]int{0, 10, 50, 30}},
		{Text: "World", Box: [4]int{60, 10, 110, 30}},
	}
	merged := mergeLineResults(results)
	if len(merged) != 2 {
		t.Fatalf("expected 2 results, got %d", len(merged))
	}
	// Same y-center should keep original order
	if merged[0].Text != "Hello" || merged[1].Text != "World" {
		t.Errorf("unexpected order after merge: %v, %v", merged[0].Text, merged[1].Text)
	}
}

func TestMergeLineResults_DifferentLines(t *testing.T) {
	results := []Result{
		{Text: "World", Box: [4]int{60, 100, 110, 120}},
		{Text: "Hello", Box: [4]int{0, 10, 50, 30}},
	}
	merged := mergeLineResults(results)
	if len(merged) != 2 {
		t.Fatalf("expected 2 results, got %d", len(merged))
	}
	// Should be sorted by y-center (top to bottom)
	if merged[0].Text != "Hello" || merged[1].Text != "World" {
		t.Errorf("expected top-to-bottom order, got: %s, %s", merged[0].Text, merged[1].Text)
	}
}

func TestMergeLineResults_MergeItems(t *testing.T) {
	results := []Result{
		{Text: "Hello", Box: [4]int{0, 10, 50, 30}},
		{Text: "World", Box: [4]int{55, 12, 110, 32}},
		{Text: "Footer", Box: [4]int{0, 100, 60, 120}},
	}
	merged := mergeLineResults(results)
	// "Hello" and "World" are on the same line (y-center within maxH*3/4)
	if len(merged) != 3 {
		t.Fatalf("expected 3 results, got %d", len(merged))
	}
}

// ---------------------------------------------------------------------------
// Result.Box ordering
// ---------------------------------------------------------------------------

func TestResult_BoxProperties(t *testing.T) {
	r := Result{Text: "test", Box: [4]int{10, 20, 50, 80}}
	if r.Box[0] != 10 || r.Box[1] != 20 || r.Box[2] != 50 || r.Box[3] != 80 {
		t.Errorf("unexpected box values: %v", r.Box)
	}
}

// ---------------------------------------------------------------------------
// toNRGBA
// ---------------------------------------------------------------------------

func TestToNRGBA_AlreadyNRGBA(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	out := toNRGBA(src)
	if out != src {
		t.Error("toNRGBA should return same pointer for NRGBA input")
	}
}

func TestToNRGBA_RGBAToNRGBA(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 5, 5))
	out := toNRGBA(src)
	if out == nil {
		t.Fatal("toNRGBA returned nil")
	}
	b := out.Bounds()
	if b.Dx() != 5 || b.Dy() != 5 {
		t.Errorf("expected 5x5, got %dx%d", b.Dx(), b.Dy())
	}
}

// ---------------------------------------------------------------------------
// cropRect
// ---------------------------------------------------------------------------

func TestCropRect(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	cropped := cropRect(src, image.Rect(10, 20, 50, 60))
	b := cropped.Bounds()
	if b.Dx() != 40 || b.Dy() != 40 {
		t.Errorf("expected 40x40 crop, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestCropRect_OutOfBounds(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 50, 50))
	cropped := cropRect(src, image.Rect(-10, -10, 100, 100))
	if cropped == nil {
		t.Fatal("cropRect returned nil for out-of-bounds crop")
	}
}

func TestCropRect_Empty(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	cropped := cropRect(src, image.Rect(200, 200, 300, 300))
	// Should return a minimal 1x1 image instead of empty
	b := cropped.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 {
		t.Errorf("expected at least 1x1 for empty crop, got %dx%d", b.Dx(), b.Dy())
	}
}

// ---------------------------------------------------------------------------
// normalizeHWC2CHW
// ---------------------------------------------------------------------------

func TestNormalizeHWC2CHW(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	// Fill with a known color
	img.SetNRGBA(0, 0, color.NRGBA{R: 128, G: 64, B: 32, A: 255})

	data := normalizeHWC2CHW(img, [3]float32{0.5, 0.5, 0.5}, [3]float32{0.5, 0.5, 0.5})
	if len(data) != 3*2*2 {
		t.Errorf("expected 12 elements, got %d", len(data))
	}
	// Pixel (0,0) R channel: (128/255 - 0.5) / 0.5 = (0.502 - 0.5) / 0.5 ≈ 0.004
	r00 := data[0]
	if r00 < 0.003 || r00 > 0.005 {
		t.Errorf("expected R(0,0) ≈ 0.004, got %v", r00)
	}
}

// ---------------------------------------------------------------------------
// resizeBilinear
// ---------------------------------------------------------------------------

func TestResizeBilinear_SameSize(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 10, 20))
	out := resizeBilinear(src, 10, 20)
	if out != src {
		t.Error("resizeBilinear should return same image when dimensions unchanged")
	}
}

func TestResizeBilinear_DifferentSize(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	out := resizeBilinear(src, 20, 20)
	if out == nil {
		t.Fatal("resizeBilinear returned nil")
	}
	b := out.Bounds()
	if b.Dx() != 20 || b.Dy() != 20 {
		t.Errorf("expected 20x20, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestResizeBilinear_Smaller(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	out := resizeBilinear(src, 32, 32)
	b := out.Bounds()
	if b.Dx() != 32 || b.Dy() != 32 {
		t.Errorf("expected 32x32, got %dx%d", b.Dx(), b.Dy())
	}
}

// ---------------------------------------------------------------------------
// fileExists
// ---------------------------------------------------------------------------

func TestFileExists_NotExists(t *testing.T) {
	if fileExists("/nonexistent/path/that/should/not/exist") {
		t.Error("fileExists should return false for nonexistent path")
	}
}

func TestFileExists_Exists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if !fileExists(path) {
		t.Error("fileExists should return true for existing file")
	}
}

func TestFileExists_DirectoryExists(t *testing.T) {
	tmp := t.TempDir()
	// fileExists checks os.Stat which succeeds for directories too
	if !fileExists(tmp) {
		t.Error("fileExists should return true for existing directory (os.Stat succeeds)")
	}
}

// ---------------------------------------------------------------------------
// isReady
// ---------------------------------------------------------------------------

func TestIsReady_NotReady(t *testing.T) {
	if isReady("/nonexistent/cache") {
		t.Error("isReady should return false for nonexistent cache dir")
	}
}

func TestIsReady_Ready(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, readyMarker)
	if err := os.WriteFile(marker, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if !isReady(tmp) {
		t.Error("isReady should return true when marker exists")
	}
}

// ---------------------------------------------------------------------------
// mirrorCandidates / isGitHubURL / toJsDelivr
// ---------------------------------------------------------------------------

func TestIsGitHubURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://github.com/user/repo/releases/download/v1/model.onnx", true},
		{"https://raw.githubusercontent.com/user/repo/main/file.txt", true},
		{"https://objects.githubusercontent.com/something", true},
		{"https://example.com/something", false},
		{"https://cdn.jsdelivr.net/gh/user/repo@ref/file", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isGitHubURL(tt.url)
		if got != tt.want {
			t.Errorf("isGitHubURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestToJsDelivr_RawGitHubURL(t *testing.T) {
	url := "https://raw.githubusercontent.com/PaddlePaddle/PaddleOCR/main/ppocr/utils/dict/ppocrv5_dict.txt"
	jsd := toJsDelivr(url)
	want := "https://cdn.jsdelivr.net/gh/PaddlePaddle/PaddleOCR@main/ppocr/utils/dict/ppocrv5_dict.txt"
	if jsd != want {
		t.Errorf("toJsDelivr(%q) = %q, want %q", url, jsd, want)
	}
}

func TestToJsDelivr_NonRawURL(t *testing.T) {
	url := "https://github.com/user/repo/releases/download/v1/model.onnx"
	jsd := toJsDelivr(url)
	if jsd != "" {
		t.Errorf("expected empty for non-raw URL, got %q", jsd)
	}
}

func TestToJsDelivr_TooFewParts(t *testing.T) {
	url := "https://raw.githubusercontent.com/user/repo"
	jsd := toJsDelivr(url)
	if jsd != "" {
		t.Errorf("expected empty for URL with insufficient parts, got %q", jsd)
	}
}

func TestMirrorCandidates_BaseURL(t *testing.T) {
	url := "https://github.com/user/repo/releases/download/v1/model.onnx"
	candidates := mirrorCandidates(url)
	if len(candidates) == 0 {
		t.Fatal("expected at least one candidate")
	}
	// Last candidate should be the original URL
	if candidates[len(candidates)-1] != url {
		t.Errorf("last candidate should be original URL, got %q", candidates[len(candidates)-1])
	}
}

func TestMirrorCandidates_NonGitHub(t *testing.T) {
	url := "https://example.com/something"
	candidates := mirrorCandidates(url)
	if len(candidates) != 1 {
		t.Errorf("expected only 1 candidate for non-GitHub URL, got %d", len(candidates))
	}
	if candidates[0] != url {
		t.Errorf("expected original URL, got %q", candidates[0])
	}
}

func TestMirrorCandidates_Deduplicate(t *testing.T) {
	// Two different proxies may produce the same URL — dedup should handle it
	url := "https://github.com/user/repo"
	candidates := mirrorCandidates(url)
	seen := make(map[string]bool)
	for _, c := range candidates {
		if seen[c] {
			t.Errorf("duplicate URL found: %s", c)
		}
		seen[c] = true
	}
}

func TestMirrorCandidates_DisableEnv(t *testing.T) {
	t.Setenv("MADY_DISABLE_GH_MIRROR", "1")
	url := "https://github.com/user/repo"
	candidates := mirrorCandidates(url)
	if len(candidates) != 1 || candidates[0] != url {
		t.Errorf("expected only original URL when disabled, got %v", candidates)
	}
}

func TestMirrorCandidates_WithProxy(t *testing.T) {
	t.Setenv("MADY_GH_PROXY", "https://my-proxy.example.com")
	url := "https://github.com/user/repo"
	candidates := mirrorCandidates(url)
	if len(candidates) < 2 {
		t.Fatalf("expected multiple candidates, got %d", len(candidates))
	}
	if candidates[0] != "https://my-proxy.example.com/"+url {
		t.Errorf("first candidate should be custom proxy, got %q", candidates[0])
	}
}

// ---------------------------------------------------------------------------
// DefaultCacheDir
// ---------------------------------------------------------------------------

func TestDefaultCacheDir(t *testing.T) {
	dir := DefaultCacheDir()
	if dir == "" {
		t.Error("DefaultCacheDir should not be empty")
	}
}

// ---------------------------------------------------------------------------
// OCR constructor and simple methods
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	o := New("/tmp/test-ocr-cache")
	if o == nil {
		t.Fatal("New returned nil")
	}
	if o.CacheDir() != "/tmp/test-ocr-cache" {
		t.Errorf("expected cache dir '/tmp/test-ocr-cache', got %q", o.CacheDir())
	}
	if o.IsReady() {
		t.Error("New OCR should not be ready without EnsureAssets")
	}
}

func TestGlobal(t *testing.T) {
	o1 := Global()
	o2 := Global()
	if o1 != o2 {
		t.Error("Global() should return the same instance")
	}
}

// ---------------------------------------------------------------------------
// fitQuad (geometric fitting)
// ---------------------------------------------------------------------------

func TestFitQuad_DensePoints(t *testing.T) {
	// Points approximating a rectangle near (10,10)-(50,10)-(50,30)-(10,30)
	var pts []pxPoint
	for y := 10; y <= 30; y++ {
		for x := 10; x <= 50; x++ {
			pts = append(pts, pxPoint{X: x, Y: y})
		}
	}
	q := fitQuad(pts)
	// The fitted quad should be a valid quad with positive area
	area := polygonArea(q)
	if area <= 0 {
		t.Errorf("fitted quad should have positive area, got %v", area)
	}
}

func TestFitQuad_MinPoints(t *testing.T) {
	// Minimum 3 points
	pts := []pxPoint{
		{X: 0, Y: 0},
		{X: 10, Y: 0},
		{X: 10, Y: 10},
	}
	q := fitQuad(pts)
	if q.P[0].X+q.P[0].Y+q.P[1].X+q.P[1].Y+q.P[2].X+q.P[2].Y+q.P[3].X+q.P[3].Y == 0 {
		t.Error("fitQuad should produce non-zero result for 3+ points")
	}
}

// ---------------------------------------------------------------------------
// loadImage error path (no actual file)
// ---------------------------------------------------------------------------

func TestLoadImage_NotFound(t *testing.T) {
	_, err := loadImage("/nonexistent/image.png")
	if err == nil {
		t.Error("expected error for nonexistent image")
	}
}
