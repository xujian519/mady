package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
)

// emitReconnectEvent 发出 ReconnectEvent 用于可观测性，与 HTTP 客户端对齐。
func (c *Client) emitReconnectEvent(phase, reason string, attempt int, err error) {
	evt := ReconnectEvent{
		At:        time.Now(),
		Extension: c.extensionName(),
		Transport: "stdio",
		Phase:     phase,
		Reason:    reason,
		Attempt:   attempt,
	}
	if err != nil {
		evt.Error = err.Error()
	}
	c.emitRuntimeEvent(evt)
}

func (c *Client) tryReconnect(ctx context.Context) bool {
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	// If we already have a working connection (readLoop is running), don't reconnect
	if !c.isConnectionDeadLocked() {
		c.mu.Unlock()
		return true // connection is fine
	}
	c.mu.Unlock()

	c.reconnectAttempts++
	c.emitReconnectEvent(ReconnectPhaseStarted, "connection_closed", c.reconnectAttempts, nil)

	backoff := c.reconnectBackoff
	if backoff == 0 {
		backoff = initialReconnectDelay
	}
	c.reconnectBackoff = backoff * 2
	if c.reconnectBackoff > maxReconnectBackoff {
		c.reconnectBackoff = maxReconnectBackoff
	}

	// 连续重连失败超过阈值后重置退避，防止长时间停留在最大间隔。
	// 注意：reconnectAttempts 在本函数开头已递增（++），此处重置为 0 后，
	// 下一次 tryReconnect 调用时将从 1 开始重新计数。
	if c.reconnectAttempts > maxReconnectAttempts {
		c.reconnectBackoff = initialReconnectDelay
		c.reconnectAttempts = 0
	}

	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
	}

	// Spawn new subprocess
	cmd := exec.Command(c.cfg.Command, c.cfg.Args...)
	if c.cfg.Dir != "" {
		cmd.Dir = c.cfg.Dir
	}
	if len(c.cfg.Env) > 0 {
		cmd.Env = append(cmd.Environ(), c.cfg.Env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		c.emitReconnectEvent(ReconnectPhaseFailed, "stdin_pipe", c.reconnectAttempts, err)
		return false
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		c.emitReconnectEvent(ReconnectPhaseFailed, "stdout_pipe", c.reconnectAttempts, err)
		return false
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		c.emitReconnectEvent(ReconnectPhaseFailed, "stderr_pipe", c.reconnectAttempts, err)
		return false
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		c.emitReconnectEvent(ReconnectPhaseFailed, "cmd_start", c.reconnectAttempts, err)
		return false
	}

	// Replace pipes atomically
	c.mu.Lock()
	oldStdin := c.stdin
	oldStdout := c.stdout
	oldStderr := c.stderr
	c.cmd = cmd
	c.stdin = stdin
	c.stdout = stdout
	c.stderr = stderr
	c.readErr = nil
	c.errBuf.Reset()
	// Reset pending map — old requests are canceled
	for id, ch := range c.pending {
		delete(c.pending, id)
		close(ch)
	}
	c.mu.Unlock()

	// Close old pipes
	if oldStdin != nil {
		_ = oldStdin.Close()
	}
	if oldStdout != nil {
		_ = oldStdout.Close()
	}
	if oldStderr != nil {
		_ = oldStderr.Close()
	}

	// Start new readers
	go c.readLoop()
	go c.captureStderr(stderr)

	// Re-initialize MCP protocol
	if err := c.initialize(ctx); err != nil {
		// 仅清理本次重连产生的子进程和管道，不永久关闭客户端。
		// 保留连接死亡状态，后续调用仍可再次触发重连。
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		// 异步回收子进程，避免 reconnect 阻塞，但给一个超时避免无限泄漏。
		go func() {
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case <-done:
			case <-timer.C:
				log.Printf("mcp client: cmd.Wait did not return after reconnect cleanup")
			}
		}()
		c.emitReconnectEvent(ReconnectPhaseFailed, "initialize_failed", c.reconnectAttempts, err)
		return false
	}

	c.reconnectBackoff = initialReconnectDelay
	c.reconnectAttempts = 0
	c.emitReconnectEvent(ReconnectPhaseSucceeded, "reconnect", 0, nil)
	return true
}

func (c *Client) isConnectionDeadLocked() bool {
	return c.readErr != nil
}

func (c *Client) stdoutContext() context.Context {
	return context.Background()
}

func (c *Client) handleServerMessage(ctx context.Context, line string, methodRaw json.RawMessage, idRaw json.RawMessage) error {
	var method string
	if err := json.Unmarshal(methodRaw, &method); err != nil {
		return fmt.Errorf("mcp unmarshal method: %w", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		return fmt.Errorf("mcp unmarshal envelope: %w", err)
	}
	params := envelope["params"]
	if len(params) == 0 {
		params = json.RawMessage(`null`)
	}
	if len(idRaw) == 0 {
		if err := c.handleDiscoveryNotification(ctx, method, params); err != nil {
			c.reportAsyncError("notification", method, err, true)
		}
		for _, hook := range c.notificationHookSnapshot() {
			if err := hook(ctx, method, params); err != nil {
				c.reportAsyncError("notification", method, err, true)
			}
		}
		if c.cfg.NotificationHandler != nil {
			if err := c.cfg.NotificationHandler(ctx, method, params); err != nil {
				c.reportAsyncError("notification", method, err, true)
			}
		}
		return nil
	}
	var reqID any
	if err := json.Unmarshal(idRaw, &reqID); err != nil {
		return fmt.Errorf("mcp unmarshal request id: %w", err)
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

func (c *Client) respondToServerRequest(ctx context.Context, id any, result any, handlerErr error) error {
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
	return c.writeMessage(msg)
}

func (c *Client) reportAsyncError(operation, reason string, err error, recoverable bool) {
	if err == nil {
		return
	}
	c.emitTransportError(operation, reason, err, recoverable)
	if c.cfg.ErrorHandler != nil {
		c.cfg.ErrorHandler(context.Background(), err)
	}
}

func (c *Client) AddNotificationHook(h func(context.Context, string, json.RawMessage) error) {
	if h == nil {
		return
	}
	c.hooksMu.Lock()
	defer c.hooksMu.Unlock()
	c.notificationHooks = append(c.notificationHooks, h)
}

func (c *Client) notificationHookSnapshot() []func(context.Context, string, json.RawMessage) error {
	c.hooksMu.RLock()
	defer c.hooksMu.RUnlock()
	return append([]func(context.Context, string, json.RawMessage) error(nil), c.notificationHooks...)
}

func (c *Client) SetEventSink(emit func(agentcore.Event)) {
	c.eventSink.Set(emit)
}

func (c *Client) emitRuntimeEvent(event agentcore.Event) {
	c.eventSink.Emit(event)
}

func (c *Client) emitTransportError(operation, reason string, err error, recoverable bool) {
	c.emitRuntimeEvent(TransportErrorEvent{
		At:          time.Now(),
		Extension:   c.extensionName(),
		Transport:   "stdio",
		Operation:   operation,
		Reason:      reason,
		Message:     util.ErrorString(err),
		Recoverable: recoverable,
	})
}

func (c *Client) extensionName() string {
	return util.DefaultString(c.cfg.Name, "mcp")
}
