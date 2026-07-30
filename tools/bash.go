package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	"github.com/xujian519/mady/agentcore"
)

// BashOperations defines pluggable operations for the bash tool.
type BashOperations interface {
	Exec(ctx context.Context, command string, cwd string, env map[string]string, timeoutSecs *int, onData func(data []byte)) (int, error)
}

// DefaultBashOperations uses the local shell.
type DefaultBashOperations struct{}

func (d DefaultBashOperations) Exec(ctx context.Context, command string, cwd string, env map[string]string, timeoutSecs *int, onData func(data []byte)) (int, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.CommandContext(ctx, shell, "-c", command) //nolint:gosec // G204: shell execution by design; tools module provides CLI interface
	cmd.Dir = cwd
	// Setpgid creates a new process group so killProcessTree(-pgid) only
	// affects this command's children, preventing PID-reuse collateral damage.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if env != nil {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return -1, err
	}

	if err := cmd.Start(); err != nil {
		return -1, err
	}

	// Set up timeout.
	var timer *time.Timer
	var killedByTimeout atomic.Bool
	if timeoutSecs != nil && *timeoutSecs > 0 {
		timer = time.AfterFunc(time.Duration(*timeoutSecs)*time.Second, func() {
			if cmd.Process != nil {
				_ = killProcessTree(cmd.Process.Pid)
				killedByTimeout.Store(true)
			}
		})
	}

	// Stream output.
	var wg sync.WaitGroup
	readPipe := func(pipe *bufio.Reader) {
		defer wg.Done()
		for {
			line, err := pipe.ReadBytes('\n')
			if len(line) > 0 {
				onData(line)
			}
			if err != nil {
				break
			}
		}
	}

	wg.Add(2)
	go readPipe(bufio.NewReader(stdout))
	go readPipe(bufio.NewReader(stderr))
	wg.Wait()

	if timer != nil {
		timer.Stop()
	}

	err = cmd.Wait()
	// 超时标志仅在进程被信号终止时才使用；
	// 如果进程在超时时刻自行退出，err == nil 则正常返回退出码。
	if err != nil && killedByTimeout.Load() {
		// Process was terminated by timeout.
		return -1, fmt.Errorf("process killed after timeout %ds", *timeoutSecs)
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	var errs []error
	// Try to kill the process group first.
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		errs = append(errs, fmt.Errorf("kill process group %d: %w", -pid, err))
	}
	// Fallback: kill the process itself.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		errs = append(errs, fmt.Errorf("kill process %d: %w", pid, err))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func stripAnsi(text string) string {
	var b []byte
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			i += 2
			for i < len(runes) && ((runes[i] >= '0' && runes[i] <= '9') || runes[i] == ';' || runes[i] == '?') {
				i++
			}
			if i < len(runes) && ((runes[i] >= 'A' && runes[i] <= 'Z') || (runes[i] >= 'a' && runes[i] <= 'z')) {
				i++
			}
			continue
		}
		b = append(b, string(runes[i])...)
		i++
	}
	return string(b)
}

func sanitizeBinaryOutput(text string) string {
	var builder []rune
	for _, r := range text {
		if unicode.Is(unicode.C, r) && r != '\n' && r != '\r' && r != '\t' {
			builder = append(builder, '\uFFFD')
		} else {
			builder = append(builder, r)
		}
	}
	return string(builder)
}

// BashToolConfig configures the bash tool.
// NOTE: Unlike file tools, bash does NOT implement sandbox path enforcement.
// The Sandbox field exists on file tools (read/write/edit etc.) for path-level
// sandboxing, but bash executes arbitrary shell commands — no path restriction
// is possible at the shell level. Process-level sandboxing (seccomp, sandbox-exec)
// must be configured externally if required.
type BashToolConfig struct {
	Operations BashOperations
	MaxBytes   int64
	MaxLines   int64

	// Sandbox is included for structural compatibility with other tools, but
	// NOTE: the bash tool does NOT enforce the WorkingDir sandbox boundary.
	// Shell commands can access any path on the filesystem (cat /etc/passwd,
	// curl to exfiltrate data, etc.). The Sandbox field exists only because
	// propagateSandbox (tools.go) injects it into every tool config.
	//
	// The real security boundary for the bash tool is the DisableTools /
	// EnabledTools gating in ExtensionConfig. To completely prevent shell
	// access, exclude "bash" (and "process", "execute_code") from the
	// tool set for agents that don't need it.
	Sandbox WorkingDirSandbox

	// DangerousPatterns is a list of regex patterns that the bash tool
	// rejects before execution. Each pattern is matched against the full
	// command string via regexp.MatchString.
	//
	// Default (when empty): blocks backtick and $() command substitution,
	// which are the primary vectors for arbitrary nested command execution.
	// Set to nil explicitly to disable all pattern checks (not recommended).
	//
	// This is a defense-in-depth measure. The primary security boundary is
	// the Sandbox + DisableTools mechanism in ExtensionConfig.
	DangerousPatterns []string
}

