package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ReadOperations defines pluggable filesystem operations for the read tool.
type ReadOperations interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (os.FileInfo, error)
}

// DefaultReadOperations uses the local filesystem.
type DefaultReadOperations struct{}

func (d DefaultReadOperations) ReadFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func (d DefaultReadOperations) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }

// ReadToolConfig configures the read tool.
type ReadToolConfig struct {
	Operations ReadOperations
	MaxBytes   int64
	MaxLines   int64
	Sandbox    WorkingDirSandbox
}

func (c *ReadToolConfig) defaults() {
	if c.Operations == nil {
		c.Operations = DefaultReadOperations{}
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = DefaultMaxBytes
	}
	if c.MaxLines <= 0 {
		c.MaxLines = DefaultMaxLines
	}
}

// ReadToolInput is the JSON arguments for the read tool.
type ReadToolInput struct {
	Path   string `json:"path"`
	Offset *int   `json:"offset,omitempty"`
	Limit  *int   `json:"limit,omitempty"`
}

// ReadToolDetails carries truncation metadata.
type ReadToolDetails struct {
	Truncation *TruncationResult `json:"truncation,omitempty"`
}

// NewReadTool creates a read file tool.
func NewReadTool(cwd string, cfg *ReadToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &ReadToolConfig{}
	}
	cfg.defaults()
	cfg.Sandbox.WorkingDir = cwd

	return &agentcore.Tool{
		Name:        "read",
		Description: fmt.Sprintf("读取文件内容。返回完整文本内容。输出会被截断至 %d 行或 %s（以先达到的为准）。使用 offset 和 limit 参数读取指定段落。", cfg.MaxLines, FormatSize(cfg.MaxBytes)),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "要读取的文件路径（相对或绝对路径）"},
				"offset": map[string]any{"type": "integer", "description": "开始读取的行号（从 1 开始计数）"},
				"limit":  map[string]any{"type": "integer", "description": "要读取的最大行数"},
			},
			"required": []any{"path"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input ReadToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}

			resolved, err := resolvePathSandboxed(input.Path, cwd, cfg.Sandbox)
			if err != nil {
				return resultErrf("%v", err)
			}
			info, err := cfg.Operations.Stat(resolved)
			if err != nil {
				return resultErrf("file not found: %s", input.Path)
			}
			if info.IsDir() {
				entries, err := os.ReadDir(resolved)
				if err != nil {
					return resultErrf("cannot read directory: %s", input.Path)
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, "Directory listing for %s:\n", input.Path)
				for _, entry := range entries {
					name := entry.Name()
					if entry.IsDir() {
						name += "/"
					}
					info, _ := entry.Info()
					size := ""
					if info != nil && !entry.IsDir() {
						size = fmt.Sprintf("  (%s)", FormatSize(info.Size()))
					}
					fmt.Fprintf(&sb, "  %s%s\n", name, size)
				}
				return result(sb.String(), nil)
			}

			data, err := cfg.Operations.ReadFile(resolved)
			if err != nil {
				return resultErrf("failed to read file: %w", err)
			}

			if isImageFile(resolved, data) {
				return resultErrf("Cannot read %q (this model does not support image input). Inform the user.", input.Path)
			}

			content := string(data)

			// Apply offset/limit if specified.
			if input.Offset != nil || input.Limit != nil {
				lines := strings.Split(content, "\n")
				offset := 1
				if input.Offset != nil && *input.Offset > 0 {
					offset = *input.Offset
				}
				limit := len(lines)
				if input.Limit != nil && *input.Limit > 0 {
					limit = *input.Limit
				}
				start := offset - 1
				if start < 0 {
					start = 0
				}
				end := start + limit
				if end > len(lines) {
					end = len(lines)
				}
				content = strings.Join(lines[start:end], "\n")
			}

			truncation := TruncateHead(content, TruncationOptions{
				MaxLines: int(cfg.MaxLines),
				MaxBytes: int(cfg.MaxBytes),
			})

			output := truncation.Content
			if truncation.Truncated {
				notices := []string{}
				if truncation.TruncatedBy == "lines" {
					notices = append(notices, fmt.Sprintf("%d lines limit reached", cfg.MaxLines))
				} else {
					notices = append(notices, fmt.Sprintf("%s limit reached", FormatSize(cfg.MaxBytes)))
				}
				if truncation.FirstLineExceeds {
					notices = append(notices, "first line exceeds byte limit")
				}
				output += fmt.Sprintf("\n\n[%s]", strings.Join(notices, ". "))
			}

			return result(output, ReadToolDetails{Truncation: &truncation})
		},
	}
}

func isImageFile(path string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico", ".tiff", ".tif":
		return true
	}

	mimeType := mime.TypeByExtension(ext)
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}

	// Detect images that don't match their extension
	if mimeType := detectImageMIME(data); mimeType != "application/octet-stream" {
		return true
	}

	return false
}
