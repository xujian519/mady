package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (c *HTTPClient) call(ctx context.Context, method string, params any, out any) (http.Header, error) {
	headers, err := c.callOnce(ctx, method, params, out)
	if err == nil || !errors.Is(err, errSessionExpired) || isInitializeMethod(method) {
		return headers, err
	}
	staleSession := expiredSessionID(err)
	c.emitReconnectEvent(ReconnectPhaseStarted, ReconnectReasonSessionExpired, 1, staleSession, "", "", nil)
	if err := c.reinitializeSession(ctx, expiredSessionID(err)); err != nil {
		c.emitReconnectEvent(ReconnectPhaseFailed, ReconnectReasonSessionExpired, 1, staleSession, "", "", err)
		return nil, err
	}
	sessionID, _ := c.sessionState()
	c.emitReconnectEvent(ReconnectPhaseSucceeded, ReconnectReasonSessionExpired, 1, staleSession, sessionID, "", nil)
	return c.callOnce(ctx, method, params, out)
}

func (c *HTTPClient) callOnce(ctx context.Context, method string, params any, out any) (http.Header, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.cfg.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.RequestTimeout)
		defer cancel()
	}
	id := strconv.FormatInt(c.nextID.Add(1), 10)
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	resp, err := c.doJSONRPC(ctx, msg, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rpcResp, err := c.decodeHTTPRPCResponse(ctx, resp.Body, resp.Header.Get("Content-Type"), id)
	if err != nil {
		return resp.Header, fmt.Errorf("mcp %s decode response: %w", method, err)
	}
	if rpcResp.Error != nil {
		return resp.Header, fmt.Errorf("mcp %s: %s", method, rpcResp.Error.Message)
	}
	if out != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, out); err != nil {
			return resp.Header, fmt.Errorf("mcp %s decode result: %w", method, err)
		}
	}
	return resp.Header, nil
}

func (c *HTTPClient) doJSONRPC(ctx context.Context, msg any, expectResponse bool) (*http.Response, error) {
	if c.closed.Load() {
		return nil, errClientClosed
	}
	var cancel context.CancelFunc
	ctx, cancel = mergeContext(ctx, c.bgCtx)
	defer cancel()
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("mcp marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("mcp create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if expectResponse {
		req.Header.Set("Accept", "application/json, text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	sessionID, negotiatedProto := c.sessionState()
	c.applyHeadersWithState(req, !isInitializeRequest(msg), !isInitializeRequest(msg), sessionID, negotiatedProto)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mcp http request: %w", err)
	}
	if expectResponse && resp.StatusCode == http.StatusNotFound && requestIncludesSession(msg) {
		resp.Body.Close()
		return nil, sessionExpiredError{sessionID: sessionID}
	}
	if expectResponse {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
			resp.Body.Close()
			return nil, fmt.Errorf("mcp http status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return resp, nil
	}
	if resp.StatusCode == http.StatusNotFound && requestIncludesSession(msg) {
		resp.Body.Close()
		return nil, sessionExpiredError{sessionID: sessionID}
	}
	if resp.StatusCode != http.StatusAccepted && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
		return nil, fmt.Errorf("mcp notify status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (c *HTTPClient) applyHeaders(req *http.Request, includeProtocol bool, includeSession bool) {
	sessionID, negotiatedProto := c.sessionState()
	c.applyHeadersWithState(req, includeProtocol, includeSession, sessionID, negotiatedProto)
}

func (c *HTTPClient) applyHeadersWithState(req *http.Request, includeProtocol bool, includeSession bool, sessionID, negotiatedProto string) {
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, v)
	}
	if includeSession && sessionID != "" {
		req.Header.Set(headerSessionID, sessionID)
	}
	if includeProtocol && negotiatedProto != "" {
		req.Header.Set(headerProtocolVersion, negotiatedProto)
	}
}

func isInitializeRequest(msg any) bool {
	m, ok := msg.(map[string]any)
	if !ok {
		return false
	}
	method, _ := m["method"].(string)
	return method == "initialize"
}

func isInitializeMethod(method string) bool {
	return method == "initialize" || method == "notifications/initialized"
}

func requestIncludesSession(msg any) bool {
	return !isInitializeRequest(msg)
}

func (c *HTTPClient) callInitialize(ctx context.Context, params any, out any) (http.Header, error) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      strconv.FormatInt(c.nextID.Add(1), 10),
		"method":  "initialize",
		"params":  params,
	}
	resp, err := c.doJSONRPC(ctx, msg, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return resp.Header, fmt.Errorf("mcp initialize decode response: %w", err)
	}
	if rpcResp.Error != nil {
		return resp.Header, fmt.Errorf("mcp initialize: %s", rpcResp.Error.Message)
	}
	if out != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, out); err != nil {
			return resp.Header, fmt.Errorf("mcp initialize decode result: %w", err)
		}
	}
	return resp.Header, nil
}

func (c *HTTPClient) notifyInitialize(ctx context.Context) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	resp, err := c.doJSONRPC(ctx, msg, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
