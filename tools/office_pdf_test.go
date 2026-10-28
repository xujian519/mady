package tools

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFakeEngine 写一个可执行的假引擎脚本。
func writeFakeEngine(t *testing.T, dir, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("假引擎脚本仅适用于类 Unix 平台")
	}
	path := filepath.Join(dir, "fake-engine")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil { //nolint:gosec // 测试用可执行脚本
		t.Fatalf("写假引擎: %v", err)
	}
	return path
}

// fakeLibreOfficeBody 从 --outdir 与最后一个参数取路径，生成同名 PDF。
const fakeLibreOfficeBody = `
out=""
input=""
while [ $# -gt 0 ]; do
  case "$1" in
    --outdir) out="$2"; shift 2 ;;
    -env:*) shift ;;
    --convert-to) shift 2 ;;
    --headless|--norestore|--invisible) shift ;;
    *) input="$1"; shift ;;
  esac
done
base=$(basename "$input")
base=${base%.*}
printf '%%PDF-1.4\n' > "$out/$base.pdf"
printf 'fake pdf body\n%%%%EOF\n' >> "$out/$base.pdf"
`

// fakePandocBody 从 -o 取输出路径。
const fakePandocBody = `
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then out="$a"; fi
  prev="$a"
done
printf '%%PDF-1.4\n' > "$out"
printf 'fake pdf body\n%%%%EOF\n' >> "$out"
`

func newOfficeInput(t *testing.T, dir, ext string) string {
	t.Helper()
	path := filepath.Join(dir, "sample."+ext)
	if err := os.WriteFile(path, []byte("docx-bytes"), 0o600); err != nil {
		t.Fatalf("写样本: %v", err)
	}
	return path
}

func runOffice(t *testing.T, cfg *OfficePDFConfig, args map[string]any) any {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := NewOfficeToPDFTool(cfg).Func(context.Background(), raw)
	if err != nil {
		t.Fatalf("转换: %v", err)
	}
	return out
}

// 六种格式在两种引擎组合下的路由与拒绝。
func TestChooseEngineRouting(t *testing.T) {
	const libre = "/usr/bin/soffice"
	const pandoc = "/usr/bin/pandoc"
	exe := map[string]bool{"doc": true, "docx": true, "xls": true, "xlsx": true, "ppt": true, "pptx": true}

	cases := []struct {
		name     string
		ext      string
		libre    string
		pandoc   string
		engine   string
		rejected bool
		reason   string
	}{
		{"docx 有 LibreOffice 走原生", "docx", libre, pandoc, EngineLibreOffice, false, ""},
		{"docx 无 LibreOffice 降级 pandoc", "docx", "", pandoc, EnginePandocTypst, false, ""},
		{"doc 无 LibreOffice 只能拒绝", "doc", "", pandoc, "", true, "unsupported_format"},
		{"xls 无 LibreOffice 只能拒绝", "xls", "", pandoc, "", true, "unsupported_format"},
		{"ppt 无 LibreOffice 只能拒绝", "ppt", "", pandoc, "", true, "unsupported_format"},
		{"pptx 无 LibreOffice 降级 pandoc", "pptx", "", pandoc, EnginePandocTypst, false, ""},
		{"xlsx 无 LibreOffice 降级 pandoc", "xlsx", "", pandoc, EnginePandocTypst, false, ""},
		{"两个引擎都没有", "docx", "", "", "", true, "engine_missing"},
	}
	for _, tc := range cases {
		if !exe[tc.ext] {
			t.Fatalf("未覆盖格式 %s", tc.ext)
		}
		t.Run(tc.name, func(t *testing.T) {
			engine, rejected := chooseEngine("", tc.ext, tc.libre, tc.pandoc)
			if tc.rejected {
				if rejected == nil {
					t.Fatalf("应拒绝：engine=%q", engine)
				}
				if rejected.Reason != tc.reason {
					t.Fatalf("reason = %q，期望 %q", rejected.Reason, tc.reason)
				}
				if rejected.Hint == "" {
					t.Fatal("拒绝必须给出安装指引")
				}
				return
			}
			if rejected != nil {
				t.Fatalf("不应拒绝：%+v", rejected)
			}
			if engine != tc.engine {
				t.Fatalf("engine = %q，期望 %q", engine, tc.engine)
			}
		})
	}

	// 显式指定 pandoc 与旧版格式：拒绝而不是假装能转。
	if _, rejected := chooseEngine(EnginePandocTypst, "doc", "", pandoc); rejected == nil ||
		rejected.Reason != "unsupported_format" {
		t.Fatalf("pandoc + doc 应拒绝：%+v", rejected)
	}
	// 显式要求原生通道但没装：拒绝而不是偷偷降级。
	if _, rejected := chooseEngine(EngineLibreOffice, "docx", "", pandoc); rejected == nil ||
		rejected.Reason != "engine_missing" {
		t.Fatalf("显式 LibreOffice 缺失应拒绝：%+v", rejected)
	}
	// 显式指定 LibreOffice 且可用：即使 pandoc 也在，也用原生。
	if engine, rejected := chooseEngine(EngineLibreOffice, "xlsx", libre, pandoc); rejected != nil ||
		engine != EngineLibreOffice {
		t.Fatalf("显式 LibreOffice：engine=%q rejected=%+v", engine, rejected)
	}
}

