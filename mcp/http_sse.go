package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (c *HTTPClient) decodeHTTPRPCResponse(ctx context.Context, body io.Reader, contentType string, expectedID string) (*rpcResponse, error) {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return c.decodeSSERPCResponse(ctx, body, expectedID)
	}
	var rpcResp rpcResponse
	if err := json.NewDecoder(body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	return &rpcResp, nil
}

func (c *HTTPClient) decodeSSERPCResponse(ctx context.Context, body io.Reader, expectedID string) (*rpcResponse, error) {
	state := sseStreamState{}
	resp, nextState, err := readSSERPCResponse(body, expectedID)
	if err != nil {
		return nil, err
	}
	if resp != nil {
		return resp, nil
	}
	state.merge(nextState)
	for {
		if state.lastEventID == "" {
			return nil, fmt.Errorf("mcp sse stream ended before response %q", expectedID)
		}
		if err := sleepContext(ctx, state.retry); err != nil {
			return nil, err
		}
		resumeResp, err := c.resumeSSE(ctx, state.lastEventID)
		if err != nil {
			return nil, err
		}
		resp, nextState, readErr := readSSERPCResponse(resumeResp.Body, expectedID)
		_ = resumeResp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp != nil {
			return resp, nil
		}
		state.merge(nextState)
	}
}

func readSSERPCResponse(body io.Reader, expectedID string) (*rpcResponse, sseStreamState, error) {
	var matchedResp *rpcResponse
	state, err := consumeSSEStream(body, func(evt sseEvent) (bool, error) {
		resp, matched, err := handleSSEEvent(evt, expectedID)
		if err != nil {
			return false, err
		}
		if matched {
			matchedResp = resp
			return true, nil
		}
		return false, nil
	})
	return matchedResp, state, err
}

func consumeSSEStream(body io.Reader, handler func(sseEvent) (bool, error)) (sseStreamState, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	var evt sseEvent
	var state sseStreamState
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			state.merge(eventState(evt))
			stop, err := handler(evt)
			if err != nil {
				return state, err
			}
			if stop {
				return state, nil
			}
			evt = sseEvent{}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value := splitSSEField(line)
		switch field {
		case "data":
			evt.data = append(evt.data, value)
		case "id":
			evt.id = value
		case "event":
			evt.event = value
		case "retry":
			evt.retry = value
		}
	}
	if err := scanner.Err(); err != nil {
		return state, err
	}
	if len(evt.data) > 0 || evt.id != "" || evt.event != "" || evt.retry != "" {
		state.merge(eventState(evt))
		stop, err := handler(evt)
		if err != nil {
			return state, err
		}
		if stop {
			return state, nil
		}
	}
	return state, nil
}

type sseEvent struct {
	id    string
	event string
	retry string
	data  []string
}

type sseStreamState struct {
	lastEventID string
	retry       time.Duration
}

func eventState(evt sseEvent) sseStreamState {
	state := sseStreamState{lastEventID: strings.TrimSpace(evt.id)}
	if evt.retry != "" {
		if ms, err := strconv.Atoi(strings.TrimSpace(evt.retry)); err == nil && ms >= 0 {
			state.retry = time.Duration(ms) * time.Millisecond
		}
	}
	return state
}

func (s *sseStreamState) merge(other sseStreamState) {
	if other.lastEventID != "" {
		s.lastEventID = other.lastEventID
	}
	if other.retry > 0 {
		s.retry = other.retry
	}
}

func handleSSEEvent(evt sseEvent, expectedID string) (*rpcResponse, bool, error) {
	payload := strings.TrimSpace(strings.Join(evt.data, "\n"))
	if payload == "" {
		return nil, false, nil
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return nil, false, fmt.Errorf("invalid sse payload: %w", err)
	}

	idRaw, ok := envelope["id"]
	if !ok {
		return nil, false, nil
	}
	var msgID any
	if err := json.Unmarshal(idRaw, &msgID); err != nil {
		return nil, false, fmt.Errorf("decode sse response id: %w", err)
	}
	if fmt.Sprint(msgID) != expectedID {
		return nil, false, nil
	}

	var resp rpcResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		return nil, false, fmt.Errorf("decode sse response: %w", err)
	}
	return &resp, true, nil
}

func splitSSEField(line string) (string, string) {
	field, value, ok := strings.Cut(line, ":")
	if !ok {
		return line, ""
	}
	value = strings.TrimPrefix(value, " ")
	return field, value
}

func (c *HTTPClient) resumeSSE(ctx context.Context, lastEventID string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.Endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp create resume request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set(headerLastEventID, lastEventID)
	c.applyHeaders(req, true, true)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mcp resume request: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		sessionID, _ := c.sessionState()
		return nil, sessionExpiredError{sessionID: sessionID}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
		return nil, fmt.Errorf("mcp resume status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
		return nil, fmt.Errorf("mcp resume expected text/event-stream, got %q: %s", resp.Header.Get("Content-Type"), strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
