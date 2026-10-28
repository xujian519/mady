package generate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleDrawing = `<svg xmlns="http://www.w3.org/2000/svg" width="100mm" height="50mm" viewBox="0 0 100 50">
  <rect x="10" y="10" width="30" height="20" fill="none" stroke="#000000"/>
  <text x="15" y="25" font-family="sans-serif" font-size="3.5" fill="#000000">1</text>
</svg>`

func TestBuildSubmissionPageSingleFigure(t *testing.T) {
	page, err := BuildSubmissionPage(SubmissionInput{
		DrawingSVG: sampleDrawing, Office: OfficeCNIPA, FigureNumber: 1,
	})
	if err != nil {
		t.Fatalf("BuildSubmissionPage: %v", err)
	}
	if page.Caption != "" {
		t.Errorf("单幅附图不应编号，got %q", page.Caption)
	}
	if page.SheetNumber != "" {
		t.Errorf("未给页码参数时不应标页码，got %q", page.SheetNumber)
	}
	if !almost(page.Metrics.PageScale, 1.7) {
		t.Errorf("pageScale = %v, want 1.7", page.Metrics.PageScale)
	}
	if !almost(page.Metrics.PlacedWidthMm, 170) || !almost(page.Metrics.PlacedHeightMm, 85) {
		t.Errorf("placed = %v x %v, want 170 x 85", page.Metrics.PlacedWidthMm, page.Metrics.PlacedHeightMm)
	}
	for _, want := range []string{`width="210mm"`, `height="297mm"`, `viewBox="0 0 210 297"`} {
		if !strings.Contains(page.SVG, want) {
			t.Errorf("落版页缺少 %q", want)
		}
	}
}

func TestBuildSubmissionPagePCTMultiFigure(t *testing.T) {
	page, err := BuildSubmissionPage(SubmissionInput{
		DrawingSVG: sampleDrawing, Office: OfficePCT,
		FigureNumber: 2, FigureCount: 3, SheetIndex: 1, SheetTotal: 2,
	})
	if err != nil {
		t.Fatalf("BuildSubmissionPage: %v", err)
	}
	if page.Caption != "Fig. 2" {
		t.Errorf("caption = %q, want Fig. 2", page.Caption)
	}
	if page.SheetNumber != "1/2" {
		t.Errorf("sheet = %q, want 1/2", page.SheetNumber)
	}
	if !strings.Contains(page.SVG, ">Fig. 2<") || !strings.Contains(page.SVG, ">1/2<") {
		t.Errorf("落版页缺少图号或页码文本")
	}
}

func TestBuildSubmissionPageCaptionOverride(t *testing.T) {
	page, err := BuildSubmissionPage(SubmissionInput{
		DrawingSVG: sampleDrawing, Office: OfficeCNIPA, Caption: "图 2（俯视）",
	})
	if err != nil {
		t.Fatalf("BuildSubmissionPage: %v", err)
	}
	if page.Caption != "图 2（俯视）" {
		t.Errorf("caption = %q", page.Caption)
	}
	if !strings.Contains(page.SVG, ">图 2（俯视）<") {
		t.Error("自定义图号未写入页面")
	}
}

func TestBuildSubmissionPageRotation(t *testing.T) {
	page, err := BuildSubmissionPage(SubmissionInput{
		DrawingSVG: sampleDrawing, Office: OfficePCT,
		FigureNumber: 1, FigureCount: 2, RotateDeg: 90,
	})
	if err != nil {
		t.Fatalf("BuildSubmissionPage: %v", err)
	}
	if !almost(page.Metrics.PageScale, 2.55) {
		t.Errorf("pageScale = %v, want 2.55", page.Metrics.PageScale)
	}
	if !almost(page.Metrics.PlacedWidthMm, 127.5) || !almost(page.Metrics.PlacedHeightMm, 255) {
		t.Errorf("placed = %v x %v, want 127.5 x 255", page.Metrics.PlacedWidthMm, page.Metrics.PlacedHeightMm)
	}
	if !strings.Contains(page.SVG, `rotate(90)`) {
		t.Error("页面缺少旋转变换")
	}
	if _, err := BuildSubmissionPage(SubmissionInput{
		DrawingSVG: sampleDrawing, Office: OfficePCT, RotateDeg: 45,
	}); errorCode(t, err) != ErrInvalidPage {
		t.Fatalf("rotate 45: %v", err)
	}
}

func TestBuildSubmissionPageClampsScale(t *testing.T) {
	small := `<svg xmlns="http://www.w3.org/2000/svg" width="10mm" height="5mm" viewBox="0 0 10 5">
  <rect x="1" y="1" width="4" height="2" fill="none" stroke="#000000"/>
</svg>`
	page, err := BuildSubmissionPage(SubmissionInput{DrawingSVG: small, Office: OfficeCNIPA})
	if err != nil {
		t.Fatalf("BuildSubmissionPage: %v", err)
	}
	if !almost(page.Metrics.PageScale, maxPageScale) {
		t.Errorf("pageScale = %v, want clamp at %v", page.Metrics.PageScale, maxPageScale)
	}
	if len(page.Warnings) == 0 || !strings.Contains(page.Warnings[0], "放大被限制") {
		t.Errorf("warnings = %v, want 放大被限制", page.Warnings)
	}
}

