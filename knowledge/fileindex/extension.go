package fileindex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// Extension implements agentcore.Extension + ToolProvider.
// It provides the search_project_files tool to the Agent.
type Extension struct {
	config ExtensionConfig

	mu          sync.Mutex
	fi          *FileIndex // may be nil; set via SetFileIndex
	fallbackDir string     // runtime working dir when FileIndex is nil
}

// ExtensionConfig configures the fileindex extension.
type ExtensionConfig struct {
	// FileIndex is optional; set at runtime via SetFileIndex.
	// When nil, the extension falls back to FallbackDir for direct filesystem access.
	FileIndex *FileIndex
	// FallbackDir is used as the working directory when FileIndex is nil.
	// Typically the initial CWD when Mady TUI starts.
	FallbackDir string
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

// SetFallbackDir updates the runtime fallback working directory.
// Used when case context is cleared to reset the working directory to the initial CWD.
func (e *Extension) SetFallbackDir(dir string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.fallbackDir = dir
}

// workingDir returns the effective working directory:
// runtime fallbackDir first, then config FallbackDir.
func (e *Extension) workingDir() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fallbackDir != "" {
		return e.fallbackDir
	}
	return e.config.FallbackDir
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
			Description: "在项目文件夹中搜索文件。已关联案件且索引可用时，支持文件名、路径和文件内容搜索（RRF 排序）；基础模式仅匹配文件名和路径。返回按相关性排序的文件列表。",
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
	if fi != nil {
		// ---- Indexed mode (FileIndex available) ----
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

	// ---- Fallback mode: simple filename/path match via WalkDir ----
	rootDir := e.workingDir()
	if rootDir == "" {
		return searchResult{Message: "未设置工作目录。请在案件文件夹下启动 Mady，系统会自动识别。"}, nil
	}
	return searchFallback(ctx, rootDir, input.Query, input.MaxResults), nil
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
	var rootDir string
	if fi != nil {
		// Ensure the index is fresh (so the file record exists).
		if err := fi.Refresh(ctx); err != nil {
			return map[string]string{"error": fmt.Sprintf("刷新文件索引失败: %s", err.Error())}, nil
		}
		rootDir = fi.Dir()
		if rootDir == "" {
			return map[string]string{"error": "未设置工作目录。请在案件文件夹下启动 Mady，系统会自动识别。"}, nil
		}
	} else {
		rootDir = e.workingDir()
		if rootDir == "" {
			return map[string]string{"error": "未设置工作目录。请在案件文件夹下启动 Mady，系统会自动识别。"}, nil
		}
	}
	fullPath := input.Path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(rootDir, fullPath)
	}

	// Verify the resolved path is within the root directory (sandbox).
	rel, err := filepath.Rel(rootDir, fullPath)
	sep := string(filepath.Separator)
	if err != nil || strings.HasPrefix(rel, ".."+sep) || rel == ".." {
		return map[string]string{
			"error": fmt.Sprintf("路径 %s 不在项目目录 %s 内", input.Path, rootDir),
		}, nil
	}

	// Create a FileReader and read the file (FileReader does its own stat, no need to duplicate).
	reader := NewFileReader(rootDir)
	result, err := reader.ReadProjectFile(ctx, input.Path)
	if err != nil {
		return map[string]string{"error": err.Error()}, nil
	}

	return result, nil
}

// searchFallback performs a simple filename/path keyword search when no FileIndex is available.
// Walks the project directory and matches files whose relative path contains the query
// (case-insensitive). Results are ranked by match quality and returned sorted by relevance.
func searchFallback(ctx context.Context, rootDir, query string, maxResults int) searchResult {
	var matches []FileCandidate
	lowerQuery := strings.ToLower(query)

	walkErr := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		// Check for context cancellation periodically (every directory entry).
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			base := d.Name()
			// Skip hidden directories and common large dependency trees.
			if base != "." && (strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return nil
		}

		relLower := strings.ToLower(relPath)
		if !strings.Contains(relLower, lowerQuery) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		cat := classifyFile(path)
		preview := extractPreview(path, cat)

		// Score by match quality: exact > prefix > contains in filename > path-only.
		// Strip file extension for fair name comparison.
		base := strings.ToLower(filepath.Base(relPath))
		baseNoExt := strings.TrimSuffix(base, filepath.Ext(base))
		var relevance float64
		switch {
		case base == lowerQuery || baseNoExt == lowerQuery:
			relevance = 1.0
		case strings.HasPrefix(base, lowerQuery) || strings.HasPrefix(baseNoExt, lowerQuery):
			relevance = 0.8
		case strings.Contains(base, lowerQuery) || strings.Contains(baseNoExt, lowerQuery):
			relevance = 0.6
		default:
			relevance = 0.3 // only in directory path
		}

		matches = append(matches, FileCandidate{
			Path:       relPath,
			Category:   cat,
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime(),
			Relevance:  relevance,
			Preview:    preview,
		})

		if len(matches) >= maxResults {
			return filepath.SkipAll
		}
		return nil
	})

	if walkErr != nil && !os.IsNotExist(walkErr) {
		return searchResult{
			Message: fmt.Sprintf("搜索文件失败: %s", walkErr.Error()),
		}
	}

	if len(matches) == 0 {
		return searchResult{
			Message: fmt.Sprintf("在 %s 中未找到匹配 '%s' 的文件。尝试使用不同的关键词。", rootDir, query),
		}
	}

	// Sort by relevance descending, then by path for stable ordering.
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Relevance != matches[j].Relevance {
			return matches[i].Relevance > matches[j].Relevance
		}
		return matches[i].Path < matches[j].Path
	})

	return searchResult{Files: matches, Total: len(matches)}
}

// compile-time check
var _ agentcore.Extension = (*Extension)(nil)
var _ interface{ Tools() []*agentcore.Tool } = (*Extension)(nil)
