package component

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func makeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImageFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "img.png")
	if err := os.WriteFile(path, makeTestPNG(t, 8, 8), 0o644); err != nil {
		t.Fatal(err)
	}
	im, err := NewImageFromFile(path)
	if err != nil {
		t.Fatalf("expected load success: %v", err)
	}
	if im.Protocol() != ImageProtocolAuto {
		t.Fatalf("expected auto protocol, got %v", im.Protocol())
	}
	// Missing file.
	if _, err := NewImageFromFile(filepath.Join(dir, "nope.png")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestImageFromBytesInvalid(t *testing.T) {
	if _, err := NewImageFromBytes([]byte("not an image")); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestImageRenderProtocols(t *testing.T) {
	im, err := NewImageFromBytes(makeTestPNG(t, 8, 8))
	if err != nil {
		t.Fatal(err)
	}
	im.SetMaxSize(10, 10)

	// ASCII protocol renders textual lines.
	im.SetProtocol(ImageProtocolASCII)
	lines := im.Render(20)
	if len(lines) == 0 {
		t.Fatal("expected ascii render")
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 20 {
			t.Fatalf("ascii line width %d > 20", w)
		}
	}

	// Cache hit at same width.
	again := im.Render(20)
	if len(again) != len(lines) {
		t.Fatalf("expected cached render, got %d vs %d lines", len(again), len(lines))
	}

	// Half-block default.
	im.SetProtocol(ImageProtocolAuto)
	im.Invalidate()
	if lines := im.Render(20); len(lines) == 0 {
		t.Fatal("expected half-block render")
	}

	// Kitty protocol (may be unavailable in test env — must not panic).
	im.SetProtocol(ImageProtocolKitty)
	im.Render(20)
	im.SetProtocol(ImageProtocolITerm2)
	im.Render(20)
	im.Update(core.KeyMsg{Data: "x"}) // no-op
}

func TestImageNilGuard(t *testing.T) {
	im := &Image{}
	if lines := im.Render(20); lines != nil {
		t.Fatalf("expected nil render for nil image, got %v", lines)
	}
}
