package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/ocr"
)

type OCRToolConfig struct {
	// WorkingDir is the base directory for relative paths.
	WorkingDir string
	Sandbox    WorkingDirSandbox

	// CacheDir overrides the default OCR model cache location.
	CacheDir string
}

func (c *OCRToolConfig) defaults() {
	if c.WorkingDir == "" {
		c.WorkingDir, _ = os.Getwd()
	}
	if c.CacheDir == "" {
		c.CacheDir = ocr.DefaultCacheDir()
	}
}

type OCRToolInput struct {
	Path string `json:"path"`
}

func NewOCRTool(cfg *OCRToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &OCRToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name: "ocr",
		Description: "对本地图片做 OCR 识别，返回图片中的全部文字。基于 PaddleOCR PP-OCRv5，" +
			"支持中英文、水平文本和倾斜文本。首次调用会自动下载 ~40MB 模型到 " +
			ocr.DefaultCacheDir() + "，后续秒级响应。" +
			"支持 PNG/JPEG/GIF 格式。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "图片的本地文件路径，支持 PNG/JPEG/GIF",
				},
			},
			"required": []any{"path"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input OCRToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}
			if strings.TrimSpace(input.Path) == "" {
				return resultErrf("path is required")
			}

			abs, err := resolvePathSandboxed(input.Path, cfg.WorkingDir, cfg.Sandbox)
			if err != nil {
				return resultErrf("path not allowed: %w", err)
			}

			engine := ocr.Global()
			if !engine.IsReady() {
				if err := engine.EnsureAssets(nil); err != nil {
					return resultErrf("OCR model download failed: %v\nCache dir: %s", err, engine.CacheDir())
				}
			}

			text, err := engine.RecognizeText(abs)
			if err != nil {
				return resultErrf("OCR failed: %v", err)
			}
			if strings.TrimSpace(text) == "" {
				return result("(no text found in image)", nil)
			}
			return result(text, nil)
		},
		ReadOnly: true,
	}
}

func resolveOCRCacheDir() string {
	d := ocr.DefaultCacheDir()
	os.MkdirAll(d, 0755)
	return d
}

func SweepOCRCache(maxAge int64) error {
	cacheDir := filepath.Join(ocr.DefaultCacheDir(), "cache")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Unix() < maxAge {
			os.Remove(filepath.Join(cacheDir, e.Name()))
		}
	}
	return nil
}
