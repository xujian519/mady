package fileindex

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Cheap metadata extraction (no full document parsing)
// ---------------------------------------------------------------------------

// classifyFile determines the file category by extension.
func classifyFile(path string) FileCategory {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".md", ".go", ".py", ".js", ".ts", ".java", ".rb", ".c", ".cpp", ".h",
		".hpp", ".rs", ".swift", ".kt", ".scala", ".php", ".css", ".html", ".xml", ".json",
		".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".log", ".sh", ".bash", ".zsh",
		".fish", ".ps1", ".bat", ".sql", ".r", ".lua", ".pl", ".pm", ".tcl":
		return CategoryTextDoc
	case ".pdf":
		return CategoryPdf
	case ".doc", ".docx", ".odt", ".rtf":
		return CategoryTextDoc
	case ".xls", ".xlsx", ".csv", ".ods":
		return CategorySpreadsheet
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff", ".tif", ".webp", ".svg", ".ico":
		return CategoryImage
	case ".mp3", ".m4a", ".wav", ".wma", ".ogg", ".flac", ".aac", ".aiff", ".opus", ".wv":
		return CategoryAudio
	default:
		return CategoryUnknown
	}
}

// extractPreview reads the first 512 bytes for text files, empty for others.
func extractPreview(path string, cat FileCategory) string {
	switch cat {
	case CategoryTextDoc, CategoryPdf, CategorySpreadsheet:
		// Read only first 512 bytes to avoid loading large files into memory.
		f, err := os.Open(path) //nolint:gosec // path is from fileindex watcher, validated absolute path
		if err != nil {
			return filepath.Base(path)
		}
		defer func() { _ = f.Close() }()
		var buf [512]byte
		n, _ := io.ReadFull(f, buf[:])
		if n == 0 {
			return filepath.Base(path)
		}
		data := buf[:n]
		// Check if it looks like a binary file (null bytes).
		for _, b := range data {
			if b == 0 {
				return filepath.Base(path)
			}
		}
		return string(data)
	default:
		return ""
	}
}

// quickChecksum returns a base64 string using a fast sampling approach:
// first 4KB + mod time, avoiding full-file reads for large files.
func quickChecksum(path string, info os.FileInfo) string {
	f, err := os.Open(path) //nolint:gosec // path is from file walk
	if err != nil {
		return fmt.Sprintf("%d_%d", info.Size(), info.ModTime().UnixNano())
	}
	defer func() { _ = f.Close() }()
	var buf [4096]byte
	n, _ := io.ReadFull(f, buf[:])
	if n == 0 {
		return fmt.Sprintf("%d_%d", info.Size(), info.ModTime().UnixNano())
	}
	hash := sha256.Sum256(buf[:n])
	return base64.StdEncoding.EncodeToString(hash[:16])
}

// truncatePreview shortens preview text for display.
func truncatePreview(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
