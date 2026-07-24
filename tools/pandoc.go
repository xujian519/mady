package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// PandocToolConfig configures the document conversion tool powered by Pandoc.
type PandocToolConfig struct {
	// PandocPath is the pandoc executable path. Defaults to PATH lookup for "pandoc".
	PandocPath string

	// Timeout is the maximum execution time in seconds. Default 120.
	Timeout int

	// WorkingDir constrains input/output paths when non-empty.
	WorkingDir string
}

// PandocToolConfigDefaults returns sensible defaults.
func PandocToolConfigDefaults() *PandocToolConfig {
	return &PandocToolConfig{
		PandocPath: "pandoc",
		Timeout:    120,
	}
}

// PandocToolInput is the JSON arguments for the convert_document tool.
type PandocToolInput struct {
	Input        string `json:"input"`
	FromFormat   string `json:"from_format,omitempty"`
	ToFormat     string `json:"to_format"`
	Output       string `json:"output,omitempty"`
	ReferenceDoc string `json:"reference_doc,omitempty"`
	TOC          bool   `json:"toc,omitempty"`
	Standalone   bool   `json:"standalone,omitempty"`
}

// NewPandocTool creates a document conversion tool backed by Pandoc.
// It converts between formats: markdown, docx, html, pdf, epub, rst, latex, etc.
// Pandoc must be installed on the system (pandoc.org).
func NewPandocTool(cfg *PandocToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = PandocToolConfigDefaults()
	}
	if cfg.PandocPath == "" {
		cfg.PandocPath = "pandoc"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120
	}

	return &agentcore.Tool{
		Name:        "convert_document",
		Description: "文档格式转换（需系统安装 Pandoc）。支持 Markdown、DOCX、HTML、PDF、EPUB、LaTeX、RST 等格式互转。适用于将专利文档从 Markdown 转为 DOCX 提交专利局，或将客户 DOCX 转为 Markdown 分析。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "输入文件路径（相对工作目录）",
				},
				"from_format": map[string]any{
					"type":        "string",
					"description": "输入格式（可选，如 markdown/docx/html，默认自动检测）",
				},
				"to_format": map[string]any{
					"type":        "string",
					"description": "目标格式（markdown/docx/html/pdf/epub/latex/rst 等）",
				},
				"output": map[string]any{
					"type":        "string",
					"description": "输出文件路径（可选，不指定则返回转换后的文本内容）",
				},
				"reference_doc": map[string]any{
					"type":        "string",
					"description": "参考文档模板路径（用于 DOCX/PPTX 样式继承，如专利局标准模板）",
				},
				"toc": map[string]any{
					"type":        "boolean",
					"description": "是否生成目录（默认 false）",
				},
				"standalone": map[string]any{
					"type":        "boolean",
					"description": "是否生成完整独立文档（含 header/footer，默认 false）",
				},
			},
			"required": []any{"input", "to_format"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input PandocToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}
			if input.Input == "" {
				return resultErrf("input is required")
			}
			if input.ToFormat == "" {
				return resultErrf("to_format is required")
			}

			inputPath, err := cfg.resolvePath(input.Input)
			if err != nil {
				return resultErrf("input path: %w", err)
			}
			if _, err := os.Stat(inputPath); err != nil {
				return resultErrf("input file not found: %w", err)
			}

			pandocArgs := []string{}
			if input.FromFormat != "" {
				pandocArgs = append(pandocArgs, "-f", input.FromFormat)
			}
			pandocArgs = append(pandocArgs, "-t", input.ToFormat)

			if input.ReferenceDoc != "" {
				refPath, err := cfg.resolvePath(input.ReferenceDoc)
				if err != nil {
					return resultErrf("reference_doc path: %w", err)
				}
				pandocArgs = append(pandocArgs, "--reference-doc", refPath)
			}
			if input.TOC {
				pandocArgs = append(pandocArgs, "--toc")
			}
			if input.Standalone || input.Output != "" {
				pandocArgs = append(pandocArgs, "-s")
			}

			writeToOutput := input.Output != ""
			useStdin := input.Input == "-"

			if writeToOutput {
				outputPath, err := cfg.resolvePath(input.Output)
				if err != nil {
					return resultErrf("output path: %w", err)
				}
				if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
					return resultErrf("create output dir: %w", err)
				}
				pandocArgs = append(pandocArgs, "-o", outputPath)
				if !useStdin {
					pandocArgs = append(pandocArgs, inputPath)
				}
			} else if !useStdin {
				pandocArgs = append(pandocArgs, inputPath)
			}

			timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Second)
			defer cancel()

			cmd := exec.CommandContext(timeoutCtx, cfg.PandocPath, pandocArgs...)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				errMsg := err.Error()
				if stderr.Len() > 0 {
					errMsg = fmt.Sprintf("%s: %s", err, strings.TrimSpace(stderr.String()))
				}
				return resultErrf("pandoc conversion failed: %s", errMsg)
			}

			if writeToOutput {
				return result(fmt.Sprintf("转换成功：%s → %s（%s）",
					filepath.Base(inputPath), input.Output, input.ToFormat), nil)
			}

			content := stdout.String()
			truncation := TruncateHead(content, TruncationOptions{
				MaxBytes: int(DefaultMaxBytes * 4),
				MaxLines: DefaultMaxLines,
			})
			output := truncation.Content
			details := map[string]any{
				"from_format": input.FromFormat,
				"to_format":   input.ToFormat,
			}
			if truncation.Truncated {
				details["truncation"] = &truncation
				output += fmt.Sprintf("\n\n[output truncated at %s]", FormatSize(int64(DefaultMaxBytes*4)))
			}
			return result(output, details)
		},
	}
}

func (c *PandocToolConfig) resolvePath(p string) (string, error) {
	if p == "-" {
		return p, nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if c.WorkingDir != "" {
		wd, _ := filepath.Abs(c.WorkingDir)
		rel, err := filepath.Rel(wd, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("path %q escapes working directory", p)
		}
	}
	return abs, nil
}
