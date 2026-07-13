package fileindex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// Extension implements agentcore.Extension + ToolProvider.
// It provides the search_project_files tool to the Agent.
type Extension struct {
	config ExtensionConfig

	mu sync.Mutex
	fi *FileIndex // may be nil; set via SetFileIndex
}

// ExtensionConfig configures the fileindex extension.
type ExtensionConfig struct {
	// FileIndex is optional; set at runtime via SetFileIndex.
	// When nil, the search tool returns an error asking the user to set a project.
	FileIndex *FileIndex
}

// NewExtension creates a fileindex extension.
func NewExtension(cfg ExtensionConfig) *Extension {
	return &Extension{config: cfg, fi: cfg.FileIndex}
}

// SetFileIndex updates the FileIndex at runtime (e.g., when a project is selected).
func (e *Extension) SetFileIndex(fi *FileIndex) {
	e.mu.Lock()
	e.fi = fi
	e.mu.Unlock()
}

// FileIndex returns the current FileIndex (may be nil).
func (e *Extension) FileIndex() *FileIndex {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.fi
}

// ---------------------------------------------------------------------------
// agentcore.Extension interface
// ---------------------------------------------------------------------------

func (e *Extension) Name() string                                           { return "fileindex" }
func (e *Extension) Init(ctx context.Context, agent *agentcore.Agent) error { return nil }
func (e *Extension) Dispose() error                                         { return nil }

// Tools implements ToolProvider.
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		{
			Name:        "read_project_file",
			Description: "读取并深度处理案件文件夹中的文件。根据文件类型自动选择提取方式（文本直接读、PDF 通过 pdftotext 提取、docx 解压提取、CSV 表格格式化）。图片和音频文件返回元数据和成本提示。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "文件路径（相对于案件根目录，或绝对路径）",
					},
				},
				"required": []string{"path"},
			},
			Func: e.handleReadFile,
		},
		{
			Name:        "search_project_files",
			Description: "在案件文件夹中搜索文件。返回按相关性排序的文件列表，包含路径、类型和预览文本。支持按文件名、路径和文件内容搜索。使用前请确保已通过 /case 切换到案件。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "搜索关键词（文件名、路径或文件内容中的关键词）",
					},
					"max_results": map[string]any{
						"type":        "integer",
						"description": "最大返回数量（1-50，默认 10）",
						"default":     10,
					},
				},
				"required": []string{"query"},
			},
			Func: e.handleSearch,
		},
	}
}

// ---------------------------------------------------------------------------
// Tool handler
// ---------------------------------------------------------------------------

type searchInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

type searchResult struct {
	Files   []FileCandidate `json:"files"`
	Total   int             `json:"total"`
	Message string          `json:"message,omitempty"`
}

func (e *Extension) handleSearch(ctx context.Context, args json.RawMessage) (any, error) {
	var input searchInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("search_project_files: invalid arguments: %w", err)
	}
	if input.Query == "" {
		return searchResult{Message: "请提供搜索关键词"}, nil
	}
	if input.MaxResults <= 0 {
		input.MaxResults = 10
	}
	if input.MaxResults > 50 {
		input.MaxResults = 50
	}

	fi := e.FileIndex()
	if fi == nil {
		return searchResult{Message: "未设置案件文件夹。请先使用 /case 命令切换到案件目录。"}, nil
	}

	// Ensure the index is fresh.
	if err := fi.Refresh(ctx); err != nil {
		return searchResult{
			Message: fmt.Sprintf("扫描文件夹失败: %s", err.Error()),
		}, nil
	}

	files, err := fi.Search(ctx, input.Query, input.MaxResults)
	if err != nil {
		return searchResult{Message: fmt.Sprintf("搜索失败: %s", err.Error())}, nil
	}

	if len(files) == 0 {
		return searchResult{Message: "未找到匹配的文件。尝试使用不同的关键词，或在 /case 中确认案件目录已设置。"}, nil
	}

	return searchResult{Files: files, Total: len(files)}, nil
}

type readInput struct {
	Path string `json:"path"`
}

func (e *Extension) handleReadFile(ctx context.Context, args json.RawMessage) (any, error) {
	var input readInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("read_project_file: invalid arguments: %w", err)
	}
	if input.Path == "" {
		return map[string]string{"error": "请提供文件路径"}, nil
	}

	fi := e.FileIndex()
	if fi == nil {
		return map[string]string{"error": "未设置案件文件夹。请先使用 /case 命令切换到案件目录。"}, nil
	}

	// Ensure the index is fresh (so the file record exists).
	if err := fi.Refresh(ctx); err != nil {
		return map[string]string{"error": fmt.Sprintf("刷新文件索引失败: %s", err.Error())}, nil
	}

	// Resolve path relative to the project directory.
	rootDir := fi.Dir()
	fullPath := input.Path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(rootDir, fullPath)
	}

	// Verify the resolved path is within the root directory (sandbox).
	rel, err := filepath.Rel(rootDir, fullPath)
	if err != nil || (len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator)) || rel == ".." {
		return map[string]string{
			"error": fmt.Sprintf("路径 %s 不在项目目录 %s 内", input.Path, rootDir),
		}, nil
	}

	// Check file exists and is readable.
	if _, err := os.Stat(fullPath); err != nil {
		return map[string]string{
			"error": fmt.Sprintf("文件不存在或无法读取: %s", input.Path),
		}, nil
	}

	// Create a FileReader and read the file.
	reader := NewFileReader(rootDir)
	result, err := reader.ReadProjectFile(ctx, input.Path)
	if err != nil {
		return map[string]string{"error": err.Error()}, nil
	}

	return result, nil
}

// compile-time check
var _ agentcore.Extension = (*Extension)(nil)
var _ interface{ Tools() []*agentcore.Tool } = (*Extension)(nil)
