package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// --- Navigation handlers ---

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

	if session.backendType == BackendEgoLite {
		navResult, navErr := session.egoLiteManager.Send(ctx, "navigate", map[string]any{
			"url":     parsedURL.String(),
			"timeout": float64(20),
		})
		if navErr != nil {
			return nil, fmt.Errorf("egolite navigate: %w", navErr)
		}
		sv, _ := navResult.(string)
		// get pageInfo to update session
		if piResult, piErr := session.egoLiteManager.Send(ctx, "pageInfo", nil); piErr == nil {
			if pi, ok := piResult.(map[string]any); ok {
				session.mu.Lock()
				if u, ok := pi["url"].(string); ok {
					session.url = u
				}
				if t, ok := pi["title"].(string); ok {
					session.title = t
				}
				session.lastActivity = time.Now()
				session.mu.Unlock()
			}
		}
		session.mu.RLock()
		url := session.url
		title := session.title
		session.mu.RUnlock()
		if title == "" {
			title = "(unknown)"
		}
		return result(fmt.Sprintf("Navigated to %s\nTitle: %s\n\n%s", url, title, sv), nil)
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
