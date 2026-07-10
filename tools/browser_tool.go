package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/xujian519/mady/agentcore"
)

// browserToolInput is the parsed argument structure for browser tool calls.
type browserToolInput struct {
	Action     string                 `json:"action"`
	URL        string                 `json:"url"`
	Ref        string                 `json:"ref"`
	Text       string                 `json:"text"`
	Direction  string                 `json:"direction"`
	Key        string                 `json:"key"`
	FullPage   bool                   `json:"full_page"`
	Full       bool                   `json:"full"`
	Mode       string                 `json:"mode"`
	Expression string                 `json:"expression"`
	FrameID    string                 `json:"frame_id"`
	DialogID   string                 `json:"dialog_id"`
	Accept     bool                   `json:"accept"`
	PromptText string                 `json:"prompt_text"`
	Question   string                 `json:"question"`
	Annotate   bool                   `json:"annotate"`
	CDPMethod  string                 `json:"cdp_method"`
	CDPParams  map[string]interface{} `json:"cdp_params"`
}

// browserActionHandler is a function that implements a single browser action.
type browserActionHandler func(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error)

// browserActionHandlers dispatches each action string to its implementation.
var browserActionHandlers = map[string]browserActionHandler{
	"navigate":   handleNavigate,
	"snapshot":   handleSnapshot,
	"click":      handleClick,
	"type":       handleType,
	"scroll":     handleScroll,
	"back":       handleBack,
	"press":      handlePress,
	"screenshot": handleScreenshot,
	"evaluate":   handleEvaluate,
	"dialog":     handleDialog,
	"vision":     handleVision,
	"console":    handleConsole,
	"cdp":        handleCdp,
	"get_images": handleGetImages,
}

