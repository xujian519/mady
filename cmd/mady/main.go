// Command mady is the unified entry point for the Mady agent framework.
//
// It exposes three subcommands:
//
//	mady tui   — interactive terminal chat (default)
//	mady serve — HTTP/SSE API server with multi-domain routing
//	mady acp   — run as an ACP (Agent Client Protocol) server for editors like Zed
//
// All configuration is via environment variables (see package agentconfig):
//
//	PROVIDER   deepseek | zhipu | kimi | generic   (default: deepseek)
//	API_KEY    your LLM API key (required)
//	BASE_URL   override the provider's default endpoint
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xujian519/mady/acp"
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/loader"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/server"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/tui"
	"github.com/xujian519/mady/tui/agentadapter"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

const defaultSystemPrompt = "你是 Mady 智能助手，一个能力完备的通用 AI 代理。" +
	"你可以调用工具、检索知识、多步推理。请用简洁清晰的中文回答用户。"

// defaultManifestDir is the default directory for Agent Manifest files.
const defaultManifestDir = "./manifests"

// defaultWorkspaceDir is the default workspace directory.
const defaultWorkspaceDir = "./workspace"

// loadWikiStore initializes the knowledge store from a wiki directory.
// It returns the store and a retrieval hook, or nil if WIKI_PATH is not set.
func loadWikiStore() (*knowledge.Store, agentcore.LifecycleHook) {
	wikiPath := os.Getenv("WIKI_PATH")
	if wikiPath == "" {
		return nil, nil
	}
	store := knowledge.NewStore()
	wikiLoader := loader.NewWikiLoader(store, wikiPath)
	stats, err := wikiLoader.ImportWiki()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wiki: import failed: %v\n", err)
		return nil, nil
	}
	fmt.Fprintf(os.Stderr, "wiki: imported %d docs, %d chunks\n",
		stats.Imported, store.Stats().TotalChunks)
	hook := store.RetrievalHook("patent", retrieval.RetrievalConfig{
		TopK:     5,
		MaxChars: 4000,
		Prefix:   "以下是知识库中检索到的相关专利法律信息，请参考使用：\n",
	})
	return store, hook
}

// frameworkContext 封装入口之间共享的初始化资源。
type frameworkContext struct {
	BaseConfig      agentcore.Config
	ProjectRegistry *domains.ProjectRegistry
	WikiHook        agentcore.LifecycleHook
	WikiStore       *knowledge.Store
	Manifests       []agentcore.AgentManifest
	ManifestErrs    []agentcore.ManifestLoadError
}

// setupFrameworkContext 执行三个入口共享的初始化逻辑：
//   - Provider 构建
//   - Manifest 扫描（可选的 MANIFEST_DIR 环境变量）
//   - Wiki 知识库加载（可选的 WIKI_PATH 环境变量）
//   - ProjectRegistry 初始化
func setupFrameworkContext() *frameworkContext {
	fc := &frameworkContext{}

	provider := agentconfig.BuildProvider()
	model := agentconfig.DefaultModel()

	fc.BaseConfig = agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:      "mady-router",
			Model:     model,
			Provider:  provider,
			Streaming: true,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          25,
			ExecutionMode:     agentcore.ModeSerial,
			ValidateArguments: true,
		},
		CompactionConfig: agentcore.CompactionConfig{
			ContextWindow:    128000,
			ReserveTokens:    32000,
			KeepRecentTokens: 4000,
		},
		RetryConfig: &agentcore.RetryConfig{
			MaxRetries:  3,
			BaseDelayMs: 1000,
			MaxDelayMs:  15000,
		},
	}

	// Wiki 知识库
	fc.WikiStore, fc.WikiHook = loadWikiStore()

	// Manifest 加载
	manifestDir := os.Getenv("MANIFEST_DIR")
	if manifestDir == "" {
		manifestDir = defaultManifestDir
	}
	manifests, errs, err := agentcore.ScanManifests(manifestDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "manifest: %v（使用默认无 Manifest 模式）\n", err)
	} else {
		fc.Manifests = manifests
		fc.ManifestErrs = errs
		if len(manifests) > 0 {
			fmt.Fprintf(os.Stderr, "manifest: 已加载 %d 个 Agent\n", len(manifests))
			for _, m := range manifests {
				fmt.Fprintf(os.Stderr, "  - %s (%s)\n", m.Name, m.Domain)
			}
		}
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "manifest: [警告] %s: %s\n", e.Path, e.Error)
			}
		}
	}

	// ProjectRegistry
	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir == "" {
		workspaceDir = defaultWorkspaceDir
	}
	projectDir := workspaceDir + "/projects"
	fc.ProjectRegistry = domains.NewProjectRegistryOrEmpty(projectDir)

	return fc
}