func TestOfficePDFRejectsBadInputs(t *testing.T) {
	dir := t.TempDir()
	cfg := OfficePDFConfigDefaults()
	cfg.WorkingDir = dir
	cfg.PandocPath = filepath.Join(dir, "no-pandoc")
	cfg.LibreOfficePath = filepath.Join(dir, "no-soffice")

	tool := NewOfficeToPDFTool(cfg)
	call := func(args map[string]any) error {
		raw, _ := json.Marshal(args)
		_, err := tool.Func(context.Background(), raw)
		return err
	}

	if err := call(map[string]any{"input": "missing.docx"}); err == nil {
		t.Fatal("不存在的文件应报错")
	}

	txt := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(txt, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := call(map[string]any{"input": txt}); err == nil ||
		!strings.Contains(err.Error(), "不支持的格式") {
		t.Fatalf("txt 应被拒：%v", err)
	}

	docx := newOfficeInput(t, dir, "docx")
	if err := call(map[string]any{"input": docx, "output": docx}); err == nil ||
		!strings.Contains(err.Error(), "不能与源文件相同") {
		t.Fatalf("输出等于输入应被拒：%v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "big.docx"), make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	small := OfficePDFConfigDefaults()
	small.WorkingDir = dir
	small.MaxInputBytes = 100
	raw, _ := json.Marshal(map[string]any{"input": filepath.Join(dir, "big.docx")})
	if _, err := NewOfficeToPDFTool(small).Func(context.Background(), raw); err == nil ||
		!strings.Contains(err.Error(), "超过上限") {
		t.Fatalf("超限应被拒：%v", err)
	}
}

// 引擎缺失时返回结构化拒绝，而不是错误：模型需要读到安装指引。
func TestOfficePDFRejectionWhenNoEngine(t *testing.T) {
	dir := t.TempDir()
	docx := newOfficeInput(t, dir, "docx")
	cfg := OfficePDFConfigDefaults()
	cfg.WorkingDir = dir
	// 显式指向不存在的路径，模拟两个引擎都未安装。
	cfg.LibreOfficePath = filepath.Join(dir, "no-soffice")
	cfg.PandocPath = filepath.Join(dir, "no-pandoc")

	out := runOffice(t, cfg, map[string]any{"input": docx})
	result, ok := out.(OfficePDFResult)
	if !ok {
		t.Fatalf("输出类型 = %T", out)
	}
	if result.Status != "rejected" || result.Reason != "engine_missing" || result.Hint == "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(strings.TrimSuffix(docx, ".docx") + ".pdf"); err == nil {
		t.Fatal("拒绝时不应产出 PDF")
	}
}

// LibreOffice 通道：命令构造、产物落位与结果字段。
func TestOfficePDFWithFakeLibreOffice(t *testing.T) {
	dir := t.TempDir()
	docx := newOfficeInput(t, dir, "docx")
	cfg := OfficePDFConfigDefaults()
	cfg.WorkingDir = dir
	cfg.LibreOfficePath = writeFakeEngine(t, dir, fakeLibreOfficeBody)
	cfg.PandocPath = filepath.Join(dir, "no-pandoc")

	out := runOffice(t, cfg, map[string]any{"input": docx})
	result, ok := out.(OfficePDFResult)
	if !ok {
		t.Fatalf("输出类型 = %T", out)
	}
	if result.Status != "converted" || result.Engine != EngineLibreOffice || result.Degraded {
		t.Fatalf("result = %+v", result)
	}
	if result.Bytes == 0 {
		t.Fatal("字节数未记录")
	}
	data, err := os.ReadFile(result.Output)
	if err != nil {
		t.Fatalf("读产物: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatalf("产物不是 PDF：%q", string(data[:8]))
	}
}

// pandoc 通道：结果标注 degraded。
func TestOfficePDFWithFakePandocIsDegraded(t *testing.T) {
	dir := t.TempDir()
	docx := newOfficeInput(t, dir, "docx")
	cfg := OfficePDFConfigDefaults()
	cfg.WorkingDir = dir
	cfg.LibreOfficePath = filepath.Join(dir, "no-soffice")
	cfg.PandocPath = writeFakeEngine(t, dir, fakePandocBody)

	out := runOffice(t, cfg, map[string]any{"input": docx})
	result := out.(OfficePDFResult)
	if result.Status != "converted" || result.Engine != EnginePandocTypst {
		t.Fatalf("result = %+v", result)
	}
	if !result.Degraded || result.Note == "" {
		t.Fatalf("降级通道必须标注：%+v", result)
	}
}

// 引擎「成功」但没产物、产物不是 PDF、产物为空：都要失败。
func TestOfficePDFRejectsBadOutput(t *testing.T) {
	cases := []string{"没有产物", "不是 PDF", "空产物"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			docx := newOfficeInput(t, dir, "docx")
			body := "exit 0"
			switch name {
			case "空产物":
				body = fakeLibreOfficeBody + "\n: > \"$out/$base.pdf\"\n"
			case "不是 PDF":
				body = fakeLibreOfficeBody + "\nprintf 'not a pdf' > \"$out/$base.pdf\"\n"
			}
			cfg := OfficePDFConfigDefaults()
			cfg.WorkingDir = dir
			cfg.LibreOfficePath = writeFakeEngine(t, dir, body)

			raw, _ := json.Marshal(map[string]any{"input": docx})
			if _, err := NewOfficeToPDFTool(cfg).Func(context.Background(), raw); err == nil {
				t.Fatal("坏产物应报错")
			}
		})
	}
}

// 超时要报错，不能挂住。
func TestOfficePDFTimeout(t *testing.T) {
	dir := t.TempDir()
	docx := newOfficeInput(t, dir, "docx")
	cfg := OfficePDFConfigDefaults()
	cfg.WorkingDir = dir
	cfg.Timeout = 1
	cfg.LibreOfficePath = writeFakeEngine(t, dir, "sleep 5\n"+fakeLibreOfficeBody)

	raw, _ := json.Marshal(map[string]any{"input": docx})
	_, err := NewOfficeToPDFTool(cfg).Func(context.Background(), raw)
	if err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("应超时：%v", err)
	}
}

