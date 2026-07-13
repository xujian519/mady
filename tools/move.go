package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/agentcore"
)

// MoveOperations defines pluggable operations for the move tool.
type MoveOperations interface {
	Stat(path string) (os.FileInfo, error)
	Rename(oldPath, newPath string) error
	MkdirAll(path string, perm os.FileMode) error
}

// DefaultMoveOperations uses the local filesystem.
type DefaultMoveOperations struct{}

func (d DefaultMoveOperations) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }
func (d DefaultMoveOperations) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
func (d DefaultMoveOperations) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// MoveToolConfig configures the move tool.
type MoveToolConfig struct {
	Operations MoveOperations
	// Sandbox enforces the WorkingDir boundary when Enabled.
	Sandbox WorkingDirSandbox
}

func (c *MoveToolConfig) defaults() {
	if c.Operations == nil {
		c.Operations = DefaultMoveOperations{}
	}
}

// MoveToolInput is the JSON arguments for the move tool.
type MoveToolInput struct {
	Source string `json:"source"`
	Dest   string `json:"dest"`
}

// NewMoveTool creates a file/directory move/rename tool.
func NewMoveTool(cwd string, cfg *MoveToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &MoveToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name: "move",
		Description: "移动或重命名文件或目录。目标父目录若不存在则自动创建。" +
			"如果目标已存在，将被覆盖。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{
					"type":        "string",
					"description": "源文件或目录路径",
				},
				"dest": map[string]any{
					"type":        "string",
					"description": "目标路径",
				},
			},
			"required": []any{"source", "dest"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input MoveToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}

			if input.Source == "" {
				return resultErrf("source is required")
			}
			if input.Dest == "" {
				return resultErrf("dest is required")
			}

			sourcePath, err := resolvePathSandboxed(input.Source, cwd, cfg.Sandbox)
			if err != nil {
				return resultErrf("source: %v", err)
			}
			destPath, err := resolvePathSandboxed(input.Dest, cwd, cfg.Sandbox)
			if err != nil {
				return resultErrf("dest: %v", err)
			}
			// When sandbox is enabled, pin the source inode to detect
			// symlink swaps between validation and the actual operation.
			if cfg.Sandbox.Enabled {
				pinF, pinErr := os.Open(sourcePath)
				if pinErr != nil {
					return resultErrf("path not found: %s", input.Source)
				}
				if err := verifyOpenedInode(pinF, sourcePath); err != nil {
					pinF.Close()
					return resultErrf("%v", err)
				}
				pinF.Close()
			}

			// Verify source exists.
			_, err = cfg.Operations.Stat(sourcePath)
			if err != nil {
				if os.IsNotExist(err) {
					return resultErrf("source not found: %s", input.Source)
				}
				return resultErrf("cannot stat source: %w", err)
			}

			// Ensure destination parent exists.
			parentDir := filepath.Dir(destPath)
			if err := cfg.Operations.MkdirAll(parentDir, 0755); err != nil {
				return resultErrf("failed to create destination parent directory: %w", err)
			}

			// Perform move.
			if err := cfg.Operations.Rename(sourcePath, destPath); err != nil {
				return resultErrf("failed to move: %w", err)
			}

			return result(fmt.Sprintf("Moved %s -> %s", input.Source, input.Dest), nil)
		},
	}
}
