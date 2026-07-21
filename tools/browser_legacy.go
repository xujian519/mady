package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/xujian519/mady/agentcore"
)

// chromedpNavigateAndSnapshot runs the shared chromedp-based navigation flow
// used by all chromedp-backed browser backends (Lightpanda, Local, CDP,
// Browserbase, BrowserUse, Firecrawl, AgentBrowser).
//
// The flow is:
//  1. Inject stealth JS to hide automation fingerprints.
//  2. Navigate to parsedURL and wait for the page to become interactive.
//  3. Apply stealth JS again post-navigation.
//  4. Read the page title.
//  5. Generate an accessibility snapshot via generateSnapshot.
//  6. Update session state (url, title, lastActivity).
//
// navErrPrefix scopes error messages (e.g. "lightpanda navigation failed"
// vs "navigation failed") so callers can distinguish backends in errors.
//
// Returns the generated snapshot (which may be empty on partial failure)
// and any error. On success the caller owns the returned snapshot; on
// error the caller should propagate err.
func chromedpNavigateAndSnapshot(
	session *BrowserSession,
	parsedURL *url.URL,
	navTimeout time.Duration,
	navErrPrefix string,
) (string, error) {
	timeoutCtx, cancel := context.WithTimeout(session.ctx, navTimeout)
	defer cancel()

	// Inject stealth JS before navigation.
	chromedp.Run(timeoutCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, _ = page.AddScriptToEvaluateOnNewDocument(stealthJavaScript).Do(ctx)
		return nil
	}))

	// Navigate.
	if err := chromedp.Run(timeoutCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, _, _, err := page.Navigate(parsedURL.String()).Do(ctx)
		return err
	})); err != nil {
		if isDeadlineError(err) {
			return "", navigationTimeoutError(parsedURL.String(), navTimeout)
		}
		return "", fmt.Errorf("%s: %w", navErrPrefix, err)
	}

	// Wait for DOM to be interactive or URL to match target host.
	readyCtx, readyCancel := context.WithTimeout(timeoutCtx, 30*time.Second)
	var ready bool
	for i := 0; i < 30; i++ {
		var state string
		chromedp.Run(readyCtx, chromedp.Evaluate(`
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
		return "", fmt.Errorf("navigation timed out: page did not become interactive")
	}

	time.Sleep(1 * time.Second)

	// Re-apply stealth JS post-navigation.
	stealthCtx, stealthCancel := context.WithTimeout(timeoutCtx, 3*time.Second)
	chromedp.Run(stealthCtx, chromedp.Evaluate(stealthJavaScript, nil))
	stealthCancel()

	var title string
	titleCtx, titleCancel := context.WithTimeout(timeoutCtx, 5*time.Second)
	chromedp.Run(titleCtx, chromedp.Title(&title))
	titleCancel()

	snapshot, err := generateSnapshot(timeoutCtx, false, session.refMapper)

	session.mu.Lock()
	session.url = parsedURL.String()
	session.title = title
	session.lastActivity = time.Now()
	session.mu.Unlock()

	if err != nil {
		snapshot = fmt.Sprintf("(snapshot unavailable: %v)", err)
	}

	return snapshot, nil
}

// ---------------------------------------------------------------------------
// Standalone tool constructors
// ---------------------------------------------------------------------------
// Each NewBrowser*Tool function builds an agentcore.Tool whose Func closure
// parses the incoming JSON arguments, obtains the active browser session,
// dispatches to the appropriate backend, and returns a formatted result.
// Most share the same skeleton — the factory newBrowserTool removes the
// nil-check / cfg.defaults() boilerplate.

// refPrefixNormalized returns ref prefixed with "@" if not already present.
func refPrefixNormalized(ref string) string {
	if !strings.HasPrefix(ref, "@") {
		return "@" + ref
	}
	return ref
}

// touchSession updates the session's lastActivity timestamp.
// Safe to call with a nil session (no-op).
func touchSession(session *BrowserSession) {
	if session == nil {
		return
	}
	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()
}

// chromedpBackends is the set of backends that use chromedp for page automation.
var chromedpBackends = map[BrowserBackendType]bool{
	BackendLightpanda:   true,
	BackendLocal:        true,
	BackendCDP:          true,
	BackendBrowserbase:  true,
	BackendBrowserUse:   true,
	BackendFirecrawl:    true,
	BackendAgentBrowser: true,
}

// isChromedpBackend returns true when backend is backed by chromedp.
func isChromedpBackend(bt BrowserBackendType) bool { return chromedpBackends[bt] }

// ---------------------------------------------------------------------------
// toolConfig wraps the common NewBrowser*Tool preamble.
func toolConfig(cfg *BrowserToolConfig) *BrowserToolConfig {
	if cfg == nil {
		cfg = &BrowserToolConfig{}
	}
	cfg.defaults()
	return cfg
}

// toolParams builds a *agentcore.Tool with the given identity and func.
// All browser tools share the same preamble (nil-check + defaults), which
// this helper eliminates.
func toolParams(name, desc string, params map[string]any, fn agentcore.ToolFunc) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        name,
		Description: desc,
		Parameters:  params,
		Func:        fn,
	}
}

// chromedpEval runs a chromedp action with cfg.CommandTimeout, canceling on
// return. It is a small convenience for the one-liner chromedp calls that
// appear in most browser tools.
func chromedpEval(session *BrowserSession, cfg *BrowserToolConfig, action chromedp.Action) error {
	timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
	defer cancel()
	return chromedp.Run(timeoutCtx, action)
}

// ---------------------------------------------------------------------------
// NewBrowserNavigateTool creates a new browser_navigate tool.
// This is the only browser tool that also initializes a BrowserManager.
func NewBrowserNavigateTool(cfg *BrowserToolConfig) *agentcore.Tool {
	cfg = toolConfig(cfg)

	bm := NewBrowserManager(&BrowserConfig{
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
	SetDefaultBrowserManager(bm)

	return toolParams(
		"browser_navigate",
		"导航到指定 URL 并返回无障碍快照。用于用户提供的 URL、交互式页面、登录流程或 JavaScript 密集型页面。简单信息检索请优先使用 web_search（更快、更便宜、无浏览器开销）。",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "要导航到的 URL（http:// 或 https://）",
				},
			},
			"required": []any{"url"},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
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

			var snapshot string
			switch session.backendType {
			case BackendCamofox:
				snapshot, err = session.camofoxClient.Navigate(sessionID, parsedURL.String())
				if err != nil {
					return nil, fmt.Errorf("camofox navigation failed: %w", err)
				}
			case BackendLightpanda:
				navTimeout := navigationTimeout(cfg.CommandTimeout)
				snapshot, err = chromedpNavigateAndSnapshot(session, parsedURL, navTimeout, "lightpanda navigation failed")
				if err != nil {
					return nil, err
				}
				if NeedsLightpandaFallback(cfg.Engine, snapshot, 0, nil) {
					fallbackResult, fallbackErr := RunChromeFallbackCommand(ctx, "navigate", map[string]any{"url": input.URL}, cfg.CommandTimeout)
					if fallbackErr == nil {
						snapshot, _ = fallbackResult["snapshot"].(string)
						title, _ := fallbackResult["title"].(string)
						session.mu.Lock()
						session.title = title
						session.mu.Unlock()
						AnnotateLightpandaFallback(nil)
					}
				}
			case BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
				navTimeout := navigationTimeout(cfg.CommandTimeout)
				snapshot, err = chromedpNavigateAndSnapshot(session, parsedURL, navTimeout, "navigation failed")
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("backend %s not yet supported for navigation", session.backendType)
			}

			session.mu.RLock()
			url := session.url
			title := session.title
			session.mu.RUnlock()

			if title == "" {
				title = "(unknown)"
			}

			if err := ValidatePostRedirectURL(session.url, input.URL, cfg.AllowPrivate, cfg.AutoLocalForPrivate); err != nil {
				return nil, err
			}

			if session.recorder != nil && !session.recorder.IsRecording() {
				if session.ctx != nil {
					session.recorder.StartRecording(ctx, session.ctx, session.sessionID)
				}
			}

			var extraInfo string
			if session.supervisor != nil {
				dialogs := session.supervisor.GetPendingDialogs()
				if len(dialogs) > 0 {
					extraInfo = formatDialogs(dialogs)
				}
			}

			return result(fmt.Sprintf("Navigated to %s\nTitle: %s\n\n%s%s", url, title, snapshot, extraInfo), nil)
		},
	)
}

func navigationTimeout(commandTimeout time.Duration) time.Duration {
	if commandTimeout > 60*time.Second {
		return commandTimeout
	}
	return 60 * time.Second
}

func isDeadlineError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

func navigationTimeoutError(url string, timeout time.Duration) error {
	return fmt.Errorf("navigation timed out after %s while opening %s. The page may be slow, blocked, or still loading; try again, use browser_snapshot if the page partially loaded, or raise the browser command timeout", timeout.Round(time.Second), url)
}

// NewBrowserSnapshotTool creates a browser_snapshot tool.
func NewBrowserSnapshotTool(cfg *BrowserToolConfig) *agentcore.Tool {
	cfg = toolConfig(cfg)
	return toolParams(
		"browser_snapshot",
		"获取当前页面的基于文本的无障碍树快照。使用 ref ID（例如 @e5）与元素交互。",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"full": map[string]any{
					"type":        "boolean",
					"description": "显示完整页面内容（默认：false，仅显示交互元素）",
					"default":     false,
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "快照模式。\"default\"：基于 JS 的无障碍树，含 XPath 引用。\"aria\"：Chrome 原生 aria 树。（默认：\"default\"）",
					"enum":        []string{"default", "aria"},
				},
			},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Full bool   `json:"full"`
				Mode string `json:"mode"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if input.Mode == "" {
				input.Mode = "default"
			}

			session, err := RequireActiveSession()
			if err != nil {
				return nil, err
			}

			var snapshot string
			switch session.backendType {
			case BackendCamofox:
				snapshot, err = session.camofoxClient.GetSnapshot(session.sessionID, input.Full)
			case BackendLightpanda, BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
				timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
				snapshot, err = GeneratePageSnapshot(timeoutCtx, input.Full, session.refMapper, input.Mode)
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

			return result(fmt.Sprintf("Page: %s\nTitle: %s\n\n%s%s", url, title, snapshot, extraInfo), nil)
		},
	)
}

// refLookupErr formats a ref-not-found error based on whether the ref mapper
// has ever seen any refs (0 hits → page state unknown; otherwise stale state).
func refLookupErr(ref string, mapper *RefMapper) error {
	if mapper.Count() == 0 {
		return fmt.Errorf("ref %s not found. Page state is unknown. Call browser_snapshot first", ref)
	}
	return fmt.Errorf("ref %s not found in current page state. Call browser_snapshot to refresh", ref)
}

// refLookupWithFallback looks up a ref in the refMapper and, on a miss,
// attempts to retrieve the XPath from the in-browser __covoRefMap.
// timeoutCtx must be backed by a chromedp context.
func refLookupWithFallback(timeoutCtx context.Context, ref string, session *BrowserSession) (string, error) {
	xpath, ok := session.refMapper.Get(ref)
	if ok {
		return xpath, nil
	}
	var jsXpath string
	js := fmt.Sprintf(`window.__covoRefMap && window.__covoRefMap[%q] || null`, ref)
	if err := chromedp.Run(timeoutCtx, chromedp.Evaluate(js, &jsXpath)); err == nil && jsXpath != "" {
		session.refMapper.Set(ref, jsXpath)
		return jsXpath, nil
	}
	return "", refLookupErr(ref, session.refMapper)
}

// NewBrowserClickTool creates a browser_click tool.
func NewBrowserClickTool(cfg *BrowserToolConfig) *agentcore.Tool {
	cfg = toolConfig(cfg)
	return toolParams(
		"browser_click",
		"通过 ref ID（例如 @e5）点击元素。ref ID 显示在 browser_snapshot 输出中。",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ref": map[string]any{
					"type":        "string",
					"description": "快照中的元素 ref ID（例如 @e5 或 e5）",
				},
			},
			"required": []any{"ref"},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Ref string `json:"ref"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			ref := refPrefixNormalized(input.Ref)
			session, err := RequireActiveSession()
			if err != nil {
				return nil, err
			}

			var snapshot string

			switch session.backendType {
			case BackendCamofox:
				snapshot, err = session.camofoxClient.Click(session.sessionID, ref)
			case BackendLightpanda, BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
				timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
				xpath, lookupErr := refLookupWithFallback(timeoutCtx, ref, session)
				if lookupErr != nil {
					cancel()
					return nil, lookupErr
				}
				if err := chromedp.Run(timeoutCtx, chromedp.Click(xpath, chromedp.BySearch)); err != nil {
					cancel()
					return nil, fmt.Errorf("click failed for %s: %w", ref, err)
				}
				cancel()
				time.Sleep(500 * time.Millisecond)
				snapshot, err = generateSnapshot(session.ctx, false, session.refMapper)
			default:
				err = fmt.Errorf("backend %s not supported for click", session.backendType)
			}

			if err != nil {
				return nil, err
			}

			touchSession(session)
			return result(fmt.Sprintf("Clicked %s\n\n%s", ref, snapshot), nil)
		},
	)
}

// NewBrowserTypeTool creates a browser_type tool.
func NewBrowserTypeTool(cfg *BrowserToolConfig) *agentcore.Tool {
	cfg = toolConfig(cfg)
	return toolParams(
		"browser_type",
		"在输入字段中键入文本。先清除字段，然后输入文本。",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ref": map[string]any{
					"type":        "string",
					"description": "快照中的元素 ref ID（例如 @e5 或 e5）",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "要输入到字段中的文本",
				},
			},
			"required": []any{"ref", "text"},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Ref  string `json:"ref"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			ref := refPrefixNormalized(input.Ref)
			session, err := RequireActiveSession()
			if err != nil {
				return nil, err
			}

			var resultMsg string

			switch session.backendType {
			case BackendCamofox:
				resultMsg, err = session.camofoxClient.Type(session.sessionID, ref, input.Text)
			case BackendLightpanda, BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
				timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
				xpath, lookupErr := refLookupWithFallback(timeoutCtx, ref, session)
				if lookupErr != nil {
					cancel()
					return nil, lookupErr
				}
				if cErr := chromedp.Run(timeoutCtx,
					chromedp.Clear(xpath, chromedp.BySearch),
					chromedp.SendKeys(xpath, input.Text, chromedp.BySearch),
				); cErr != nil {
					cancel()
					return nil, fmt.Errorf("type failed for %s: %w", ref, cErr)
				}
				cancel()
				resultMsg = fmt.Sprintf("Typed \"%s\" into %s", input.Text, ref)
			default:
				err = fmt.Errorf("backend %s not supported for type", session.backendType)
			}

			if err != nil {
				return nil, err
			}

			touchSession(session)
			return result(resultMsg, nil)
		},
	)
}

