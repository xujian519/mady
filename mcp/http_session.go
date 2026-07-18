package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
)

func (c *HTTPClient) emitReconnectEvent(phase, reason string, attempt int, staleSessionID, sessionID, lastEventID string, err error) {
	c.emitRuntimeEvent(ReconnectEvent{
		At:             time.Now(),
		Extension:      c.extensionName(),
		Transport:      "http",
		Phase:          phase,
		Reason:         reason,
		Attempt:        attempt,
		StaleSessionID: staleSessionID,
		SessionID:      sessionID,
		LastEventID:    lastEventID,
		Error:          util.ErrorString(err),
	})
}

func (c *HTTPClient) emitTransportError(operation, reason string, err error, statusCode int, sessionID, lastEventID string, recoverable bool) {
	c.emitRuntimeEvent(TransportErrorEvent{
		At:          time.Now(),
		Extension:   c.extensionName(),
		Transport:   "http",
		Operation:   operation,
		Reason:      reason,
		Message:     util.ErrorString(err),
		StatusCode:  statusCode,
		SessionID:   sessionID,
		LastEventID: lastEventID,
		Recoverable: recoverable,
	})
}

func (c *HTTPClient) sessionState() (string, string) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.sessionID, c.negotiatedProto
}

func (c *HTTPClient) setSessionState(sessionID, negotiatedProto string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.sessionID = sessionID
	c.negotiatedProto = negotiatedProto
}

func (c *HTTPClient) reinitializeSession(ctx context.Context, staleSession string) error {
	if c.closed.Load() {
		return errClientClosed
	}
	c.initMu.Lock()
	defer c.initMu.Unlock()
	if c.closed.Load() {
		return errClientClosed
	}
	currentSession, _ := c.sessionState()
	if staleSession != "" && currentSession != "" && currentSession != staleSession {
		c.emitReconnectEvent(ReconnectPhaseSkipped, ReconnectReasonSessionExpired, 0, staleSession, currentSession, "", nil)
		return nil
	}
	return c.initializeSession(ctx)
}

func expiredSessionID(err error) string {
	var sessionErr sessionExpiredError
	if errors.As(err, &sessionErr) {
		return sessionErr.sessionID
	}
	return ""
}

func (c *HTTPClient) runServerStream() {
	defer close(c.streamDone)
	state := sseStreamState{}
	reconnectAttempts := 0
	for {
		select {
		case <-c.bgCtx.Done():
			return
		default:
		}

		nextState, err := c.listenServerStreamOnce(c.bgCtx, state.lastEventID)
		if err == nil {
			if reconnectAttempts > 0 {
				c.emitReconnectEvent(ReconnectPhaseSucceeded, ReconnectReasonServerStreamWake, reconnectAttempts, "", "", state.lastEventID, nil)
				reconnectAttempts = 0
			}
			state.merge(nextState)
			if state.retry <= 0 {
				state.retry = time.Second
			}
			if err := sleepContext(c.bgCtx, state.retry); err != nil {
				return
			}
			continue
		}
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, errServerStreamUnsupported) {
			c.emitTransportError("server_stream", "server_stream_unsupported", err, 0, "", state.lastEventID, false)
			return
		}
		if errors.Is(err, errSessionExpired) {
			reconnectAttempts++
			staleSession := expiredSessionID(err)
			c.emitReconnectEvent(ReconnectPhaseStarted, ReconnectReasonServerStream404, reconnectAttempts, staleSession, "", state.lastEventID, nil)
			if reinitErr := c.reinitializeSession(c.bgCtx, staleSession); reinitErr != nil {
				c.emitReconnectEvent(ReconnectPhaseFailed, ReconnectReasonServerStream404, reconnectAttempts, staleSession, "", state.lastEventID, reinitErr)
				c.reportAsyncError(reinitErr)
				if err := sleepContext(c.bgCtx, time.Second); err != nil {
					return
				}
				continue
			}
			sessionID, _ := c.sessionState()
			c.emitReconnectEvent(ReconnectPhaseSucceeded, ReconnectReasonServerStream404, reconnectAttempts, staleSession, sessionID, state.lastEventID, nil)
			state = sseStreamState{}
			continue
		}
		reconnectAttempts++
		c.emitTransportError("server_stream", ReconnectReasonServerStreamEOF, err, 0, "", state.lastEventID, true)
		c.reportAsyncError(err)
		if state.retry <= 0 {
			state.retry = time.Second
		}
		if err := sleepContext(c.bgCtx, state.retry); err != nil {
			return
		}
	}
}

