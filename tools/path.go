package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// WorkingDirSandbox holds sandbox configuration for file tool path resolution.
// When Enabled is true, resolvePathSandboxed enforces that all file operations
// stay within the WorkingDir boundary.
type WorkingDirSandbox struct {
	Enabled    bool
	WorkingDir string
}

// SandboxDisabled returns a sandbox that allows all paths (backward compatible).
func SandboxDisabled(workingDir string) WorkingDirSandbox {
	return WorkingDirSandbox{
		Enabled:    false,
		WorkingDir: workingDir,
	}
}

// resolvePathSandboxed resolves a user-provided path and enforces the WorkingDir
// boundary when the sandbox is enabled. When disabled, falls back to resolvePath.
// Returns the resolved absolute path, or an error if the path escapes the sandbox.
func resolvePathSandboxed(userPath, cwd string, sbx WorkingDirSandbox) (string, error) {
	resolved := resolvePath(userPath, cwd)
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("路径解析失败: %w", err)
	}

	if !sbx.Enabled {
		// 沙箱禁用时仍尝试 macOS NFD 标准化，保持向后兼容性。
		return resolveNFD(abs), nil
	}

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("工作目录解析失败: %w", err)
	}

	// 解析符号链接，防止 link_to_etc -> /etc 逃逸沙箱边界
	realAbs, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("路径解析失败: %w", err)
	}
	realCwd, err := filepath.EvalSymlinks(absCwd)
	if err != nil {
		return "", fmt.Errorf("工作目录解析失败: %w", err)
	}

	// 确保在 WorkingDir 子树内
	rel, err := filepath.Rel(realCwd, realAbs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("路径 %q 不在工作目录 %q 范围内", userPath, absCwd)
	}

	return resolveNFD(realAbs), nil
}

// resolveNFD 尝试 macOS NFD 标准化。
// macOS 以 NFD（分解）形式存储文件名。如果用户提供的路径是 NFC（组合）形式，
// os.Stat 将找不到文件。此函数先尝试原路径，失败时回退到 NFD 标准化后的路径。
func resolveNFD(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	nfdPath := norm.NFD.String(path)
	if nfdPath != path {
		if _, err := os.Stat(nfdPath); err == nil {
			return nfdPath
		}
	}
	return path
}

// resolvePath resolves a user-provided path against the working directory.
// Handles ~ expansion, absolute paths, and empty strings.
func resolvePath(userPath, cwd string) string {
	if userPath == "" {
		return cwd
	}

	// Expand ~ to home directory.
	if userPath == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return userPath
	}
	if strings.HasPrefix(userPath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, userPath[2:])
		}
	}

	// If already absolute, return as-is.
	if filepath.IsAbs(userPath) {
		return userPath
	}

	// Resolve relative to cwd.
	return filepath.Join(cwd, userPath)
}

// resolveReadPath resolves a path and handles macOS NFD normalization.
// macOS stores filenames in NFD (decomposed) form. If a user-supplied
// path is in NFC (composed) form, the file won't be found by the OS.
func resolveReadPath(userPath, cwd string) string {
	resolved := resolvePath(userPath, cwd)

	if _, err := os.Stat(resolved); err == nil {
		return resolved
	}

	// Only attempt NFD normalization if the original path contains
	// characters that differ between NFC and NFD forms.
	nfdPath := norm.NFD.String(resolved)
	if nfdPath != resolved {
		if _, err := os.Stat(nfdPath); err == nil {
			return nfdPath
		}
	}

	return resolved
}
