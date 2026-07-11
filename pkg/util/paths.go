package util

import (
	"fmt"
	"os"
	"path/filepath"
)

// appDirName 是 Mady 在用户家目录下的应用数据目录名。
// 全仓统一约定为 ".mady"，与 acp/server_app.go、psychological/store.go 一致。
const appDirName = ".mady"

// MadyHome 返回 Mady 应用数据根目录（绝对路径），并确保目录存在。
//
// 解析优先级：
//  1. $MADY_HOME 环境变量（若设置为绝对路径）
//  2. $HOME/.mady（用户家目录下的隐藏目录，跨平台约定）
//  3. ./.mady（最终回退，当 UserHomeDir 不可用时使用，cwd 相对）
//
// 返回的路径始终经过 filepath.Abs 规范化。调用方可放心在其下拼接子目录
// （如 manifests、workspace、sessions），无需关心路径分隔符或存在性。
func MadyHome() (string, error) {
	// 1. 显式环境变量优先（空字符串视为未设置）
	if env := os.Getenv("MADY_HOME"); env != "" {
		abs, err := filepath.Abs(filepath.Clean(env))
		if err != nil {
			return "", fmt.Errorf("resolve MADY_HOME=%q: %w", env, err)
		}
		if err := EnsureDir(abs); err != nil {
			return "", err
		}
		return abs, nil
	}

	// 2. 家目录下的 .mady
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		dir := filepath.Join(home, appDirName)
		if err := EnsureDir(dir); err != nil {
			return "", err
		}
		return dir, nil
	}

	// 3. 最终回退：cwd 下的 .mady（UserHomeDir 不可用时，极少见）
	fallback, err := filepath.Abs(appDirName)
	if err != nil {
		return "", fmt.Errorf("resolve fallback mady home: %w", err)
	}
	if err := EnsureDir(fallback); err != nil {
		return "", err
	}
	return fallback, nil
}

// EnsureDir 确保 dir 存在（含全部父目录），权限 0755。
// 若 dir 已存在则为 no-op。空字符串直接返回 nil。
func EnsureDir(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure dir %q: %w", dir, err)
	}
	return nil
}

// ResolveDataDir 返回 MadyHome 下的子目录（绝对路径），并确保目录存在。
//
// 例如：ResolveDataDir("workspace") → ~/.mady/workspace（已创建）
//
// subdir 为空时返回 MadyHome 本身。
func ResolveDataDir(subdir string) (string, error) {
	home, err := MadyHome()
	if err != nil {
		return "", err
	}
	if subdir == "" {
		return home, nil
	}
	dir := filepath.Join(home, subdir)
	if err := EnsureDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}
