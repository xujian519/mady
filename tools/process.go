package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/csync"
)

// ProcessRegistry manages background processes.
type ProcessRegistry struct {
	processes csync.Map[string, *ProcessEntry]
}

// ProcessEntry tracks a background process.
type ProcessEntry struct {
	ID        string     `json:"id"`
	Command   string     `json:"command"`
	PID       int        `json:"pid"`
	Status    string     `json:"status"` // running, completed, failed, killed
	ExitCode  int        `json:"exit_code"`
	Output    []byte     `json:"-"`
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	cmd       *exec.Cmd
	mu        sync.Mutex
}

// NewProcessRegistry creates a new process registry.
func NewProcessRegistry() *ProcessRegistry {
	return &ProcessRegistry{}
}

// Register adds a process to the registry.
func (r *ProcessRegistry) Register(entry *ProcessEntry) {
	r.processes.Set(entry.ID, entry)
}

// Get retrieves a process by ID.
func (r *ProcessRegistry) Get(id string) (*ProcessEntry, bool) {
	return r.processes.Get(id)
}

// List returns all process IDs.
func (r *ProcessRegistry) List() []string {
	ids := make([]string, 0, r.processes.Len())
	for id := range r.processes.Copy() {
		ids = append(ids, id)
	}
	return ids
}

func (r *ProcessRegistry) ListAll() []*ProcessEntry {
	entries := make([]*ProcessEntry, 0, r.processes.Len())
	for _, entry := range r.processes.Copy() {
		entries = append(entries, entry)
	}
	return entries
}

// Cleanup removes completed processes ending more than maxAge ago.
// When maxAge is 0, all completed processes (regardless of end time) are removed.
func (r *ProcessRegistry) Cleanup(maxAge time.Duration) int {
	now := time.Now()
	removed := 0
	for id, entry := range r.processes.Copy() {
		if entry.Status != "running" && entry.EndTime != nil {
			if now.Sub(*entry.EndTime) >= maxAge {
				r.processes.Del(id)
				removed++
			}
		}
	}
	return removed
}

// ProcessOperations defines pluggable operations for the process tool.
type ProcessOperations interface {
	Spawn(command string, cwd string) (*ProcessEntry, error)
	Kill(pid int) error
	Poll(entry *ProcessEntry) (string, int, []byte)
}

// DefaultProcessOperations uses the local system.
type DefaultProcessOperations struct {
	registry  *ProcessRegistry
	idCounter int
	mu        sync.Mutex
}

// NewDefaultProcessOperations creates default process operations.
func NewDefaultProcessOperations(registry *ProcessRegistry) *DefaultProcessOperations {
	if registry == nil {
		registry = NewProcessRegistry()
	}
	return &DefaultProcessOperations{
		registry: registry,
	}
}

func (d *DefaultProcessOperations) nextID() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idCounter++
	return fmt.Sprintf("proc-%d-%d", time.Now().Unix(), d.idCounter)
}

func (d *DefaultProcessOperations) Spawn(command string, cwd string) (*ProcessEntry, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell, "-c", command)
	cmd.Dir = cwd

	// Create output capture.
	output := &outputBuffer{maxBytes: 200 * 1024} // 200KB rolling buffer
	cmd.Stdout = output
	cmd.Stderr = output

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	entry := &ProcessEntry{
		ID:        d.nextID(),
		Command:   command,
		PID:       cmd.Process.Pid,
		Status:    "running",
		StartTime: time.Now(),
		cmd:       cmd,
	}

	d.registry.Register(entry)

	// Monitor in background.
	go func() {
		err := cmd.Wait()
		entry.mu.Lock()
		defer entry.mu.Unlock()
		now := time.Now()
		entry.EndTime = &now
		entry.Output = output.Bytes()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				entry.ExitCode = exitErr.ExitCode()
				entry.Status = "failed"
			} else {
				entry.Status = "failed"
				entry.ExitCode = -1
			}
		} else {
			entry.Status = "completed"
			entry.ExitCode = 0
		}
	}()

	return entry, nil
}

func (d *DefaultProcessOperations) Kill(pid int) error {
	// Try process group first, then direct kill.
	syscall.Kill(-pid, syscall.SIGKILL)
	return syscall.Kill(pid, syscall.SIGKILL)
}

func (d *DefaultProcessOperations) Poll(entry *ProcessEntry) (string, int, []byte) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return entry.Status, entry.ExitCode, entry.Output
}