func TestBuildSubmissionPageCharHeight(t *testing.T) {
	page, err := BuildSubmissionPage(SubmissionInput{
		DrawingSVG: sampleDrawing, Office: OfficeCNIPA, BodyFontSize: 3.5,
	})
	if err != nil {
		t.Fatalf("BuildSubmissionPage: %v", err)
	}
	want := 3.5 * 1 * charHeightRatio * 1.7
	if !almost(page.Metrics.CharHeightMm, want) {
		t.Errorf("charHeight = %v, want %v", page.Metrics.CharHeightMm, want)
	}
	if !almost(page.Metrics.ReducedCharHeightMm, want*2.0/3.0) {
		t.Errorf("reduced = %v, want %v", page.Metrics.ReducedCharHeightMm, want*2.0/3.0)
	}
}

func TestBuildSubmissionPageErrors(t *testing.T) {
	cases := []struct {
		name string
		in   SubmissionInput
		want ErrorCode
	}{
		{"unknown office", SubmissionInput{DrawingSVG: sampleDrawing, Office: "epo"}, ErrInvalidPage},
		{"not svg", SubmissionInput{DrawingSVG: "hello", Office: OfficeCNIPA}, ErrInvalidSVG},
		{"no viewport", SubmissionInput{
			DrawingSVG: `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`,
			Office:     OfficeCNIPA,
		}, ErrInvalidSVG},
		{"sheet index over total", SubmissionInput{
			DrawingSVG: sampleDrawing, Office: OfficeCNIPA, SheetIndex: 3, SheetTotal: 2,
		}, ErrInvalidPage},
	}
	for _, tc := range cases {
		_, err := BuildSubmissionPage(tc.in)
		if got := errorCode(t, err); got != tc.want {
			t.Errorf("%s: code = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSubmissionToolWritesPage(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "fig.svg"), []byte(sampleDrawing), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewSubmissionTool(ToolConfig{WorkingDir: workDir})
	raw, err := tool.Func(context.Background(), json.RawMessage(`{
		"drawing": "fig.svg", "output": "pages/page1.svg", "office": "pct",
		"figure_number": 2, "figure_count": 3, "sheet_index": 1, "sheet_total": 2
	}`))
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	result, ok := raw.(SubmissionToolResult)
	if !ok {
		t.Fatalf("result type = %T", raw)
	}
	if result.Caption != "Fig. 2" || result.SheetNumber != "1/2" {
		t.Errorf("caption/sheet = %q/%q", result.Caption, result.SheetNumber)
	}
	if result.File != filepath.Join("pages", "page1.svg") {
		t.Errorf("file = %q", result.File)
	}
	written, err := os.ReadFile(filepath.Join(workDir, "pages", "page1.svg")) //nolint:gosec // 测试路径
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(written), ">Fig. 2<") {
		t.Error("落版页缺少图号")
	}
	if len(result.CheckFindings) != 0 {
		t.Errorf("clean page should not have findings: %+v", result.CheckFindings)
	}
}

func TestSubmissionToolRejectsEscapes(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "fig.svg"), []byte(sampleDrawing), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewSubmissionTool(ToolConfig{WorkingDir: workDir})
	cases := []struct{ name, args string }{
		{"output escape", `{"drawing": "fig.svg", "output": "../evil.svg"}`},
		{"drawing absolute", `{"drawing": "/etc/hosts", "output": "page.svg"}`},
		{"missing drawing", `{"output": "page.svg"}`},
	}
	for _, tc := range cases {
		_, err := tool.Func(context.Background(), json.RawMessage(tc.args))
		if errorCode(t, err) != ErrPathEscape {
			t.Errorf("%s: err = %v, want path_escape", tc.name, err)
		}
	}
}

func TestSubmissionToolRejectsSymlinkEscape(t *testing.T) {
	workDir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "fig.svg"), []byte(sampleDrawing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workDir, "link")); err != nil {
		t.Skipf("当前平台不支持符号链接: %v", err)
	}
	tool := NewSubmissionTool(ToolConfig{WorkingDir: workDir})
	_, err := tool.Func(context.Background(), json.RawMessage(
		`{"drawing": "link/fig.svg", "output": "page.svg"}`))
	if err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("expected symlink escape error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "page.svg")); statErr == nil {
		t.Error("越界输入不应产生落版页")
	}
}

func TestSubmissionToolFailOnFindings(t *testing.T) {
	workDir := t.TempDir()
	overflow := `<svg xmlns="http://www.w3.org/2000/svg" width="100mm" height="50mm" viewBox="0 0 100 50">
  <rect x="300" y="10" width="10" height="10" fill="none" stroke="#000000"/>
</svg>`
	if err := os.WriteFile(filepath.Join(workDir, "overflow.svg"), []byte(overflow), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewSubmissionTool(ToolConfig{WorkingDir: workDir})
	_, err := tool.Func(context.Background(), json.RawMessage(`{
		"drawing": "overflow.svg", "output": "page.svg", "fail_on_findings": true
	}`))
	if errorCode(t, err) != ErrRenderFailed {
		t.Fatalf("err = %v, want render_failed", err)
	}
	if !strings.Contains(err.Error(), "越出画布") {
		t.Errorf("err = %v, want 越出画布", err)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "page.svg")); statErr != nil {
		t.Errorf("落版页应保留在磁盘：%v", statErr)
	}

	// 不阻断时同一张图仍可导出，且发现被记录。
	raw, err := tool.Func(context.Background(), json.RawMessage(`{
		"drawing": "overflow.svg", "output": "page2.svg"
	}`))
	if err != nil {
		t.Fatalf("non-strict: %v", err)
	}
	result := raw.(SubmissionToolResult)
	if !hasBlocking(result.CheckFindings) {
		t.Errorf("findings = %+v, want blocking ink-outside", result.CheckFindings)
	}
}

func hasBlocking(findings []CheckFinding) bool {
	for _, finding := range findings {
		if finding.Blocking {
			return true
		}
	}
	return false
}
