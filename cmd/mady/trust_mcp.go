package main

// 本文件实现 `mady trust-mcp` 子命令：将 MCP 配置文件（默认 $PWD/.mcp.json）
// 的当前内容哈希写入信任存储（$MADY_HOME/trusted-mcp.json）。
// 被信任的 $PWD/.mcp.json 中的 stdio command 才允许在启动时执行（C7 修复）。

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/mcp"
	"github.com/xujian519/mady/pkg/util"
)

func runTrustMCP(_ []string) {
	// 默认信任当前目录的 .mcp.json；也可显式指定配置文件路径。
	path := ".mcp.json"
	if len(os.Args) > 2 {
		path = os.Args[2]
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trust-mcp: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stat(abs); err != nil {
		fmt.Fprintf(os.Stderr, "trust-mcp: %v\n", err)
		os.Exit(1)
	}

	madyHome, err := util.MadyHome()
	if err != nil {
		fmt.Fprintf(os.Stderr, "trust-mcp: %v\n", err)
		os.Exit(1)
	}
	if err := mcp.TrustMCPConfigFile(abs, madyHome); err != nil {
		fmt.Fprintf(os.Stderr, "trust-mcp: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已信任 MCP 配置：%s\n（内容变化后需重新执行本命令）\n", abs)
}
