package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// GitOperations defines pluggable operations for git tools.
type GitOperations interface {
	Exec(args []string, cwd string) (string, int, error)
}

// DefaultGitOperations executes git commands locally.
type DefaultGitOperations struct{}

func (d DefaultGitOperations) Exec(args []string, cwd string) (string, int, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return "", -1, err
		}
	}
	return string(output), exitCode, nil
}

// GitToolConfig configures git tools.
type GitToolConfig struct {
	Operations GitOperations
}

func (c *GitToolConfig) defaults() {
	if c.Operations == nil {
		c.Operations = DefaultGitOperations{}
	}
}

// --- git_status ---

type GitStatusInput struct{}

func NewGitStatusTool(cwd string, cfg *GitToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &GitToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name:        "git_status",
		Description: "显示工作区状态。返回已修改、已暂存、未跟踪和有冲突的文件列表。",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			output, code, err := cfg.Operations.Exec([]string{"status", "--short", "--branch"}, cwd)
			if err != nil {
				return resultErrf("git status failed: %w", err)
			}
			if code != 0 {
				return resultErrf("git status exited with code %d: %s", code, output)
			}

			output = strings.TrimSpace(output)
			if output == "" {
				return result("Working tree clean", nil)
			}
			return result(output, nil)
		},
	}
}

// --- git_diff ---

type GitDiffInput struct {
	Target   string `json:"target,omitempty"`
	Staged   bool   `json:"staged,omitempty"`
	FilePath string `json:"file_path,omitempty"`
}

func NewGitDiffTool(cwd string, cfg *GitToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &GitToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name:        "git_diff",
		Description: "显示提交之间的变更、提交与工作区之间的变更等。默认显示未暂存的变更。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":    map[string]any{"type": "string", "description": "要对比的提交、分支或文件（默认：工作区 vs HEAD）"},
				"staged":    map[string]any{"type": "boolean", "description": "显示已暂存的变更而非未暂存的变更"},
				"file_path": map[string]any{"type": "string", "description": "要显示差异的具体文件"},
			},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input GitDiffInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}

			gitArgs := []string{"diff"}
			if input.Staged {
				gitArgs = append(gitArgs, "--staged")
			}
			if input.Target != "" {
				gitArgs = append(gitArgs, input.Target)
			}
			if input.FilePath != "" {
				gitArgs = append(gitArgs, "--", input.FilePath)
			}

			output, code, err := cfg.Operations.Exec(gitArgs, cwd)
			if err != nil {
				return resultErrf("git diff failed: %w", err)
			}
			if code != 0 {
				return resultErrf("git diff exited with code %d: %s", code, output)
			}

			output = strings.TrimSpace(output)
			if output == "" {
				return result("No changes", nil)
			}
			return result(output, nil)
		},
	}
}

// --- git_log ---

type GitLogInput struct {
	MaxCount *int   `json:"max_count,omitempty"`
	FilePath string `json:"file_path,omitempty"`
	Oneline  bool   `json:"oneline,omitempty"`
}

func NewGitLogTool(cwd string, cfg *GitToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &GitToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name:        "git_log",
		Description: "显示提交日志。默认以单行格式显示最近 10 条提交。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_count": map[string]any{"type": "integer", "description": "显示的最大提交数（默认：10）"},
				"file_path": map[string]any{"type": "string", "description": "仅显示影响此文件的提交"},
				"oneline":   map[string]any{"type": "boolean", "description": "每行显示一条提交（默认：true）"},
			},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input GitLogInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}

			maxCount := 10
			if input.MaxCount != nil && *input.MaxCount > 0 {
				maxCount = *input.MaxCount
			}

			gitArgs := []string{"log"}
			if input.Oneline || input.Oneline == false && maxCount <= 20 {
				gitArgs = append(gitArgs, "--oneline")
			}
			gitArgs = append(gitArgs, fmt.Sprintf("-%d", maxCount))
			if input.FilePath != "" {
				gitArgs = append(gitArgs, "--", input.FilePath)
			}

			output, code, err := cfg.Operations.Exec(gitArgs, cwd)
			if err != nil {
				return resultErrf("git log failed: %w", err)
			}
			if code != 0 {
				return resultErrf("git log exited with code %d: %s", code, output)
			}

			output = strings.TrimSpace(output)
			if output == "" {
				return result("No commits found", nil)
			}
			return result(output, nil)
		},
	}
}
