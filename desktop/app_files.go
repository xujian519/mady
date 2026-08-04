//go:build darwin

package main

// app_files.go — 文件内容读取、写入、删除与目录操作。
//
// 所有文件操作经过沙箱路径校验（isPathWithinSandbox），防止路径穿越攻击。
// 文本/Markdown 文件返回 UTF-8 内容；图片/PDF 返回 base64 编码。

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/pkg/util"
)

// FileEntry 是文件系统条目的概要信息。
type FileEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
}

// maxReadFileSize 是 ReadFile 允许读取的单文件上限（20MB）。
const maxReadFileSize = 20 << 20

// FileContent 描述一个已读出的文件内容。
// 文本类（text/md）通过 Text 返回 UTF-8 内容；
// 二进制类（image/pdf）通过 Data 返回 base64 编码内容。
type FileContent struct {
	Name string `json:"name"`           // 文件名
	Path string `json:"path"`           // 相对项目根的路径
	Kind string `json:"kind"`           // text | md | image | pdf
	Text string `json:"text,omitempty"` // kind=text/md 时的内容
	Data string `json:"data,omitempty"` // kind=image/pdf 时的 base64 内容
	Mime string `json:"mime,omitempty"` // image/png、application/pdf 等
	Size int64  `json:"size"`
}

// classifyFileKind 按扩展名归类文件类型。
// svg 按图片处理（前端 <img> 渲染）；未知扩展名默认 text，
// 由 ReadFile 在读出后做二进制嗅探兜底。
func classifyFileKind(name string) (kind, mime string) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "md", "text/markdown"
	case ".pdf":
		return "pdf", "application/pdf"
	case ".png":
		return "image", "image/png"
	case ".jpg", ".jpeg":
		return "image", "image/jpeg"
	case ".gif":
		return "image", "image/gif"
	case ".webp":
		return "image", "image/webp"
	case ".svg":
		return "image", "image/svg+xml"
	case ".bmp":
		return "image", "image/bmp"
	case ".ico":
		return "image", "image/x-icon"
	default:
		return "text", "text/plain"
	}
}

// isBinaryContent 嗅探内容是否为二进制（前 8KB 内含 NUL 字节即判定）。
func isBinaryContent(data []byte) bool {
	const sniffLen = 8192
	n := len(data)
	if n > sniffLen {
		n = sniffLen
	}
	return bytes.Contains(data[:n], []byte{0})
}

// resolveSandboxedPath 将路径解析为沙箱内的绝对路径。
// relPath 可以是相对路径（相对于 sandboxRoot）或绝对路径。
// 越狱路径返回错误。
func resolveSandboxedPath(relPath, sandboxRoot string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("path is required")
	}
	abs := relPath
	if !filepath.IsAbs(relPath) {
		abs = filepath.Join(sandboxRoot, relPath)
	}
	if !isPathWithinSandbox(abs, sandboxRoot) {
		return "", fmt.Errorf("path escape detected: %s is outside %s", abs, sandboxRoot)
	}
	return abs, nil
}

// resolveSandboxedPathMulti 尝试将 relPath 解析为沙箱内的绝对路径。
// 先在 sandboxRoots[0]（项目根）中解析，失败后依次尝试后续沙箱根（如 MADY_HOME）。
// 全部失败时返回组合错误。
func resolveSandboxedPathMulti(relPath string, sandboxRoots ...string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("path is required")
	}
	var errs []string
	for _, root := range sandboxRoots {
		abs, err := resolveSandboxedPath(relPath, root)
		if err == nil {
			return abs, nil
		}
		errs = append(errs, err.Error())
	}
	return "", fmt.Errorf("path not allowed: %s", strings.Join(errs, "; "))
}

// ReadFile 读取项目沙箱或 MADY_HOME 沙箱内的文件内容。
// relPath 是相对于项目根目录的路径（项目文件）或相对/绝对路径（全局技能文件）。
// 文本/Markdown 返回 Text；图片/PDF 返回 base64 编码的 Data。
// 其他二进制文件返回错误，不向前端暴露原始字节。
func (a *App) ReadFile(relPath string) (*FileContent, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return nil, fmt.Errorf("readFile: %w", err)
	}

	roots := []string{cwd}
	if home, err := util.MadyHome(); err == nil && home != cwd {
		roots = append(roots, home)
	}

	abs, err := resolveSandboxedPathMulti(relPath, roots...)
	if err != nil {
		return nil, fmt.Errorf("readFile: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("readFile: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("readFile: %s is a directory", relPath)
	}
	if info.Size() > maxReadFileSize {
		return nil, fmt.Errorf("readFile: file too large (%d bytes, limit %d)", info.Size(), maxReadFileSize)
	}

	raw, err := os.ReadFile(abs) //nolint:gosec // 路径已过沙箱校验
	if err != nil {
		return nil, fmt.Errorf("readFile: %w", err)
	}

	kind, mime := classifyFileKind(info.Name())
	fc := &FileContent{
		Name: info.Name(),
		Path: relPath,
		Kind: kind,
		Mime: mime,
		Size: info.Size(),
	}

	switch kind {
	case "text", "md":
		// S-12：md 同样做二进制嗅探——含 NUL 的 .md（如误命名的二进制）以文本原样
		// 返回会污染渲染/后续处理，统一拒绝。
		if isBinaryContent(raw) {
			return nil, fmt.Errorf("readFile: %s appears to be a binary file", relPath)
		}
		fc.Text = string(raw)
	case "image", "pdf":
		fc.Data = base64.StdEncoding.EncodeToString(raw)
	}
	return fc, nil
}

