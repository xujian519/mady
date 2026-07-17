package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestVisionToolLocalFile(t *testing.T) {
	// 未配置任何视觉能力时必须返回明确错误，而非伪造的占位分析。
	t.Setenv(EnvVisionModel, "")
	tmpDir := t.TempDir()

	// Create a minimal valid PNG file (1x1 pixel).
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xD7, 0x63, 0xF8, 0x0F, 0x00, 0x00,
		0x01, 0x01, 0x00, 0x05, 0x18, 0xD8, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}
	testFile := filepath.Join(tmpDir, "test.png")
	os.WriteFile(testFile, pngData, 0644)

	tool := NewVisionTool(tmpDir, nil)

	args, _ := json.Marshal(map[string]string{
		"image":  "test.png",
		"prompt": "Describe this image",
	})
	_, err := tool.Func(context.Background(), args)
	if err == nil {
		t.Fatal("expected error when vision is not configured")
	}
	if !strings.Contains(err.Error(), "视觉分析未配置") {
		t.Errorf("expected not-configured error, got: %v", err)
	}
	if strings.Contains(err.Error(), "placeholder") {
		t.Errorf("must not return placeholder text, got: %v", err)
	}
}

func TestVisionToolBase64(t *testing.T) {
	// 未配置任何视觉能力时必须返回明确错误，而非伪造的占位分析。
	t.Setenv(EnvVisionModel, "")
	tmpDir := t.TempDir()
	tool := NewVisionTool(tmpDir, nil)

	// Minimal PNG as base64.
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xD7, 0x63, 0xF8, 0x0F, 0x00, 0x00,
		0x01, 0x01, 0x00, 0x05, 0x18, 0xD8, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}
	b64 := base64.StdEncoding.EncodeToString(pngData)

	args, _ := json.Marshal(map[string]string{
		"base64": b64,
		"prompt": "What's in this image?",
	})
	_, err := tool.Func(context.Background(), args)
	if err == nil {
		t.Fatal("expected error when vision is not configured")
	}
	if !strings.Contains(err.Error(), "视觉分析未配置") {
		t.Errorf("expected not-configured error, got: %v", err)
	}
}

func TestVisionToolMissingImage(t *testing.T) {
	tmpDir := t.TempDir()
	tool := NewVisionTool(tmpDir, nil)

	args, _ := json.Marshal(map[string]string{
		"prompt": "Describe this image",
	})
	_, err := tool.Func(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for missing image")
	}
	if !strings.Contains(err.Error(), "either image") {
		t.Errorf("expected image required error, got: %v", err)
	}
}

func TestVisionToolInvalidBase64(t *testing.T) {
	tmpDir := t.TempDir()
	tool := NewVisionTool(tmpDir, nil)

	args, _ := json.Marshal(map[string]string{
		"base64": "not-valid-base64!!!",
		"prompt": "Describe",
	})
	_, err := tool.Func(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
	if !strings.Contains(err.Error(), "invalid base64") {
		t.Errorf("expected base64 error, got: %v", err)
	}
}

func TestDetectImageMIME(t *testing.T) {
	tests := []struct {
		data     []byte
		expected string
	}{
		{[]byte{0xFF, 0xD8, 0xFF}, "image/jpeg"},
		{[]byte{0x89, 0x50, 0x4E, 0x47}, "image/png"},
		{[]byte{'G', 'I', 'F', '8'}, "image/gif"},
		{[]byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'}, "image/webp"},
		{[]byte{'B', 'M'}, "image/bmp"},
		{[]byte{}, "application/octet-stream"},
	}

	for _, tt := range tests {
		result := detectImageMIME(tt.data)
		if result != tt.expected {
			t.Errorf("detectImageMIME(%v) = %s, want %s", tt.data, result, tt.expected)
		}
	}
}

func TestMimeFromExt(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"test.jpg", "image/jpeg"},
		{"test.jpeg", "image/jpeg"},
		{"test.png", "image/png"},
		{"test.gif", "image/gif"},
		{"test.webp", "image/webp"},
		{"test.bmp", "image/bmp"},
		{"test.tiff", "image/tiff"},
		{"test.tif", "image/tiff"},
		{"test.svg", "image/svg+xml"},
		{"test.unknown", ""},
	}

	for _, tt := range tests {
		result := mimeFromExt(tt.path)
		if result != tt.expected {
			t.Errorf("mimeFromExt(%s) = %s, want %s", tt.path, result, tt.expected)
		}
	}
}