// outputBuffer is a thread-safe rolling buffer for process output.
type outputBuffer struct {
	mu       sync.Mutex
	data     []byte
	maxBytes int
}

func (b *outputBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > b.maxBytes {
		// Keep last maxBytes.
		b.data = b.data[len(b.data)-b.maxBytes:]
	}
	return len(p), nil
}

func (b *outputBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...)
}

// ProcessToolConfig configures the process tool.
type ProcessToolConfig struct {
	Operations ProcessOperations
	Registry   *ProcessRegistry
	MaxBytes   int64
	MaxLines   int64
}

func (c *ProcessToolConfig) defaults() {
	if c.MaxBytes <= 0 {
		c.MaxBytes = DefaultMaxBytes
	}
	if c.MaxLines <= 0 {
		c.MaxLines = DefaultMaxLines
	}
}

// ProcessToolInput is the JSON arguments for the process tool.
type ProcessToolInput struct {
	Action    string `json:"action"` // spawn, status, wait, kill, list
	Command   string `json:"command,omitempty"`
	ProcessID string `json:"process_id,omitempty"`
	Timeout   *int   `json:"timeout,omitempty"`
}

// ProcessToolDetails carries process metadata.
type ProcessToolDetails struct {
	ProcessID  string            `json:"process_id,omitempty"`
	Status     string            `json:"status,omitempty"`
	PID        int               `json:"pid,omitempty"`
	ExitCode   int               `json:"exit_code,omitempty"`
	StartTime  string            `json:"start_time,omitempty"`
	EndTime    string            `json:"end_time,omitempty"`
	Duration   string            `json:"duration,omitempty"`
	OutputSize int               `json:"output_size,omitempty"`
	Truncation *TruncationResult `json:"truncation,omitempty"`
}

// NewProcessTool creates a process management tool.
func NewProcessTool(cwd string, cfg *ProcessToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &ProcessToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name: "process",
		Description: "管理后台进程。操作：spawn（启动后台命令）、" +
			"status（检查进程状态）、wait（等待完成）、kill（终止进程）、" +
			"list（显示所有跟踪的进程）。" +
			"输出会被截断至最后 " + fmt.Sprintf("%d 行或 %s", cfg.MaxLines, FormatSize(cfg.MaxBytes)) + "。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "要执行的操作：spawn、status、wait、kill、list",
					"enum":        []any{"spawn", "status", "wait", "kill", "list"},
				},
				"command": map[string]any{
					"type":        "string",
					"description": "要执行的命令（spawn 操作必需）",
				},
				"process_id": map[string]any{
					"type":        "string",
					"description": "进程 ID（status、wait、kill 操作必需）",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "wait 操作的超时时间（秒），可选参数",
				},
			},
			"required": []any{"action"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input ProcessToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}

			if cfg.Operations == nil {
				return resultErrf("process operations not configured")
			}

			switch input.Action {
			case "spawn":
				return handleSpawn(cfg, cwd, input)
			case "status":
				return handleStatus(cfg, input)
			case "wait":
				return handleWait(cfg, input)
			case "kill":
				return handleKill(cfg, input)
			case "list":
				return handleList(cfg)
			default:
				return resultErrf("unknown action: %s", input.Action)
			}
		},
	}
}

func handleSpawn(cfg *ProcessToolConfig, cwd string, input ProcessToolInput) (any, error) {
	if input.Command == "" {
		return resultErrf("command is required for spawn")
	}

	entry, err := cfg.Operations.Spawn(input.Command, cwd)
	if err != nil {
		return resultErrf("failed to spawn process: %w", err)
	}

	// Lock to read status safely — the background goroutine in Spawn
	// may have already updated it for a fast-exiting process.
	entry.mu.Lock()
	status := entry.Status
	pid := entry.PID
	startTime := entry.StartTime
	entry.mu.Unlock()

	return result(
		fmt.Sprintf("Spawned process %s (PID %d): %s", entry.ID, pid, input.Command),
		ProcessToolDetails{
			ProcessID: entry.ID,
			Status:    status,
			PID:       pid,
			StartTime: startTime.Format(time.RFC3339),
		},
	)
}

