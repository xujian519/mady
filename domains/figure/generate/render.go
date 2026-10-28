package generate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// 渲染默认值。
const (
	// DefaultRenderTimeout 是单次渲染超时。
	DefaultRenderTimeout = 60 * time.Second
	// DefaultProbeTimeout 是 dot -V 探测超时。
	DefaultProbeTimeout = 5 * time.Second
	// maxOutputBytes 是渲染产物大小上限。
	maxOutputBytes = 100 << 20
)

// dotCandidates 是各平台常见的 Graphviz dot 安装路径。
var dotCandidates = []string{
	"/opt/homebrew/bin/dot",
	"/usr/local/bin/dot",
	"/opt/local/bin/dot",
	"/usr/bin/dot",
	`C:\Program Files\Graphviz\bin\dot.exe`,
	`C:\Program Files (x86)\Graphviz\bin\dot.exe`,
	`C:\ProgramData\chocolatey\bin\dot.exe`,
}

// FindDot 解析 dot 可执行文件：显式覆盖 → MADY_GRAPHVIZ_DOT → PATH → 平台常见路径。
// 显式覆盖不存在时返回空，不回落自动探测——配置写错应当被看见。
func FindDot(override string) string {
	if override != "" {
		if isExecutable(override) {
			return override
		}
		return ""
	}
	if env := os.Getenv("MADY_GRAPHVIZ_DOT"); env != "" {
		if isExecutable(env) {
			return env
		}
		return ""
	}
	if path, err := exec.LookPath("dot"); err == nil {
		return path
	}
	for _, candidate := range dotCandidates {
		if isExecutable(candidate) {
			return candidate
		}
	}
	return ""
}

func isExecutable(path string) bool {
	info, err := os.Stat(path) //nolint:gosec // path 来自 FindDot 的白名单候选或显式配置
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// InstallHint 返回 Graphviz 缺失时的安装与配置指引。
func InstallHint(executable string) string {
	hint := "未找到 Graphviz dot 可执行文件。"
	if executable != "" {
		hint = fmt.Sprintf("已配置路径 %s 不存在或不可执行。", executable)
	}
	return hint + "请安装 Graphviz（macOS：brew install graphviz；Ubuntu/Debian：sudo apt install graphviz；" +
		"Windows：winget install graphviz 或 choco install graphviz），" +
		"或用 MADY_GRAPHVIZ_DOT 环境变量指定 dot 路径。"
}

// ProbeResult 是 Graphviz 可用性探测结果。
type ProbeResult struct {
	Ready      bool   `json:"ready"`
	Executable string `json:"executable,omitempty"`
	Version    string `json:"version,omitempty"`
	Message    string `json:"message,omitempty"`
}

var versionRE = regexp.MustCompile(`version (\d+(?:\.\d+)*)`)

// Probe 执行 `dot -V` 探测可用性（数十毫秒）。
func Probe(ctx context.Context, executable string) ProbeResult {
	path := FindDot(executable)
	if path == "" {
		return ProbeResult{Message: InstallHint(executable)}
	}
	probeCtx, cancel := context.WithTimeout(ctx, DefaultProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, path, "-V") //nolint:gosec // dot 路径来自白名单探测，参数为固定字面量
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return ProbeResult{Executable: path, Message: fmt.Sprintf("dot -V 调用失败：%v", err)}
	}
	result := ProbeResult{Ready: true, Executable: path}
	if match := versionRE.FindStringSubmatch(output.String()); match != nil {
		result.Version = match[1]
	}
	return result
}

// RenderSpec 是一次渲染请求。
type RenderSpec struct {
	// DOT 是 DOT 文本，经 stdin 传入（Graphviz ≥15 拒绝尾随 '-' 参数）。
	DOT string
	// Filename 是输出文件名（不含扩展名）。
	Filename string
	Format   Format
	Engine   Engine
	// OutputDir 是输出目录（必须已存在）。
	OutputDir string
}

// RenderConfig 是渲染器配置。
type RenderConfig struct {
	// Executable 覆盖 dot 路径；为空时自动探测。
	Executable string
	// Timeout 为单次渲染超时；为零时用 DefaultRenderTimeout。
	Timeout time.Duration
}

