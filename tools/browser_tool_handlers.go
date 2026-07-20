package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// --- Browser action handlers ---

// handleNavigate opens a URL in the browser and returns an accessibility
// snapshot of the loaded page.
func handleNavigate(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	if input.URL == "" {
		return nil, fmt.Errorf("url is required for navigate action")
	}

	parsedURL, err := validateURL(input.URL, cfg.AllowPrivate || cfg.AutoLocalForPrivate)
	if err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	sessionID := "default"
	sessionTargetURL := ""
	backend := DetectBackend(&DefaultBrowserManager().config)
	if backend == BackendCamofox || (DefaultBrowserManager().config.AutoLocalForPrivate && IsPrivateURL(parsedURL.String())) {
		sessionTargetURL = input.URL
	}

	session, err := DefaultBrowserManager().CreateSession(ctx, sessionID, sessionTargetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create browser session: %w", err)
	}

	if session.backendType == BackendCamofox {
		snapshot, err := session.camofoxClient.Navigate(sessionID, parsedURL.String())
		if err != nil {
			return nil, fmt.Errorf("camofox navigation failed: %w", err)
		}
		session.mu.RLock()
		url := session.url
		session.mu.RUnlock()
		if url == "" {
			url = parsedURL.String()
		}
		return result(fmt.Sprintf("Navigated to %s\n\n%s", url, snapshot), nil)
	}

	// chromedp-based backends (local, CDP, lightpanda, cloud, agent-browser).
	navTimeout := navigationTimeout(cfg.CommandTimeout)
	timeoutCtx, cancel := context.WithTimeout(session.ctx, navTimeout)
	defer cancel()

	// 1. Fire navigation via CDP Page.navigate (does not wait for load).
	if err := chromedp.Run(timeoutCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, _, _, err := page.Navigate(parsedURL.String()).Do(ctx)
		return err
	})); err != nil {
		if isDeadlineError(err) {
			return nil, navigationTimeoutError(parsedURL.String(), navTimeout)
		}
		return nil, fmt.Errorf("navigation failed: %w", err)
	}

	// 2. Wait for DOM to be interactive or URL to match.
	readyCtx, readyCancel := context.WithTimeout(timeoutCtx, 30*time.Second)
	var ready bool
	var lastReadyErr error
	for i := 0; i < 30; i++ {
		var state string
		if err := chromedp.Run(readyCtx, chromedp.Evaluate(`
			(function() {
				return JSON.stringify({
					state: document.readyState,
					url: window.location.href
				});
			})()
		`, &state)); err != nil {
			lastReadyErr = err
			time.Sleep(1 * time.Second)
			continue
		}

		var navResult struct {
			State string `json:"state"`
			URL   string `json:"url"`
		}
		if json.Unmarshal([]byte(state), &navResult) == nil {
			if navResult.State == "interactive" || navResult.State == "complete" {
				ready = true
				break
			}
			if strings.Contains(navResult.URL, parsedURL.Host) {
				ready = true
				break
			}
		}
		time.Sleep(1 * time.Second)
	}
	readyCancel()

	if !ready {
		if lastReadyErr != nil {
			return nil, fmt.Errorf("navigation timed out: %w", lastReadyErr)
		}
		return nil, fmt.Errorf("navigation timed out: page did not become interactive")
	}

	// 3. SPA 渲染缓冲：readyState 进入 interactive/complete 后，单页应用通常
	//    仍需数百毫秒完成首屏 hydration/动态注入。此处用固定 1s 缓冲是经验值
	//    （项目内无 SPA 集成测试可校准），改短或改 readyState 轮询前需先用真实
	//    SPA 站点验证 snapshot 完整性。
	time.Sleep(1 * time.Second)

	// 4. Apply stealth JS to hide automation fingerprints.
	stealthCtx, stealthCancel := context.WithTimeout(timeoutCtx, 3*time.Second)
	if err := chromedp.Run(stealthCtx, chromedp.Evaluate(stealthJavaScript, nil)); err != nil {
		slog.Warn("browser: stealth js injection failed", "err", err)
	}
	stealthCancel()

	// 5. Get title.
	var title string
	titleCtx, titleCancel := context.WithTimeout(timeoutCtx, 5*time.Second)
	if err := chromedp.Run(titleCtx, chromedp.Title(&title)); err != nil {
		slog.Warn("browser: get title failed", "err", err)
	}
	titleCancel()

	// 6. Generate snapshot to update ref map.
	snapshot, snapErr := generateSnapshot(timeoutCtx, false, session.refMapper)

	session.mu.Lock()
	session.url = parsedURL.String()
	session.title = title
	session.lastActivity = time.Now()
	session.mu.Unlock()

	if snapErr != nil {
		snapshot = fmt.Sprintf("(snapshot unavailable: %v)", snapErr)
	}

	// 7. Lightpanda-specific fallback when the snapshot is empty.
	if session.backendType == BackendLightpanda && NeedsLightpandaFallback(cfg.Engine, snapshot, 0, nil) {
		fallbackResult, fallbackErr := RunChromeFallbackCommand(ctx, "navigate",
			map[string]any{"url": input.URL}, cfg.CommandTimeout)
		if fallbackErr == nil {
			if v, ok := fallbackResult["snapshot"].(string); ok {
				snapshot = v
			}
			if v, ok := fallbackResult["title"].(string); ok {
				session.mu.Lock()
				session.title = v
				title = v
				session.mu.Unlock()
			}
			AnnotateLightpandaFallback(nil)
		}
	}

	session.mu.RLock()
	url := session.url
	title = session.title
	session.mu.RUnlock()

	if title == "" {
		title = "(unknown)"
	}

	if err := ValidatePostRedirectURL(session.url, input.URL, cfg.AllowPrivate, cfg.AutoLocalForPrivate); err != nil {
		return nil, err
	}

	if session.recorder != nil && !session.recorder.IsRecording() && session.ctx != nil {
		session.recorder.StartRecording(ctx, session.ctx, session.sessionID)
	}

	var extraInfo string
	if session.supervisor != nil {
		dialogs := session.supervisor.GetPendingDialogs()
		if len(dialogs) > 0 {
			extraInfo = formatDialogs(dialogs)
		}
	}

	return result(fmt.Sprintf("Navigated to %s\nTitle: %s\n\n%s%s", url, title, snapshot, extraInfo), nil)
}

