// Command mady is the unified entry point for the Mady agent framework.
//
// It exposes several subcommands:
//
//	mady tui   — interactive terminal chat (default)
//	mady serve — HTTP/SSE API server with multi-domain routing
//	mady acp   — run as an ACP (Agent Client Protocol) server for editors like Zed
//	mady trust-mcp — trust an MCP config file so its commands may run at startup
//	mady trust-knowledge — manage sandbox read-only whitelist for knowledge bases
//	mady mcp-install — wire Mady as an MCP server into coding agents (e.g. claude)
//	mady eval  — run evaluation benchmarks (static or live) and generate reports
//	mady evidence — evidence judgment CLI
//	mady patent — patent analysis CLI (novelty analysis, OA response drafting)
//	mady util  — utility tools (list-prompts, etc.)
//	mady help  — show usage help
//
// All configuration is via environment variables (see package agentconfig):
//
//	PROVIDER   deepseek | zhipu | kimi | generic   (default: deepseek)
//	API_KEY    your LLM API key (required)
//	BASE_URL   override the provider's default endpoint
package main

// 本文件只保留主入口 main 与用法说明 printUsage。共享装配与子命令实现
// 分布在同包兄弟文件和内部包中：
//   - framework.go — frameworkContext + setupFrameworkContext 等共享装配
//   - knowledge.go — 知识库（SQLite/wiki/embedder/reranker）装配
//   - tui.go + tui_session.go + tui_session_config.go + tui_session_agent.go
//     + tui_helpers.go + tui_storage.go + tui_deferred.go + slash_registry.go — `mady tui`
//   - server.go    — `mady serve`
//   - acp.go       — `mady acp`
//   - subcmd/      — 其余子命令实现（evidence/eval/patent/mcp-install/trust-mcp/trust-knowledge/ocr/util）

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	_ "github.com/joho/godotenv/autoload" // 自动加载 .env 文件（如有）

	"github.com/xujian519/mady/cmd/mady/subcmd"
	"github.com/xujian519/mady/pkg/omlx"
	_ "github.com/xujian519/mady/provider/adapter"
)

// commitHash and buildTime are injected via -ldflags at build time
// (see LDFLAGS in Makefile). Default to "unknown" for dev builds.
var commitHash = "unknown" // set via ldflags at build time
var buildTime = "unknown"  // set via ldflags at build time
var _, _ = commitHash, buildTime

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
		if err := runTui(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "mady:", err)
			os.Exit(1)
		}
	case "serve":
		if err := runServer(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "acp":
		if err := runAcp(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "mady:", err)
			os.Exit(1)
		}
	case "trust-mcp":
		if err := subcmd.RunTrustMCP(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "mady:", err)
			os.Exit(1)
		}
	case "trust-knowledge":
		if err := subcmd.RunTrustKnowledge(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "mady:", err)
			os.Exit(1)
		}
	case "mcp-install":
		if err := subcmd.RunMCPInstall(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "mady:", err)
			os.Exit(1)
		}
	case "eval":
		if err := subcmd.RunEval(ctx, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "mady:", err)
			os.Exit(1)
		}
	case "evidence":
		if err := subcmd.RunEvidenceCLI(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "mady:", err)
			os.Exit(1)
		}
	case "patent":
		if err := subcmd.RunPatentCLI(ctx, os.Args); err != nil {
			fmt.Fprintln(os.Stderr, "mady:", err)
			os.Exit(1)
		}
	case "util":
		if err := subcmd.RunUtil(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "mady:", err)
			os.Exit(1)
		}
	case "ocr":
		if err := subcmd.RunOCRCLI(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "mady:", err)
			os.Exit(1)
		}
	case "start-embeddings", "stop-embeddings", "status-embeddings":
		runEmbeddingsCLI(ctx, os.Args[1])
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
  evidence  Evidence judgment CLI: triple, burden, standard, type-specific
        analysis, conflict detection, type-specific evidence judgment.
  eval  Run evaluation benchmarks (static or live) and generate reports.
  patent  Patent analysis CLI: novelty, OA response, invalidation,
        infringement, reexamination.
  util    Utility commands (list-prompts, etc.).
  ocr     Local OCR CLI: recognize (rec), ensure, status.
  start-embeddings  Start the oMLX embedding/reranking server.
  stop-embeddings   Stop the oMLX embedding/reranking server.
  status-embeddings Check the oMLX embedding/reranking server status.
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

// runEmbeddingsCLI handles start-embeddings / stop-embeddings / status-embeddings.
func runEmbeddingsCLI(ctx context.Context, cmd string) {
	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "mady: OMLX_API_KEY 未设置，嵌入服务不可用")
		fmt.Fprintln(os.Stderr, "  提示: 在 .env 文件中设置 OMLX_API_KEY=your_key")
		os.Exit(1)
	}

	mgr := omlx.NewManager(8000, apiKey)

	switch cmd {
	case "start-embeddings":
		fmt.Fprintln(os.Stderr, "正在启动 oMLX 嵌入服务...")
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "mady: 启动失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "oMLX 嵌入服务已启动（http://127.0.0.1:8000）")
		// 阻塞等待退出信号（前台模式运行）。
		<-ctx.Done()
		if err := mgr.Stop(); err != nil {
			slog.Warn("mady: embed-server stop error", "err", err)
		}
		fmt.Fprintln(os.Stderr, "\noMLX 嵌入服务已停止")

	case "stop-embeddings":
		if mgr.IsRunning() {
			if err := mgr.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "mady: 停止失败: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stderr, "oMLX 嵌入服务已停止")
		} else {
			// Try omlx stop command directly for background-managed instances.
			stopCmd := exec.CommandContext(ctx, "omlx", "stop")
			if output, err := stopCmd.CombinedOutput(); err != nil {
				fmt.Fprintln(os.Stderr, "oMLX 嵌入服务未在运行")
			} else {
				fmt.Fprint(os.Stderr, string(output))
			}
		}

	case "status-embeddings":
		if mgr.IsRunning() {
			fmt.Fprintln(os.Stderr, "oMLX 嵌入服务: 运行中 (http://127.0.0.1:8000)")
		} else {
			if _, err := exec.LookPath("omlx"); err != nil {
				fmt.Fprintln(os.Stderr, "oMLX 嵌入服务: 未安装 (brew install omlx)")
			} else {
				fmt.Fprintln(os.Stderr, "oMLX 嵌入服务: 未运行 (mady start-embeddings)")
			}
		}
	}
}
