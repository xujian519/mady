package adapter

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// cliSession wraps an exec.Cmd as an AgentSession for CLI-based agents
// (Claude Code, Codex, etc.) that communicate via stdin/stdout.
type cliSession struct {
	cmd        *exec.Cmd
	stdin      *bufio.Writer
	stdout     *bufio.Scanner
	stderrBuf  *bytes.Buffer // captured stderr for diagnostics on failure
	stderrPipe io.Closer     // closed on Close() to unblock the stderr copy goroutine
	stderrDone chan struct{} // closed when the stderr copy goroutine exits
}

// newCLISession launches a CLI agent process and returns a session wrapping it.
// bin is the CLI binary name, subcmd is the subcommand (e.g., "-p" for Claude, "exec" for Codex).
func newCLISession(ctx context.Context, bin, subcmd string, cfg SpawnConfig) (AgentSession, error) {
	args := []string{subcmd}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	args = append(args, cfg.ExtraArgs...)

	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // safe: hardcoded binary name from adapter config ("claude"/"codex")
	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}
	if len(cfg.Env) > 0 {
		cmd.Env = cmd.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("%s stdin: %w", bin, err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%s stdout: %w", bin, err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("%s stderr: %w", bin, err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s start: %w", bin, err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	// Allow single lines up to 10MB (matches existing convention in mcp/ and session/).
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	var stderrBuf bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		// stderrPipe is closed explicitly in Close(), unblocking io.Copy.
		_, _ = io.Copy(&stderrBuf, stderrPipe)
	}()

	return &cliSession{
		cmd:        cmd,
		stdin:      bufio.NewWriter(stdinPipe),
		stdout:     scanner,
		stderrBuf:  &stderrBuf,
		stderrPipe: stderrPipe,
		stderrDone: stderrDone,
	}, nil
}

func (s *cliSession) Send(ctx context.Context, input string) (string, error) {
	if _, err := s.stdin.WriteString(input + "\n"); err != nil {
		return "", fmt.Errorf("write stdin: %w", err)
	}
	if err := s.stdin.Flush(); err != nil {
		return "", fmt.Errorf("flush stdin: %w", err)
	}

	var output strings.Builder
	for s.stdout.Scan() {
		select {
		case <-ctx.Done():
			return output.String(), ctx.Err()
		default:
		}
		output.WriteString(s.stdout.Text())
		output.WriteByte('\n')
	}
	if err := s.stdout.Err(); err != nil {
		return strings.TrimSpace(output.String()), fmt.Errorf("scan stdout: %w (stderr: %s)", err, s.stderrBuf.String())
	}
	return strings.TrimSpace(output.String()), nil
}

// Stream sends input and streams output line by line.
// The returned channel must be drained to completion; otherwise the
// underlying goroutine may leak until the process exits.
func (s *cliSession) Stream(ctx context.Context, input string) (<-chan StreamChunk, error) {
	if _, err := s.stdin.WriteString(input + "\n"); err != nil {
		return nil, fmt.Errorf("write stdin: %w", err)
	}
	if err := s.stdin.Flush(); err != nil {
		return nil, fmt.Errorf("flush stdin: %w", err)
	}

	ch := make(chan StreamChunk, 4)
	go func() {
		defer close(ch)
		for s.stdout.Scan() {
			// Check context cancellation between each scanned line.
			// Scan() itself may block, but process exit (via Close)
			// closes the pipe and unblocks it.
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Prioritize graceful send; if the receiver falls behind or
			// stops reading, drain into a short timeout to prevent the
			// goroutine from leaking indefinitely.
			select {
			case ch <- StreamChunk{Content: s.stdout.Text()}:
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				// Receiver stalled for 10s — treat as dead connection.
				return
			}
		}
		if err := s.stdout.Err(); err != nil {
			select {
			case ch <- StreamChunk{Error: fmt.Errorf("scan stdout: %w (stderr: %s)", err, s.stderrBuf.String())}:
			case <-ctx.Done():
			case <-time.After(10 * time.Second):
			}
		} else {
			select {
			case ch <- StreamChunk{Done: true}:
			case <-ctx.Done():
			case <-time.After(10 * time.Second):
			}
		}
	}()
	return ch, nil
}

// Close kills the agent process, waits for the stderr copy goroutine to
// finish, and releases all OS resources (including the process descriptor
// via Wait).
func (s *cliSession) Close() error {
	// Close stderrPipe first to unblock the stderr copy goroutine.
	if s.stderrPipe != nil {
		if err := s.stderrPipe.Close(); err != nil {
			slog.Warn("session: close stderr pipe", "err", err)
		}
	}

	// Wait for the stderr copy goroutine to finish, with a safety timeout.
	select {
	case <-s.stderrDone:
	case <-time.After(5 * time.Second):
		slog.Warn("session: stderr copy did not finish within 5s")
	}

	if s.cmd.Process == nil {
		return nil
	}

	killErr := s.cmd.Process.Kill()
	waitErr := s.cmd.Wait()
	// Return the first error; Wait error after successful Kill is expected
	// (the process was killed, not exited cleanly), so prefer KillErr.
	if killErr != nil {
		return fmt.Errorf("kill: %w", killErr)
	}
	if waitErr != nil {
		return fmt.Errorf("wait: %w", waitErr)
	}
	return nil
}