func NewBrowserTool(cfg *BrowserToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &BrowserToolConfig{}
	}
	cfg.defaults()

	// Close previous manager to avoid leaking browser sessions and goroutines.
	if defaultBrowserManager != nil {
		defaultBrowserManager.CloseAll()
	}
	defaultBrowserManager = NewBrowserManager(&BrowserConfig{
		Headless:            cfg.Headless,
		AllowPrivate:        cfg.AllowPrivate,
		CommandTimeout:      cfg.CommandTimeout,
		CDPURL:              cfg.CDPURL,
		CamofoxURL:          cfg.CamofoxURL,
		CloudProvider:       cfg.CloudProvider,
		Engine:              cfg.Engine,
		DialogPolicy:        cfg.DialogPolicy,
		DialogTimeout:       cfg.DialogTimeout,
		AutoLocalForPrivate: cfg.AutoLocalForPrivate,
		RecordSessions:      cfg.RecordSessions,
		RecordingDir:        cfg.RecordingDir,
		InactivityTimeout:   cfg.InactivityTimeout,
		UserAgent:           cfg.UserAgent,
		AcceptLanguage:      cfg.AcceptLanguage,
		ProxyURL:            cfg.ProxyURL,
		ViewportWidth:       cfg.ViewportWidth,
		ViewportHeight:      cfg.ViewportHeight,
		AgentBrowserEnabled: cfg.AgentBrowserEnabled,
	})

	return &agentcore.Tool{
		Name:        "browser",
		Description: "控制网页浏览器。用于用户提供的 URL、交互式页面、登录流程、JavaScript 密集型页面，或作为 web_fetch 被屏蔽时的后备方案。简单信息检索请优先使用 web_search（更快、更便宜、无浏览器开销）。操作：navigate（打开 URL）、snapshot（获取页面文本及交互元素）、click（按 ref ID 点击元素）、type（按 ref ID 在元素中输入文字）、scroll（上/下滚动）、back（历史后退）、press（键盘按键）、screenshot（视口截屏）、evaluate（执行 JS）、dialog（处理 alert/confirm/prompt 对话框）、vision（让 AI 分析页面截图）、console（获取控制台日志）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "要执行的浏览器操作",
					"enum":        []string{"navigate", "snapshot", "click", "type", "scroll", "back", "press", "screenshot", "evaluate", "dialog", "vision", "console", "cdp", "get_images"},
				},
				"url": map[string]any{
					"type":        "string",
					"description": "要导航到的 URL（action=navigate 时必需）",
				},
				"ref": map[string]any{
					"type":        "string",
					"description": "快照中的元素 ref ID（例如 @e5 或 e5，action=click 和 action=type 时必需）",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "要输入到字段中的文本（action=type 时必需）",
				},
				"direction": map[string]any{
					"type":        "string",
					"description": "滚动方向（action=scroll 时必需）",
					"enum":        []string{"up", "down"},
				},
				"key": map[string]any{
					"type":        "string",
					"description": "要按的按键（action=press 时必需）。常用按键：Enter、Tab、Escape、ArrowUp、ArrowDown、ArrowLeft、ArrowRight。",
				},
				"full_page": map[string]any{
					"type":        "boolean",
					"description": "截取完整页面截图（用于 action=screenshot，默认：false，仅截取视口）",
				},
				"full": map[string]any{
					"type":        "boolean",
					"description": "显示完整页面内容（用于 action=snapshot，默认：false，仅显示交互元素）",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "快照模式（用于 action=snapshot）。\"default\"：基于 JS 的无障碍树，含 XPath 引用。\"aria\"：Chrome 原生 aria 树，角色/名称信息更丰富。（默认：\"default\"）",
					"enum":        []string{"default", "aria"},
				},
				"expression": map[string]any{
					"type":        "string",
					"description": "要执行的 JavaScript 表达式（action=evaluate 时必需）",
				},
				"frame_id": map[string]any{
					"type":        "string",
					"description": "OOPIF 评估的可选 frame ID（来自快照 frame 树，用于 action=evaluate）",
				},
				"dialog_id": map[string]any{
					"type":        "string",
					"description": "待处理对话框列表中的对话框 ID（action=dialog 时必需）",
				},
				"accept": map[string]any{
					"type":        "boolean",
					"description": "是否接受（true）或关闭（false）对话框（action=dialog 时必需）",
				},
				"prompt_text": map[string]any{
					"type":        "string",
					"description": "在 prompt 对话框中输入的文本（用于 action=dialog）",
				},
				"question": map[string]any{
					"type":        "string",
					"description": "关于页面的问题（action=vision 时必需）",
				},
				"annotate": map[string]any{
					"type":        "boolean",
					"description": "在交互元素上叠加编号标签（用于 action=vision，默认：false）",
				},
				"cdp_method": map[string]any{
					"type":        "string",
					"description": "Chrome DevTools Protocol 方法（action=cdp 时必需，例如 Page.captureScreenshot、Runtime.evaluate、DOM.getDocument）",
				},
				"cdp_params": map[string]any{
					"type":        "object",
					"description": "CDP 方法参数，以 JSON 对象形式提供（用于 action=cdp）",
				},
			},
			"required": []any{"action"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input browserToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Dispatch via handler map (strategy pattern) instead of
			// a monolithic switch for each action/backend combination.
			handler, ok := browserActionHandlers[input.Action]
			if !ok {
				return nil, fmt.Errorf("unknown browser action: %s (valid: navigate, snapshot, click, type, scroll, back, press, screenshot, evaluate, dialog, vision, console, cdp, get_images)", input.Action)
			}
			return handler(ctx, input, cfg)
		},
	}
}

// ---------------------------------------------------------------------------
// Handler helpers
// ---------------------------------------------------------------------------

// normalizeRef adds a "@" prefix to a ref string if one is not present.
func normalizeRef(ref string) string {
	if !strings.HasPrefix(ref, "@") {
		return "@" + ref
	}
	return ref
}

// isChromedpBackend returns true when the browser backend uses chromedp (local,
// CDP, or any cloud provider that exposes a CDP endpoint).
func isChromedpBackend(bt BrowserBackendType) bool {
	switch bt {
	case BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendLightpanda, BackendAgentBrowser:
		return true
	default:
		return false
	}
}

// getXPathFromRef looks up an XPath for the given ref in the session's ref
// mapper. If not found it falls back to the JavaScript ref map stored in the
// page.
func getXPathFromRef(ctx context.Context, session *BrowserSession, ref string) (string, error) {
	xpath, ok := session.refMapper.Get(ref)
	if ok {
		return xpath, nil
	}
	// Fallback: try window.__covoRefMap in the browser.
	var jsXpath string
	js := fmt.Sprintf(`window.__covoRefMap && window.__covoRefMap[%q] || null`, ref)
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &jsXpath)); err == nil && jsXpath != "" {
		session.refMapper.Set(ref, jsXpath)
		return jsXpath, nil
	}
	if session.refMapper.Count() == 0 {
		return "", fmt.Errorf("ref %s not found. Page state is unknown. Call browser_snapshot first", ref)
	}
	return "", fmt.Errorf("ref %s not found in current page state. Call browser_snapshot to refresh", ref)
}