// Validate checks that the bash tool configuration is valid.
func (c BashToolConfig) Validate() error {
	if c.MaxBytes <= 0 {
		return fmt.Errorf("MaxBytes must be positive, got %d", c.MaxBytes)
	}
	if c.MaxLines <= 0 {
		return fmt.Errorf("MaxLines must be positive, got %d", c.MaxLines)
	}
	for _, p := range c.DangerousPatterns {
		if _, err := regexp.Compile(p); err != nil {
			return fmt.Errorf("invalid dangerous pattern %q: %w", p, err)
		}
	}
	return nil
}

// DefaultDangerousPatterns returns the built-in set of patterns that block
// the most common shell injection vectors.
func DefaultDangerousPatterns() []string {
	return []string{
		"`[^`]*`",     // backtick command substitution: `cmd`
		`\$\([^)]*\)`, // $() command substitution: $(cmd)
	}
}

func (c *BashToolConfig) defaults() {
	if c.Operations == nil {
		c.Operations = DefaultBashOperations{}
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = DefaultMaxBytes
	}
	if c.MaxLines <= 0 {
		c.MaxLines = DefaultMaxLines
	}
}

// BashToolInput is the JSON arguments for the bash tool.
type BashToolInput struct {
	Command string `json:"command"`
	Timeout *int   `json:"timeout,omitempty"`
}

// BashToolDetails carries truncation metadata.
type BashToolDetails struct {
	Truncation     *TruncationResult `json:"truncation,omitempty"`
	FullOutputPath string            `json:"full_output_path,omitempty"`
}

// bashOutputCollector 收集 bash 命令输出，支持临时文件溢出和滚动缓冲区。
type bashOutputCollector struct {
	mu         sync.Mutex
	chunks     [][]byte
	totalBytes int
	tempFile   *os.File
	tempPath   string
	maxBytes   int64
	maxLines   int64
}

func newBashOutputCollector(maxBytes, maxLines int64) *bashOutputCollector {
	return &bashOutputCollector{
		maxBytes: maxBytes,
		maxLines: maxLines,
	}
}

// Write 接收输出数据，累计到滚动缓冲区；当超过 maxBytes 时自动创建临时文件保存完整输出。
func (c *bashOutputCollector) Write(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.totalBytes += len(data)
	c.chunks = append(c.chunks, data)

	// 当累计输出超过阈值时开始写入临时文件（回填所有已有数据）。
	if c.totalBytes > int(c.maxBytes) && c.tempFile == nil {
		var tempFileErr error
		c.tempFile, tempFileErr = os.CreateTemp("", "mady-bash-*.log")
		if tempFileErr != nil {
			c.tempFile = nil
			fmt.Fprintf(os.Stderr, "bash: 创建日志临时文件失败: %v（输出截断后无法提供完整日志）\n", tempFileErr)
		}
		if c.tempFile != nil {
			c.tempPath = c.tempFile.Name()
			for _, chunk := range c.chunks {
				if _, werr := c.tempFile.Write(chunk); werr != nil {
					c.cleanupTempFile()
					break
				}
			}
		}
	}
	// 如果临时文件仍可用，追加当前数据块。
	if c.tempFile != nil {
		if _, werr := c.tempFile.Write(data); werr != nil {
			c.cleanupTempFile()
		}
	}

	// 保持滚动缓冲区，只保留最近的内容（上限 2x maxBytes）。
	maxChunksBytes := int(c.maxBytes) * 2
	chunksBytes := 0
	for _, ch := range c.chunks {
		chunksBytes += len(ch)
	}
	for chunksBytes > maxChunksBytes && len(c.chunks) > 1 {
		chunksBytes -= len(c.chunks[0])
		c.chunks = c.chunks[1:]
	}
}

// cleanupTempFile 移除并关闭临时文件，重置 tempFile 和 tempPath。
func (c *bashOutputCollector) cleanupTempFile() {
	if c.tempPath != "" {
		_ = os.Remove(c.tempPath)
	}
	if c.tempFile != nil {
		if cerr := c.tempFile.Close(); cerr != nil {
			log.Printf("close temp log file: %v", cerr)
		}
	}
	c.tempFile = nil
	c.tempPath = ""
}