// filenameSanitizer 替换路径分隔符与跨平台非法字符；保留中文等非 ASCII 字符。
var filenameSanitizer = regexp.MustCompile("[\\\\/\\x00-\\x1f:*?\"<>|]")

// sanitizeFilename 清洗输出文件名；清洗后为空时回退 diagram。
func sanitizeFilename(name string) string {
	cleaned := filenameSanitizer.ReplaceAllString(name, "_")
	if cleaned == "" {
		return "diagram"
	}
	return cleaned
}

// Render 调用 Graphviz 渲染 DOT，并校验产物完整性。
//
// graphviz >= 15 不再接受尾随 `-` 作为 stdin 来源，因此 DOT 只经 stdin 传入、
// argv 不含文件参数。退出码为 0 不保证产物完整，按格式校验文件头与结构。
func Render(ctx context.Context, spec RenderSpec, cfg RenderConfig) (string, error) {
	if !spec.Format.Valid() {
		return "", newError(ErrRenderFailed, "未知输出格式 %q", spec.Format)
	}
	if !spec.Engine.Valid() {
		return "", newError(ErrRenderFailed, "未知布局引擎 %q", spec.Engine)
	}
	executable := FindDot(cfg.Executable)
	if executable == "" {
		return "", newError(ErrNotInstalled, "%s", InstallHint(cfg.Executable))
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultRenderTimeout
	}
	outputPath := filepath.Join(spec.OutputDir, sanitizeFilename(spec.Filename)+"."+string(spec.Format))

	renderCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(renderCtx, executable, //nolint:gosec // 可执行路径来自白名单探测；格式/引擎为枚举，输出路径由调用方限定在工作目录内
		"-T"+string(spec.Format), "-K"+string(spec.Engine), "-o", outputPath)
	cmd.Stdin = strings.NewReader(spec.DOT)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if renderCtx.Err() == context.DeadlineExceeded {
			return "", newError(ErrRenderFailed, "Graphviz 渲染超时（%s），未生成可用产物", timeout)
		}
		return "", newError(ErrRenderFailed, "Graphviz 渲染失败：%v；stderr：%s", err, strings.TrimSpace(stderr.String()))
	}
	if err := validateOutput(outputPath, spec.Format); err != nil {
		return "", err
	}
	return outputPath, nil
}

// validateOutput 校验渲染产物：非空、大小上限、按格式检查文件头（SVG 另查结构）。
func validateOutput(path string, format Format) error {
	info, err := os.Stat(path)
	if err != nil {
		return newError(ErrInvalidOutput, "Graphviz 未生成输出文件：%s", path)
	}
	if info.Size() == 0 {
		return newError(ErrInvalidOutput, "渲染产物为空文件：%s", path)
	}
	if info.Size() > maxOutputBytes {
		return newError(ErrInvalidOutput, "渲染产物过大（%d 字节 > %d）：%s", info.Size(), maxOutputBytes, path)
	}
	data, err := os.ReadFile(path) //nolint:gosec // path 是本次渲染自行构造的产物路径
	if err != nil {
		return newError(ErrInvalidOutput, "读取渲染产物失败：%v", err)
	}
	switch format {
	case FormatSVG:
		svg := string(data)
		if err := AssertSafeSVG(svg, DefaultSVGMaxBytes); err != nil {
			return err
		}
		viewport := ResolveViewport(RootTag(svg))
		if viewport.ViewBox == nil {
			return newError(ErrInvalidOutput, "渲染产物 SVG 缺少可解析的 viewBox，无法确定图面尺寸")
		}
	case FormatPNG:
		if len(data) < 4 || data[0] != 0x89 || data[1] != 'P' || data[2] != 'N' || data[3] != 'G' {
			return newError(ErrInvalidOutput, "渲染产物不是合法 PNG（magic bytes 不符，可能截断）")
		}
	case FormatPDF:
		head := data
		if len(head) > 1024 {
			head = head[:1024]
		}
		if !bytes.Contains(head, []byte("%PDF")) {
			return newError(ErrInvalidOutput, "渲染产物不是合法 PDF（缺 %%PDF 头，可能截断）")
		}
	}
	return nil
}
