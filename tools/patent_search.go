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

	"github.com/xujian519/mady/agentcore"
)

// PatentToolConfig 配置 nuo-patent CLI 工具。
type PatentToolConfig struct {
	// NuoPatentPath 是 nuo-patent CLI 入口，默认为本地构建路径。
	//   如 "node /Users/xujian/projects/nuo-patent/dist/cli.js"
	NuoPatentPath string

	// DownloadDir 是专利 PDF 下载目录，默认 /tmp/mady-patents/。
	DownloadDir string

	// Timeout 是单次请求超时秒数，默认 30。
	Timeout int
}

// PatentToolConfigDefaults 返回默认配置。
// NuoPatentPath 优先从 NUO_PATENT_PATH 环境变量读取，回退到可执行名 "nuo-patent"。
// DownloadDir 默认位于系统临时目录下。
func PatentToolConfigDefaults() *PatentToolConfig {
	path := os.Getenv("NUO_PATENT_PATH")
	if path == "" {
		path = "nuo-patent"
	}
	return &PatentToolConfig{
		NuoPatentPath: path,
		DownloadDir:   filepath.Join(os.TempDir(), "mady-patents"),
		Timeout:       30,
	}
}

// runNuoPatent 执行 nuo-patent CLI 命令，返回 stdout。
func runNuoPatent(ctx context.Context, bin string, args ...string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer, error) {
	// 拆分 bin（如 "node /path/to/cli.js" → ["node", "/path/to/cli.js"]）
	parts := strings.Fields(bin)
	var argv []string
	argv = append(argv, parts...)
	argv = append(argv, args...)

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	return cmd, &stdout, &stderr, nil
}

// NewPatentScrapeTool 专利元数据查询（通过 nuo-patent Google Patents）。
func NewPatentScrapeTool(cfg *PatentToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = PatentToolConfigDefaults()
	}
	return &agentcore.Tool{
		Name:        "patent_lookup",
		Description: "查询专利元数据。输入专利号（如 US11452699B2、CN114526990A），返回标题、摘要、发明人、IPC分类号、法律状态等。支持中、美、欧、日、韩等各国专利。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patent_number": map[string]any{
					"type":        "string",
					"description": "专利申请号或公开号",
				},
			},
			"required": []any{"patent_number"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				PatentNumber string `json:"patent_number"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid input: %w", err)
			}

			cmd, stdout, stderr, err := runNuoPatent(ctx, cfg.NuoPatentPath, "scrape", input.PatentNumber, "--pretty")
			if err != nil {
				return nil, fmt.Errorf("prepare nuo-patent: %w", err)
			}
			if err := cmd.Run(); err != nil {
				return map[string]any{
					"patent_number": input.PatentNumber,
					"success":       false,
					"error":         fmt.Sprintf("%v: %s", err, stderr.String()),
				}, nil
			}
			return parseJSONOrRaw(stdout.String()), nil
		},
		ReadOnly: true,
	}
}

// NewPatentDownloadTool PDF 下载。
func NewPatentDownloadTool(cfg *PatentToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = PatentToolConfigDefaults()
	}
	return &agentcore.Tool{
		Name:        "patent_download",
		Description: "下载专利 PDF。输入专利号（多个用空格分隔），将 PDF 保存到本地。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patent_numbers": map[string]any{
					"type":        "string",
					"description": "专利号，多个用空格分隔",
				},
			},
			"required": []any{"patent_numbers"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				PatentNumbers string `json:"patent_numbers"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid input: %w", err)
			}

			numbers := strings.Fields(input.PatentNumbers)
			if len(numbers) == 0 {
				return nil, fmt.Errorf("至少需要一个专利号")
			}
			if err := os.MkdirAll(cfg.DownloadDir, 0755); err != nil {
				return nil, fmt.Errorf("create download dir: %w", err)
			}

			allArgs := []string{"download", "--output", cfg.DownloadDir, "--max-workers", "4"}
			allArgs = append(allArgs, numbers...)

			cmd, stdout, stderr, err := runNuoPatent(ctx, cfg.NuoPatentPath, allArgs...)
			if err != nil {
				return nil, fmt.Errorf("prepare nuo-patent: %w", err)
			}
			if err := cmd.Run(); err != nil {
				return nil, fmt.Errorf("专利下载失败: %w\n%s", err, stderr.String())
			}
			return parseJSONOrRaw(stdout.String()), nil
		},
	}
}

// NewPatentLegalStatusTool 法律状态/年费查询。
func NewPatentLegalStatusTool(cfg *PatentToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = PatentToolConfigDefaults()
	}
	return &agentcore.Tool{
		Name:        "patent_legal",
		Description: "查询专利法律状态和年费信息。输入一个或多个专利号。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patent_numbers": map[string]any{
					"type": "string",
				},
			},
			"required": []any{"patent_numbers"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				PatentNumbers string `json:"patent_numbers"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid input: %w", err)
			}

			numbers := strings.Fields(input.PatentNumbers)
			if len(numbers) == 0 {
				return nil, fmt.Errorf("至少需要一个专利号")
			}

			allArgs := append([]string{"legal-status", "--max-concurrency", "4"}, numbers...)
			cmd, stdout, stderr, err := runNuoPatent(ctx, cfg.NuoPatentPath, allArgs...)
			if err != nil {
				return nil, fmt.Errorf("prepare nuo-patent: %w", err)
			}
			if err := cmd.Run(); err != nil {
				return nil, fmt.Errorf("法律状态查询失败: %w\n%s", err, stderr.String())
			}
			return parseJSONOrRaw(stdout.String()), nil
		},
	}
}

// parseJSONOrRaw 尝试解析 JSON，失败返回原文。
func parseJSONOrRaw(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	return v
}
