// Command mady is the unified entry point for the Mady agent framework.
//
// It exposes eight subcommands:
//
//	mady tui   — interactive terminal chat (default)
//	mady serve — HTTP/SSE API server with multi-domain routing
//	mady acp   — run as an ACP (Agent Client Protocol) server for editors like Zed
//	mady trust-mcp — trust an MCP config file so its commands may run at startup
//	mady trust-knowledge — manage sandbox read-only whitelist for knowledge bases
//	mady mcp-install — wire Mady as an MCP server into coding agents (e.g. claude)
//	mady eval  — run evaluation benchmarks (static or live) and generate reports
//	mady patent — patent analysis CLI (novelty analysis, OA response drafting)
//	mady help  — show usage help
//
// All configuration is via environment variables (see package agentconfig):
//
//	PROVIDER   deepseek | zhipu | kimi | generic   (default: deepseek)
//	API_KEY    your LLM API key (required)
//	BASE_URL   override the provider's default endpoint
package main

// 本文件只保留主入口 main 与用法说明 printUsage。共享装配与子命令实现
// 分布在同包兄弟文件中：
//   - framework.go — frameworkContext + setupFrameworkContext 等共享装配
//   - knowledge.go — 知识库（SQLite/wiki/embedder/reranker）装配
//   - tui.go + tui_session.go + tui_session_config.go + tui_session_agent.go
//     + tui_helpers.go + tui_storage.go + tui_deferred.go + slash_registry.go — `mady tui`
//   - server.go    — `mady serve`
//   - acp.go       — `mady acp`
//   - trust_mcp.go — `mady trust-mcp`
//   - mcp_install.go — `mady mcp-install`
//   - patent.go    — `mady patent`

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/joho/godotenv/autoload" // 自动加载 .env 文件（如有）
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(os.Args) < 2 {
		printUsage()
		stop()
		os.Exit(0) //nolint:gocritic // exitAfterDefer: stop() manually called above; defer is a panic safety-net
	}

	switch os.Args[1] {
	case "tui":
		runTui(ctx)
	case "serve":
		runServer(ctx)
	case "acp":
		runAcp(ctx)
	case "trust-mcp":
		runTrustMCP(os.Args)
	case "trust-knowledge":
		runTrustKnowledge(os.Args[2:])
	case "mcp-install":
		if err := runMCPInstall(ctx, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "mcp-install:", err)
			os.Exit(1)
		}
	case "eval":
		if err := runEval(ctx, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "eval:", err)
			os.Exit(1)
		}
	case "patent":
		runPatentCLI(ctx, os.Args)
	case "util":
		runUtil(ctx, os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printUsage()
		stop()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `mady — Mady agent framework

Usage:
  mady <command> [flags]

Commands:
  tui   Launch the interactive terminal chat (default).
  serve Run as an HTTP/SSE API server with multi-domain routing.
  acp   Run as an ACP server (stdio JSON-RPC) for editors like Zed.
  mcp-install [--list|<agent>]  Wire Mady as an MCP server into coding agents
        (e.g. mady mcp-install claude). Use --list to see detected agents.
  trust-mcp [path]  Trust an MCP config file (default: ./.mcp.json) so its
        commands may run at startup (records a SHA-256 in trusted-mcp.json).
  trust-knowledge <path>  Add a directory to the sandbox read-only whitelist
        so file tools can access knowledge bases outside WorkingDir.
        Use --list to show, --remove <path> to delete.
  eval  Run evaluation benchmarks (static or live) and generate reports.
  patent  Patent analysis CLI: novelty analysis, OA response drafting.
  util    Utility commands (list-prompts, etc.).
  help  Show this help message.

Configuration (environment variables):
  PROVIDER   deepseek | zhipu | kimi | generic   (default: deepseek)
  API_KEY    LLM API key (required)
  BASE_URL   override provider endpoint

Examples:
  PROVIDER=deepseek API_KEY=sk-... mady tui
  PROVIDER=zhipu API_KEY=... mady acp
  mady eval --suite p2a --mode static
  mady eval --case patent_exam_2009_a22_01 --format json
  mady eval --format enhanced --baseline baseline.json --suite p2a --mode static`)
}
