package filing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// requireDocxPython 在缺少 python-docx 的环境跳过成文测试；CI 会预装该依赖。
func requireDocxPython(t *testing.T) {
	t.Helper()
	if _, err := findPython(context.Background(), ""); err != nil {
		t.Skipf("跳过：%v", err)
	}
}

func writeFigure(t *testing.T, dir string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 240, 160))
	for x := range 240 {
		for y := range 160 {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 255})
		}
	}
	path := filepath.Join(dir, "fig1.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建附图: %v", err)
	}
	defer func() { _ = file.Close() }()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("编码附图: %v", err)
	}
	return path
}

func sampleContent(t *testing.T, dir string) Content {
	t.Helper()
	return Content{
		Abstract: []string{"本发明公开了一种测试装置，包括壳体与转子。"},
		Claims:   []string{"1. 一种测试装置，其特征在于，包括壳体与转子。"},
		Specification: []any{
			map[string]any{"kind": "h3", "text": "技术领域"},
			map[string]any{"kind": "p", "text": "本发明涉及机械领域。"},
			map[string]any{"kind": "h3", "text": "背景技术"},
			map[string]any{"kind": "p", "text": "现有装置结构复杂。"},
			map[string]any{"kind": "h3", "text": "发明内容"},
			map[string]any{"kind": "p", "text": "为解决上述问题，本发明提供一种测试装置。"},
			map[string]any{"kind": "h3", "text": "附图说明"},
			map[string]any{"kind": "p", "text": "图1为本发明结构示意图。"},
			map[string]any{"kind": "h3", "text": "具体实施方式"},
			map[string]any{"kind": "p", "text": "下面结合附图对本发明作进一步说明。"},
		},
		Figures: []string{writeFigure(t, dir)},
	}
}

func TestComposeProducesFiveSections(t *testing.T) {
	requireDocxPython(t)
	t.Setenv("MADY_HOME", t.TempDir())
	dir := t.TempDir()
	output := filepath.Join(dir, "申请文件.docx")

	result, err := Compose(context.Background(), sampleContent(t, dir), output, Config{})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if info, err := os.Stat(output); err != nil || info.Size() == 0 {
		t.Fatalf("产物缺失或为空: %v", err)
	}
	sections, ok := result.Report["sections"].([]any)
	if !ok || len(sections) != 5 {
		t.Fatalf("sections = %v, want 5 节（report=%v）", result.Report["sections"], result.Report)
	}
	// 本 spec 的五个节都是 numbering: none，编号数应为 0。
	if got, _ := result.Report["numbering_total"].(float64); got != 0 {
		t.Fatalf("编号数 = %v, 本 spec 各节不编号", got)
	}
}

func TestComposeRejectsForbiddenText(t *testing.T) {
	requireDocxPython(t)
	t.Setenv("MADY_HOME", t.TempDir())
	dir := t.TempDir()
	content := sampleContent(t, dir)
	content.Abstract = append(content.Abstract, "本文件由 AI 辅助生成。")

	_, err := Compose(context.Background(), content, filepath.Join(dir, "bad.docx"), Config{})
	if err == nil {
		t.Fatal("含禁用词的出件应失败")
	}
	if !strings.Contains(err.Error(), "验收未通过") && !strings.Contains(err.Error(), "成文失败") {
		t.Fatalf("错误应说明失败原因：%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "bad.docx")); statErr == nil {
		t.Fatal("失败时不应留下产物")
	}
}

func TestComposeRejectsMissingOutputPath(t *testing.T) {
	if _, err := Compose(context.Background(), Content{}, "", Config{}); err == nil {
		t.Fatal("空输出路径应报错")
	}
}

// 体例真相源的指纹必须与 TEMPLATE-IDENTITY.md 登记的一致，防止模板被替换。
func TestTemplateFingerprint(t *testing.T) {
	const registered = "f589a5bcd75ffa859254bd95eceabb87e025ad5ea739d652f5cc181ff9ccff0e"
	data, err := assets.ReadFile("data/template/申请文件模板.docx")
	if err != nil {
		t.Fatalf("读取模板: %v", err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != registered {
		t.Fatalf("模板指纹 = %s, 登记值 = %s", got, registered)
	}
}

func TestMaterializeIsIdempotent(t *testing.T) {
	t.Setenv("MADY_HOME", t.TempDir())
	first, err := Materialize()
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	before, err := os.ReadFile(first.Template)
	if err != nil {
		t.Fatalf("读取模板: %v", err)
	}
	second, err := Materialize()
	if err != nil {
		t.Fatalf("二次 Materialize: %v", err)
	}
	after, err := os.ReadFile(second.Template)
	if err != nil {
		t.Fatalf("二次读取模板: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("重复解压不应改变模板内容")
	}
}
