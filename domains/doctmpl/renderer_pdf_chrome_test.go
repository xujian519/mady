package doctmpl

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestPDFChromeRenderer_Format(t *testing.T) {
	r := PDFChromeRenderer{}
	if got := r.Format(); got != FormatPDF {
		t.Fatalf("Format() = %q, want %q", got, FormatPDF)
	}
}

func TestPDFChromeRenderer_NilReceiver(t *testing.T) {
	var r *PDFChromeRenderer
	_, err := r.Render("# Test", RenderMeta{})
	if err == nil {
		t.Fatal("nil receiver should return error")
	}
}

func TestPDFAutoRenderer_Format(t *testing.T) {
	r := NewPDFAutoRenderer()
	if got := r.Format(); got != FormatPDF {
		t.Fatalf("Format() = %q, want %q", got, FormatPDF)
	}
}

func TestPDFAutoRenderer_NilReceiver(t *testing.T) {
	var r *PDFAutoRenderer
	_, err := r.Render("# Test", RenderMeta{})
	if err == nil {
		t.Fatal("nil receiver should return error")
	}
}

// TestPDFAutoRenderer_Render 验证自动降级渲染器生成有效 PDF。
// Chrome 可用时走 Chrome 路径；不可用时走 gopdf（需要中文字体）。
// 两者都不可用时 skip。
func TestPDFAutoRenderer_Render(t *testing.T) {
	r := NewPDFAutoRenderer()
	pdf, err := r.Render("# 测试标题\n\n这是一段测试文本。", RenderMeta{Title: "测试"})
	if err != nil {
		t.Skipf("PDF rendering unavailable in this environment: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("rendered PDF is empty")
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("output does not start with %%PDF header, got: %q", pdf[:min(8, len(pdf))])
	}
}

// TestPDFAutoRenderer_ProbeOnce 验证 Chrome 探测只执行一次。
func TestPDFAutoRenderer_ProbeOnce(t *testing.T) {
	r := NewPDFAutoRenderer()
	// 连续两次 Render，once 确保探测仅一次。
	_, _ = r.Render("# A", RenderMeta{})
	first := r.useChrome
	_, _ = r.Render("# B", RenderMeta{})
	if r.useChrome != first {
		t.Fatalf("useChrome changed after first render: %v → %v", first, r.useChrome)
	}
}

func TestProbeChrome(t *testing.T) {
	// 直接测试探测函数——在无 Chrome 的 CI 上应返回 false。
	r := &PDFChromeRenderer{Timeout: 5}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		done <- probeChrome(r)
	}()

	select {
	case result := <-done:
		// 探测完成——无论是 true 还是 false 都不应阻塞。
		_ = result
	case <-ctx.Done():
		t.Fatal("probeChrome timed out")
	}
}
