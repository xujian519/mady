package tools

// Debug/utility handlers extracted from browser_tool_handlers.go.
// RequireActiveSession is defined in browser_session.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// --- Debug/utility handlers ---

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

	if session.backendType == BackendEgoLite {
		return nil, fmt.Errorf("cdp not supported for egolite backend")
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