// Close 关闭临时文件（不删除），由调用方在 Exec 结束后调用。
func (c *bashOutputCollector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tempFile != nil {
		_ = c.tempFile.Close()
	}
}

// Result 合并输出、清理 ANSI/二进制字符、执行尾部截断，返回结果文本和详情。
func (c *bashOutputCollector) Result() (string, BashToolDetails) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var fullOutput []byte
	for _, chunk := range c.chunks {
		fullOutput = append(fullOutput, chunk...)
	}
	outputText := string(fullOutput)

	outputText = stripAnsi(outputText)
	outputText = sanitizeBinaryOutput(outputText)

	truncation := TruncateTail(outputText, TruncationOptions{
		MaxLines: int(c.maxLines),
		MaxBytes: int(c.maxBytes),
	})

	var details BashToolDetails
	resultText := truncation.Content

	if truncation.Truncated {
		details.Truncation = &truncation
		details.FullOutputPath = c.tempPath
		startLine := truncation.TotalLines - truncation.OutputLines + 1
		endLine := truncation.TotalLines
		switch {
		case truncation.LastLinePartial:
			resultText += fmt.Sprintf("\n\n[Showing last %s of line %d. Full output: %s]",
				FormatSize(int64(truncation.OutputBytes)), endLine, c.tempPath)
		case truncation.TruncatedBy == "lines":
			resultText += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Full output: %s]",
				startLine, endLine, truncation.TotalLines, c.tempPath)
		default:
			resultText += fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Full output: %s]",
				startLine, endLine, truncation.TotalLines, FormatSize(c.maxBytes), c.tempPath)
		}
	}

	return resultText, details
}

// parseBashInput 解析 bash 工具参数，校验必填字段。
func parseBashInput(args json.RawMessage) (BashToolInput, error) {
	var input BashToolInput
	if err := json.Unmarshal(args, &input); err != nil {
		return input, fmt.Errorf("invalid arguments: %w", err)
	}
	if input.Command == "" {
		return input, errors.New("command is required")
	}
	return input, nil
}

// checkDangerousPatterns 校验命令中是否包含危险模式。当 patterns 为 nil 时使用默认集合。
func checkDangerousPatterns(command string, patterns []string) error {
	if patterns == nil {
		patterns = DefaultDangerousPatterns()
	}
	for _, pat := range patterns {
		if matched, err := regexp.MatchString(pat, command); err != nil {
			return fmt.Errorf("command rejected: invalid pattern %q: %w", pat, err)
		} else if matched {
			return fmt.Errorf("command rejected: contains dangerous pattern %q", pat)
		}
	}
	return nil
}

// NewBashTool creates a shell execution tool.
func NewBashTool(cwd string, cfg *BashToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &BashToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name: "bash",
		Description: fmt.Sprintf("在当前工作目录中执行 bash 命令。返回 stdout 和 stderr。"+
			"输出会被截断至最后 %d 行或 %s（以先达到的为准）。"+
			"如果被截断，完整输出会保存到临时文件。可选提供超时时间（秒）。", cfg.MaxLines, FormatSize(cfg.MaxBytes)),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "要执行的 bash 命令"},
				"timeout": map[string]any{"type": "integer", "description": "超时时间（秒），可选参数，无默认超时"},
			},
			"required": []any{"command"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			input, err := parseBashInput(args)
			if err != nil {
				return resultErrf("%w", err)
			}

			if err := checkDangerousPatterns(input.Command, cfg.DangerousPatterns); err != nil {
				return resultErrf("%w", err)
			}

			collector := newBashOutputCollector(cfg.MaxBytes, cfg.MaxLines)
			exitCode, execErr := cfg.Operations.Exec(ctx, input.Command, cwd, nil, input.Timeout, collector.Write)

			collector.Close()

			// 调度临时文件延迟清理（Agent 可能在截断消息中引用此路径）。
			if collector.tempPath != "" {
				go func(path string) {
					timer := time.NewTimer(10 * time.Minute)
					<-timer.C
					_ = os.Remove(path)
				}(collector.tempPath)
			}

			resultText, details := collector.Result()
			if resultText == "" {
				resultText = "(no output)"
			}

			if execErr != nil {
				return resultErrf("command failed: %w", execErr)
			}

			if exitCode != 0 {
				resultText += fmt.Sprintf("\n\nCommand exited with code %d", exitCode)
				return result(resultText, details)
			}

			return result(resultText, details)
		},
	}
}
