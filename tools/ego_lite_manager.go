// ego_lite_manager.go — EgoLite 浏览器子进程管理器
package tools

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed ego_lite_bridge.js
var egoLiteBridgeScript []byte

type egoLiteJSONRequest struct {
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

type egoLiteJSONResponse struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// EgoLiteManager 管理一个持久化 ego-browser 子进程。
// Send() 方法是唯一的公开 API，内部保证命令串行化。
type EgoLiteManager struct {
	mu      sync.Mutex
	pending map[string]chan egoLiteJSONResponse
	counter atomic.Int64

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner

	taskName    string
	currentTask string

	closed atomic.Bool
	ctx    context.Context
	cancel context.CancelFunc
}

// egoLiteAvailable 检查 ego-browser 是否可执行。
func egoLiteAvailable() bool {
	_, err := exec.LookPath("ego-browser")
	return err == nil
}

// NewEgoLiteManager 创建一个未启动的管理器。首次 Send() 调用时延迟启动子进程。
func NewEgoLiteManager(taskName string) (*EgoLiteManager, error) {
	if !egoLiteAvailable() {
		return nil, fmt.Errorf("egolite: ego-browser not found on PATH (install ego lite: https://lite.ego.app/)")
	}
	if taskName == "" {
		taskName = "mady-agent"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &EgoLiteManager{
		taskName: taskName,
		pending:  make(map[string]chan egoLiteJSONResponse),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// Send 发送命令并等待响应。mu 确保同一时刻只有一个命令在执行。
func (m *EgoLiteManager) Send(ctx context.Context, method string, params map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed.Load() {
		return nil, fmt.Errorf("egolite: manager closed")
	}
	if m.cmd == nil || m.stdin == nil {
		if err := m.start(ctx); err != nil {
			return nil, fmt.Errorf("egolite: start failed: %w", err)
		}
	}

	result, err := m.sendRaw(ctx, method, params)
	if err != nil {
		slog.Warn("egolite: send failed, attempting restart", "method", method, "err", err)
		if restartErr := m.restart(ctx); restartErr != nil {
			return nil, fmt.Errorf("egolite: send failed and restart error: %w (original: %w)", restartErr, err)
		}
		result, err = m.sendRaw(ctx, method, params)
		if err != nil {
			return nil, fmt.Errorf("egolite: send after restart failed: %w", err)
		}
	}
	return result, nil
}

// start 启动 ego-browser 子进程。写入 bridge 脚本到 stdin 后初始化任务空间。
func (m *EgoLiteManager) start(ctx context.Context) error {
	m.cmd = exec.CommandContext(m.ctx, "ego-browser", "nodejs")

	stdin, err := m.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	m.stdout = bufio.NewScanner(stdout)
	m.stdout.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("start ego-browser: %w", err)
	}

	// Write bridge script to stdin first. ego-browser nodejs reads script from stdin,
	// then the bridge's readline interface begins processing JSON commands.
	if _, err := stdin.Write(egoLiteBridgeScript); err != nil {
		return fmt.Errorf("write bridge script: %w", err)
	}
	if _, err := stdin.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write newline after bridge: %w", err)
	}
	m.stdin = stdin

	// Start stdout reader goroutine
	go m.readLoop()

	// Initialize task space
	result, err := m.sendRaw(ctx, "initTaskSpace", map[string]any{"name": m.taskName})
	if err != nil {
		m.cmd.Process.Kill()
		_ = m.cmd.Wait()
		m.cmd = nil
		m.stdin = nil
		m.stdout = nil
		return fmt.Errorf("init task space: %w", err)
	}

	if resultMap, ok := result.(map[string]any); ok {
		if id, ok := resultMap["id"]; ok {
			m.currentTask = fmt.Sprintf("%v", id)
		}
	}
	slog.Info("egolite: manager started", "task", m.currentTask)
	return nil
}

// sendRaw 发送命令但不持有 m.mu（内部使用，调用者必须已持有 mu）。
func (m *EgoLiteManager) sendRaw(ctx context.Context, method string, params map[string]any) (any, error) {
	id := fmt.Sprintf("r%d", m.counter.Add(1))
	req := egoLiteJSONRequest{ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	ch := make(chan egoLiteJSONResponse, 1)
	m.pending[id] = ch
	defer func() { delete(m.pending, id) }()

	if _, err := m.stdin.Write(append(data, '\n')); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		if !resp.OK {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// readLoop 持续从 stdout 读取 JSON 行并匹配 pending 请求。
func (m *EgoLiteManager) readLoop() {
	scanner := m.stdout
	for scanner.Scan() {
		line := scanner.Bytes()
		var resp egoLiteJSONResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			slog.Warn("egolite: bad JSON from bridge", "line", string(line), "err", err)
			continue
		}
		m.mu.Lock()
		if ch, ok := m.pending[resp.ID]; ok {
			ch <- resp
		}
		m.mu.Unlock()
	}
	if !m.closed.Load() {
		slog.Warn("egolite: bridge stdout closed unexpectedly")
	}
}

// restart 杀死旧子进程并重新启动。
func (m *EgoLiteManager) restart(ctx context.Context) error {
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
		_ = m.cmd.Wait()
	}
	m.cmd = nil
	m.stdin = nil
	m.stdout = nil
	return m.start(ctx)
}

// Close 关闭管理器，清理桥进程。幂等。
func (m *EgoLiteManager) Close() error {
	// 1. Attempt graceful shutdown first (bypass closed check)
	m.mu.Lock()
	if m.cmd != nil && m.stdin != nil && m.currentTask != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = m.sendRaw(ctx, "completeTaskSpace", map[string]any{"keep": false})
	}
	m.mu.Unlock()

	// 2. Mark closed and terminate
	if !m.closed.CompareAndSwap(false, true) {
		return nil
	}
	m.cancel()

	m.mu.Lock()
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
		_ = m.cmd.Wait()
	}
	m.mu.Unlock()
	return nil
}

// TaskID 返回当前任务空间 ID。
func (m *EgoLiteManager) TaskID() string { return m.currentTask }