func TestDefaultCJKFontNotEmpty(t *testing.T) {
	if defaultCJKFont() == "" {
		t.Fatal("平台默认中文字体不能为空")
	}
}

func TestLibreOfficeInstallHintNotInstanceSpecific(t *testing.T) {
	if !strings.Contains(libreOfficeInstallHint(), "LibreOffice") {
		t.Fatal("安装指引应提到 LibreOffice")
	}
	if !strings.Contains(pandocInstallHint(), "pandoc") {
		t.Fatal("安装指引应提到 pandoc")
	}
}

// 真实 pandoc + typst 通道：最小 docx → PDF，验证降级通道在本机可用。
func TestOfficePDFRealPandocConversion(t *testing.T) {
	pandoc, err := exec.LookPath("pandoc")
	if err != nil {
		t.Skip("本机未安装 pandoc")
	}
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("本机未安装 typst")
	}

	dir := t.TempDir()
	src := writeMinimalDocx(t, dir)
	cfg := OfficePDFConfigDefaults()
	cfg.WorkingDir = dir
	cfg.PandocPath = pandoc
	cfg.LibreOfficePath = filepath.Join(dir, "no-soffice")
	cfg.CJKFont = defaultCJKFont()

	out := runOffice(t, cfg, map[string]any{"input": src})
	result := out.(OfficePDFResult)
	if result.Status != "converted" || result.Engine != EnginePandocTypst || !result.Degraded {
		t.Fatalf("result = %+v", result)
	}
	info, err := os.Stat(result.Output)
	if err != nil {
		t.Fatalf("产物: %v", err)
	}
	if info.Size() < 500 {
		t.Fatalf("产物过小：%d 字节", info.Size())
	}
}

