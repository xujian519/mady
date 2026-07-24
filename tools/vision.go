package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// VisionOperations defines pluggable operations for the vision tool.
type VisionOperations interface {
	// Analyze sends an image to a vision-capable LLM for analysis.
	Analyze(ctx context.Context, imageData []byte, mimeType string, prompt string) (string, error)
	// Download fetches an image from a URL.
	Download(ctx context.Context, url string) ([]byte, string, error)
}

// 视觉分析环境变量配置键。当 VisionToolConfig 未显式注入 Operations/Provider 时，
// defaults() 按以下环境变量构造 OpenAI 兼容的视觉分析客户端：
// MADY_VISION_MODEL（必填，多模态模型 id）、
// MADY_VISION_API_KEY（缺省回退 API_KEY）、
// MADY_VISION_BASE_URL（缺省回退 BASE_URL → OPENAI_BASE_URL，可传 base 地址或完整
// /chat/completions 路径）。
const (
	EnvVisionModel   = "MADY_VISION_MODEL"
	EnvVisionAPIKey  = "MADY_VISION_API_KEY" //#nosec G101 -- 环境变量名常量，非凭证
	EnvVisionBaseURL = "MADY_VISION_BASE_URL"
)

// DefaultVisionOperations uses an external OpenAI-compatible vision API
// (Chat Completions + image_url data URL).
type DefaultVisionOperations struct {
	// Model 是多模态模型 id（如 gpt-4o、glm-5v-turbo）。为空视为未配置。
	Model  string
	APIURL string
	APIKey string
	Client *http.Client
}

func (d DefaultVisionOperations) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	// Use SSRF-safe client to prevent access to internal/private network
	// ranges (loopback, cloud metadata endpoints, etc.). This reuses the
	// same dial control as web_fetch.go.
	return newSSRFSafeHTTPClient(60 * time.Second)
}

func (d DefaultVisionOperations) Download(ctx context.Context, url string) ([]byte, string, error) {
	// Validate URL.
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, "", fmt.Errorf("invalid URL: must start with http:// or https://")
	}

	req, err := newGetRequest(ctx, url)
	if err != nil {
		return nil, "", err
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Limit download size (50MB).
	const maxBytes = 50 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, "", err
	}

	// Detect MIME type.
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = detectImageMIME(data)
	}

	return data, mimeType, nil
}

// newGetRequest 构造带 ctx 的 GET 请求。
func newGetRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid image URL %q: %w", url, err)
	}
	return req, nil
}

// errVisionNotConfigured 在未配置任何视觉能力时返回，明确告知配置方式——
// 严禁返回伪造的占位分析结果（诚实性红线）。
func errVisionNotConfigured(missing ...string) error {
	msg := "视觉分析未配置"
	if len(missing) > 0 {
		msg += fmt.Sprintf("（缺少 %s）", strings.Join(missing, ", "))
	}
	msg += "：请通过 VisionToolConfig.Provider 接入多模态模型，" +
		"或设置 MADY_VISION_MODEL / MADY_VISION_API_KEY / MADY_VISION_BASE_URL 环境变量"
	return fmt.Errorf("%s", msg)
}

// visionChatRequest 是 OpenAI Chat Completions 兼容请求体（多模态最小集）。
type visionChatRequest struct {
	Model    string              `json:"model"`
	Messages []visionChatMessage `json:"messages"`
}

// visionChatMessage 是单条多模态消息：content 为文本/图片 part 列表。
type visionChatMessage struct {
	Role    string                  `json:"role"`
	Content []visionChatContentPart `json:"content"`
}

// visionChatContentPart 是文本或 image_url 内容块。
type visionChatContentPart struct {
	Type     string              `json:"type"`
	Text     string              `json:"text,omitempty"`
	ImageURL *visionChatImageURL `json:"image_url,omitempty"`
}

// visionChatImageURL 是 OpenAI image_url 块（支持 http(s) URL 与 data URL）。
type visionChatImageURL struct {
	URL string `json:"url"`
}