func (c *HTTPClient) listenServerStreamOnce(ctx context.Context, lastEventID string) (sseStreamState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.Endpoint, nil)
	if err != nil {
		return sseStreamState{}, fmt.Errorf("mcp create server stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID != "" {
		req.Header.Set(headerLastEventID, lastEventID)
	}
	c.applyHeaders(req, true, true)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return sseStreamState{}, fmt.Errorf("mcp server stream request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		sessionID, _ := c.sessionState()
		return sseStreamState{}, sessionExpiredError{sessionID: sessionID}
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		return sseStreamState{}, errServerStreamUnsupported
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		return sseStreamState{}, fmt.Errorf("mcp server stream status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		return sseStreamState{}, fmt.Errorf("mcp server stream expected text/event-stream, got %q: %s", resp.Header.Get("Content-Type"), strings.TrimSpace(string(body)))
	}

	return consumeSSEStream(resp.Body, func(evt sseEvent) (bool, error) {
		return false, c.handleServerSSEEvent(ctx, evt)
	})
}

func (c *HTTPClient) handleServerSSEEvent(ctx context.Context, evt sseEvent) error {
	payload := strings.TrimSpace(strings.Join(evt.data, "\n"))
	if payload == "" {
		return nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return fmt.Errorf("invalid server sse payload: %w", err)
	}

	methodRaw, hasMethod := envelope["method"]
	if !hasMethod {
		return nil
	}
	var method string
	if err := json.Unmarshal(methodRaw, &method); err != nil {
		return fmt.Errorf("decode server message method: %w", err)
	}
	params := envelope["params"]
	if len(params) == 0 {
		params = json.RawMessage(`null`)
	}

	idRaw, hasID := envelope["id"]
	if !hasID {
		if err := c.handleDiscoveryNotification(ctx, method, params); err != nil {
			return err
		}
		for _, hook := range c.notificationHookSnapshot() {
			if err := hook(ctx, method, params); err != nil {
				return err
			}
		}
		if c.cfg.NotificationHandler != nil {
			if err := c.cfg.NotificationHandler(ctx, method, params); err != nil {
				return fmt.Errorf("handle server notification %s: %w", method, err)
			}
		}
		return nil
	}

	var reqID any
	if err := json.Unmarshal(idRaw, &reqID); err != nil {
		return fmt.Errorf("decode server request id: %w", err)
	}
	var result any
	var handlerErr error
	if c.cfg.RequestHandler != nil {
		result, handlerErr = c.cfg.RequestHandler(ctx, method, params)
	} else {
		handlerErr = fmt.Errorf("no request handler configured")
	}
	return c.respondToServerRequest(ctx, reqID, result, handlerErr)
}

func (c *HTTPClient) respondToServerRequest(ctx context.Context, id any, result any, handlerErr error) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
	}
	if handlerErr != nil {
		msg["error"] = map[string]any{
			"code":    -32601,
			"message": handlerErr.Error(),
		}
	} else {
		msg["result"] = result
	}
	resp, err := c.doJSONRPC(ctx, msg, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *HTTPClient) reportAsyncError(err error) {
	if err == nil {
		return
	}
	if c.cfg.ErrorHandler != nil {
		c.cfg.ErrorHandler(c.bgCtx, err)
	}
}

func (c *HTTPClient) AddNotificationHook(h func(context.Context, string, json.RawMessage) error) {
	if h == nil {
		return
	}
	c.hooksMu.Lock()
	defer c.hooksMu.Unlock()
	c.notificationHooks = append(c.notificationHooks, h)
}

func (c *HTTPClient) notificationHookSnapshot() []func(context.Context, string, json.RawMessage) error {
	c.hooksMu.RLock()
	defer c.hooksMu.RUnlock()
	return append([]func(context.Context, string, json.RawMessage) error(nil), c.notificationHooks...)
}

func (c *HTTPClient) SetEventSink(emit func(agentcore.Event)) {
	c.eventSink.Set(emit)
}

func (c *HTTPClient) emitRuntimeEvent(event agentcore.Event) {
	c.eventSink.Emit(event)
}
