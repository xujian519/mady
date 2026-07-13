package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// DeleteOperations defines pluggable operations for the delete tool.
type DeleteOperations interface {
	Stat(path string) (os.FileInfo, error)
	Remove(path string) error
	RemoveAll(path string) error
}

// DefaultDeleteOperations uses the local filesystem.
type DefaultDeleteOperations struct{}

func (d DefaultDeleteOperations) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }
func (d DefaultDeleteOperations) Remove(path string) error              { return os.Remove(path) }
func (d DefaultDeleteOperations) RemoveAll(path string) error           { return os.RemoveAll(path) }

// DeleteToolConfig configures the delete tool.
type DeleteToolConfig struct {
	Operations DeleteOperations
	// ProtectedPaths are paths that cannot be deleted (exact match or prefix).
	ProtectedPaths []string
	// Sandbox enforces the WorkingDir boundary when Enabled.
	Sandbox WorkingDirSandbox
}

func (c *DeleteToolConfig) defaults() {
	if c.Operations == nil {
		c.Operations = DefaultDeleteOperations{}
	}
}

// DeleteToolInput is the JSON arguments for the delete tool.
type DeleteToolInput struct {
	Path    string `json:"path"`
	Confirm bool   `json:"confirm,omitempty"`
}

// isProtected checks if a path is protected from deletion.
func isProtected(path string, protected []string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	for _, p := range protected {
		absP, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if absPath == absP || strings.HasPrefix(absPath, absP+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// NewDeleteTool creates a file/directory deletion tool.
func NewDeleteTool(cwd string, cfg *DeleteToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &DeleteToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name: "delete",
		Description: "删除文件或目录。删除目录和受保护路径需要显式确认。" +
			"受保护路径（例如系统目录）不可删除。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "要删除的文件或目录路径",
				},
				"confirm": map[string]any{
					"type":        "boolean",
					"description": "必须为 true 才能确认删除目录或受保护路径",
				},
			},
			"required": []any{"path"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input DeleteToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}

			if input.Path == "" {
				return resultErrf("path is required")
			}

			resolved, err := resolvePathSandboxed(input.Path, cwd, cfg.Sandbox)
			if err != nil {
				return resultErrf("%v", err)
			}
			// When sandbox is enabled, pin the resolved inode to detect
			// symlink swaps between validation and the actual operation.
			if cfg.Sandbox.Enabled {
				pinF, pinErr := os.Open(resolved)
				if pinErr != nil {
					return resultErrf("path not found: %s", input.Path)
				}
				if err := verifyOpenedInode(pinF, resolved); err != nil {
					pinF.Close()
					return resultErrf("%v", err)
				}
				pinF.Close()
			}

			// Check if protected.
			if isProtected(resolved, cfg.ProtectedPaths) {
				if !input.Confirm {
					return resultErrf("path '%s' is protected. Set confirm=true to delete.", input.Path)
				}
			}

			info, err := cfg.Operations.Stat(resolved)
			if err != nil {
				if os.IsNotExist(err) {
					return resultErrf("path not found: %s", input.Path)
				}
				return resultErrf("cannot stat path: %w", err)
			}

			isDir := info.IsDir()

			// Directories require confirmation.
			if isDir && !input.Confirm {
				return resultErrf("'%s' is a directory. Set confirm=true to delete it and all contents.", input.Path)
			}

			// Perform deletion.
			if isDir {
				err = cfg.Operations.RemoveAll(resolved)
			} else {
				err = cfg.Operations.Remove(resolved)
			}
			if err != nil {
				return resultErrf("failed to delete: %w", err)
			}

			itemType := "file"
			if isDir {
				itemType = "directory"
			}
			return result(fmt.Sprintf("Deleted %s: %s", itemType, input.Path), nil)
		},
	}
}
