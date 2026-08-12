package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

//nolint:gocognit // 原因：A2A 客户端调用，含重试和指数退避
func (c *Client) call(ctx context.Context, method string, params any) (*JSONRPCResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.retryBackoff * time.Duration(1<<uint(attempt-1))
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
			timer.Stop()
		}

		rpcReq := JSONRPCRequest{JSONRPC: "2.0", ID: c.nextID(), Method: method}
		reqID := rpcReq.ID
		if params != nil {
			data, err := json.Marshal(params)
			if err != nil {
				return nil, err
			}
			rpcReq.Params = data
		}

		body, err := json.Marshal(rpcReq)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		c.setAuthHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if !isRetryableError(err) {
				return nil, err
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
			if isRetryableStatus(resp.StatusCode) {
				continue
			}
			return nil, lastErr
		}

		var rpcResp JSONRPCResponse
		decErr := json.NewDecoder(resp.Body).Decode(&rpcResp)
		// Drain any remaining bytes so the connection can be reused
		// (keep-alive). json.Decoder may stop after the JSON value.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if decErr != nil {
			return nil, fmt.Errorf("decode response: %w", decErr)
		}

		if rpcResp.ID != nil {
			if wantID, ok := reqID.(int64); ok {
				if !matchID(rpcResp.ID, wantID) {
					return nil, fmt.Errorf("response ID mismatch: want %v, got %v", reqID, rpcResp.ID)
				}
			}
		}

		return &rpcResp, nil
	}
	return nil, fmt.Errorf("after %d retries: %w", c.maxRetries, lastErr)
}

// callAndDecode calls the given JSON-RPC method and decodes the result into
// result.  If result is nil, only the error check is performed (no decode).
func (c *Client) callAndDecode(ctx context.Context, method string, req any, result any) error {
	resp, err := c.call(ctx, method, req)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	if result == nil {
		return nil
	}
	data, err := json.Marshal(resp.Result)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Timeout()
	}
	return false
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func (c *Client) nextID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.idCounter++
	return c.idCounter
}

func matchID(got any, want int64) bool {
	switch v := got.(type) {
	case float64:
		return v == float64(want)
	case int64:
		return v == want
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return false
		}
		return n == want
	case string:
		return v == fmt.Sprintf("%d", want)
	default:
		return false
	}
}
