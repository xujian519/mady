//go:build darwin

package main

import (
	"strings"
	"testing"
)

// TestDecodePastedImage 覆盖 data URL 解析与白名单。
func TestDecodePastedImage(t *testing.T) {
	// PNG 1x1 像素（base64）
	const tinyPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

	t.Run("合法 PNG", func(t *testing.T) {
		data, ext, err := decodePastedImage(tinyPNG)
		if err != nil {
			t.Fatalf("decodePastedImage: %v", err)
		}
		if ext != "png" {
			t.Fatalf("ext: want png, got %s", ext)
		}
		if len(data) == 0 {
			t.Fatal("data should not be empty")
		}
	})

	t.Run("非 image 前缀拒绝", func(t *testing.T) {
		if _, _, err := decodePastedImage("data:text/plain;base64,aGk="); err == nil {
			t.Fatal("want error for non-image data URL")
		}
	})

	t.Run("不支持的图片类型", func(t *testing.T) {
		if _, _, err := decodePastedImage("data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="); err == nil {
			t.Fatal("want error for svg (not in whitelist)")
		}
	})

	t.Run("缺少 base64 载荷", func(t *testing.T) {
		if _, _, err := decodePastedImage("data:image/png;base64"); err == nil {
			t.Fatal("want error for missing payload")
		}
	})

	t.Run("非法 base64", func(t *testing.T) {
		if _, _, err := decodePastedImage("data:image/png;base64,!!!not-base64!!!"); err == nil {
			t.Fatal("want error for invalid base64")
		}
	})
}

// TestDecodePastedImage_SizeLimit 覆盖大小上限。
func TestDecodePastedImage_SizeLimit(t *testing.T) {
	// 构造超过 10MB 明文的 base64：base64 长度需 > 10MB * 4/3 ≈ 13.33MB，
	// 用 14MB（→ 明文 ~10.5MB，超过 maxPastedImageSize）。
	big := strings.Repeat("A", 14<<20)
	dataURL := "data:image/png;base64," + big
	if _, _, err := decodePastedImage(dataURL); err == nil {
		t.Fatal("want error for oversized image")
	} else if !strings.Contains(err.Error(), "上限") {
		t.Fatalf("want size-limit error, got: %v", err)
	}
}
