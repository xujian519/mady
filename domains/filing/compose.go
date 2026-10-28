package filing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Content 是出件的内容模型（字段与引擎的 content.json 一致）。
type Content struct {
	// Abstract 是说明书摘要的段落。
	Abstract []string `json:"abstract"`
	// Claims 是权利要求书的逐项文本（含「1.」编号）。
	Claims []string `json:"claims"`
	// Specification 是说明书的段落序列（kind 取 h3/h4/p/table）。
	Specification []any `json:"specification"`
	// Figures 是附图文件的绝对路径。
	Figures []string `json:"figures"`
	// AbstractFigureIndex 指定摘要附图在 Figures 中的下标。
	AbstractFigureIndex *int `json:"abstractFigureIndex,omitempty"`
}

// Result 是一次出件的结果。
type Result struct {
	// OutputPath 是成文后的 DOCX 路径。
	OutputPath string
	// Report 是引擎输出的 JSON 报告（status/sections/numbering_total 等）。
	Report map[string]any
}

// Config 是出件配置。
type Config struct {
	// Python 是能 import docx 的解释器路径；为空时自动探测。
	Python string
	// Timeout 是引擎执行上限；默认 120 秒。
	Timeout time.Duration
}

// Compose 按内容模型成文，并返回引擎报告。
//
// 断言（节数、编号、禁用词、禁用色、字号）由引擎在写文件前执行；不通过时引擎以
// 非零退出码结束，本函数把 stderr 尾部带进错误，不产出半成品结果。
func Compose(ctx context.Context, content Content, outputPath string, cfg Config) (Result, error) {
	if strings.TrimSpace(outputPath) == "" {
		return Result{}, fmt.Errorf("filing: 未指定输出路径")
	}
	python, err := findPython(ctx, cfg.Python)
	if err != nil {
		return Result{}, err
	}
	prepared, err := Materialize()
	if err != nil {
		return Result{}, err
	}

	workDir, err := os.MkdirTemp("", "mady-filing-*")
	if err != nil {
		return Result{}, fmt.Errorf("filing: 创建临时目录: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	encoded, err := json.Marshal(content)
	if err != nil {
		return Result{}, fmt.Errorf("filing: 序列化内容: %w", err)
	}
	contentPath := filepath.Join(workDir, "content.json")
	//nolint:gosec // 临时目录内的固定文件名
	if err := os.WriteFile(contentPath, encoded, 0o600); err != nil {
		return Result{}, fmt.Errorf("filing: 写入内容: %w", err)
	}
	if dir := filepath.Dir(outputPath); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return Result{}, fmt.Errorf("filing: 创建输出目录: %w", err)
		}
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		prepared.BuildScript,
		"--spec", prepared.Spec,
		"--content", contentPath,
		"--template", prepared.Template,
		"--out", outputPath,
	}
	//nolint:gosec // python 是探测到的解释器，args 由固定脚本与本次出件的路径组成
	command := exec.CommandContext(runCtx, python, args...)
	command.Dir = prepared.Dir
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return Result{}, fmt.Errorf("filing: 成文超时（%s）", timeout)
		}
		return Result{}, fmt.Errorf("filing: 成文失败: %v；%s", err, tail(stderr.String(), 800))
	}

	report := map[string]any{}
	if trimmed := strings.TrimSpace(stdout.String()); trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &report); err != nil {
			return Result{}, fmt.Errorf("filing: 解析引擎报告: %w", err)
		}
	}
	if status, _ := report["status"].(string); status != "" && status != "ok" {
		return Result{}, fmt.Errorf("filing: 引擎报告状态为 %q", status)
	}
	if _, err := os.Stat(outputPath); err != nil {
		return Result{}, fmt.Errorf("filing: 引擎未产出文件: %w", err)
	}

	// 成文与验收是两步：build.py 只负责排版，节数/编号/禁用词/禁用色/字号的断言在
	// verify.py。验收不通过时删除成品，避免半成品被当作交付件。
	//nolint:gosec // python 是探测到的解释器，参数由固定脚本与成品路径组成
	verifyCmd := exec.CommandContext(runCtx, python,
		prepared.VerifyScript,
		"--spec", prepared.Spec,
		"--docx", outputPath,
		"--template", prepared.Template,
	)
	verifyCmd.Dir = prepared.Dir
	var verifyOut strings.Builder
	verifyCmd.Stdout = &verifyOut
	verifyCmd.Stderr = &stderr
	if err := verifyCmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		if runCtx.Err() == context.DeadlineExceeded {
			return Result{}, fmt.Errorf("filing: 成品验收超时（%s）", timeout)
		}
		return Result{}, fmt.Errorf("filing: 成品验收未通过: %v；%s", err, verificationIssues(verifyOut.String()))
	}
	return Result{OutputPath: outputPath, Report: report}, nil
}

// verificationIssues 从 verify.py 的 JSON 输出中提取未通过项，供错误信息使用。
func verificationIssues(output string) string {
	start := strings.Index(output, "{")
	if start < 0 {
		return tail(output, 400)
	}
	var report struct {
		Passed bool     `json:"passed"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(output[start:]), &report); err != nil {
		return tail(output, 400)
	}
	if len(report.Errors) > 0 {
		return strings.Join(report.Errors, "；")
	}
	return tail(output, 400)
}

// findPython 探测能 import docx 的解释器。
//
// PATH 里的 python3 未必装了 python-docx（本机默认 3.15 未装），因此先试常见
// Homebrew 绝对路径，再退回 PATH 中的版本化命令。
func findPython(ctx context.Context, explicit string) (string, error) {
	candidates := make([]string, 0, 8)
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	candidates = append(candidates,
		"/opt/homebrew/bin/python3.13",
		"/opt/homebrew/bin/python3.12",
		"/opt/homebrew/bin/python3.11",
		"python3.13", "python3.12", "python3.11", "python3",
	)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		//nolint:gosec // path 由 LookPath 解析的固定解释器候选，命令体是常量
		check := exec.CommandContext(probeCtx, path, "-c", "import docx")
		err = check.Run()
		cancel()
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("filing: 未找到可 import docx 的 Python 解释器（需 python-docx）")
}

// tail 返回文本末尾至多 n 个字符。
func tail(text string, n int) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= n {
		return trimmed
	}
	return "…" + trimmed[len(trimmed)-n:]
}
