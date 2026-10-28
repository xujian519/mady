package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// OfficeToPDFToolName 是 Office 转 PDF 工具名。
const OfficeToPDFToolName = "office_to_pdf"

// Office 转换的两种通道。
const (
	// EngineLibreOffice 原生排版引擎，支持全部六种格式。
	EngineLibreOffice = "libreoffice"
	// EnginePandocTypst 降级通道：pandoc 解析 + typst 排版，版式可能与原件不同。
	EnginePandocTypst = "pandoc-typst"
)

// officeExts 是支持的六种格式，与 dsh office-to-pdf 的覆盖面一致。
var officeExts = map[string]bool{
	"doc": true, "docx": true,
	"xls": true, "xlsx": true,
	"ppt": true, "pptx": true,
}

// pandocExts 是 pandoc 能作为输入读取的格式。旧版二进制格式（doc/xls/ppt）只能走 LibreOffice。
var pandocExts = map[string]bool{
	"docx": true, "xlsx": true, "pptx": true,
}

// OfficePDFConfig 配置 office_to_pdf 工具。
type OfficePDFConfig struct {
	// WorkingDir 约束输入输出路径；为空时相对当前目录。
	WorkingDir string
	// Sandbox 是与其他文件工具一致的路径护栏（含符号链接解析与白名单）。
	Sandbox WorkingDirSandbox
	// Timeout 是单次转换的秒数上限，默认 180（LibreOffice 首次启动较慢）。
	Timeout int
	// MaxInputBytes / MaxOutputBytes 是源文件与产物的字节上限。
	MaxInputBytes  int64
	MaxOutputBytes int64
	// CJKFont 是 pandoc 通道使用的中文字体；为空时按平台取默认。
	CJKFont string
	// LibreOfficePath / PandocPath 为空时按 PATH 与平台标准安装位置探测。
	LibreOfficePath string
	PandocPath      string
}

// OfficePDFConfigDefaults 返回默认配置。
func OfficePDFConfigDefaults() *OfficePDFConfig {
	return &OfficePDFConfig{
		Timeout:        180,
		MaxInputBytes:  100 << 20,
		MaxOutputBytes: 200 << 20,
	}
}

// OfficePDFInput 是工具入参。
type OfficePDFInput struct {
	Input  string `json:"input"`
	Output string `json:"output,omitempty"`
	Engine string `json:"engine,omitempty"`
	Font   string `json:"font,omitempty"`
}

