package omlx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

// defaultHealthCheckTimeout is how long we wait for the server to be ready
// after starting the subprocess.
const defaultHealthCheckTimeout = 30 * time.Second

// defaultHealthCheckInterval is the polling interval between readiness checks.
const defaultHealthCheckInterval = 500 * time.Millisecond

// Manager manages the lifecycle of the oMLX inference server process.
// Thread-safe.
type Manager struct {
	port   int
	apiKey string

	mu         sync.Mutex
	cmd        *exec.Cmd    // non-nil when we started the process
	httpClient *http.Client // shared client for health checks
	healthURL  string       // e.g. "http://127.0.0.1:8000/v1/models"
}

// NewManager creates a Manager bound to the given port.
// apiKey is the OMLX_API_KEY used for both health checks and forwarded to oMLX.
// When apiKey is empty, all operations return ErrNoAPIKey.
func NewManager(port int, apiKey string) *Manager {
	return &Manager{
		port:   port,
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:    4,
				IdleConnTimeout: 30 * time.Second,
			},
		},
		healthURL: fmt.Sprintf("http://127.0.0.1:%d/v1/models", port),
	}
}

// ErrNoAPIKey is returned when OMLX_API_KEY is not set.
var ErrNoAPIKey = &HealthError{"OMLX_API_KEY 未设置"}

// HealthError describes why the oMLX server is unavailable.
type HealthError struct{ msg string }

func (e *HealthError) Error() string { return e.msg }

// IsRunning checks whether the oMLX server is reachable at localhost:port.
// A nil response means the server is running and accepting requests.
func (m *Manager) IsRunning() bool {
	if m.apiKey == "" {
		return false
	}
	return m.checkHealth(context.Background()) == nil
}

// checkHealth performs a single health check request.
func (m *Manager) checkHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.healthURL, nil) // #nosec G704 — localhost only
	if err != nil {
		return err
	}
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}
	resp, err := m.httpClient.Do(req) // #nosec G704 — localhost only
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain body to allow connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
		// Unauthorized means the server is running but requires a valid key.
		// Our key may be wrong, but the server is up — return success.
		return nil
	}
	return &HealthError{fmt.Sprintf("unexpected status: %d", resp.StatusCode)}
}

// Start launches oMLX as a child process and waits for it to become ready.
// Returns ErrNoAPIKey when apiKey is empty.
// Returns an error if oMLX is not installed, cannot be started, or does not
// become ready within defaultHealthCheckTimeout.
func (m *Manager) Start(ctx context.Context) error {
	if m.apiKey == "" {
		return ErrNoAPIKey
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Re-check: maybe someone else started it while we were waiting for the lock.
	if m.cmd != nil {
		return nil // already started by us
	}

	// Locate the oMLX binary.
	path, err := exec.LookPath("omlx")
	if err != nil {
		return &HealthError{
			msg: "oMLX 未安装（brew install omlx），嵌入式向量搜索不可用",
		}
	}

	// Build command: omlx serve --port <port> --api-key <key>
	args := []string{
		"serve",
		"--port", fmt.Sprintf("%d", m.port),
		"--api-key", m.apiKey,
	}
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // safe: lookpath-resolved omlx binary

	// Capture stderr for diagnostics in case of startup failure.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return &HealthError{fmt.Sprintf("创建 oMLX 子进程管道失败: %v", err)}
	}

	if err := cmd.Start(); err != nil {
		return &HealthError{fmt.Sprintf("启动 oMLX 失败: %v", err)}
	}
	m.cmd = cmd

	// Read stderr in background to prevent deadlock when pipe buffer fills.
	go func() {
		slurp, _ := io.ReadAll(stderr)
		if len(slurp) > 0 {
			slog.Debug("omlx: stderr output", "output", string(slurp))
		}
	}()

	// Wait for the server to become ready.
	ready := make(chan error, 1)
	go func() {
		deadline := time.After(defaultHealthCheckTimeout)
		tick := time.NewTicker(defaultHealthCheckInterval)
		defer tick.Stop()
		for {
			select {
			case <-deadline:
				ready <- &HealthError{
					fmt.Sprintf("oMLX 启动超时（%s），请手动检查: omlx diagnose", defaultHealthCheckTimeout),
				}
				return
			case <-tick.C:
				if err := m.checkHealth(ctx); err == nil {
					ready <- nil
					return
				}
			}
		}
	}()

	select {
	case err := <-ready:
		if err != nil {
			m.cmd = nil // startup failed, don't track as running
			return err
		}
		slog.Info("oMLX 嵌入服务已就绪", "port", m.port, "pid", cmd.Process.Pid)
		return nil
	case <-ctx.Done():
		m.cmd = nil
		if err := cmd.Process.Kill(); err != nil {
			slog.Warn("oMLX: kill process on context cancel", "err", err)
		}
		return ctx.Err()
	}
}

// Stop terminates the oMLX child process gracefully. If the process was not
// started by this Manager, it is a no-op.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == nil {
		return nil
	}
	pid := m.cmd.Process.Pid
	if err := m.cmd.Process.Signal(softKill); err != nil {
		slog.Warn("omlx: 停止子进程失败，强制杀死", "pid", pid, "err", err)
		if err := m.cmd.Process.Kill(); err != nil {
			return &HealthError{fmt.Sprintf("强制杀死 oMLX 进程 %d 失败: %v", pid, err)}
		}
	}

	// Wait for the process to exit.
	done := make(chan struct{})
	go func() {
		_ = m.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("oMLX 嵌入服务已停止", "pid", pid)
	case <-time.After(5 * time.Second):
		slog.Warn("omlx: 停止超时，强制杀死", "pid", pid)
		_ = m.cmd.Process.Kill()
	}
	m.cmd = nil
	return nil
}

// EnsureRunning checks whether the oMLX server is already running. If not,
// it attempts to start it automatically. Fails gracefully — non-critical path.
//
// Usage:
//
//	mgr := omlx.NewManager(8000, os.Getenv("OMLX_API_KEY"))
//	mgr.EnsureRunning(context.Background())
//	// Continue regardless of result; vector search degrades gracefully.
func (m *Manager) EnsureRunning(ctx context.Context) {
	if m.apiKey == "" {
		slog.Debug("omlx: OMLX_API_KEY 未设置，跳过嵌入服务检测")
		return
	}

	// Fast path: already running.
	if m.checkHealth(ctx) == nil {
		slog.Debug("omlx: 嵌入服务已在运行", "url", m.healthURL)
		return
	}

	// Slow path: try to start.
	slog.Info("oMLX 嵌入服务未运行，尝试自动启动...")
	if err := m.Start(ctx); err != nil {
		slog.Warn("oMLX 嵌入服务启动失败，向量搜索将降级为 FTS-only",
			"error", err,
			"hint", "brew install omlx && omlx serve --port 8000",
		)
	}
}

// softKill is the signal used for graceful shutdown.
// On Unix, SIGTERM; on Windows, os.Kill (no SIGTERM support).
var softKill = osSignal()
