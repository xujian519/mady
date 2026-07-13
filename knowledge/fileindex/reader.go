package fileindex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileReadResult is the output of reading a project file.
type FileReadResult struct {
	Path       string            `json:"path"`
	Category   FileCategory      `json:"category"`
	Content    string            `json:"content"`
	Confidence float64           `json:"confidence"` // extraction confidence (<1.0 for OCR/ASR)
	Sections   []Section         `json:"sections,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CostNotice string            `json:"cost_notice,omitempty"` // e.g. "OCR not yet supported"
	Error      string            `json:"error,omitempty"`       // fatal error message
}

// Section is a logical segment within a document (typically a paragraph or heading block).
type Section struct {
	Heading string `json:"heading,omitempty"`
	Content string `json:"content"`
	PageNum int    `json:"page_num,omitempty"` // PDF page number (1-based)
}

// FileReader reads and extracts content from project files by type.
// It is designed to be cheap for text files and progressively more expensive
// for binary formats. The caller (read_project_file tool) decides when to
// trigger deep processing.
type FileReader struct {
	// RootPath is the project root directory. All file reads are scoped to
	// paths within this directory (sandbox boundary enforced by the caller).
	RootPath string
}

// NewFileReader creates a FileReader scoped to the given root directory.
func NewFileReader(rootPath string) *FileReader {
	return &FileReader{RootPath: rootPath}
}

// ReadProjectFile reads and extracts content from a file.
// The path must be within RootPath (relative paths are resolved against RootPath).
// Returns a FileReadResult with the extracted content, confidence, and any cost notice.
func (fr *FileReader) ReadProjectFile(ctx context.Context, path string) (*FileReadResult, error) {
	// Resolve path: if relative, join with RootPath; if absolute, validate within RootPath.
	fullPath, err := fr.resolvePath(path)
	if err != nil {
		return nil, fmt.Errorf("read_project_file: %w", err)
	}

	// Stat file for metadata.
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read_project_file: stat %s: %w", path, err)
	}
	if info.IsDir() {
		return &FileReadResult{
			Path:     path,
			Category: CategoryUnknown,
			Error:    "路径是一个目录，请指定文件路径",
		}, nil
	}

	cat := classifyFile(fullPath)
	ext := strings.ToLower(filepath.Ext(fullPath))
	meta := map[string]string{
		"size_bytes":  fmt.Sprintf("%d", info.Size()),
		"modified_at": info.ModTime().Format("2006-01-02 15:04:05"),
	}

	var result *FileReadResult

	// Ext-based dispatch (overrides category for Office documents).
	switch ext {
	case ".docx", ".doc":
		result, err = fr.readDocx(ctx, fullPath)
	default:
		switch cat {
		case CategoryTextDoc:
			result, err = fr.readText(ctx, fullPath)
		case CategoryPdf:
			result, err = fr.readPDF(ctx, fullPath)
		case CategorySpreadsheet:
			result, err = fr.readSpreadsheet(ctx, fullPath)
		case CategoryImage:
			result = fr.readImage(fullPath, info)
		case CategoryAudio:
			result = fr.readAudio(fullPath, info)
		default:
			result, err = fr.readText(ctx, fullPath)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("read_project_file: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("read_project_file: nil result for %s", path)
	}

	result.Path = path
	result.Category = cat
	if result.Metadata == nil {
		result.Metadata = meta
	} else {
		for k, v := range meta {
			result.Metadata[k] = v
		}
	}
	return result, nil
}

// resolvePath resolves the file path relative to RootPath and validates it
// is within the sandbox boundary.
func (fr *FileReader) resolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		// Absolute path: check it's within RootPath.
		rel, err := filepath.Rel(fr.RootPath, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("路径 %s 不在项目目录 %s 内", path, fr.RootPath)
		}
		return path, nil
	}
	// Relative path: resolve against RootPath.
	fullPath := filepath.Join(fr.RootPath, path)
	// Verify it resolves within RootPath (no ../ escape).
	rel, err := filepath.Rel(fr.RootPath, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("路径 %s 不在项目目录 %s 内", path, fr.RootPath)
	}
	return fullPath, nil
}
