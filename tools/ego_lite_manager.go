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

const egoLiteMaxRestarts = 3

// EgoLiteManager 管理一个持久化 ego-browser 子进程。
// Send() 方法是唯一的公开 API。
// mu 保护 cmd/stdin/stdout 和子进程生命周期（start/restart/Close），
// pendingMu 单独保护 pending map（readLoop 访问时不与 mu 竞争，避免死锁）。
type EgoLiteManager struct {
	mu        sync.Mutex
	pendingMu sync.Mutex
	pending   map[string]chan egoLiteJSONResponse
	counter   atomic.Int64

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner

	restarts  int
	restartMu sync.Mutex

	taskName    string
	currentTask string

	closed atomic.Bool
	ctx    context.Context
	cancel context.CancelFunc
}

func egoLiteAvailable() bool {
	_, err := exec.LookPath("ego-browser")
	return err == nil
}

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

// Send 发送命令并等待响应。mu 保护子进程生命周期和 stdin 写入序列化；
// 响应等待在 mu 之外进行，避免与 readLoop 死锁。
func (m *EgoLiteManager) Send(ctx context.Context, method string, params map[string]any) (any, error) {
	m.mu.Lock()
	if m.closed.Load() {
		m.mu.Unlock()
		return nil, fmt.Errorf("egolite: manager closed")
	}
	if m.cmd == nil || m.stdin == nil {
		if err := m.start(ctx); err != nil {
			m.mu.Unlock()
			return nil, fmt.Errorf("egolite: start failed: %w", err)
		}
	}
	// 持 mu 构建请求、注册 pending、写入 stdin，然后释放 mu
	id := fmt.Sprintf("r%d", m.counter.Add(1))
	req := egoLiteJSONRequest{ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("egolite: marshal: %w", err)
	}
	ch := make(chan egoLiteJSONResponse, 1)
	m.pendingMu.Lock()
	m.pending[id] = ch
	m.pendingMu.Unlock()
	writeErr := m.writeRequestLocked(data)
	m.mu.Unlock()

	if writeErr != nil {
		m.pendingMu.Lock()
		delete(m.pending, id)
		m.pendingMu.Unlock()
		slog.Warn("egolite: write failed, restarting", "err", writeErr)
		if restartErr := m.restart(ctx); restartErr != nil {
			return nil, fmt.Errorf("egolite: write+restart: %w", restartErr)
		}
		return m.sendWithRetry(ctx, method, params)
	}
	return m.waitResponse(ctx, id, ch, method)
}

func (m *EgoLiteManager) sendWithRetry(ctx context.Context, method string, params map[string]any) (any, error) {
	m.mu.Lock()
	if m.closed.Load() {
		m.mu.Unlock()
		return nil, fmt.Errorf("egolite: closed after restart")
	}
	id := fmt.Sprintf("r%d", m.counter.Add(1))
	req := egoLiteJSONRequest{ID: id, Method: method, Params: params}
	data, _ := json.Marshal(req)
	ch := make(chan egoLiteJSONResponse, 1)
	m.pendingMu.Lock()
	m.pending[id] = ch
	m.pendingMu.Unlock()
	writeErr := m.writeRequestLocked(data)
	m.mu.Unlock()
	if writeErr != nil {
		m.pendingMu.Lock()
		delete(m.pending, id)
		m.pendingMu.Unlock()
		return nil, fmt.Errorf("egolite: retry write: %w", writeErr)
	}
	return m.waitResponse(ctx, id, ch, method)
}

func (m *EgoLiteManager) writeRequestLocked(data []byte) error {
	if _, err := m.stdin.Write(data); err != nil {
		return err
	}
	_, err := m.stdin.Write([]byte{'\n'})
	return err
}