// visionChatResponse 是 Chat Completions 兼容响应体的最小集。
type visionChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Analyze 调用 OpenAI 兼容的多模态 Chat Completions 端点完成图片分析。
// Model/APIURL/APIKey 任一缺失时返回“未配置”错误，绝不返回占位文本。
func (d DefaultVisionOperations) Analyze(ctx context.Context, imageData []byte, mimeType string, prompt string) (string, error) {
	var missing []string
	if d.Model == "" {
		missing = append(missing, "model")
	}
	if d.APIURL == "" {
		missing = append(missing, "base URL")
	}
	if d.APIKey == "" {
		missing = append(missing, "API key")
	}
	if len(missing) > 0 {
		return "", errVisionNotConfigured(missing...)
	}

	apiURL := d.APIURL
	if !strings.Contains(apiURL, "/chat/completions") {
		apiURL = strings.TrimSuffix(apiURL, "/") + "/chat/completions"
	}

	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(imageData))
	reqBody := visionChatRequest{
		Model: d.Model,
		Messages: []visionChatMessage{
			{
				Role: "user",
				Content: []visionChatContentPart{
					{Type: "text", Text: prompt},
					{Type: "image_url", ImageURL: &visionChatImageURL{URL: dataURL}},
				},
			},
		},
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode vision request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(raw)))
	if err != nil {
		return "", fmt.Errorf("failed to build vision request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.APIKey)

	resp, err := d.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("vision API request failed: %w", err)
	}
	defer resp.Body.Close()

	// 错误响应体最多读取 4KB，避免异常大响应撑爆内存。
	const maxErrBody = 4 * 1024
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBody))
		return "", fmt.Errorf("vision API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed visionChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to decode vision API response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("视觉模型未返回文本内容（model: %s）", d.Model)
	}
	return parsed.Choices[0].Message.Content, nil
}

// detectImageMIME detects image MIME type from magic bytes.
func detectImageMIME(data []byte) string {
	if len(data) < 2 {
		return "application/octet-stream"
	}

	// JPEG: FF D8 FF
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}

	// PNG: 89 50 4E 47
	if len(data) >= 4 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}

	// GIF: GIF87a or GIF89a
	if len(data) >= 4 && data[0] == 'G' && data[1] == 'I' && data[2] == 'F' {
		return "image/gif"
	}

	// WebP: RIFF....WEBP
	if len(data) >= 12 && data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' &&
		data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P' {
		return "image/webp"
	}

	// BMP: BM
	if data[0] == 'B' && data[1] == 'M' {
		return "image/bmp"
	}

	// TIFF: II (little-endian) or MM (big-endian)
	if len(data) >= 2 {
		if magic := string(data[:2]); magic == "II" || magic == "MM" {
			return "image/tiff"
		}
	}

	return "application/octet-stream"
}

// VisionToolConfig configures the vision tool.
type VisionToolConfig struct {
	Operations VisionOperations
	MaxBytes   int64
	MaxPixels  int64

	// Provider and Model are optional. When set and Operations is nil,
	// the tool creates a default implementation that calls this provider.
	Provider agentcore.Provider
	Model    string

	// Sandbox enforces the WorkingDir boundary when Enabled.
	Sandbox WorkingDirSandbox
}

func (c *VisionToolConfig) defaults() {
	if c.Operations == nil {
		switch {
		case c.Provider != nil && c.Model != "":
			c.Operations = &providerVisionOperations{
				provider: c.Provider,
				model:    c.Model,
			}
		default:
			// 未显式注入 provider 时尝试环境变量配置；仍未配置则保留
			// 零值 DefaultVisionOperations——其 Analyze 返回明确的
			// “未配置”错误而非伪造的占位分析结果。
			if envOps := visionOperationsFromEnv(); envOps != nil {
				c.Operations = envOps
			} else {
				c.Operations = DefaultVisionOperations{}
			}
		}
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = 20 * 1024 * 1024 // 20MB base64
	}
	if c.MaxPixels <= 0 {
		c.MaxPixels = 4096 * 4096 // ~16MP
	}
}

// visionOperationsFromEnv 按 MADY_VISION_* 环境变量构造视觉分析操作。
// MADY_VISION_MODEL 为空时返回 nil，表示未通过环境变量配置视觉能力。
func visionOperationsFromEnv() VisionOperations {
	model := strings.TrimSpace(os.Getenv(EnvVisionModel))
	if model == "" {
		return nil
	}
	apiKey := strings.TrimSpace(os.Getenv(EnvVisionAPIKey))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("API_KEY"))
	}
	baseURL := strings.TrimSpace(os.Getenv(EnvVisionBaseURL))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("BASE_URL"))
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	}
	return DefaultVisionOperations{Model: model, APIKey: apiKey, APIURL: baseURL}
}

// providerVisionOperations implements VisionOperations using an agentcore.Provider.
type providerVisionOperations struct {
	provider agentcore.Provider
	model    string
}