// buildRouterConfig 根据可用的 Manifest 构建 Router Agent 配置。
// 有 Manifest 时使用声明式注册，没有时回退到硬编码 RouterConfig。
func buildRouterConfig(base agentcore.Config, manifests []agentcore.AgentManifest) agentcore.Config {
	if len(manifests) > 0 {
		return domains.RouterConfigFromManifests(base, manifests)
	}
	return domains.RouterConfig(base)
}

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
  help  Show this help message.

Configuration (environment variables):
  PROVIDER   deepseek | zhipu | kimi | generic   (default: deepseek)
  API_KEY    LLM API key (required)
  BASE_URL   override provider endpoint

Examples:
  PROVIDER=deepseek API_KEY=sk-... mady tui
  PROVIDER=zhipu API_KEY=... mady acp`)
}

// runTui launches the interactive terminal chat.
// 默认使用 Router 多域路由（当 MANIFEST_DIR 有 Manifest 时）。
// 设置 MADY_SINGLE_AGENT=1 强制使用传统单 Agent 模式。
func runTui(ctx context.Context) {
	fs := flag.NewFlagSet("mady tui", flag.ExitOnError)
	_ = fs.Parse(os.Args[2:])

	fc := setupFrameworkContext()

	provider := agentconfig.BuildProvider()
	model := agentconfig.DefaultModel()
	currentThinking := agentconfig.ThinkingFromEnv()

	// 多域模式单 Agent 模式切换。
	// 当有 Manifest 且未强制设置 MADY_SINGLE_AGENT=1 时使用多域 Router 模式。
	useMultiDomain := len(fc.Manifests) > 0 && os.Getenv("MADY_SINGLE_AGENT") != "1"

	// runMu 防止快速输入时多个 goroutine 并发调用 Agent.Run。
	var runMu sync.Mutex

	buildCfg := func() agentcore.Config {
		if useMultiDomain {
			cfg := buildRouterConfig(fc.BaseConfig, fc.Manifests)
			// 将 /thinking 命令修改的推理配置注入多域模式
			cfg.Thinking = cloneThinkingConfig(currentThinking)
			if fc.WikiHook != nil {
				cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, fc.WikiHook)
			}
			return cfg
		}

		return agentcore.Config{
			ModelConfig: agentcore.ModelConfig{
				Name:      "mady",
				Model:     model,
				Provider:  provider,
				Thinking:  cloneThinkingConfig(currentThinking),
				Streaming: true,
			},
			SystemPrompt: defaultSystemPrompt,
			ExecutionConfig: agentcore.ExecutionConfig{
				MaxTurns:          25,
				ExecutionMode:     agentcore.ModeSerial,
				ValidateArguments: true,
			},
			CompactionConfig: agentcore.CompactionConfig{
				ContextWindow:    128000,
				ReserveTokens:    32000,
				KeepRecentTokens: 4000,
			},
			RetryConfig: &agentcore.RetryConfig{
				MaxRetries:  3,
				BaseDelayMs: 1000,
				MaxDelayMs:  15000,
			},
			Lifecycle: fc.WikiHook,
		}
	}

	currentAgent := agentcore.New(buildCfg())
	defer currentAgent.Close()

	currentThemeName := "dark"

	slashSuggestions := []core.Suggestion{
		{InsertText: "/help", Label: "/help", Description: "显示快捷键"},
		{InsertText: "/clear", Label: "/clear", Description: "开始新对话"},
		{InsertText: "/new", Label: "/new", Description: "开始新对话"},
		{InsertText: "/branch", Label: "/branch", Description: "分支当前线程（需 session 存储）"},
		{InsertText: "/thinking", Label: "/thinking", Description: "查看或修改推理模式"},
		{InsertText: "/thinking summarized", Label: "/thinking summarized", Description: "显示推理摘要"},
		{InsertText: "/thinking omitted", Label: "/thinking omitted", Description: "隐藏推理块"},
		{InsertText: "/thinking effort medium", Label: "/thinking effort medium", Description: "设置推理强度"},
		{InsertText: "/thinking budget -1", Label: "/thinking budget -1", Description: "动态推理预算"},
		{InsertText: "/skill:", Label: "/skill:", Description: "显式调用技能"},
		{InsertText: "/save", Label: "/save", Description: "保存当前会话"},
		{InsertText: "/theme", Label: "/theme", Description: "切换主题"},
		{InsertText: "/theme dark", Label: "/theme dark", Description: "深色主题"},
		{InsertText: "/theme light", Label: "/theme light", Description: "浅色主题"},
		{InsertText: "/quit", Label: "/quit", Description: "退出"},
	}

	if useMultiDomain {
		slashSuggestions = append(slashSuggestions,
			core.Suggestion{InsertText: "/mode", Label: "/mode", Description: "显示当前 Agent 模式"},
		)
	}

	var app *chat.ChatApp
	app = tui.NewChatApp(chat.ChatAppConfig{
		Title:     fmt.Sprintf("mady · model=%s", model),
		ShowTurns: true,
		AltScreen: true,
		MouseMode: "auto",
		Providers: []core.AutocompleteProvider{
			&component.StaticProvider{
				TriggerStr:  "/",
				Suggestions: slashSuggestions,
			},
		},
		OnSubmit: func(ctx context.Context, input string) {
			trimmed := strings.TrimSpace(input)
			if trimmed == "" {
				return
			}

			// /thinking and its subcommands.
			if strings.HasPrefix(trimmed, "/thinking") {
				next, changed, err := parseThinkingCommand(trimmed, currentThinking)
				if err != nil {
					app.PrintError(err)
					return
				}
				if !changed {
					app.PrintSystem("推理配置: " + formatThinkingConfig(currentThinking))
					return
				}
				currentThinking = next
				prev := currentAgent
				currentAgent = agentcore.New(buildCfg())
				prev.Close()
				agentadapter.BindAgent(app, currentAgent)
				app.PrintSystem("推理配置已更新: " + formatThinkingConfig(currentThinking))
				return
			}

			// /theme and its subcommands.
			if strings.HasPrefix(trimmed, "/theme") {
				switch trimmed {
				case "/theme":
					app.PrintSystem("当前主题: " + currentThemeName)
					return
				case "/theme dark":
					theme.SetSemanticTheme(theme.DefaultSemanticDark(), theme.DetectColorMode())
					app.History().SetTheme(chat.DefaultChatHistoryTheme())
					currentThemeName = "dark"
					app.PrintSystem("已切换深色主题")
					return
				case "/theme light":
					theme.SetSemanticTheme(theme.DefaultSemanticLight(), theme.DetectColorMode())
					app.History().SetTheme(chat.DefaultChatHistoryTheme())
					currentThemeName = "light"
					app.PrintSystem("已切换浅色主题")
					return
				}
			}

			// /mode command in multi-domain mode.
			if trimmed == "/mode" && useMultiDomain {
				agentName := currentAgent.Config().Name
				app.PrintSystem(fmt.Sprintf("当前 Agent: %s（多域路由模式）", agentName))
				return
			}

			switch trimmed {
			case "/help":
				app.ToggleKeyHelp()
				return
			case "/clear", "/new":
				prev := currentAgent
				currentAgent = agentcore.New(buildCfg())
				prev.Close()
				agentadapter.BindAgent(app, currentAgent)
				app.History().Clear()
				app.PrintSystem("已开始新对话")
				return
			case "/branch":
				app.PrintSystem("mady tui 简化版不支持分支，请使用 example/cli-chat 配合 SESSION_DIR")
				return
			case "/save":
				app.PrintSystem("mady tui 为内存模式，会话不持久化")
				return
			case "/skill:":
				app.PrintSystem("mady tui 简化版未加载技能，请使用 example/cli-chat 配合 SKILL_DIRS")
				return
			case "/quit", "exit":
				_ = app.Stop()
				return
			}

			if strings.HasPrefix(trimmed, "/skill:") {
				app.PrintSystem("mady tui 简化版未加载技能，请使用 example/cli-chat 配合 SKILL_DIRS")
				return
			}

			if strings.HasPrefix(trimmed, "/") {
				app.PrintSystem(fmt.Sprintf("未知命令: %s（输入 / 查看可用命令）", trimmed))
				return
			}

			go func() {
				runMu.Lock()
				defer runMu.Unlock()
				_, _ = currentAgent.Run(ctx, trimmed)
			}()
		},
	})
	agentadapter.BindAgent(app, currentAgent)

	modeInfo := "单 Agent 模式"
	if useMultiDomain {
		modeInfo = "多域路由模式"
	}
	app.PrintSystem(fmt.Sprintf("Mady 中观智能体已启动（%s）。输入消息开始对话，输入 / 查看命令。Ctrl+C 退出。", modeInfo))
	if fc.WikiStore != nil {
		st := fc.WikiStore.Stats()
		app.PrintSystem(fmt.Sprintf("wiki 知识库: %d 文档, %d 分块 (RAG: patent)", st.TotalDocs, st.TotalChunks))
	}
	if err := app.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
	}
	<-app.Done()
}

// runAcp launches the ACP server over stdio.
func runAcp(ctx context.Context) {
	fs := flag.NewFlagSet("mady acp", flag.ExitOnError)
	_ = fs.Parse(os.Args[2:])

	provider := agentconfig.BuildProvider()
	err := acp.RunServer(ctx, acp.RunOptions{
		Provider: provider,
		Model:    agentconfig.DefaultModel(),
		Thinking: agentconfig.ThinkingFromEnv(),
		AgentInfo: acp.AgentInfo{
			Name:    "mady",
			Version: "0.1.0",
		},
	})
	if err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "mady acp: %v\n", err)
		os.Exit(1)
	}
}

// runServer launches the HTTP/SSE API server with multi-domain routing.
func runServer(ctx context.Context) {
	fs := flag.NewFlagSet("mady serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatalf("flag: %v", err)
	}

	fc := setupFrameworkContext()

	// Build Router config from manifests (or use hardcoded fallback).
	cfg := buildRouterConfig(fc.BaseConfig, fc.Manifests)

	// Attach wiki retrieval hook if available.
	if fc.WikiHook != nil {
		cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, fc.WikiHook)
	}

	// Session persistence via JSONL file store.
	sessionDir := os.Getenv("SESSION_DIR")
	if sessionDir == "" {
		sessionDir = "./sessions"
	}
	fileStore, err := session.NewFileStore(sessionDir)
	if err != nil {
		log.Printf("session: %v (continuing without persistence)", err)
	} else {
		cfg.Store = session.NewAgentStore(fileStore, "./workspace")
	}

	// Checkpoint for durable snapshots per thread.
	cfg.Checkpoint = &agentcore.CheckpointSettings{
		Saver:    agentcore.NewMemoryCheckpointSaver(),
		ThreadID: "default",
	}

	srv := server.New(cfg)
	log.Printf("Mady server starting on %s (multi-domain routing enabled)", *addr)
	if fc.WikiStore != nil {
		st := fc.WikiStore.Stats()
		log.Printf("wiki: %d docs, %d chunks", st.TotalDocs, st.TotalChunks)
	}

	// Graceful shutdown on context cancellation.
	go func() {
		<-ctx.Done()
		log.Println("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	if err := srv.ListenAndServe(*addr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

// --- thinking config helpers (ported from example/cli-chat) ---

func cloneThinkingConfig(cfg *agentcore.ThinkingConfig) *agentcore.ThinkingConfig {
	if cfg == nil {
		return nil
	}
	cp := *cfg
	return &cp
}

func compactThinkingConfig(cfg *agentcore.ThinkingConfig) *agentcore.ThinkingConfig {
	if cfg == nil {
		return nil
	}
	if !cfg.IncludeThoughts &&
		cfg.Display == agentcore.ThinkingDisplayDefault &&
		cfg.Effort == agentcore.ThinkingEffortDefault &&
		cfg.Budget == 0 {
		return nil
	}
	return cfg
}

func formatThinkingConfig(cfg *agentcore.ThinkingConfig) string {
	if cfg == nil {
		return "default"
	}
	parts := []string{
		"display=" + string(cfg.NormalizedDisplay()),
	}
	if cfg.Effort != "" {
		parts = append(parts, "effort="+string(cfg.Effort))
	}
	if cfg.Budget != 0 {
		parts = append(parts, fmt.Sprintf("budget=%d", cfg.Budget))
	}
	parts = append(parts, fmt.Sprintf("include_thoughts=%t", cfg.IncludeThoughts))
	return strings.Join(parts, " ")
}

func parseThinkingCommand(input string, current *agentcore.ThinkingConfig) (*agentcore.ThinkingConfig, bool, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) <= 1 {
		return cloneThinkingConfig(current), false, nil
	}

	next := cloneThinkingConfig(current)
	if next == nil {
		next = &agentcore.ThinkingConfig{}
	}

	switch strings.ToLower(fields[1]) {
	case "reset":
		return nil, true, nil
	case "on", "summarized":
		next.IncludeThoughts = true
		next.Display = agentcore.ThinkingDisplaySummarized
		return compactThinkingConfig(next), true, nil
	case "off", "omitted":
		next.IncludeThoughts = false
		next.Display = agentcore.ThinkingDisplayOmitted
		return compactThinkingConfig(next), true, nil
	case "effort":
		if len(fields) < 3 {
			return nil, false, fmt.Errorf("usage: /thinking effort <low|medium|high|max|default>")
		}
		switch strings.ToLower(fields[2]) {
		case "default", "reset":
			next.Effort = agentcore.ThinkingEffortDefault
		case "low", "medium", "high", "max":
			next.Effort = agentcore.ThinkingEffort(strings.ToLower(fields[2]))
		default:
			return nil, false, fmt.Errorf("invalid thinking effort %q", fields[2])
		}
		return compactThinkingConfig(next), true, nil
	case "budget":
		if len(fields) < 3 {
			return nil, false, fmt.Errorf("usage: /thinking budget <n|default>")
		}
		if strings.EqualFold(fields[2], "default") || strings.EqualFold(fields[2], "reset") {
			next.Budget = 0
			return compactThinkingConfig(next), true, nil
		}
		v, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, false, fmt.Errorf("invalid thinking budget %q", fields[2])
		}
		next.Budget = v
		return compactThinkingConfig(next), true, nil
	case "include":
		if len(fields) < 3 {
			return nil, false, fmt.Errorf("usage: /thinking include <true|false>")
		}
		v, err := strconv.ParseBool(fields[2])
		if err != nil {
			return nil, false, fmt.Errorf("invalid thinking include value %q", fields[2])
		}
		next.IncludeThoughts = v
		if next.Display == agentcore.ThinkingDisplayDefault {
			if v {
				next.Display = agentcore.ThinkingDisplaySummarized
			} else {
				next.Display = agentcore.ThinkingDisplayOmitted
			}
		}
		return compactThinkingConfig(next), true, nil
	default:
		return nil, false, fmt.Errorf("usage: /thinking [on|off|summarized|omitted|effort <...>|budget <...>|include <true|false>|reset]")
	}
}
