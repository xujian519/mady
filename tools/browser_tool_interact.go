package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// --- Interaction handlers ---

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

	var snapshot string

	switch session.backendType {
	case BackendCamofox:
		snap, cfErr := session.camofoxClient.Click(session.sessionID, ref)
		if cfErr != nil {
			return nil, fmt.Errorf("camofox click failed: %w", cfErr)
		}
		session.mu.Lock()
		session.lastActivity = time.Now()
		session.mu.Unlock()
		return result(fmt.Sprintf("Clicked %s\n\n%s", ref, snap), nil)

	case BackendEgoLite:
		snapResult, clickErr := session.egoLiteManager.Send(ctx, "click", map[string]any{"ref": ref})
		if clickErr != nil {
			return nil, fmt.Errorf("egolite click: %w", clickErr)
		}
		sv, _ := snapResult.(string)
		snapshot = sv

	default:
		// chromedp-based backends.
		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		defer cancel()

		xpath, xErr := getXPathFromRef(timeoutCtx, session, ref)
		if xErr != nil {
			return nil, xErr
		}

		if cErr := chromedp.Run(timeoutCtx, chromedp.Click(xpath, chromedp.BySearch)); cErr != nil {
			return nil, fmt.Errorf("click failed for %s: %w", ref, cErr)
		}

		time.Sleep(500 * time.Millisecond)

		snap, snapErr := generateSnapshot(session.ctx, false, session.refMapper)
		if snapErr != nil {
			snapshot = fmt.Sprintf("(snapshot unavailable: %v)", snapErr)
		} else {
			snapshot = snap
		}
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

	var resultMsg string

	switch session.backendType {
	case BackendCamofox:
		rMsg, cfErr := session.camofoxClient.Type(session.sessionID, ref, input.Text)
		if cfErr != nil {
			return nil, fmt.Errorf("camofox type failed: %w", cfErr)
		}
		session.mu.Lock()
		session.lastActivity = time.Now()
		session.mu.Unlock()
		return result(rMsg, nil)

	case BackendEgoLite:
		typeResult, typeErr := session.egoLiteManager.Send(ctx, "typeText", map[string]any{
			"ref": ref, "text": input.Text,
		})
		if typeErr != nil {
			return nil, fmt.Errorf("egolite type: %w", typeErr)
		}
		rv, _ := typeResult.(string)
		resultMsg = rv

	default:
		// chromedp-based backends.
		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		defer cancel()

		xpath, xErr := getXPathFromRef(timeoutCtx, session, ref)
		if xErr != nil {
			return nil, xErr
		}

		if cErr := chromedp.Run(timeoutCtx,
			chromedp.Clear(xpath, chromedp.BySearch),
			chromedp.SendKeys(xpath, input.Text, chromedp.BySearch),
		); cErr != nil {
			return nil, fmt.Errorf("type failed for %s: %w", ref, cErr)
		}

		resultMsg = fmt.Sprintf("Typed \"%s\" into %s", input.Text, ref)
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(resultMsg, nil)
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

	var snapshot string

	switch session.backendType {
	case BackendCamofox:
		snap, cfErr := session.camofoxClient.Scroll(session.sessionID, input.Direction)
		if cfErr != nil {
			return nil, fmt.Errorf("camofox scroll failed: %w", cfErr)
		}
		session.mu.Lock()
		session.lastActivity = time.Now()
		session.mu.Unlock()
		return result(fmt.Sprintf("Scrolled %s\n\n%s", input.Direction, snap), nil)

	case BackendEgoLite:
		dy := any(500)
		if input.Direction == "up" {
			dy = -500
		}
		scrollResult, scrollErr := session.egoLiteManager.Send(ctx, "scroll", map[string]any{"dy": dy})
		if scrollErr != nil {
			return nil, fmt.Errorf("egolite scroll: %w", scrollErr)
		}
		sv, _ := scrollResult.(string)
		snapshot = sv

	default:
		// chromedp-based backends.
		script := "window.scrollBy(0, 500);"
		if input.Direction == "up" {
			script = "window.scrollBy(0, -500);"
		}

		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		defer cancel()

		var res any
		if cErr := chromedp.Run(timeoutCtx, chromedp.Evaluate(script, &res)); cErr != nil {
			return nil, fmt.Errorf("scroll failed: %w", cErr)
		}

		snap, snapErr := generateSnapshot(timeoutCtx, false, session.refMapper)
		if snapErr != nil {
			snapshot = fmt.Sprintf("(snapshot unavailable: %v)", snapErr)
		} else {
			snapshot = snap
		}
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(fmt.Sprintf("Scrolled %s\n\n%s", input.Direction, snapshot), nil)
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

	var resultMsg string

	switch session.backendType {
	case BackendCamofox:
		rMsg, cfErr := session.camofoxClient.Press(session.sessionID, input.Key)
		if cfErr != nil {
			return nil, fmt.Errorf("camofox key press failed: %w", cfErr)
		}
		session.mu.Lock()
		session.lastActivity = time.Now()
		session.mu.Unlock()
		return result(rMsg, nil)

	case BackendEgoLite:
		pressResult, pressErr := session.egoLiteManager.Send(ctx, "pressKey", map[string]any{"key": input.Key})
		if pressErr != nil {
			return nil, fmt.Errorf("egolite press: %w", pressErr)
		}
		rv, _ := pressResult.(string)
		resultMsg = rv

	default:
		// chromedp-based backends.
		timeoutCtx, cancel := context.WithTimeout(session.ctx, cfg.CommandTimeout)
		defer cancel()

		if cErr := chromedp.Run(timeoutCtx, chromedp.KeyEvent(input.Key)); cErr != nil {
			return nil, fmt.Errorf("key press failed: %w", cErr)
		}

		resultMsg = fmt.Sprintf("Pressed key: %s", input.Key)
	}

	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()

	return result(resultMsg, nil)
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
