package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// --- Page state and script handlers ---

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
