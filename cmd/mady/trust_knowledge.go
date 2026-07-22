package main

// runTrustKnowledge handles the "mady trust-knowledge" subcommand.
// It adds a directory to the sandbox AllowRead whitelist persisted in
// ~/.mady/config.yaml, so file tools (read, grep, glob, etc.) can access
// knowledge base files outside the WorkingDir.
//
// Usage:
//
//	mady trust-knowledge <path>   Add <path> to the read-only whitelist
//	mady trust-knowledge --list   Show current whitelist
//	mady trust-knowledge --remove <path>  Remove <path> from whitelist

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/pkg/util"
)

func runTrustKnowledge(args []string) {
	if len(args) == 0 {
		printTrustKnowledgeUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "-h", "--help":
		printTrustKnowledgeUsage()
		return
	case "--list", "-l":
		listTrustKnowledge()
	case "--remove", "-r":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "error: --remove requires a path argument")
			os.Exit(1)
		}
		removeTrustKnowledge(args[1])
	default:
		addTrustKnowledge(args[0])
	}
}

func addTrustKnowledge(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot resolve path %q: %v\n", path, err)
		os.Exit(1)
	}

	// Verify directory exists
	if info, err := os.Stat(abs); err != nil {
		fmt.Fprintf(os.Stderr, "error: path not found: %s\n", abs)
		os.Exit(1)
	} else if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "error: path is not a directory: %s\n", abs)
		os.Exit(1)
	}

	if err := util.AddKnowledgeDir(abs); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to save config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 已添加知识库路径到只读白名单: %s\n", abs)
	fmt.Println("  重启 mady 后生效，或通过 KNOWLEDGE_DIRS 环境变量即时覆盖。")
}

func listTrustKnowledge() {
	cfg, err := util.LoadSandboxConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("沙箱只读白名单 (AllowRead):")
	fmt.Println()

	// 1. config.yaml 中的
	if len(cfg.AllowRead) > 0 {
		fmt.Println("  [config.yaml]")
		for _, dir := range cfg.AllowRead {
			fmt.Printf("    %s\n", dir)
		}
	}

	// 2. 环境变量
	envDirs := util.LoadKnowledgeDirsFromEnv()
	if len(envDirs) > 0 {
		fmt.Println("  [KNOWLEDGE_DIRS 环境变量]")
		for _, dir := range envDirs {
			fmt.Printf("    %s\n", dir)
		}
	}

	// 3. 自动白名单
	if home, err := util.MadyHome(); err == nil {
		docTmpl := filepath.Join(home, "doc-templates")
		if info, err := os.Stat(docTmpl); err == nil && info.IsDir() {
			fmt.Println("  [自动]")
			fmt.Printf("    %s\n", docTmpl)
		}
	}

	if len(cfg.AllowRead) == 0 && len(envDirs) == 0 {
		fmt.Println("  （空）使用 mady trust-knowledge <path> 添加")
	}

	fmt.Println()
	fmt.Println("沙箱读写白名单 (AllowWrite):")
	if len(cfg.AllowWrite) > 0 {
		for _, dir := range cfg.AllowWrite {
			fmt.Printf("  %s\n", dir)
		}
	} else {
		fmt.Printf("  %s （自动）\n", filepath.Join(os.TempDir(), "mady"))
	}
}

func removeTrustKnowledge(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot resolve path %q: %v\n", path, err)
		os.Exit(1)
	}

	cfg, err := util.LoadSandboxConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot load config: %v\n", err)
		os.Exit(1)
	}

	found := false
	var filtered []string
	for _, dir := range cfg.AllowRead {
		if dir == abs {
			found = true
		} else {
			filtered = append(filtered, dir)
		}
	}

	if !found {
		fmt.Fprintf(os.Stderr, "路径不在白名单中: %s\n", abs)
		os.Exit(1)
	}

	cfg.AllowRead = filtered
	if err := util.SaveSandboxConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to save config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 已从白名单移除: %s\n", abs)
}

func printTrustKnowledgeUsage() {
	fmt.Fprintln(os.Stderr, `mady trust-knowledge — 管理沙箱只读白名单

Usage:
  mady trust-knowledge <path>          添加目录到只读白名单
  mady trust-knowledge --list, -l      列出当前白名单
  mady trust-knowledge --remove <path> 从白名单移除
  mady trust-knowledge --help, -h      显示帮助

白名单目录中的文件可被 read/grep/glob/find/ls/view 工具访问，
但不允许 write/edit/delete/move 操作。

也可通过环境变量 KNOWLEDGE_DIRS 临时配置（冒号分隔）：
  KNOWLEDGE_DIRS=/path/a:/path/b mady tui`)
}
