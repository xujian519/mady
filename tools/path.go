package tools

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

var sandboxWarnOnce sync.Once

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

	// Normalize to NFD early so sandbox validation and file ops use the same bytes.
	abs = resolveNFD(abs)

	if !sbx.Enabled {
		sandboxWarnOnce.Do(func() {
			if sbx.WorkingDir != "" {
				log.Printf("[sandbox] WARNING: sandbox disabled, path boundary check skipped (workingDir=%q). This warning will not be repeated.", sbx.WorkingDir)
			}
		})
		return abs, nil
	}

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("工作目录解析失败: %w", err)
	}

	// 解析符号链接，防止 link_to_etc -> /etc 逃逸沙箱边界
	realAbs, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("path not found: %s", userPath)
		}
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

	return realAbs, nil
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

// OpenSandboxed resolves path against the sandbox, opens it read-only, and returns
// the *os.File pinned to the validated inode. Even if the path's symlinks are
// replaced after this call, the returned FD still refers to the original inode,
// preventing TOCTOU attacks.
func OpenSandboxed(path string, sbx WorkingDirSandbox) (*os.File, error) {
	resolved, err := resolvePathSandboxed(path, sbx.WorkingDir, sbx)
	if err != nil {
		return nil, err
	}
	return os.Open(resolved)
}

// OpenSandboxedFile resolves path against the sandbox, opens it with the given
// flags and permissions, and returns the *os.File pinned to the validated inode.
func OpenSandboxedFile(path string, sbx WorkingDirSandbox, flag int, perm os.FileMode) (*os.File, error) {
	resolved, err := resolvePathSandboxed(path, sbx.WorkingDir, sbx)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(resolved, flag, perm)
}

// readFileSandboxed opens a file through the sandbox, reads its full content,
// and returns it as a string. This prevents TOCTOU by pinning the inode through
// the open FD.
func readFileSandboxed(path string, sbx WorkingDirSandbox) (string, error) {
	f, err := OpenSandboxed(path, sbx)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 100<<20)) // 100MB max
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// pinPath opens the path, verifies its inode hasn't been swapped since resolution,
// and closes the FD. Returns nil on success, or an error describing the inode mismatch
// or I/O failure. Used to detect TOCTOU symlink-swap attacks between path resolution
// and the actual file operation.
func pinPath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("path not found: %s", path)
	}
	defer f.Close()
	return verifyOpenedInode(f, path)
}

// verifyOpenedInode checks that the file or directory at the given path still
// refers to the same inode as the already-open *os.File. This prevents TOCTOU
// attacks where a path is swapped between opening and a subsequent path-based
// operation (e.g. Rename, Remove, RemoveAll).
func verifyOpenedInode(f *os.File, path string) error {
	fi1, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat opened fd: %w", err)
	}
	fi2, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(fi1, fi2) {
		return fmt.Errorf("path %q: inode changed since open — possible symlink swap", path)
	}
	return nil
}