// handleSnapshot returns a text-based accessibility tree of the current page.
func handleSnapshot(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	mode := input.Mode
	if mode == "" {
		mode = "default"
	}

	var snapshot string

	switch session.backendType {
	case BackendCamofox:
		snapshot, err = session.camofoxClient.GetSnapshot(session.sessionID, input.Full)
	case BackendLightpanda, BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		snapshot, err = GeneratePageSnapshot(timeoutCtx, input.Full, session.refMapper, mode)
		cancel()
	default:
		err = fmt.Errorf("backend %s not supported for snapshot", session.backendType)
	}

	if err != nil {
		return nil, fmt.Errorf("snapshot failed: %w", err)
	}

	session.mu.RLock()
	url := session.url
	title := session.title
	session.mu.RUnlock()

	var extraInfo string
	if session.supervisor != nil {
		dialogs := session.supervisor.GetPendingDialogs()
		if len(dialogs) > 0 {
			extraInfo = formatDialogs(dialogs)
		}
		frames := session.supervisor.GetFrameTree()
		if len(frames) > 0 {
			extraInfo += formatFrameTree(frames, session.supervisor.IsTruncated())
		}
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(fmt.Sprintf("Page: %s\nTitle: %s\n\n%s%s", url, title, snapshot, extraInfo), nil)
}

// handleClick clicks an element identified by its snapshot ref ID.
func handleClick(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	if input.Ref == "" {
		return nil, fmt.Errorf("ref is required for click action")
	}

	ref := normalizeRef(input.Ref)

	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	if session.backendType == BackendCamofox {
		snapshot, err := session.camofoxClient.Click(session.sessionID, ref)
		if err != nil {
			return nil, fmt.Errorf("camofox click failed: %w", err)
		}
		session.mu.Lock()
		session.lastActivity = time.Now()
		session.mu.Unlock()
		return result(fmt.Sprintf("Clicked %s\n\n%s", ref, snapshot), nil)
	}

	// chromedp-based backends.
	timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
	defer cancel()

	xpath, err := getXPathFromRef(timeoutCtx, session, ref)
	if err != nil {
		return nil, err
	}

	if err := chromedp.Run(timeoutCtx, chromedp.Click(xpath, chromedp.BySearch)); err != nil {
		return nil, fmt.Errorf("click failed for %s: %w", ref, err)
	}

	time.Sleep(500 * time.Millisecond)

	snapshot, snapErr := generateSnapshot(session.ctx, false, session.refMapper)
	if snapErr != nil {
		snapshot = fmt.Sprintf("(snapshot unavailable: %v)", snapErr)
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(fmt.Sprintf("Clicked %s\n\n%s", ref, snapshot), nil)
}

// handleType types text into an input field identified by its snapshot ref ID.
func handleType(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	if input.Ref == "" {
		return nil, fmt.Errorf("ref is required for type action")
	}

	ref := normalizeRef(input.Ref)

	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	if session.backendType == BackendCamofox {
		resultMsg, err := session.camofoxClient.Type(session.sessionID, ref, input.Text)
		if err != nil {
			return nil, fmt.Errorf("camofox type failed: %w", err)
		}
		session.mu.Lock()
		session.lastActivity = time.Now()
		session.mu.Unlock()
		return result(resultMsg, nil)
	}

	// chromedp-based backends.
	timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
	defer cancel()

	xpath, err := getXPathFromRef(timeoutCtx, session, ref)
	if err != nil {
		return nil, err
	}

	if err := chromedp.Run(timeoutCtx,
		chromedp.Clear(xpath, chromedp.BySearch),
		chromedp.SendKeys(xpath, input.Text, chromedp.BySearch),
	); err != nil {
		return nil, fmt.Errorf("type failed for %s: %w", ref, err)
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(fmt.Sprintf("Typed \"%s\" into %s", input.Text, ref), nil)
}

// handleScroll scrolls the page up or down.
func handleScroll(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	if input.Direction != "up" && input.Direction != "down" {
		return nil, fmt.Errorf("direction must be \"up\" or \"down\"")
	}

	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	if session.backendType == BackendCamofox {
		snapshot, err := session.camofoxClient.Scroll(session.sessionID, input.Direction)
		if err != nil {
			return nil, fmt.Errorf("camofox scroll failed: %w", err)
		}
		session.mu.Lock()
		session.lastActivity = time.Now()
		session.mu.Unlock()
		return result(fmt.Sprintf("Scrolled %s\n\n%s", input.Direction, snapshot), nil)
	}

	// chromedp-based backends.
	script := "window.scrollBy(0, 500);"
	if input.Direction == "up" {
		script = "window.scrollBy(0, -500);"
	}

	timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
	defer cancel()

	var res any
	if err := chromedp.Run(timeoutCtx, chromedp.Evaluate(script, &res)); err != nil {
		return nil, fmt.Errorf("scroll failed: %w", err)
	}

	snapshot, snapErr := generateSnapshot(timeoutCtx, false, session.refMapper)
	if snapErr != nil {
		snapshot = fmt.Sprintf("(snapshot unavailable: %v)", snapErr)
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(fmt.Sprintf("Scrolled %s\n\n%s", input.Direction, snapshot), nil)
}

// handleBack navigates back in browser history.
func handleBack(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	var url, title string
	var snapshot string

	switch session.backendType {
	case BackendCamofox:
		snapshot, err = session.camofoxClient.Back(session.sessionID)
		if err != nil {
			return nil, fmt.Errorf("camofox back failed: %w", err)
		}
	case BackendLightpanda:
		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		if err := chromedp.Run(timeoutCtx,
			chromedp.NavigateBack(),
			chromedp.WaitReady("body", chromedp.ByQuery),
		); err != nil {
			cancel()
			return nil, fmt.Errorf("back navigation failed: %w", err)
		}
		if err := chromedp.Run(timeoutCtx,
			chromedp.Location(&url),
			chromedp.Title(&title),
		); err != nil {
			slog.Warn("browser: get location/title failed", "err", err)
		}
		snapshot, err = generateSnapshot(session.ctx, false, session.refMapper)
		cancel()
	case BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		defer cancel()

		if err := chromedp.Run(timeoutCtx, chromedp.NavigateBack()); err != nil {
			return nil, fmt.Errorf("back navigation failed: %w", err)
		}
		if err := chromedp.Run(timeoutCtx,
			chromedp.Location(&url),
			chromedp.Title(&title),
		); err != nil {
			slog.Warn("browser: get location/title failed", "err", err)
		}
		snapshot, err = generateSnapshot(timeoutCtx, false, session.refMapper)
	default:
		err = fmt.Errorf("backend %s not supported for back", session.backendType)
	}

	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	session.url = url
	session.title = title
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(fmt.Sprintf("Navigated back\nURL: %s\nTitle: %s\n\n%s", url, title, snapshot), nil)
}

// handlePress presses a keyboard key.
func handlePress(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	if input.Key == "" {
		return nil, fmt.Errorf("key is required for press action")
	}

	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	if session.backendType == BackendCamofox {
		resultMsg, err := session.camofoxClient.Press(session.sessionID, input.Key)
		if err != nil {
			return nil, fmt.Errorf("camofox key press failed: %w", err)
		}
		session.mu.Lock()
		session.lastActivity = time.Now()
		session.mu.Unlock()
		return result(resultMsg, nil)
	}

	// chromedp-based backends.
	timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
	defer cancel()

	if err := chromedp.Run(timeoutCtx, chromedp.KeyEvent(input.Key)); err != nil {
		return nil, fmt.Errorf("key press failed: %w", err)
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(fmt.Sprintf("Pressed key: %s", input.Key), nil)
}

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

// handleEvaluate executes JavaScript in the page context.
func handleEvaluate(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	if input.Expression == "" {
		return nil, fmt.Errorf("expression is required for evaluate action")
	}

	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	if session.backendType == BackendCamofox {
		return nil, fmt.Errorf("JS evaluation not supported for camofox backend")
	}

	var evalResult string

	if session.supervisor != nil && input.FrameID != "" {
		evalResult, err = session.supervisor.EvaluateJS(input.Expression, input.FrameID)
	} else {
		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		var result string
		if err := chromedp.Run(timeoutCtx, chromedp.EvaluateAsDevTools(input.Expression, &result)); err != nil {
			cancel()
			return nil, fmt.Errorf("evaluation failed: %w", err)
		}
		cancel()
		evalResult = result
	}

	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(fmt.Sprintf("Result: %s", evalResult), nil)
}

// handleDialog handles a pending JavaScript dialog (alert/confirm/prompt).
func handleDialog(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	if input.DialogID == "" {
		return nil, fmt.Errorf("dialog_id is required for dialog action")
	}

	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	if session.supervisor == nil {
		return nil, fmt.Errorf("CDP supervisor not available (requires CDP endpoint)")
	}

	if err := session.supervisor.HandleDialog(input.DialogID, input.Accept, input.PromptText); err != nil {
		return nil, fmt.Errorf("dialog handling failed: %w", err)
	}

	action := "dismissed"
	if input.Accept {
		action = "accepted"
	}
	msg := fmt.Sprintf("Dialog %s %s", input.DialogID, action)
	if input.PromptText != "" {
		msg += fmt.Sprintf(" with text: %s", input.PromptText)
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(msg, nil)
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
// Operations.Analyze 返回明确的“未配置”错误，绝不伪造分析结果。
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

// handleConsole returns a notice that console messages are captured asynchronously.
func handleConsole(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result("Console messages are captured asynchronously. Use browser_evaluate to check page state.", nil)
}

// handleCdp executes a raw Chrome DevTools Protocol command.
func handleCdp(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	if input.CDPMethod == "" {
		return nil, fmt.Errorf("cdp_method is required for cdp action")
	}

	session, err := RequireActiveSession()
	if err != nil {
		return nil, err
	}

	var params json.RawMessage
	if input.CDPParams != nil {
		var marshalErr error
		params, marshalErr = json.Marshal(input.CDPParams)
		if marshalErr != nil {
			return nil, fmt.Errorf("invalid cdp_params: %w", marshalErr)
		}
	}

	timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
	defer cancel()

	// Execute CDP command via raw CDP dispatch using the chromedp target.
	var cdpResult json.RawMessage
	if err := chromedp.Run(timeoutCtx, chromedp.ActionFunc(func(cdpCtx context.Context) error {
		target := chromedp.FromContext(cdpCtx).Target
		if target == nil {
			return fmt.Errorf("no chromedp target available")
		}
		return target.Execute(cdpCtx, input.CDPMethod, params, &cdpResult)
	})); err != nil {
		return nil, fmt.Errorf("cdp command %s failed: %w", input.CDPMethod, err)
	}

	var resultStr string
	if len(cdpResult) > 0 && string(cdpResult) != "null" {
		resultStr = string(cdpResult)
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(fmt.Sprintf("CDP command %s executed successfully\nResult: %s", input.CDPMethod, resultStr), nil)
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

	var images []map[string]any
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
		src, _ := img["src"].(string)
		alt, _ := img["alt"].(string)
		w, _ := img["width"].(float64)
		h, _ := img["height"].(float64)
		displayed, _ := img["displayed"].(bool)

		index, _ := img["index"].(float64)
		fmt.Fprintf(&sb, "  [%d] ", int(index))
		if !displayed {
			sb.WriteString("(hidden) ")
		}
		fmt.Fprintf(&sb, "%dx%d", int(w), int(h))
		if alt != "" {
			fmt.Fprintf(&sb, " alt=%q", alt)
		}
		fmt.Fprintf(&sb, "\n       src: %s\n", src)
	}

	return result(sb.String(), map[string]any{
		"count":  len(images),
		"images": images,
	})
}