// NewBrowserScrollTool creates a browser_scroll tool.
func NewBrowserScrollTool(cfg *BrowserToolConfig) *agentcore.Tool {
	cfg = toolConfig(cfg)
	return toolParams(
		"browser_scroll",
		"向上或向下滚动页面。",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"direction": map[string]any{
					"type":        "string",
					"description": "滚动方向：\"up\" 或 \"down\"",
					"enum":        []string{"up", "down"},
				},
			},
			"required": []any{"direction"},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Direction string `json:"direction"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if input.Direction != "up" && input.Direction != "down" {
				return nil, fmt.Errorf("direction must be \"up\" or \"down\"")
			}

			session, err := RequireActiveSession()
			if err != nil {
				return nil, err
			}

			var snapshot string
			script := "window.scrollBy(0, 500);"
			if input.Direction == "up" {
				script = "window.scrollBy(0, -500);"
			}

			switch session.backendType {
			case BackendCamofox:
				snapshot, err = session.camofoxClient.Scroll(session.sessionID, input.Direction)
			case BackendLightpanda, BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
				timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
				if cErr := chromedp.Run(timeoutCtx, chromedp.Evaluate(script, &struct{}{})); cErr != nil {
					cancel()
					return nil, fmt.Errorf("scroll failed: %w", cErr)
				}
				snapshot, err = generateSnapshot(timeoutCtx, false, session.refMapper)
				cancel()
			default:
				err = fmt.Errorf("backend %s not supported for scroll", session.backendType)
			}

			if err != nil {
				return nil, err
			}

			touchSession(session)
			return result(fmt.Sprintf("Scrolled %s\n\n%s", input.Direction, snapshot), nil)
		},
	)
}

// NewBrowserBackTool creates a browser_back tool.
func NewBrowserBackTool(cfg *BrowserToolConfig) *agentcore.Tool {
	cfg = toolConfig(cfg)
	return toolParams(
		"browser_back",
		"在浏览器历史记录中后退。",
		map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			session, err := RequireActiveSession()
			if err != nil {
				return nil, err
			}

			var url, title string
			var snapshot string

			switch session.backendType {
			case BackendCamofox:
				snapshot, err = session.camofoxClient.Back(session.sessionID)
			case BackendLightpanda:
				timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
				if cErr := chromedp.Run(timeoutCtx,
					chromedp.NavigateBack(),
					chromedp.WaitReady("body", chromedp.ByQuery),
				); cErr != nil {
					cancel()
					return nil, fmt.Errorf("back navigation failed: %w", cErr)
				}
				chromedp.Run(timeoutCtx, chromedp.Location(&url), chromedp.Title(&title))
				cancel()
				snapshot, err = generateSnapshot(session.ctx, false, session.refMapper)
			case BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
				timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
				if cErr := chromedp.Run(timeoutCtx, chromedp.NavigateBack()); cErr != nil {
					cancel()
					return nil, fmt.Errorf("back navigation failed: %w", cErr)
				}
				chromedp.Run(timeoutCtx, chromedp.Location(&url), chromedp.Title(&title))
				snapshot, err = generateSnapshot(timeoutCtx, false, session.refMapper)
				cancel()
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
		},
	)
}

// NewBrowserPressTool creates a browser_press tool.
func NewBrowserPressTool(cfg *BrowserToolConfig) *agentcore.Tool {
	cfg = toolConfig(cfg)
	return toolParams(
		"browser_press",
		"按下键盘按键。常用按键：Enter、Tab、Escape、ArrowUp、ArrowDown、ArrowLeft、ArrowRight。",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{
					"type":        "string",
					"description": "要按下的按键（例如 Enter、Tab、Escape、ArrowDown）",
				},
			},
			"required": []any{"key"},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Key string `json:"key"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			session, err := RequireActiveSession()
			if err != nil {
				return nil, err
			}

			var resultMsg string

			switch session.backendType {
			case BackendCamofox:
				resultMsg, err = session.camofoxClient.Press(session.sessionID, input.Key)
			case BackendLightpanda, BackendLocal, BackendCDP, BackendBrowserbase, BackendBrowserUse, BackendFirecrawl, BackendAgentBrowser:
				if cErr := chromedpEval(session, cfg, chromedp.KeyEvent(input.Key)); cErr != nil {
					return nil, fmt.Errorf("key press failed: %w", cErr)
				}
				resultMsg = fmt.Sprintf("Pressed key: %s", input.Key)
			default:
				err = fmt.Errorf("backend %s not supported for press", session.backendType)
			}

			if err != nil {
				return nil, err
			}

			touchSession(session)
			return result(resultMsg, nil)
		},
	)
}