func (m *EgoLiteManager) waitResponse(ctx context.Context, id string, ch chan egoLiteJSONResponse, method string) (any, error) {
	defer func() {
		m.pendingMu.Lock()
		delete(m.pending, id)
		m.pendingMu.Unlock()
	}()
	select {
	case resp := <-ch:
		if !resp.OK {
			return nil, fmt.Errorf("egolite: %s: %s", method, resp.Error)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *EgoLiteManager) start(ctx context.Context) error {
	m.cmd = exec.CommandContext(m.ctx, "ego-browser", "nodejs")
	stdin, err := m.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	m.stdout = bufio.NewScanner(stdout)
	m.stdout.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	// 写入 bridge 脚本，失败时清理子进程（避免孤儿进程）
	if _, err := stdin.Write(egoLiteBridgeScript); err != nil {
		m.cmd.Process.Kill()
		_ = m.cmd.Wait()
		m.cmd = nil
		return fmt.Errorf("write bridge: %w", err)
	}
	if _, err := stdin.Write([]byte("\n")); err != nil {
		m.cmd.Process.Kill()
		_ = m.cmd.Wait()
		m.cmd = nil
		return fmt.Errorf("write nl: %w", err)
	}
	m.stdin = stdin
	go m.readLoop()
	// initTaskSpace：注册 pending → 写入 → 等待（mu 外）
	id := fmt.Sprintf("r%d", m.counter.Add(1))
	req := egoLiteJSONRequest{ID: id, Method: "initTaskSpace", Params: map[string]any{"name": m.taskName}}
	initData, _ := json.Marshal(req)
	ch := make(chan egoLiteJSONResponse, 1)
	m.pendingMu.Lock()
	m.pending[id] = ch
	m.pendingMu.Unlock()
	if err := m.writeRequestLocked(initData); err != nil {
		m.cmd.Process.Kill()
		_ = m.cmd.Wait()
		m.cmd, m.stdin, m.stdout = nil, nil, nil
		m.pendingMu.Lock()
		delete(m.pending, id)
		m.pendingMu.Unlock()
		return fmt.Errorf("write init: %w", err)
	}
	result, err := m.waitResponse(ctx, id, ch, "initTaskSpace")
	if err != nil {
		m.cmd.Process.Kill()
		_ = m.cmd.Wait()
		m.cmd, m.stdin, m.stdout = nil, nil, nil
		return fmt.Errorf("init: %w", err)
	}
	if rm, ok := result.(map[string]any); ok {
		if id, ok := rm["id"]; ok {
			m.currentTask = fmt.Sprintf("%v", id)
		}
	}
	slog.Info("egolite: started", "task", m.currentTask)
	return nil
}

func (m *EgoLiteManager) readLoop() {
	scanner := m.stdout
	for scanner.Scan() {
		line := scanner.Bytes()
		var resp egoLiteJSONResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			slog.Warn("egolite: bad json", "err", err)
			continue
		}
		m.pendingMu.Lock()
		if ch, ok := m.pending[resp.ID]; ok {
			ch <- resp
		}
		m.pendingMu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("egolite: scanner", "err", err)
	}
	if !m.closed.Load() {
		slog.Warn("egolite: stdout closed")
	}
}

func (m *EgoLiteManager) restart(ctx context.Context) error {
	m.restartMu.Lock()
	defer m.restartMu.Unlock()
	if m.restarts >= egoLiteMaxRestarts {
		return fmt.Errorf("egolite: max restarts (%d)", egoLiteMaxRestarts)
	}
	m.restarts++
	m.mu.Lock()
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
		_ = m.cmd.Wait()
	}
	m.cmd, m.stdin, m.stdout = nil, nil, nil
	m.mu.Unlock()
	if m.restarts > 1 {
		select {
		case <-time.After(time.Duration(m.restarts) * 500 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.start(ctx)
}

func (m *EgoLiteManager) Close() error {
	// 尽力发送 completeTaskSpace（fire-and-forget）
	m.mu.Lock()
	if m.cmd != nil && m.stdin != nil && m.currentTask != "" {
		id := fmt.Sprintf("r%d", m.counter.Add(1))
		req := egoLiteJSONRequest{ID: id, Method: "completeTaskSpace", Params: map[string]any{"keep": false}}
		if data, err := json.Marshal(req); err == nil {
			_ = m.writeRequestLocked(data)
		}
	}
	m.mu.Unlock()
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

func (m *EgoLiteManager) TaskID() string { return m.currentTask }