// ---------------------------------------------------------------------------
// Action handlers
// ---------------------------------------------------------------------------

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
	backend := DetectBackend(&defaultBrowserManager.config)
	if backend == BackendCamofox || (defaultBrowserManager.config.AutoLocalForPrivate && IsPrivateURL(parsedURL.String())) {
		sessionTargetURL = input.URL
	}

	session, err := defaultBrowserManager.CreateSession(ctx, sessionID, sessionTargetURL)
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
	for i := 0; i < 30; i++ {
		var state string
		_ = chromedp.Run(readyCtx, chromedp.Evaluate(`
			(function() {
				return JSON.stringify({
					state: document.readyState,
					url: window.location.href
				});
			})()
		`, &state))

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
		return nil, fmt.Errorf("navigation timed out: page did not become interactive")
	}

	// 3. Small sleep to allow SPA rendering.
	time.Sleep(1 * time.Second)

	// 4. Apply stealth JS to hide automation fingerprints.
	stealthCtx, stealthCancel := context.WithTimeout(timeoutCtx, 3*time.Second)
	_ = chromedp.Run(stealthCtx, chromedp.Evaluate(stealthJavaScript, nil))
	stealthCancel()

	// 5. Get title.
	var title string
	titleCtx, titleCancel := context.WithTimeout(timeoutCtx, 5*time.Second)
	_ = chromedp.Run(titleCtx, chromedp.Title(&title))
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
		fallbackResult, fallbackErr := RunChromeFallbackCommand(context.Background(), "navigate",
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
		session.recorder.StartRecording(ctx, session.sessionID, session.ctx)
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
	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
	}

	mode := input.Mode
	if mode == "" {
		mode = "default"
	}

	var snapshot string
	var err error

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

	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
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

	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
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

	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
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

	var res interface{}
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
	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
	}

	var url, title string
	var snapshot string
	var err error

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
		_ = chromedp.Run(timeoutCtx,
			chromedp.Location(&url),
			chromedp.Title(&title),
		)
		snapshot, err = generateSnapshot(session.ctx, false, session.refMapper)
		cancel()
	case BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		defer cancel()

		if err := chromedp.Run(timeoutCtx, chromedp.NavigateBack()); err != nil {
			return nil, fmt.Errorf("back navigation failed: %w", err)
		}
		_ = chromedp.Run(timeoutCtx,
			chromedp.Location(&url),
			chromedp.Title(&title),
		)
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

	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
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
	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
	}

	var sizeBytes int
	var err error

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

	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
	}

	if session.backendType == BackendCamofox {
		return nil, fmt.Errorf("JS evaluation not supported for camofox backend")
	}

	var evalResult string
	var err error

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

	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
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

// handleVision takes a screenshot and returns a placeholder analysis result.
func handleVision(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	if input.Question == "" {
		return nil, fmt.Errorf("question is required for vision action")
	}

	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
	}

	var screenshotData []byte
	var err error

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

	_ = screenshotData // available for vision model integration

	analysis := fmt.Sprintf("Vision analysis requested for: %s (model: %s)", input.Question, cfg.VisionModel)

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(analysis, nil)
}

// handleConsole returns a notice that console messages are captured asynchronously.
func handleConsole(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error) {
	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
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

	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
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
	session, ok := defaultBrowserManager.GetActiveSession("default")
	if !ok {
		return nil, fmt.Errorf("no active browser session. Call browser_navigate first")
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
	sb.WriteString(fmt.Sprintf("Found %d images on the page:\n\n", len(images)))
	for _, img := range images {
		src, _ := img["src"].(string)
		alt, _ := img["alt"].(string)
		w, _ := img["width"].(float64)
		h, _ := img["height"].(float64)
		displayed, _ := img["displayed"].(bool)

		sb.WriteString(fmt.Sprintf("  [%d] ", int(img["index"].(float64))))
		if !displayed {
			sb.WriteString("(hidden) ")
		}
		sb.WriteString(fmt.Sprintf("%dx%d", int(w), int(h)))
		if alt != "" {
			sb.WriteString(fmt.Sprintf(" alt=%q", alt))
		}
		sb.WriteString(fmt.Sprintf("\n       src: %s\n", src))
	}

	return result(sb.String(), map[string]any{
		"count":  len(images),
		"images": images,
	})
}

func cdpParamsToJSON(params map[string]interface{}) string {
	if params == nil {
		return "{}"
	}
	b, _ := json.Marshal(params)
	return string(b)
}