// NewBrowserScreenshotTool creates a browser_screenshot tool.
func NewBrowserScreenshotTool(cfg *BrowserToolConfig) *agentcore.Tool {
	cfg = toolConfig(cfg)
	return toolParams(
		"browser_screenshot",
		"截取当前页面截图。返回 base64 编码的 PNG 数据。",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"full_page": map[string]any{
					"type":        "boolean",
					"description": "截取完整页面（默认：false，仅截取视口）",
					"default":     false,
				},
			},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				FullPage bool `json:"full_page"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

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
				var buf []byte
				if cErr := chromedpEval(session, cfg, chromedp.CaptureScreenshot(&buf)); cErr != nil {
					return nil, fmt.Errorf("screenshot failed: %w", cErr)
				}
				sizeBytes = len(buf)
			default:
				err = fmt.Errorf("backend %s not supported for screenshot", session.backendType)
			}

			if err != nil {
				return nil, err
			}

			touchSession(session)
			return result(fmt.Sprintf("Screenshot captured (%d bytes)", sizeBytes), map[string]any{
				"format":     "png",
				"size_bytes": sizeBytes,
			})
		},
	)
}

// NewBrowserEvaluateTool creates a browser_evaluate tool.
func NewBrowserEvaluateTool(cfg *BrowserToolConfig) *agentcore.Tool {
	cfg = toolConfig(cfg)
	return toolParams(
		"browser_evaluate",
		"在页面上下文中执行 JavaScript。返回执行结果。",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{
					"type":        "string",
					"description": "要执行的 JavaScript 表达式",
				},
				"frame_id": map[string]any{
					"type":        "string",
					"description": "OOPIF 评估的可选 frame ID（来自 browser_snapshot 的 frame 树）",
				},
			},
			"required": []any{"expression"},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Expression string `json:"expression"`
				FrameID    string `json:"frame_id"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			session, err := RequireActiveSession()
			if err != nil {
				return nil, err
			}

			var evalResult string

			switch {
			case session.supervisor != nil && input.FrameID != "":
				evalResult, err = session.supervisor.EvaluateJS(input.Expression, input.FrameID)
			case isChromedpBackend(session.backendType):
				if cErr := chromedpEval(session, cfg, chromedp.EvaluateAsDevTools(input.Expression, &evalResult)); cErr != nil {
					return nil, fmt.Errorf("evaluation failed: %w", cErr)
				}
			default:
				err = fmt.Errorf("JS evaluation not supported for backend %s", session.backendType)
			}

			if err != nil {
				return nil, err
			}

			touchSession(session)
			return result(fmt.Sprintf("Result: %s", evalResult), nil)
		},
	)
}

