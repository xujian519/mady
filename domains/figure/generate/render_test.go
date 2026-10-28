package generate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const simpleDOT = `digraph G {
    rankdir=TB;
    a [label="开始"];
    b [label="结束"];
    a -> b;
}`

func requireDot(t *testing.T) string {
	t.Helper()
	executable := FindDot("")
	if executable == "" {
		t.Skip("本机未安装 Graphviz，跳过真实渲染用例")
	}
	return executable
}

func TestFindDotRejectsMissingOverride(t *testing.T) {
	if got := FindDot(filepath.Join(t.TempDir(), "missing-dot")); got != "" {
		t.Errorf("显式覆盖不存在时应返回空，got %q", got)
	}
}

func TestProbe(t *testing.T) {
	executable := requireDot(t)
	result := Probe(context.Background(), executable)
	if !result.Ready {
		t.Fatalf("probe 未就绪: %+v", result)
	}
	if result.Version == "" {
		t.Errorf("未解析到版本号: %+v", result)
	}
}

func TestRenderFormats(t *testing.T) {
	requireDot(t)
	outDir := t.TempDir()
	cases := []struct {
		format Format
	}{
		{FormatSVG}, {FormatPNG}, {FormatPDF},
	}
	for _, tc := range cases {
		path, err := Render(context.Background(), RenderSpec{
			DOT: simpleDOT, Filename: "simple", Format: tc.format, Engine: EngineDot, OutputDir: outDir,
		}, RenderConfig{})
		if err != nil {
			t.Fatalf("%s: %v", tc.format, err)
		}
		if filepath.Ext(path) != "."+string(tc.format) {
			t.Errorf("%s 产物扩展名 = %s", tc.format, filepath.Ext(path))
		}
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			t.Fatalf("%s 产物缺失或为空: %v", tc.format, err)
		}
		if tc.format == FormatSVG {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			viewport := ResolveViewport(RootTag(string(data)))
			if viewport.ViewBox == nil {
				t.Errorf("SVG 缺少 viewBox:\n%s", string(data)[:200])
			}
			if width, height, ok := viewport.DiagramSizeMm(); !ok || width <= 0 || height <= 0 {
				t.Errorf("SVG 尺寸不可解析: %f×%f", width, height)
			}
		}
	}
}

func TestRenderRejectsBadDOT(t *testing.T) {
	requireDot(t)
	_, err := Render(context.Background(), RenderSpec{
		DOT: "digraph {", Filename: "bad", Format: FormatSVG, Engine: EngineDot, OutputDir: t.TempDir(),
	}, RenderConfig{})
	if err == nil {
		t.Fatal("语法错误的 DOT 应渲染失败")
	}
	if classified, ok := err.(*Error); !ok || classified.Code != ErrRenderFailed {
		t.Errorf("应报 render_failed, got %v", err)
	}
}

func TestRenderRejectsUnknownFormat(t *testing.T) {
	_, err := Render(context.Background(), RenderSpec{
		DOT: simpleDOT, Filename: "x", Format: Format("bmp"), Engine: EngineDot, OutputDir: t.TempDir(),
	}, RenderConfig{})
	if err == nil {
		t.Fatal("未知格式应报错")
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"fig1":  "fig1",
		"图1":    "图1",
		"a/b":   "a_b",
		`a\b`:   "a_b",
		"a:b*?": "a_b__",
		"   ":   "   ",
	}
	for input, want := range cases {
		if got := sanitizeFilename(input); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", input, got, want)
		}
	}
	if got := sanitizeFilename(""); got != "diagram" {
		t.Errorf("空名应回退 diagram, got %q", got)
	}
	if !strings.Contains(InstallHint(""), "brew install graphviz") {
		t.Error("安装指引应包含 brew 命令")
	}
}

// 显式配置的 dot 不存在时应报 not_installed 并给出安装指引，不回落自动探测。
func TestRenderNotInstalled(t *testing.T) {
	_, err := Render(context.Background(), RenderSpec{
		DOT: simpleDOT, Filename: "x", Format: FormatSVG, Engine: EngineDot, OutputDir: t.TempDir(),
	}, RenderConfig{Executable: filepath.Join(t.TempDir(), "missing-dot")})
	if err == nil {
		t.Fatal("expected not_installed error")
	}
	classified, ok := err.(*Error)
	if !ok || classified.Code != ErrNotInstalled {
		t.Fatalf("expected not_installed, got %v", err)
	}
	if !strings.Contains(err.Error(), "brew install graphviz") {
		t.Errorf("message 缺少安装指引: %v", err)
	}
}
