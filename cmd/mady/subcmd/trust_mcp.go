package subcmd

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

// RunTrustMCP handles the "mady trust-mcp" subcommand.
func RunTrustMCP(cmdArgs []string) error {
	// 默认信任当前目录的 .mcp.json；也可显式指定配置文件路径。
	path := ".mcp.json"
	if len(cmdArgs) > 0 {
		path = cmdArgs[0]
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("trust-mcp: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("trust-mcp: %w", err)
	}

	madyHome, err := util.MadyHome()
	if err != nil {
		return fmt.Errorf("trust-mcp: %w", err)
	}
	if err := mcp.TrustMCPConfigFile(abs, madyHome); err != nil {
		return fmt.Errorf("trust-mcp: %w", err)
	}
	fmt.Printf("已信任 MCP 配置：%s\n（内容变化后需重新执行本命令）\n", abs)
	return nil
}
