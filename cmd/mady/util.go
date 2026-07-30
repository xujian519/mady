package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/prompt"
)

// runUtil dispatches utility subcommands that do not need a full framework
// context. Currently supports:
//   - list-prompts: print the catalog of built-in + user prompt templates.
func runUtil(args []string) error {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: mady util <subcommand>\n\nSubcommands:\n  list-prompts  列出可用的提示词模板")
		return fmt.Errorf("util: missing subcommand")
	}

	switch args[0] {
	case "list-prompts":
		return runListPrompts()
	default:
		fmt.Fprintln(os.Stderr, "usage: mady util <subcommand>\n\nSubcommands:\n  list-prompts  列出可用的提示词模板")

		return fmt.Errorf("util: unknown subcommand %q", args[0])
	}
}

// runListPrompts loads the prompt template store and prints a human-readable
// index. It respects $MADY_HOME/prompt-templates/ overrides.
func runListPrompts() error {
	madyHome, err := util.MadyHome()
	if err != nil {
		return fmt.Errorf("无法解析 MadyHome: %w", err)
	}

	userDir := filepath.Join(madyHome, "prompt-templates")
	store, err := prompt.NewPromptStore(userDir)
	if err != nil {
		return fmt.Errorf("加载提示词模板失败: %w", err)
	}

	idx := store.Index()
	if idx == "" {
		fmt.Println("没有可用的提示词模板。")
		return nil
	}
	fmt.Println(idx)
	fmt.Printf("总计: %d 个模板\n", store.Count())
	return nil
}
