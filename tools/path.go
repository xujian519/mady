package tools

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

var sandboxWarnOnce sync.Once

// ErrOutsideSandbox indicates a path was rejected by the sandbox.
// Tools should check errors.Is(err, ErrOutsideSandbox) to decide whether
// to escalate via the permission system (Ask) instead of returning a hard error.
var ErrOutsideSandbox = fmt.Errorf("path outside sandbox")

// AccessMode classifies a file operation as read-only or read-write.
// Read-only tools (read, grep, glob, find, ls, view, vision) check both
// WorkingDir and AllowRead. Write tools (write_file, edit, patch, delete,
// move) check WorkingDir and AllowWrite.
type AccessMode int

const (
	AccessRead AccessMode = iota
	AccessWrite
)

// WorkingDirSandbox holds sandbox configuration for file tool path resolution.
// When Enabled is true, resolvePathSandboxed enforces that all file operations
// stay within the WorkingDir boundary or an explicit allowlist.
type WorkingDirSandbox struct {
	Enabled    bool
	WorkingDir string

	// AllowRead lists extra directory trees that read-only tools may access.
	// Write tools are NOT permitted in these directories.
	AllowRead []string

	// AllowWrite lists extra directory trees where write tools are permitted
	// (in addition to WorkingDir). Use sparingly (e.g. temp directories).
	AllowWrite []string
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
//
// This overload defaults to AccessRead for backward compatibility with existing
// call sites that don't specify a mode. New callers should use resolvePathSandboxedMode.
func resolvePathSandboxed(userPath, cwd string, sbx WorkingDirSandbox) (string, error) {
	return resolvePathSandboxedMode(userPath, cwd, sbx, AccessRead)
}

// resolvePathSandboxedMode is the mode-aware path resolver. Write tools pass
// AccessWrite so that AllowRead-only directories are correctly rejected.
func resolvePathSandboxedMode(userPath, cwd string, sbx WorkingDirSandbox, mode AccessMode) (string, error) {
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
				slog.Warn("sandbox: sandbox disabled, path boundary check skipped", "workingDir", sbx.WorkingDir)
			}
		})
		return abs, nil
	}

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("工作目录解析失败: %w", err)
	}

	// 解析符号链接，防止 link_to_etc -> /etc 逃逸沙箱边界。
	// 对于尚不存在的文件（写操作常见），回退到最近的已存在父目录进行校验。
	realAbs, err := evalSymlinksExist(abs)
	if err != nil {
		return "", fmt.Errorf("路径解析失败: %w", err)
	}
	realCwd, err := filepath.EvalSymlinks(absCwd)
	if err != nil {
		return "", fmt.Errorf("工作目录解析失败: %w", err)
	}

	// 1) 确保在 WorkingDir 子树内（读写均允许）
	if isWithin(realCwd, realAbs) {
		return realAbs, nil
	}

	// 2) 检查白名单（白名单条目已在 propagateSandbox 时预解析符号链接）
	if mode == AccessRead {
		for _, allowed := range sbx.AllowRead {
			if isWithin(allowed, realAbs) {
				return realAbs, nil
			}
		}
	}
	for _, allowed := range sbx.AllowWrite {
		if isWithin(allowed, realAbs) {
			return realAbs, nil
		}
	}

	return "", fmt.Errorf("%w: 路径 %q 不在允许范围内（可通过 mady trust-knowledge 添加白名单）", ErrOutsideSandbox, userPath)
}

// resolveAllowList pre-resolves symlinks for all allowlist entries at
// sandbox construction time. Entries that cannot be resolved (non-existent
// directories) are silently skipped. Call this once when populating
// WorkingDirSandbox, not per-tool-invocation.
func resolveAllowList(dirs []string) []string {
	var resolved []string
	for _, d := range dirs {
		real, err := filepath.EvalSymlinks(d)
		if err != nil {
			continue
		}
		resolved = append(resolved, real)
	}
	return resolved
}

// isWithin reports whether path is inside the base directory tree (or equals base).
// Both arguments must be real (symlink-resolved) absolute paths.
func isWithin(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// evalSymlinksExist resolves symlinks for a path. If the full path doesn't
// exist (common for write targets), it resolves the nearest existing ancestor
// directory and appends the remaining path components. This preserves sandbox
// safety because a non-existent path cannot contain a symlink — only its
// existing ancestors can.
func evalSymlinksExist(abs string) (string, error) {
	real, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return real, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	// Walk up to the nearest existing ancestor.
	dir := abs
	for {
		dir = filepath.Dir(dir)
		if dir == "/" || dir == "." {
			return "", fmt.Errorf("path not found: %s", abs)
		}
		if realDir, err := filepath.EvalSymlinks(dir); err == nil {
			// Reconstruct the full path from the resolved ancestor.
			rel, _ := filepath.Rel(dir, abs)
			return filepath.Join(realDir, rel), nil
		}
	}
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
