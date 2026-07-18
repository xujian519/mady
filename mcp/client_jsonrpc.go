package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	return c.callWithRetry(ctx, method, params, out, 3)
}

func (c *Client) callWithRetry(ctx context.Context, method string, params any, out any, retriesLeft int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.cfg.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.RequestTimeout)
		defer cancel()
	}
	id := strconv.FormatInt(c.nextID.Add(1), 10)
	ch := make(chan rpcResponse, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errClientClosed
	}
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	if err := c.writeMessage(msg); err != nil {
		if errors.Is(err, errClientClosed) && retriesLeft > 0 && c.tryReconnect(ctx) {
			return c.callWithRetry(ctx, method, params, out, retriesLeft-1)
		}
		return fmt.Errorf("mcp %s: write: %w", method, err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			err := c.connectionError()
			if retriesLeft > 0 && c.tryReconnect(ctx) {
				return c.callWithRetry(ctx, method, params, out, retriesLeft-1)
			}
			return err
		}
		if resp.Error != nil {
			return fmt.Errorf("mcp %s: %s", method, resp.Error.Message)
		}
		if out == nil || len(resp.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("mcp %s decode result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closeCh:
		err := c.connectionError()
		if retriesLeft > 0 && c.tryReconnect(ctx) {
			return c.callWithRetry(ctx, method, params, out, retriesLeft-1)
		}
		return err
	}
}

func (c *Client) notify(method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	return c.writeMessage(msg)
}

func (c *Client) writeMessage(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mcp marshal request: %w", err)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return errClientClosed
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		// stdin 写失败意味着管道已断开（进程崩溃/退出），
		// 包裹 errClientClosed 以便 callWithRetry 触发重连。
		return fmt.Errorf("%w: %w", errClientClosed, err)
	}
	return nil
}