// tinyPNG 返回 1x1 像素的最小合法 PNG。
func tinyPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xD7, 0x63, 0xF8, 0x0F, 0x00, 0x00,
		0x01, 0x01, 0x00, 0x05, 0x18, 0xD8, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}
}

// mockVisionProvider 实现 agentcore.Provider，用于捕获视觉请求并返回预置响应。
type mockVisionProvider struct {
	lastReq *agentcore.ProviderRequest
	lastCtx context.Context
	resp    *agentcore.ProviderResponse
	err     error
}

func (m *mockVisionProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	m.lastCtx = ctx
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func (m *mockVisionProvider) Stream(context.Context, *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, fmt.Errorf("stream not implemented in mock")
}

// TestProviderVisionOperationsRequest 验证 provider 路径的请求构造：
// 模型 id 透传、prompt 为文本块、图片为 data URL 图像块、ctx 原样传递。
func TestProviderVisionOperationsRequest(t *testing.T) {
	mock := &mockVisionProvider{
		resp: &agentcore.ProviderResponse{Content: "图中是一只猫"},
	}
	ops := &providerVisionOperations{provider: mock, model: "glm-5v-turbo"}

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "trace-1")

	got, err := ops.Analyze(ctx, tinyPNG(), "image/png", "描述这张图片")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got != "图中是一只猫" {
		t.Errorf("content = %q", got)
	}
	if mock.lastCtx != ctx {
		t.Errorf("caller ctx not propagated to provider")
	}
	if mock.lastReq == nil {
		t.Fatal("provider did not receive a request")
	}
	if mock.lastReq.Model != "glm-5v-turbo" {
		t.Errorf("model = %q", mock.lastReq.Model)
	}
	if len(mock.lastReq.Messages) != 1 {
		t.Fatalf("messages = %d", len(mock.lastReq.Messages))
	}
	msg := mock.lastReq.Messages[0]
	if msg.Role != "user" {
		t.Errorf("role = %q", msg.Role)
	}

	var textOK, imageOK bool
	for _, bl := range msg.Blocks {
		switch bl.Kind {
		case agentcore.BlockKindText:
			if bl.Text == "描述这张图片" {
				textOK = true
			}
		case agentcore.BlockKindImage:
			wantPrefix := "data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG())[:16]
			if strings.HasPrefix(bl.URL, wantPrefix) {
				imageOK = true
			}
		}
	}
	if !textOK {
		t.Errorf("prompt text block missing: %#v", msg.Blocks)
	}
	if !imageOK {
		t.Errorf("image data URL block missing: %#v", msg.Blocks)
	}
}

// TestProviderVisionOperationsError 验证 provider 错误原样返回，不伪造内容。
func TestProviderVisionOperationsError(t *testing.T) {
	mock := &mockVisionProvider{err: fmt.Errorf("upstream 429")}
	ops := &providerVisionOperations{provider: mock, model: "m"}
	_, err := ops.Analyze(context.Background(), tinyPNG(), "image/png", "q")
	if err == nil || !strings.Contains(err.Error(), "upstream 429") {
		t.Fatalf("expected upstream error, got: %v", err)
	}
}

// TestProviderVisionOperationsEmptyContent 验证模型返回空文本时给出明确错误。
func TestProviderVisionOperationsEmptyContent(t *testing.T) {
	mock := &mockVisionProvider{resp: &agentcore.ProviderResponse{Content: "  "}}
	ops := &providerVisionOperations{provider: mock, model: "m"}
	_, err := ops.Analyze(context.Background(), tinyPNG(), "image/png", "q")
	if err == nil || !strings.Contains(err.Error(), "未返回文本内容") {
		t.Fatalf("expected empty-content error, got: %v", err)
	}
}

// TestDefaultVisionOperationsNotConfigured 验证零值配置返回明确错误而非占位文本。
func TestDefaultVisionOperationsNotConfigured(t *testing.T) {
	_, err := DefaultVisionOperations{}.Analyze(context.Background(), tinyPNG(), "image/png", "q")
	if err == nil {
		t.Fatal("expected not-configured error")
	}
	if !strings.Contains(err.Error(), "视觉分析未配置") {
		t.Errorf("error = %v", err)
	}
	for _, field := range []string{"model", "base URL", "API key"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("error should mention missing %s: %v", field, err)
		}
	}
	if strings.Contains(err.Error(), "placeholder") {
		t.Errorf("must not contain placeholder text: %v", err)
	}
}