// OfficePDFResult 是一次转换的结论。
//
// 引擎缺失或格式无可用通道时返回 status=rejected 而不是报错：这是可预期的事实，
// 模型需要读到安装指引并转告用户，而不是把整次工具调用当故障。
type OfficePDFResult struct {
	Status   string `json:"status"`
	Engine   string `json:"engine,omitempty"`
	Input    string `json:"input"`
	Output   string `json:"output,omitempty"`
	Format   string `json:"format"`
	Bytes    int64  `json:"bytes,omitempty"`
	Degraded bool   `json:"degraded,omitempty"`
	Note     string `json:"note,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Hint     string `json:"hint,omitempty"`
}

// NewOfficeToPDFTool 创建 Office 转 PDF 工具。
func NewOfficeToPDFTool(cfg *OfficePDFConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = OfficePDFConfigDefaults()
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 180
	}
	if cfg.MaxInputBytes <= 0 {
		cfg.MaxInputBytes = 100 << 20
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 200 << 20
	}

	return &agentcore.Tool{
		Name: OfficeToPDFToolName,
		Description: "把 Office 文档转成 PDF：优先用 LibreOffice 原生排版，" +
			"缺失时用 pandoc + typst 降级（版式可能与原件不同，结果会标注 degraded）。" +
			"支持 doc/docx/xls/xlsx/ppt/pptx；旧版二进制格式只能用 LibreOffice。" +
			"两种引擎都不可用时返回结构化拒绝与安装指引，不会静默失败。",
		Parameters: map[string]any{
			"type":     "object",
			"required": []string{"input"},
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "待转换的 Office 文件路径。",
				},
				"output": map[string]any{
					"type":        "string",
					"description": "PDF 输出路径；省略时与源文件同目录同名。",
				},
				"engine": map[string]any{
					"type":        "string",
					"enum":        []string{"auto", EngineLibreOffice, EnginePandocTypst},
					"description": "转换通道；默认 auto（LibreOffice 优先，缺失时降级）。",
				},
				"font": map[string]any{
					"type":        "string",
					"description": "pandoc 通道使用的中文字体；省略时按平台默认（macOS 苹方、Windows 雅黑、Linux Noto Sans CJK SC）。",
				},
			},
		},
		Func: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in OfficePDFInput
			if err := json.Unmarshal(raw, &in); err != nil {
				return resultErrf("office_to_pdf: 参数解析失败: %w", err)
			}
			return runOfficeToPDF(ctx, cfg, in)
		},
	}
}

// runOfficeToPDF 执行一次转换。
func runOfficeToPDF(ctx context.Context, cfg *OfficePDFConfig, in OfficePDFInput) (any, error) {
	inputPath, err := cfg.resolvePath(in.Input, AccessRead)
	if err != nil {
		return nil, fmt.Errorf("office_to_pdf: 输入路径: %w", err)
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("office_to_pdf: 源文件不可读: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("office_to_pdf: %s 不是普通文件", in.Input)
	}
	if info.Size() > cfg.MaxInputBytes {
		return nil, fmt.Errorf("office_to_pdf: 源文件 %d MB 超过上限 %d MB",
			info.Size()>>20, cfg.MaxInputBytes>>20)
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(inputPath), "."))
	if !officeExts[ext] {
		return nil, fmt.Errorf("office_to_pdf: 不支持的格式 %q（支持 doc/docx/xls/xlsx/ppt/pptx）", ext)
	}

	outputPath := in.Output
	if outputPath == "" {
		outputPath = strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".pdf"
	} else {
		if outputPath, err = cfg.resolvePath(outputPath, AccessWrite); err != nil {
			return nil, fmt.Errorf("office_to_pdf: 输出路径: %w", err)
		}
	}
	if outputPath == inputPath {
		return nil, fmt.Errorf("office_to_pdf: 输出路径不能与源文件相同")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return nil, fmt.Errorf("office_to_pdf: 创建输出目录: %w", err)
	}

	libre := findLibreOffice(cfg.LibreOfficePath)
	pandoc := findPandoc(cfg.PandocPath)

	engine, rejected := chooseEngine(in.Engine, ext, libre, pandoc)
	if rejected != nil {
		rejected.Input = inputPath
		rejected.Output = outputPath
		return *rejected, nil
	}

	tmpDir, err := os.MkdirTemp("", "mady-office-*")
	if err != nil {
		return nil, fmt.Errorf("office_to_pdf: 创建临时目录: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	var produced string
	switch engine {
	case EngineLibreOffice:
		produced, err = convertWithLibreOffice(timeoutCtx, libre, inputPath, tmpDir)
	case EnginePandocTypst:
		produced, err = convertWithPandoc(timeoutCtx, pandoc, inputPath, tmpDir, in.Font, cfg.CJKFont)
	}
	if err != nil {
		return nil, err
	}
	if err := checkPDF(produced, cfg.MaxOutputBytes); err != nil {
		return nil, err
	}
	_, statErr := os.Stat(outputPath)
	overwrote := statErr == nil
	size, err := moveIntoPlace(produced, outputPath)
	if err != nil {
		return nil, err
	}

	result := OfficePDFResult{
		Status: "converted",
		Engine: engine,
		Input:  inputPath,
		Output: outputPath,
		Format: ext,
		Bytes:  size,
	}
	if engine == EnginePandocTypst {
		result.Degraded = true
		result.Note = "pandoc + typst 降级通道：分页、页眉页脚与复杂表格的版式可能与原文档不同，交付前请目检。"
	}
	if overwrote {
		result.Note = strings.TrimSpace(result.Note + " 已覆盖同名 PDF。")
	}
	return result, nil
}

// chooseEngine 按请求与可用性决定通道；无法转换时返回结构化拒绝。
func chooseEngine(requested, ext string, libre, pandoc string) (string, *OfficePDFResult) {
	reject := func(reason, hint string) (string, *OfficePDFResult) {
		return "", &OfficePDFResult{
			Status: "rejected",
			Format: ext,
			Reason: reason,
			Hint:   hint,
		}
	}

	switch requested {
	case EngineLibreOffice:
		if libre == "" {
			return reject("engine_missing", libreOfficeInstallHint())
		}
		return EngineLibreOffice, nil
	case EnginePandocTypst:
		if pandoc == "" {
			return reject("engine_missing", pandocInstallHint())
		}
		if !pandocExts[ext] {
			return reject("unsupported_format", fmt.Sprintf(
				"pandoc 不能读取 .%s；该格式只能走 LibreOffice。%s", ext, libreOfficeInstallHint()))
		}
		return EnginePandocTypst, nil
	}

	if libre != "" {
		return EngineLibreOffice, nil
	}
	if pandoc != "" && pandocExts[ext] {
		return EnginePandocTypst, nil
	}
	if pandoc != "" {
		return reject("unsupported_format", fmt.Sprintf(
			"没有 LibreOffice 时，.%s 无法转换（pandoc 只能读取 docx/xlsx/pptx）。%s",
			ext, libreOfficeInstallHint()))
	}
	return reject("engine_missing", libreOfficeInstallHint()+"\n"+pandocInstallHint())
}

// runEngine 运行转换命令。超时时结束整个进程组，避免遗留子进程占住 LibreOffice profile。
func runEngine(ctx context.Context, cmd *exec.Cmd, stderr *bytes.Buffer) error {
	configureCommand(cmd)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		killCommandTree(cmd)
		<-done
		return ctx.Err()
	}
}

// convertWithLibreOffice 用 LibreOffice headless 转换，产物落在 tmpDir。
func convertWithLibreOffice(ctx context.Context, soffice, input, tmpDir string) (string, error) {
	profile := filepath.Join(tmpDir, "profile")
	args := []string{
		"--headless", "--norestore", "--invisible",
		"-env:UserInstallation=file://" + filepath.ToSlash(profile),
		"--convert-to", "pdf",
		"--outdir", tmpDir,
		input,
	}
	cmd := exec.CommandContext(ctx, soffice, args...) //nolint:gosec // G204: soffice 路径由探测或配置给出，参数分离
	var stderr bytes.Buffer
	if err := runEngine(ctx, cmd, &stderr); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("office_to_pdf: LibreOffice 超时或已取消: %w", ctx.Err())
		}
		return "", fmt.Errorf("office_to_pdf: LibreOffice 转换失败: %v: %s",
			err, strings.TrimSpace(stderr.String()))
	}

	produced := filepath.Join(tmpDir,
		strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))+".pdf")
	if _, err := os.Stat(produced); err != nil {
		return "", fmt.Errorf("office_to_pdf: LibreOffice 未生成 PDF（%s）", strings.TrimSpace(stderr.String()))
	}
	return produced, nil
}

// convertWithPandoc 用 pandoc + typst 转换，产物落在 tmpDir。
func convertWithPandoc(ctx context.Context, pandoc, input, tmpDir, font, cfgFont string) (string, error) {
	if font == "" {
		font = cfgFont
	}
	if font == "" {
		font = defaultCJKFont()
	}
	produced := filepath.Join(tmpDir, "out.pdf")
	args := []string{
		input,
		"-o", produced,
		"--pdf-engine=typst",
		"-V", "mainfont=" + font,
	}
	cmd := exec.CommandContext(ctx, pandoc, args...) //nolint:gosec // G204: pandoc 路径由探测或配置给出，参数分离
	var stderr bytes.Buffer
	if err := runEngine(ctx, cmd, &stderr); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("office_to_pdf: pandoc 超时或已取消: %w", ctx.Err())
		}
		return "", fmt.Errorf("office_to_pdf: pandoc 转换失败（字体 %s）: %v: %s",
			font, err, strings.TrimSpace(stderr.String()))
	}
	return produced, nil
}

// checkPDF 校验产物是像样的 PDF：头尾都看，只读必要字节，不把大文件整个读进内存。
func checkPDF(path string, maxBytes int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("office_to_pdf: 产物不可读: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("office_to_pdf: 产物不是普通文件")
	}
	if info.Size() == 0 {
		return fmt.Errorf("office_to_pdf: 产物为空")
	}
	if info.Size() > maxBytes {
		return fmt.Errorf("office_to_pdf: 产物 %d MB 超过上限 %d MB", info.Size()>>20, maxBytes>>20)
	}
	//nolint:gosec // path 是本次转换在临时目录里生成的产物
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("office_to_pdf: 打开产物: %w", err)
	}
	defer func() { _ = f.Close() }()

	head := make([]byte, 5)
	if _, err := io.ReadFull(f, head); err != nil || string(head) != "%PDF-" {
		return fmt.Errorf("office_to_pdf: 产物不是 PDF（缺少 PDF 文件头）")
	}
	// 截断的 PDF 也可能带文件头；尾部必须有 EOF 标记。
	tailSize := int64(1024)
	if info.Size() < tailSize {
		tailSize = info.Size()
	}
	tail := make([]byte, tailSize)
	if _, err := f.ReadAt(tail, info.Size()-tailSize); err != nil {
		return fmt.Errorf("office_to_pdf: 读取产物尾部: %w", err)
	}
	if !bytes.Contains(tail, []byte("%%EOF")) {
		return fmt.Errorf("office_to_pdf: 产物不完整（缺少 PDF 结尾标记）")
	}
	return nil
}

// moveIntoPlace 把临时产物搬到目标路径，返回字节数。
//
// 先写同目录临时文件再 rename：目标要么是完整的新文件，要么保持原样，
// 不会出现写到一半的半成品；也不会跟随输出路径上的符号链接。
func moveIntoPlace(produced, outputPath string) (int64, error) {
	src, err := os.Open(produced) //nolint:gosec // 临时目录内的本次产物
	if err != nil {
		return 0, fmt.Errorf("office_to_pdf: 打开产物: %w", err)
	}
	defer func() { _ = src.Close() }()

	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".office-pdf-*")
	if err != nil {
		return 0, fmt.Errorf("office_to_pdf: 创建临时输出: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	size, err := io.Copy(tmp, src)
	if err != nil {
		_ = tmp.Close()
		return 0, fmt.Errorf("office_to_pdf: 写入 PDF: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("office_to_pdf: 关闭临时输出: %w", err)
	}
	//nolint:gosec // outputPath 由调用方给出，是本次交付的落盘位置
	if err := os.Rename(tmpName, outputPath); err != nil {
		return 0, fmt.Errorf("office_to_pdf: 落盘 PDF: %w", err)
	}
	return size, nil
}

// resolvePath 复用全库统一的工作目录护栏：符号链接会被解析，白名单按读写模式区分。
func (c *OfficePDFConfig) resolvePath(p string, mode AccessMode) (string, error) {
	return resolvePathSandboxedMode(p, c.WorkingDir, c.Sandbox, mode)
}

// findLibreOffice 返回可用的 soffice 路径；显式配置优先，其次 PATH，最后平台标准安装位置。
func findLibreOffice(configured string) string {
	if configured != "" {
		if isExecutable(configured) {
			return configured
		}
		return ""
	}
	for _, name := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, candidate := range libreOfficeCandidates() {
		if isExecutable(candidate) {
			return candidate
		}
	}
	return ""
}

// libreOfficeCandidates 是平台标准安装位置。
func libreOfficeCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	case "windows":
		return []string{
			`C:\Program Files\LibreOffice\program\soffice.exe`,
			`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
		}
	default:
		return []string{
			"/usr/bin/soffice",
			"/usr/local/bin/soffice",
			"/opt/libreoffice/program/soffice",
			"/snap/bin/libreoffice",
		}
	}
}