// NewBrowserDialogTool creates a browser_dialog tool.
func NewBrowserDialogTool(cfg *BrowserToolConfig) *agentcore.Tool {
	_ = toolConfig(cfg)
	return toolParams(
		"browser_dialog",
		"处理待处理的 JavaScript 对话框（alert/confirm/prompt）。在 browser_snapshot 显示待处理对话框后使用。",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dialog_id": map[string]any{
					"type":        "string",
					"description": "待处理对话框列表中的对话框 ID",
				},
				"accept": map[string]any{
					"type":        "boolean",
					"description": "是否接受（true）或关闭（false）对话框",
				},
				"prompt_text": map[string]any{
					"type":        "string",
					"description": "在 prompt 对话框中输入的文本",
				},
			},
			"required": []any{"dialog_id", "accept"},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				DialogID   string `json:"dialog_id"`
				Accept     bool   `json:"accept"`
				PromptText string `json:"prompt_text"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
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

			touchSession(session)
			return result(msg, nil)
		},
	)
}

// NewBrowserConsoleTool creates a browser_console tool.
func NewBrowserConsoleTool(cfg *BrowserToolConfig) *agentcore.Tool {
	_ = toolConfig(cfg)
	return toolParams(
		"browser_console",
		"获取当前页面的控制台消息和 JavaScript 错误。",
		map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			session, err := RequireActiveSession()
			if err != nil {
				return nil, err
			}
			touchSession(session)
			return result("Console messages are captured asynchronously. Use browser_evaluate to check page state.", nil)
		},
	)
}

// NewBrowserVisionTool creates a browser_vision tool.
func NewBrowserVisionTool(cfg *BrowserToolConfig) *agentcore.Tool {
	cfg = toolConfig(cfg)
	return toolParams(
		"browser_vision",
		"截取截图并使用视觉 AI 模型分析。询问关于页面内容的问题。",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "关于页面的问题（例如'主要标题是什么？'、'列出所有可见链接'）",
				},
				"annotate": map[string]any{
					"type":        "boolean",
					"description": "在交互元素上叠加编号标签（默认：false）",
					"default":     false,
				},
			},
			"required": []any{"question"},
		},
		func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Question string `json:"question"`
				Annotate bool   `json:"annotate"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
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
				if cErr := chromedpEval(session, cfg, chromedp.CaptureScreenshot(&screenshotData)); cErr != nil {
					return nil, fmt.Errorf("screenshot failed: %w", cErr)
				}
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

			touchSession(session)
			return result(analysis, nil)
		},
	)
}