// TestDefaultVisionOperationsHTTPFlow 用 httptest 验证 OpenAI 兼容请求构造与响应解析。
func TestDefaultVisionOperationsHTTPFlow(t *testing.T) {
	var gotAuth, gotPath, gotMethod string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"页面标题是示例"}}]}`))
	}))
	defer srv.Close()

	ops := DefaultVisionOperations{
		Model:  "gpt-4o-mini",
		APIURL: srv.URL + "/v1", // 传 base 地址，应自动拼接 /chat/completions
		APIKey: "test-key",
	}
	got, err := ops.Analyze(context.Background(), tinyPNG(), "image/png", "标题是什么？")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got != "页面标题是示例" {
		t.Errorf("content = %q", got)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s", gotMethod)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %s", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if gotBody["model"] != "gpt-4o-mini" {
		t.Errorf("body model = %#v", gotBody["model"])
	}
	msgs, ok := gotBody["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("body messages = %#v", gotBody["messages"])
	}
	parts, ok := msgs[0].(map[string]any)["content"].([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("body content parts = %#v", msgs[0])
	}
	if parts[0].(map[string]any)["type"] != "text" ||
		parts[0].(map[string]any)["text"] != "标题是什么？" {
		t.Errorf("text part = %#v", parts[0])
	}
	imgPart := parts[1].(map[string]any)
	if imgPart["type"] != "image_url" {
		t.Fatalf("image part type = %#v", imgPart["type"])
	}
	imgURL := imgPart["image_url"].(map[string]any)["url"].(string)
	if !strings.HasPrefix(imgURL, "data:image/png;base64,") {
		t.Errorf("image_url.url prefix = %q", imgURL[:min(40, len(imgURL))])
	}
}

// TestDefaultVisionOperationsHTTPError 验证非 200 响应返回带状态码的错误。
func TestDefaultVisionOperationsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	ops := DefaultVisionOperations{Model: "m", APIURL: srv.URL, APIKey: "k"}
	_, err := ops.Analyze(context.Background(), tinyPNG(), "image/png", "q")
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected HTTP 500 error, got: %v", err)
	}
}

// TestVisionToolConfigEnvFallback 验证 MADY_VISION_* 环境变量兜底逻辑。
func TestVisionToolConfigEnvFallback(t *testing.T) {
	t.Run("未设置模型时不启用 env 配置", func(t *testing.T) {
		t.Setenv(EnvVisionModel, "")
		cfg := &VisionToolConfig{}
		cfg.defaults()
		dvo, ok := cfg.Operations.(DefaultVisionOperations)
		if !ok {
			t.Fatalf("Operations type = %T", cfg.Operations)
		}
		if dvo.Model != "" || dvo.APIURL != "" || dvo.APIKey != "" {
			t.Errorf("unexpected env config: %+v", dvo)
		}
	})

	t.Run("设置模型后按环境变量构造", func(t *testing.T) {
		t.Setenv(EnvVisionModel, "glm-5v-turbo")
		t.Setenv(EnvVisionAPIKey, "vision-key")
		t.Setenv(EnvVisionBaseURL, "https://vision.example.com/v1")
		cfg := &VisionToolConfig{}
		cfg.defaults()
		dvo, ok := cfg.Operations.(DefaultVisionOperations)
		if !ok {
			t.Fatalf("Operations type = %T", cfg.Operations)
		}
		if dvo.Model != "glm-5v-turbo" || dvo.APIKey != "vision-key" ||
			dvo.APIURL != "https://vision.example.com/v1" {
			t.Errorf("env config = %+v", dvo)
		}
	})

	t.Run("key/base 回退到通用环境变量", func(t *testing.T) {
		t.Setenv(EnvVisionModel, "m")
		t.Setenv(EnvVisionAPIKey, "")
		t.Setenv(EnvVisionBaseURL, "")
		t.Setenv("API_KEY", "generic-key")
		t.Setenv("BASE_URL", "https://generic.example.com/v1")
		cfg := &VisionToolConfig{}
		cfg.defaults()
		dvo := cfg.Operations.(DefaultVisionOperations)
		if dvo.APIKey != "generic-key" || dvo.APIURL != "https://generic.example.com/v1" {
			t.Errorf("fallback config = %+v", dvo)
		}
	})
}

// TestVisionToolEnvConfiguredEndToEnd 验证 env 配置下 vision_analyze 工具全链路：
// base64 图片 → OpenAI 兼容端点 → 返回真实分析文本。
func TestVisionToolEnvConfiguredEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"env 全链路分析结果"}}]}`))
	}))
	defer srv.Close()

	t.Setenv(EnvVisionModel, "test-vision")
	t.Setenv(EnvVisionAPIKey, "k")
	t.Setenv(EnvVisionBaseURL, srv.URL)

	tool := NewVisionTool(t.TempDir(), nil)
	args, _ := json.Marshal(map[string]string{
		"base64": base64.StdEncoding.EncodeToString(tinyPNG()),
		"prompt": "描述",
	})
	res, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("tool.Func: %v", err)
	}
	tr := res.(ToolResult)
	if tr.Content != "env 全链路分析结果" {
		t.Errorf("content = %q", tr.Content)
	}
}

