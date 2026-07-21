package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// --- Media/DOM handlers ---

// handleScreenshot captures a screenshot of the current page.
func handleScreenshot(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	var sizeBytes int

	switch session.backendType {
	case BackendCamofox:
		var buf []byte
		buf, err = session.camofoxClient.Screenshot(session.sessionID)
		if err == nil {
			sizeBytes = len(buf)
		}
	case BackendLightpanda, BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		var buf []byte
		if err := chromedp.Run(timeoutCtx, chromedp.CaptureScreenshot(&buf)); err != nil {
			cancel()
			return nil, fmt.Errorf("screenshot failed: %w", err)
		}
		cancel()
		sizeBytes = len(buf)
	default:
		err = fmt.Errorf("backend %s not supported for screenshot", session.backendType)
	}

	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(fmt.Sprintf("Screenshot captured (%d bytes)", sizeBytes), map[string]any{
		"format":     "png",
		"size_bytes": sizeBytes,
	})
}

// handleVision takes a screenshot and analyzes it with the configured vision operations.
func handleVision(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	if input.Question == "" {
		return nil, fmt.Errorf("question is required for vision action")
	}

	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	var screenshotData []byte

	switch session.backendType {
	case BackendCamofox:
		screenshotData, err = session.camofoxClient.Screenshot(session.sessionID)
	case BackendLightpanda, BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		if err := chromedp.Run(timeoutCtx, chromedp.CaptureScreenshot(&screenshotData)); err != nil {
			cancel()
			return nil, fmt.Errorf("screenshot failed: %w", err)
		}
		cancel()
	default:
		return nil, fmt.Errorf("backend %s not supported for vision", session.backendType)
	}

	if err != nil {
		return nil, fmt.Errorf("screenshot failed: %w", err)
	}

	analysis, err := analyzeBrowserScreenshot(ctx, cfg, input.Question, screenshotData)
	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(analysis, nil)
}

// analyzeBrowserScreenshot 把浏览器截图交给配置的视觉操作完成分析。
// cfg.Vision 在 BrowserToolConfig.defaults() 中保证非 nil；未配置视觉能力时
// Operations.Analyze 返回明确的"未配置"错误，绝不伪造分析结果。
func analyzeBrowserScreenshot(ctx context.Context, cfg *BrowserToolConfig, question string, screenshot []byte) (string, error) {
	if cfg.Vision == nil || cfg.Vision.Operations == nil {
		return "", errVisionNotConfigured()
	}
	mimeType := detectImageMIME(screenshot)
	if !strings.HasPrefix(mimeType, "image/") {
		return "", fmt.Errorf("screenshot is not a recognizable image (detected: %s)", mimeType)
	}
	base64Size := len(base64.StdEncoding.EncodeToString(screenshot))
	if int64(base64Size) > cfg.Vision.MaxBytes {
		return "", fmt.Errorf(
			"screenshot too large: %s base64 (limit: %s)",
			FormatSize(int64(base64Size)), FormatSize(cfg.Vision.MaxBytes),
		)
	}
	analysis, err := cfg.Vision.Operations.Analyze(ctx, screenshot, mimeType, question)
	if err != nil {
		return "", fmt.Errorf("vision analysis failed: %w", err)
	}
	return analysis, nil
}

// imageInfo holds metadata about a single image element on the page.
// Parsed from the getImagesJS evaluation result via JSON unmarshalling.
type imageInfo struct {
	Index     int     `json:"index"`
	Src       string  `json:"src"`
	Alt       string  `json:"alt"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	Displayed bool    `json:"displayed"`
}

// handleGetImages returns a list of images found on the current page.
func handleGetImages(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	if session.backendType == BackendCamofox {
		return nil, fmt.Errorf("get_images not supported for camofox backend")
	}

	timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
	defer cancel()

	const getImagesJS = `
(function() {
	const images = Array.from(document.images);
	return JSON.stringify(images.map(function(img, i) {
		return {
			index: i,
			src: img.src || '',
			alt: img.alt || '',
			width: img.naturalWidth || img.width || 0,
			height: img.naturalHeight || img.height || 0,
			displayed: window.getComputedStyle(img).display !== 'none'
		};
	}));
})();
`

	var resultJSON string
	if err := chromedp.Run(timeoutCtx, chromedp.Evaluate(getImagesJS, &resultJSON)); err != nil {
		return nil, fmt.Errorf("get_images failed: %w", err)
	}

	var images []imageInfo
	if err := json.Unmarshal([]byte(resultJSON), &images); err != nil {
		return nil, fmt.Errorf("failed to parse image data: %w", err)
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	if len(images) == 0 {
		return result("No images found on the current page.", nil)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d images on the page:\n\n", len(images))
	for _, img := range images {
		fmt.Fprintf(&sb, "  [%d] ", img.Index)
		if !img.Displayed {
			sb.WriteString("(hidden) ")
		}
		fmt.Fprintf(&sb, "%dx%d", int(img.Width), int(img.Height))
		if img.Alt != "" {
			fmt.Fprintf(&sb, " alt=%q", img.Alt)
		}
		fmt.Fprintf(&sb, "\n       src: %s\n", img.Src)
	}

	return result(sb.String(), map[string]any{
		"count":  len(images),
		"images": images,
	})
}