// writeMinimalDocx 构造一个最小可读 docx（含中文段落），不依赖 testdata 二进制。
func writeMinimalDocx(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "mini.docx")
	f, err := os.Create(path) //nolint:gosec // 测试临时文件
	if err != nil {
		t.Fatalf("创建 docx: %v", err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	parts := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body><w:p><w:r><w:t>一种智能传感器，其特征在于，包括传感器本体。</w:t></w:r></w:p></w:body></w:document>`,
	}
	for name, body := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("写 %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("写 %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 docx: %v", err)
	}
	return path
}

// 沙箱开启时，输入输出都受工作目录边界约束；符号链接不能穿越。
func TestOfficePDFRespectsSandbox(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	cfg := OfficePDFConfigDefaults()
	cfg.WorkingDir = work
	cfg.Sandbox = WorkingDirSandbox{Enabled: true, WorkingDir: work}
	cfg.LibreOfficePath = writeFakeEngine(t, work, fakeLibreOfficeBody)
	cfg.PandocPath = filepath.Join(work, "no-pandoc")
	tool := NewOfficeToPDFTool(cfg)
	call := func(args map[string]any) error {
		raw, _ := json.Marshal(args)
		_, err := tool.Func(context.Background(), raw)
		return err
	}

	docx := newOfficeInput(t, work, "docx")
	if err := call(map[string]any{"input": docx}); err != nil {
		t.Fatalf("工作目录内应允许：%v", err)
	}

	outsideDoc := filepath.Join(outside, "secret.docx")
	if err := os.WriteFile(outsideDoc, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := call(map[string]any{"input": outsideDoc}); err == nil ||
		!strings.Contains(err.Error(), "不在允许范围") {
		t.Fatalf("工作目录外应拒绝：%v", err)
	}

	link := filepath.Join(work, "escape.docx")
	if err := os.Symlink(outsideDoc, link); err != nil {
		t.Skipf("无法创建符号链接: %v", err)
	}
	if err := call(map[string]any{"input": link}); err == nil ||
		!strings.Contains(err.Error(), "不在允许范围") {
		t.Fatalf("符号链接逃逸应拒绝：%v", err)
	}

	if err := call(map[string]any{"input": docx, "output": filepath.Join(outside, "out.pdf")}); err == nil ||
		!strings.Contains(err.Error(), "不在允许范围") {
		t.Fatalf("输出越界应拒绝：%v", err)
	}
}

// engine=auto 是 schema 声明的合法值，与省略同路径。
func TestChooseEngineAcceptsAuto(t *testing.T) {
	engine, rejected := chooseEngine("auto", "docx", "/usr/bin/soffice", "/usr/bin/pandoc")
	if rejected != nil || engine != EngineLibreOffice {
		t.Fatalf("engine=%q rejected=%+v", engine, rejected)
	}
}

// 装配层必须把工作目录与沙箱注入 office_to_pdf，否则路径约束静默失效。
func TestPropagateSandboxToOfficePDF(t *testing.T) {
	cfg := &ExtensionConfig{WorkingDir: "/tmp/mady-work", SandboxEnabled: true}
	propagateSandbox(cfg)

	if cfg.OfficePDF == nil {
		t.Fatal("OfficePDF 未初始化")
	}
	if !cfg.OfficePDF.Sandbox.Enabled || cfg.OfficePDF.Sandbox.WorkingDir != "/tmp/mady-work" {
		t.Fatalf("Sandbox 未传播：%+v", cfg.OfficePDF.Sandbox)
	}
	if cfg.OfficePDF.WorkingDir != "/tmp/mady-work" {
		t.Fatalf("WorkingDir 未传播：%q", cfg.OfficePDF.WorkingDir)
	}
}
