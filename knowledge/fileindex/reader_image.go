package fileindex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readImage returns metadata for image files. OCR is not performed here;
// that would require Tesseract or an OCR service (M3+).
func (fr *FileReader) readImage(path string, info os.FileInfo) *FileReadResult {
	ext := strings.ToLower(filepath.Ext(path))

	// Attempt to detect if the image might contain text based on naming.
	fileName := strings.ToLower(info.Name())
	textHints := []string{"scan", "ocr", "文档", "文件", "text", "文字"}
	mayContainText := false
	for _, hint := range textHints {
		if strings.Contains(fileName, hint) {
			mayContainText = true
			break
		}
	}

	meta := map[string]string{
		"type":             ext,
		"size_bytes":       fmt.Sprintf("%d", info.Size()),
		"may_contain_text": fmt.Sprintf("%t", mayContainText),
	}

	notice := "图片文件暂不支持内容提取。建议人工查看。"
	if mayContainText {
		notice = "此文件可能是扫描件或含文字的图片。当前版本不支持 OCR，建议人工查看或等待 OCR 支持。"
	}

	return &FileReadResult{
		Content:    "",
		Confidence: 0.0,
		Metadata:   meta,
		CostNotice: notice,
	}
}