// maxWriteFileSize 是 WriteFile 允许写入的内容上限（20MB）。
const maxWriteFileSize = 20 << 20

// WriteFile 将文本内容写入项目沙箱或 MADY_HOME 沙箱内的文件。
// 仅允许写入 text/md 类文件（按扩展名判定），图片/PDF 等二进制不可写。
// 采用临时文件 + rename 的原子写策略，避免写一半留下损坏文件。
func (a *App) WriteFile(relPath, content string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if len(content) > maxWriteFileSize {
		return fmt.Errorf("writeFile: content too large (%d bytes, limit %d)", len(content), maxWriteFileSize)
	}

	kind, _ := classifyFileKind(relPath)
	if kind != "text" && kind != "md" {
		return fmt.Errorf("writeFile: %s is not a writable text file", relPath)
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}

	roots := []string{cwd}
	if home, err := util.MadyHome(); err == nil && home != cwd {
		roots = append(roots, home)
	}

	abs, err := resolveSandboxedPathMulti(relPath, roots...)
	if err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}
	// 原子写：同目录临时文件 + rename
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".mady-write-*")
	if err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writeFile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}
	// 保留既有文件权限（M-2）：已存在文件沿用原权限，新文件默认 0600。
	mode := os.FileMode(0600)
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}
	log.Printf("[mady-desktop] wrote file: %s (%d bytes)", abs, len(content))
	return nil
}

// DeleteEntry 删除项目沙箱内的文件或空目录。
// 目录必须为空才允许删除（递归删除本期不支持）。
func (a *App) DeleteEntry(relPath string) error {
	if err := a.ready(); err != nil {
		return err
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return fmt.Errorf("deleteEntry: %w", err)
	}

	abs, err := resolveSandboxedPath(relPath, cwd)
	if err != nil {
		return fmt.Errorf("deleteEntry: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("deleteEntry: %w", err)
	}

	if info.IsDir() {
		entries, err := os.ReadDir(abs)
		if err != nil {
			return fmt.Errorf("deleteEntry: %w", err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("deleteEntry: directory %s is not empty", relPath)
		}
	}

	if err := os.Remove(abs); err != nil {
		return fmt.Errorf("deleteEntry: %w", err)
	}
	log.Printf("[mady-desktop] deleted: %s", abs)
	return nil
}

// ListDirectory 返回指定路径下的文件和文件夹列表。
// relPath 是相对于项目根目录的路径，空字符串表示根目录。
func (a *App) ListDirectory(relPath string) ([]FileEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return nil, fmt.Errorf("listDirectory: %w", err)
	}

	targetDir := cwd
	if relPath != "" {
		targetDir = filepath.Join(cwd, relPath)
	}

	// 沙箱边界校验（ListDirectory 也需校验，防止读越狱路径）
	if !isPathWithinSandbox(targetDir, cwd) {
		return nil, fmt.Errorf("listDirectory: path escape detected: %s is outside %s", targetDir, cwd)
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, fmt.Errorf("listDirectory: %w", err)
	}

	var result []FileEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, FileEntry{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		})
	}
	return result, nil
}

// CreateFolder 在指定父目录下创建子文件夹。
// parentPath 是相对于项目根目录的路径，空字符串表示根目录。
// folderName 为要创建的文件夹名称。
// 操作经过沙箱路径校验，越狱路径将被拒绝。
func (a *App) CreateFolder(parentPath, folderName string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	if folderName == "" {
		return "", fmt.Errorf("createFolder: folderName is required")
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return "", fmt.Errorf("createFolder: %w", err)
	}

	targetDir := cwd
	if parentPath != "" {
		targetDir = filepath.Join(cwd, parentPath)
	}

	newDir := filepath.Join(targetDir, folderName)

	// 沙箱边界校验
	if !isPathWithinSandbox(newDir, cwd) {
		return "", fmt.Errorf("createFolder: path escape detected: %s is outside %s", newDir, cwd)
	}

	if err := os.MkdirAll(newDir, 0750); err != nil {
		return "", fmt.Errorf("createFolder: %w", err)
	}
	log.Printf("[mady-desktop] created folder: %s", newDir)
	return newDir, nil
}

// RenameFolder 重命名指定路径的文件夹。
// oldPath 为当前完整路径（相对于项目根），
// newName 为新文件夹名称。
// 操作经过沙箱路径校验，越狱路径将被拒绝。
func (a *App) RenameFolder(oldPath, newName string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if oldPath == "" || newName == "" {
		return fmt.Errorf("renameFolder: oldPath and newName are required")
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return fmt.Errorf("renameFolder: %w", err)
	}

	oldDir := filepath.Join(cwd, oldPath)
	parentDir := filepath.Dir(oldDir)
	newDir := filepath.Join(parentDir, newName)

	// 沙箱边界校验
	if !isPathWithinSandbox(oldDir, cwd) {
		return fmt.Errorf("renameFolder: path escape detected: %s is outside %s", oldDir, cwd)
	}
	if !isPathWithinSandbox(newDir, cwd) {
		return fmt.Errorf("renameFolder: path escape detected: %s is outside %s", newDir, cwd)
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		return fmt.Errorf("renameFolder: %w", err)
	}
	log.Printf("[mady-desktop] renamed folder: %s → %s", oldDir, newDir)
	return nil
}
