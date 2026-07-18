package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

func (c *Client) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			c.reportAsyncError("read_loop", "panic",
				fmt.Errorf("readLoop panic: %v\n%s", r, debug.Stack()), true)
		}
	}()
	scanner := bufio.NewScanner(c.stdout)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			c.reportAsyncError("read_loop", "unmarshal",
				fmt.Errorf("unmarshal line: %w", err), false)
			continue
		}
		methodRaw, hasMethod := raw["method"]
		idRaw, ok := raw["id"]
		if !ok {
			if hasMethod {
				if err := c.handleServerMessage(c.stdoutContext(), line, methodRaw, nil); err != nil {
					c.reportAsyncError("notification", "server_message", err, true)
				}
			}
			continue
		}
		if hasMethod {
			if err := c.handleServerMessage(c.stdoutContext(), line, methodRaw, idRaw); err != nil {
				c.reportAsyncError("request", "server_message", err, true)
			}
			continue
		}
		var id any
		if err := json.Unmarshal(idRaw, &id); err != nil {
			continue
		}
		key := fmt.Sprint(id)
		var resp rpcResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		c.mu.Lock()
		ch := c.pending[key]
		if ch != nil {
			select {
			case ch <- resp:
			default:
				// Channel full — duplicate or stale response. Clean up to avoid
				// further drops and report for observability.
				delete(c.pending, key)
				c.mu.Unlock()
				c.reportAsyncError("read_loop", "duplicate_response",
					fmt.Errorf("dropped response for id %q: pending channel full", key), true)
				continue
			}
		}
		c.mu.Unlock()
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	c.mu.Lock()
	c.readErr = err
	for id, ch := range c.pending {
		delete(c.pending, id)
		close(ch)
	}
	closed := c.closed
	c.mu.Unlock()
	if !closed {
		c.reportAsyncError("read_loop", "connection_closed", c.connectionError(), false)
	}
}

func (c *Client) captureStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 16*1024), 1024*1024)
	for scanner.Scan() {
		c.mu.Lock()
		c.appendStderrLine(scanner.Text())
		c.mu.Unlock()
	}
}

func (c *Client) appendStderrLine(line string) {
	if c.errBuf.Len() > 0 {
		c.errBuf.WriteByte('\n')
	}
	c.errBuf.WriteString(line)
	if c.errBuf.Len() <= stderrContextMaxBytes {
		return
	}
	data := append([]byte(nil), c.errBuf.Bytes()[c.errBuf.Len()-stderrContextMaxBytes:]...)
	c.errBuf.Reset()
	c.errBuf.Write(data)
}

func (c *Client) connectionError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readErr != nil && c.readErr != io.EOF {
		return agentcore.NewRetryableError("mcp_connection", "connection closed", c.readErr)
	}
	return errClientClosed
}
