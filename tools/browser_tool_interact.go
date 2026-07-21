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