// TestAnalyzeBrowserScreenshotNotConfigured 验证浏览器截图分析在未配置时返回明确错误。
func TestAnalyzeBrowserScreenshotNotConfigured(t *testing.T) {
	t.Setenv(EnvVisionModel, "")
	cfg := &BrowserToolConfig{}
	cfg.defaults()
	_, err := analyzeBrowserScreenshot(context.Background(), cfg, "页面上有什么？", tinyPNG())
	if err == nil {
		t.Fatal("expected not-configured error")
	}
	if !strings.Contains(err.Error(), "视觉分析未配置") {
		t.Errorf("error = %v", err)
	}
	if strings.Contains(err.Error(), "placeholder") || strings.Contains(err.Error(), "Vision analysis requested") {
		t.Errorf("must not return fabricated analysis: %v", err)
	}
}

// TestAnalyzeBrowserScreenshotWithProvider 验证浏览器截图经 provider 完成真实分析。
func TestAnalyzeBrowserScreenshotWithProvider(t *testing.T) {
	mock := &mockVisionProvider{
		resp: &agentcore.ProviderResponse{Content: "页面包含登录表单"},
	}
	cfg := &BrowserToolConfig{
		Vision: &VisionToolConfig{Provider: mock, Model: "glm-5v-turbo"},
	}
	cfg.defaults()

	got, err := analyzeBrowserScreenshot(context.Background(), cfg, "页面上有什么？", tinyPNG())
	if err != nil {
		t.Fatalf("analyzeBrowserScreenshot: %v", err)
	}
	if got != "页面包含登录表单" {
		t.Errorf("analysis = %q", got)
	}
	if mock.lastReq == nil || mock.lastReq.Model != "glm-5v-turbo" {
		t.Fatalf("provider request = %#v", mock.lastReq)
	}
	var imageOK bool
	for _, bl := range mock.lastReq.Messages[0].Blocks {
		if bl.Kind == agentcore.BlockKindImage &&
			strings.HasPrefix(bl.URL, "data:image/png;base64,") {
			imageOK = true
		}
	}
	if !imageOK {
		t.Errorf("screenshot image block missing: %#v", mock.lastReq.Messages[0].Blocks)
	}
}

// TestAnalyzeBrowserScreenshotTooLarge 验证超限截图返回明确错误而非截断硬发。
func TestAnalyzeBrowserScreenshotTooLarge(t *testing.T) {
	mock := &mockVisionProvider{
		resp: &agentcore.ProviderResponse{Content: "unused"},
	}
	cfg := &BrowserToolConfig{
		Vision: &VisionToolConfig{Provider: mock, Model: "m", MaxBytes: 8},
	}
	cfg.defaults()

	_, err := analyzeBrowserScreenshot(context.Background(), cfg, "q", tinyPNG())
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected too-large error, got: %v", err)
	}
	if mock.lastReq != nil {
		t.Errorf("provider must not be called for oversized screenshot")
	}
}

// TestExtensionWithVisionPropagatesToBrowser 验证 WithVision 同步配置浏览器视觉能力，
// 且不覆盖显式设置的 Browser.Vision。
func TestExtensionWithVisionPropagatesToBrowser(t *testing.T) {
	mock := &mockVisionProvider{}

	ext := NewExtension(ExtensionConfig{Browser: &BrowserToolConfig{}})
	ext.WithVision(mock, "m")
	if ext.config.Browser.Vision == nil || ext.config.Browser.Vision.Provider == nil {
		t.Fatal("WithVision did not propagate to Browser.Vision")
	}

	explicit := &VisionToolConfig{Model: "custom"}
	ext2 := NewExtension(ExtensionConfig{Browser: &BrowserToolConfig{Vision: explicit}})
	ext2.WithVision(mock, "m")
	if ext2.config.Browser.Vision != explicit {
		t.Error("WithVision must not override explicit Browser.Vision")
	}
}