// findPandoc 返回可用的 pandoc 路径。
func findPandoc(configured string) string {
	if configured != "" {
		if isExecutable(configured) {
			return configured
		}
		return ""
	}
	if p, err := exec.LookPath("pandoc"); err == nil {
		return p
	}
	return ""
}

// isExecutable 判断路径是可执行文件。Windows 不返回 Unix 权限位，只能看是否普通文件。
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// defaultCJKFont 按平台返回中文字体；pandoc+typst 默认字体不含汉字，必须显式指定。
func defaultCJKFont() string {
	switch runtime.GOOS {
	case "darwin":
		return "PingFang SC"
	case "windows":
		return "Microsoft YaHei"
	default:
		return "Noto Sans CJK SC"
	}
}

// libreOfficeInstallHint 返回分平台的安装指引。
func libreOfficeInstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "安装 LibreOffice：brew install --cask libreoffice（安装后会自动探测 /Applications/LibreOffice.app）。"
	case "windows":
		return "安装 LibreOffice：winget install TheDocumentFoundation.LibreOffice。"
	default:
		return "安装 LibreOffice：sudo apt install libreoffice（或 snap install libreoffice）。"
	}
}

// pandocInstallHint 返回 pandoc 通道的安装指引。
func pandocInstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "安装降级通道：brew install pandoc typst。"
	case "windows":
		return "安装降级通道：winget install JohnMacFarlane.Pandoc 与 Typst.Typst。"
	default:
		return "安装降级通道：sudo apt install pandoc 并安装 typst（https://typst.app）。"
	}
}