func (o *providerVisionOperations) Analyze(ctx context.Context, imageData []byte, mimeType, prompt string) (string, error) {
	if o.provider == nil {
		return "", errVisionNotConfigured("provider")
	}
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(imageData))
	msg := agentcore.Message{Role: "user"}
	msg = msg.AppendTextBlock(prompt)
	msg = msg.AppendImageURLBlock(dataURL)

	resp, err := o.provider.Complete(ctx, &agentcore.ProviderRequest{
		Model:    o.model,
		Messages: []agentcore.Message{msg},
	})
	if err != nil {
		return "", err
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return "", fmt.Errorf("视觉模型未返回文本内容（model: %s）", o.model)
	}
	return resp.Content, nil
}

func (o *providerVisionOperations) Download(ctx context.Context, url string) ([]byte, string, error) {
	return DefaultVisionOperations{}.Download(ctx, url)
}

// VisionToolInput is the JSON arguments for the vision tool.
type VisionToolInput struct {
	Image  string `json:"image"`            // URL or local path
	Prompt string `json:"prompt"`           // Question about the image
	Base64 string `json:"base64,omitempty"` // Direct base64 data
}

// VisionToolDetails carries vision metadata.
type VisionToolDetails struct {
	ImageSize    int    `json:"image_size"`
	MIMEType     string `json:"mime_type"`
	Base64Size   int    `json:"base64_size"`
	Resized      bool   `json:"resized"`
	OriginalSize int    `json:"original_size,omitempty"`
}

// NewVisionTool creates an image analysis tool.
func NewVisionTool(cwd string, cfg *VisionToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &VisionToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name: "vision_analyze",
		Description: "使用支持视觉能力的 LLM 分析图片。" +
			"可以提供图片 URL、本地文件路径或 base64 编码的图片数据。" +
			"超出大小限制的图片会自动调整大小。" +
			"支持的格式：JPEG、PNG、GIF、WebP、BMP、TIFF。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"image": map[string]any{
					"type":        "string",
					"description": "图片的 URL 或本地文件路径",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "关于图片的问题或指令（例如'描述这张图片'、'图片中显示什么文字？'）",
				},
				"base64": map[string]any{
					"type":        "string",
					"description": "Base64 编码的图片数据（替代图片 URL/路径）",
				},
			},
			"required": []any{"prompt"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input VisionToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}

			if input.Prompt == "" {
				return resultErrf("prompt is required")
			}

			if input.Image == "" && input.Base64 == "" {
				return resultErrf("either image (URL/path) or base64 is required")
			}

			var imageData []byte
			var mimeType string
			var err error
			resized := false
			originalSize := 0

			if input.Base64 != "" {
				// Decode base64.
				imageData, err = base64.StdEncoding.DecodeString(input.Base64)
				if err != nil {
					return resultErrf("invalid base64 data: %w", err)
				}
				mimeType = detectImageMIME(imageData)
			} else {
				// Load from URL or file.
				if strings.HasPrefix(input.Image, "http://") || strings.HasPrefix(input.Image, "https://") {
					imageData, mimeType, err = cfg.Operations.Download(ctx, input.Image)
					if err != nil {
						return resultErrf("failed to download image: %w", err)
					}
				} else {
					// Local file.
					resolved, err := resolvePathSandboxed(input.Image, cwd, cfg.Sandbox)
					if err != nil {
						return resultErrf("%w", err)
					}
					imageData, err = os.ReadFile(resolved)
					if err != nil {
						return resultErrf("failed to read image file: %w", err)
					}
					mimeType = detectImageMIME(imageData)
					if extMime := mimeFromExt(resolved); extMime != "" {
						mimeType = extMime
					}
				}
			}

			originalSize = len(imageData)

			// Validate it's an image.
			if !strings.HasPrefix(mimeType, "image/") {
				return resultErrf("not an image file (detected: %s)", mimeType)
			}

			// Check size limits.
			base64Size := len(base64.StdEncoding.EncodeToString(imageData))
			if int64(base64Size) > cfg.MaxBytes {
				return resultErrf(
					"image too large: %s base64 (limit: %s). "+
						"Consider resizing the image before sending.",
					FormatSize(int64(base64Size)), FormatSize(cfg.MaxBytes),
				)
			}

			// Call vision API.
			analysis, err := cfg.Operations.Analyze(ctx, imageData, mimeType, input.Prompt)
			if err != nil {
				return resultErrf("vision analysis failed: %w", err)
			}

			return result(analysis, VisionToolDetails{
				ImageSize:    len(imageData),
				MIMEType:     mimeType,
				Base64Size:   base64Size,
				Resized:      resized,
				OriginalSize: originalSize,
			})
		},
	}
}

// mimeFromExt gets MIME type from file extension.
func mimeFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	case ".svg":
		return "image/svg+xml"
	default:
		return ""
	}
}