func handleStatus(cfg *ProcessToolConfig, input ProcessToolInput) (any, error) {
	if input.ProcessID == "" {
		return resultErrf("process_id is required for status")
	}

	if cfg.Registry == nil {
		return resultErrf("process registry not configured")
	}

	entry, ok := cfg.Registry.Get(input.ProcessID)
	if !ok {
		return resultErrf("process not found: %s", input.ProcessID)
	}

	status, exitCode, output := cfg.Operations.Poll(entry)
	outputText := string(output)
	truncation := TruncateTail(outputText, TruncationOptions{
		MaxLines: int(cfg.MaxLines),
		MaxBytes: int(cfg.MaxBytes),
	})

	resultText := fmt.Sprintf("Process %s: status=%s, exit_code=%d", input.ProcessID, status, exitCode)
	if truncation.Content != "" {
		resultText += "\n\nOutput:\n" + truncation.Content
	}

	var details ProcessToolDetails
	if truncation.Truncated {
		details.Truncation = &truncation
	}
	details.Status = status
	details.ExitCode = exitCode
	details.OutputSize = len(output)

	return result(resultText, details)
}

func handleWait(cfg *ProcessToolConfig, input ProcessToolInput) (any, error) {
	if input.ProcessID == "" {
		return resultErrf("process_id is required for wait")
	}

	if cfg.Registry == nil {
		return resultErrf("process registry not configured")
	}

	entry, ok := cfg.Registry.Get(input.ProcessID)
	if !ok {
		return resultErrf("process not found: %s", input.ProcessID)
	}

	timeout := 300
	if input.Timeout != nil && *input.Timeout > 0 {
		timeout = *input.Timeout
	}

	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	for time.Now().Before(deadline) {
		status, exitCode, output := cfg.Operations.Poll(entry)
		if status != "running" {
			outputText := string(output)
			truncation := TruncateTail(outputText, TruncationOptions{
				MaxLines: int(cfg.MaxLines),
				MaxBytes: int(cfg.MaxBytes),
			})

			resultText := fmt.Sprintf("Process %s completed with status=%s, exit_code=%d", input.ProcessID, status, exitCode)
			if truncation.Content != "" {
				resultText += "\n\nOutput:\n" + truncation.Content
			}

			var details ProcessToolDetails
			if truncation.Truncated {
				details.Truncation = &truncation
			}
			details.Status = status
			details.ExitCode = exitCode
			details.OutputSize = len(output)

			return result(resultText, details)
		}
		time.Sleep(1 * time.Second)
	}

	return resultErrf("timeout waiting for process %s after %d seconds", input.ProcessID, timeout)
}

func handleKill(cfg *ProcessToolConfig, input ProcessToolInput) (any, error) {
	if input.ProcessID == "" {
		return resultErrf("process_id is required for kill")
	}

	if cfg.Registry == nil {
		return resultErrf("process registry not configured")
	}

	entry, ok := cfg.Registry.Get(input.ProcessID)
	if !ok {
		return resultErrf("process not found: %s", input.ProcessID)
	}

	entry.mu.Lock()
	pid := entry.PID
	status := entry.Status
	entry.mu.Unlock()

	if status != "running" {
		return result(fmt.Sprintf("Process %s is not running (status: %s)", input.ProcessID, status),
			ProcessToolDetails{ProcessID: entry.ID, Status: status, PID: pid})
	}

	if err := cfg.Operations.Kill(pid); err != nil {
		return resultErrf("failed to kill process %s: %w", input.ProcessID, err)
	}

	entry.mu.Lock()
	entry.Status = "killed"
	now := time.Now()
	entry.EndTime = &now
	entry.mu.Unlock()

	return result(fmt.Sprintf("Killed process %s (PID %d)", input.ProcessID, pid),
		ProcessToolDetails{ProcessID: entry.ID, Status: "killed", PID: pid})
}

func handleList(cfg *ProcessToolConfig) (any, error) {
	if cfg.Registry == nil {
		return resultErrf("process registry not configured")
	}

	entries := cfg.Registry.ListAll()
	if len(entries) == 0 {
		return result("No processes tracked", nil)
	}

	output := fmt.Sprintf("Tracked processes (%d):", len(entries))
	for _, entry := range entries {
		entry.mu.Lock()
		status := entry.Status
		pid := entry.PID
		cmd := entry.Command
		entry.mu.Unlock()
		output += fmt.Sprintf("\n  %s  PID=%d  status=%s  cmd=%s", entry.ID, pid, status, cmd)
	}

	return result(output, nil)
}
