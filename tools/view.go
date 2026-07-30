package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// errMaxEntries is a sentinel error indicating the directory walk hit the
// entry limit. Compared via errors.Is, not string matching.
var errMaxEntries = errors.New("max_entries_reached")

// ViewOperations defines pluggable operations for the view tool.
type ViewOperations interface {
	Stat(path string) (os.FileInfo, error)
	ReadDir(path string) ([]os.DirEntry, error)
}

// DefaultViewOperations uses the local filesystem.
type DefaultViewOperations struct{}

func (d DefaultViewOperations) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }
func (d DefaultViewOperations) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

// ViewToolConfig configures the view tool.
type ViewToolConfig struct {
	Operations ViewOperations
	MaxDepth   int
	MaxEntries int
	Sandbox    WorkingDirSandbox
}

func (c *ViewToolConfig) defaults() {
	if c.Operations == nil {
		c.Operations = DefaultViewOperations{}
	}
	if c.MaxDepth <= 0 {
		c.MaxDepth = 3
	}
	if c.MaxEntries <= 0 {
		c.MaxEntries = 200
	}
}

// ViewToolInput is the JSON arguments for the view tool.
type ViewToolInput struct {
	Path  string `json:"path,omitempty"`
	Depth *int   `json:"depth,omitempty"`
}

// directoryWalker 在约束条件下执行递归目录遍历并生成树形文本。
type directoryWalker struct {
	ops        ViewOperations
	maxDepth   int
	maxEntries int
	lines      []string
	entries    int
	root       string
}

func newDirectoryWalker(ops ViewOperations, maxDepth, maxEntries int) *directoryWalker {
	return &directoryWalker{
		ops:        ops,
		maxDepth:   maxDepth,
		maxEntries: maxEntries,
	}
}

// Walk 执行目录遍历并返回树形文本。当达到条目上限时不会返回错误，而是附带截断信息。
func (w *directoryWalker) Walk(root string) (string, error) {
	w.root = filepath.Base(root)
	w.lines = nil
	w.entries = 0
	if err := w.walk(root, "", 1); err != nil && !errors.Is(err, errMaxEntries) {
		return "", err
	}
	return w.buildOutput(), nil
}

// walk 递归遍历目录，将条目追加到 lines 中。
func (w *directoryWalker) walk(path, prefix string, depth int) error {
	if depth > w.maxDepth {
		return nil
	}
	if w.entries >= w.maxEntries {
		return errMaxEntries
	}

	entriesList, err := w.ops.ReadDir(path)
	if err != nil {
		return nil
	}

	// Sort: dirs first, then files, alphabetically.
	sort.Slice(entriesList, func(i, j int) bool {
		if entriesList[i].IsDir() != entriesList[j].IsDir() {
			return entriesList[i].IsDir()
		}
		return strings.ToLower(entriesList[i].Name()) < strings.ToLower(entriesList[j].Name())
	})

	for i, entry := range entriesList {
		if w.entries >= w.maxEntries {
			return errMaxEntries
		}

		isLast := i == len(entriesList)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		w.lines = append(w.lines, prefix+connector+name)
		w.entries++

		if entry.IsDir() {
			nextPrefix := prefix
			if isLast {
				nextPrefix += "    "
			} else {
				nextPrefix += "│   "
			}
			if err := w.walk(filepath.Join(path, entry.Name()), nextPrefix, depth+1); err != nil {
				if errors.Is(err, errMaxEntries) {
					return err
				}
			}
		}
	}
	return nil
}

// buildOutput 组装最终输出的树形文本。
func (w *directoryWalker) buildOutput() string {
	output := w.root + "/"
	if len(w.lines) > 0 {
		output += "\n" + strings.Join(w.lines, "\n")
	}
	if w.entries >= w.maxEntries {
		output += fmt.Sprintf("\n\n[%d entries limit reached]", w.maxEntries)
	}
	return output
}

// NewViewTool creates a directory tree viewing tool.
func NewViewTool(cwd string, cfg *ViewToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &ViewToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name: "view",
		Description: fmt.Sprintf("以树形结构查看目录结构。返回文件和目录的层级列表。"+
			"最大深度：%d，最大条目数：%d。用于浏览项目结构。", cfg.MaxDepth, cfg.MaxEntries),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string", "description": "要查看的目录（默认：当前目录）"},
				"depth": map[string]any{"type": "integer", "description": fmt.Sprintf("遍历的最大深度（默认：%d）", cfg.MaxDepth)},
			},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input ViewToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}

			dirPath, err := resolvePathSandboxed(input.Path, cwd, cfg.Sandbox)
			if err != nil {
				return resultErrf("%w", err)
			}
			if dirPath == "" {
				dirPath = cwd
			}
			// When sandbox is enabled, pin the resolved inode to detect
			// symlink swaps between validation and the actual operation.
			if cfg.Sandbox.Enabled {
				if err := pinPath(dirPath); err != nil {
					return resultErrf("%w", err)
				}
			}

			info, err := cfg.Operations.Stat(dirPath)
			if err != nil {
				return resultErrf("path not found: %s", input.Path)
			}

			maxDepth := cfg.MaxDepth
			if input.Depth != nil && *input.Depth > 0 {
				maxDepth = *input.Depth
			}

			// If path is a file, show file info.
			if !info.IsDir() {
				return result(fmt.Sprintf("%s (%s, %d bytes)", filepath.Base(dirPath), info.Mode(), info.Size()), nil)
			}

			walker := newDirectoryWalker(cfg.Operations, maxDepth, cfg.MaxEntries)
			output, err := walker.Walk(dirPath)
			if err != nil {
				return resultErrf("walk failed: %w", err)
			}
			return result(output, nil)
		},
	}
}
