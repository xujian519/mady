package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// resolveOutput 把输出路径限制在工作目录内，并确定渲染格式。
func resolveOutput(workDir, output, formatArg string) (string, Format, error) {
	if strings.TrimSpace(output) == "" {
		return "", "", newError(ErrPathEscape, "缺少必填字段 'output'")
	}
	if workDir == "" {
		return "", "", newError(ErrPathEscape, "未配置工作目录，无法写入附图")
	}
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return "", "", fmt.Errorf("解析工作目录失败: %w", err)
	}
	absOut := output
	if !filepath.IsAbs(absOut) {
		absOut = filepath.Join(absWork, output)
	}
	absOut = filepath.Clean(absOut)
	rel, err := filepath.Rel(absWork, absOut)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", newError(ErrPathEscape, "输出路径必须位于工作目录 %s 内：%s", absWork, output)
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(absOut), "."))
	format := Format(strings.ToLower(formatArg))
	if format == "" {
		if ext != "" {
			format = Format(ext)
		} else {
			format = FormatSVG
		}
	}
	if !format.Valid() {
		return "", "", newError(ErrInvalidOutput, "未知输出格式 %q（可选 svg/png/pdf）", format)
	}
	if ext != "" && ext != string(format) {
		return "", "", newError(ErrInvalidOutput,
			"输出扩展名 .%s 与 format=%s 不一致", ext, format)
	}
	if ext == "" {
		absOut += "." + string(format)
	}
	return absOut, format, nil
}

// ensureWithin 解析已存在祖先的符号链接后，确认目录仍在工作目录内，
// 防止工作目录内的符号链接把产物写到边界之外。
func ensureWithin(workDir, dir string) error {
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("解析工作目录失败: %w", err)
	}
	resolvedWork, err := filepath.EvalSymlinks(absWork)
	if err != nil {
		return fmt.Errorf("解析工作目录符号链接失败: %w", err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("解析输出目录失败: %w", err)
	}
	resolvedDir, err := resolveExistingPrefix(absDir)
	if err != nil {
		return newError(ErrPathEscape, "解析输出目录失败：%v", err)
	}
	rel, err := filepath.Rel(resolvedWork, resolvedDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return newError(ErrPathEscape, "输出路径经符号链接后越出工作目录：%s", resolvedDir)
	}
	return nil
}

// resolveExistingPrefix 从 dir 向上找到第一个存在的路径并解析其符号链接，
// 再把尚不存在的尾部拼回；用于目录尚未创建时的越界判定。
func resolveExistingPrefix(dir string) (string, error) {
	current := dir
	var tail []string
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		tail = append([]string{filepath.Base(current)}, tail...)
		current = parent
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{resolved}, tail...)...), nil
}

// defaultFont 返回平台常见的中文字体，避免中文标签在 Graphviz 中丢字。
func defaultFont() string {
	switch runtime.GOOS {
	case "darwin":
		return "PingFang SC"
	case "windows":
		return "Microsoft YaHei"
	default:
		return "Noto Sans CJK SC"
	}
}
