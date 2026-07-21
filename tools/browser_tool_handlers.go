package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// --- Page state and script evaluation handlers ---

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
