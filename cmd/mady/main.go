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

// This file holds the main entry point, usage, and the shared framework
// initialization (setupFrameworkContext + knowledge/manifest/skill/MCP loaders)
// used by all three subcommands. Subcommand implementations live in siblings:
//   - tui_session.go + tui_helpers.go + slash_suggestions.go — `mady tui`
//   - server.go — `mady serve`
//   - acp.go    — `mady acp`

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/domains/rules"
	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/fileindex"
	kgwgraph "github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/knowledge/loader"
	"github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/mcp"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/skill"
	"github.com/xujian519/mady/tui"
	"github.com/xujian519/mady/tui/agentadapter"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// defaultSystemPrompt 仅在多域 manifest 全部加载失败时的最终兜底。
// 正常情况下 mady 通过 go:embed 内置的 4 个领域 manifest 进入多域路由模式，
// 不会用到这个提示词。
const defaultSystemPrompt = "你是 Mady 智能助手，一个能力完备的通用 AI 代理。" +
	"你可以调用工具、检索知识、多步推理。请用简洁清晰的中文回答用户。"

// loadWikiStore initializes the knowledge retrieval system.
// It tries the SQLite backend first (vector + FTS RRF fusion via
// KNOWLEDGE_DB_DIR), falling back to the in-memory wiki store
// (WIKI_PATH) for backward compatibility.
// Returns the in-memory store (nil when using SQLite) and a retrieval hook.
func loadWikiStore(madyHome string) (*knowledge.Store, agentcore.LifecycleHook, agentcore.Extension) {
	// 1. Try SQLite backend (vector + FTS RRF fusion).
	embedder := buildEmbedder()
	backend, knowledgeDBPath := loadKnowledgeBackend(madyHome)
	if backend != nil {
		ext := knowledge.NewExtension(nil, nil, "patent", knowledge.DefaultKnowledgeExtConfig())
		ext.WithBackend(backend, embedder)
		if reranker := buildReranker(); reranker != nil {
			ext.WithReranker(reranker)
			fmt.Fprintf(os.Stderr, "knowledge: cross-encoder rerank enabled\n")
		}
		if ws := openWritableStore(madyHome, embedder, knowledgeDBPath); ws != nil {
			ext.WithWritableStore(ws)
		}

		// Wire laws-full.db + knowledge graph enhancer (same directory as knowledge.db).
		if store, ok := backend.(*sqlite.SQLiteStore); ok {
			dbDir := filepath.Dir(knowledgeDBPath)

			// Open laws-full.db for law full-text search.
			lawsPath := filepath.Join(dbDir, "laws-full.db")
			if _, err := os.Stat(lawsPath); err == nil {
				if err := store.OpenLawsDB(lawsPath); err != nil {
					fmt.Fprintf(os.Stderr, "knowledge: laws-full.db open failed: %v\n", err)
				} else {
					// Wrap SearchLaws as knowledge.LawSearcher function type.
					ext.WithLawSearcher(func(keyword string, topK int) ([]knowledge.LawRecord, error) {
						sqliteResults, err := store.SearchLaws(keyword, topK)
						if err != nil {
							return nil, err
						}
						out := make([]knowledge.LawRecord, len(sqliteResults))
						for i, r := range sqliteResults {
							out[i] = knowledge.LawRecord{
								ID: r.ID, Level: r.Level, Name: r.Name,
								Subtitle: r.Subtitle, Content: r.Content, Category: r.Category,
							}
						}
						return out, nil
					})
					fmt.Fprintf(os.Stderr, "knowledge: laws-full.db active (9121 laws)\n")
				}
			}

			// Load knowledge graph and wire graph enhancer.
			if gs, err := store.LoadGraph(); err != nil {
				fmt.Fprintf(os.Stderr, "knowledge: graph load failed: %v\n", err)
			} else if gs.NodeCount() > 0 {
				enhancer := kgwgraph.NewGraphEnhancer(gs, kgwgraph.DefaultEnhanceConfig())
				ext.WithGraph(enhancer)
				fmt.Fprintf(os.Stderr, "knowledge: graph enhancer active (%d nodes, %d edges)\n",
					gs.NodeCount(), gs.EdgeCount())
			}
		}

		hook := ext.BackendHook(retrieval.RetrievalConfig{
			TopK:     5,
			MaxChars: 4000,
			Prefix:   "以下是知识库中检索到的相关专利法律信息，请参考使用：\n",
		})
		if hook != nil {
			return nil, hook, ext
		}
	}

	// 2. Fallback: in-memory wiki store (WIKI_PATH).
	wikiPath := os.Getenv("WIKI_PATH")
	if wikiPath == "" {
		return nil, nil, nil
	}
	store := knowledge.NewStore()
	wikiLoader := loader.NewWikiLoader(store, wikiPath)
	stats, err := wikiLoader.ImportWiki()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wiki: import failed: %v\n", err)
		return nil, nil, nil
	}
	fmt.Fprintf(os.Stderr, "wiki: imported %d docs, %d chunks\n",
		stats.Imported, store.Stats().TotalChunks)
	hook := store.RetrievalHook("patent", retrieval.RetrievalConfig{
		TopK:     5,
		MaxChars: 4000,
		Prefix:   "以下是知识库中检索到的相关专利法律信息，请参考使用：\n",
	})
	return store, hook, nil
}

// extSlice wraps a single Extension into a slice, returning nil for nil input.
func extSlice(ext agentcore.Extension) []agentcore.Extension {
	if ext == nil {
		return nil
	}
	return []agentcore.Extension{ext}
}

// buildEmbedder creates an APIEmbedder from environment variables.
// Returns nil if OMLX_API_KEY is not set (vector search disabled, FTS-only).
func buildEmbedder() retrieval.Embedder {
	baseURL := os.Getenv("OMLX_BASE_URL")
	if baseURL == "" {
		baseURL = agentconfig.DefaultOMLXBaseURL
	}
	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		return nil
	}
	model := os.Getenv("OMLX_EMBED_MODEL")
	if model == "" {
		model = agentconfig.DefaultEmbedModel
	}
	return retrieval.NewAPIEmbedder(baseURL, apiKey, model)
}

// buildReranker creates a ModelReranker from environment variables.
// Returns nil if KNOWLEDGE_RERANK is not "on"/"true"/"1" or if
// OMLX_API_KEY is not set (reranker requires the same auth as embedder).
func buildReranker() retrieval.QueryReranker {
	flag := strings.ToLower(os.Getenv("KNOWLEDGE_RERANK"))
	if flag != "on" && flag != "true" && flag != "1" {
		return nil
	}
	baseURL := os.Getenv("OMLX_BASE_URL")
	if baseURL == "" {
		baseURL = agentconfig.DefaultOMLXBaseURL
	}
	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		return nil
	}
	model := os.Getenv("OMLX_RERANK_MODEL")
	if model == "" {
		model = agentconfig.DefaultRerankModel
	}
	return retrieval.NewModelReranker(baseURL, apiKey, model)
}

// loadKnowledgeBackend opens the SQLite knowledge database read-only.
// Returns nil if the database file is not found or cannot be opened.
// The second return value is the resolved knowledge.db path (empty when nil).
func loadKnowledgeBackend(madyHome string) (knowledge.KnowledgeBackend, string) {
	dbDir := os.Getenv("KNOWLEDGE_DB_DIR")
	if dbDir == "" {
		if madyHome != "" {
			dbDir = filepath.Join(madyHome, "knowledge")
		} else {
			return nil, ""
		}
	}
	dbPath := filepath.Join(dbDir, "knowledge.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, ""
	}
	store, err := sqlite.NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "knowledge: failed to open SQLite store: %v\n", err)
		return nil, ""
	}
	if err := store.PreloadVectors(); err != nil {
		fmt.Fprintf(os.Stderr, "knowledge: vector preload failed, using SQL batch fallback: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "knowledge: SQLite backend active (%s, in-memory vectors)\n", dbPath)
	}
	return store, dbPath
}

// openWritableStore opens or creates the user database (user.db) for
// user-added documents. Returns nil if the embedder is not configured
// (vector search disabled), or if opening fails (non-fatal — the system
// continues without user document support).
//
// The knowledgeDBPath is passed to OpenWritable for path-conflict
// detection: user.db must not point to the same file as knowledge.db.
func openWritableStore(madyHome string, embedder retrieval.Embedder, knowledgeDBPath string) *sqlite.WritableStore {
	if embedder == nil {
		return nil // writable store requires an embedder for vectorisation
	}
	userDBPath := os.Getenv("USER_DB_PATH")
	if userDBPath == "" {
		if madyHome == "" {
			return nil
		}
		userDBPath = filepath.Join(madyHome, "knowledge", "user.db")
	}
	// Ensure parent directory exists.
	if dir := filepath.Dir(userDBPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "knowledge: user.db dir create failed: %v\n", err)
			return nil
		}
	}
	ws, err := sqlite.OpenWritable(userDBPath, embedder, knowledgeDBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "knowledge: user.db open failed: %v\n", err)
		return nil
	}
	fmt.Fprintf(os.Stderr, "knowledge: user.db writable store active (%s)\n", userDBPath)
	return ws
}

// frameworkContext 封装入口之间共享的初始化资源。
type frameworkContext struct {
	BaseConfig      agentcore.Config
	ProjectRegistry *domains.ProjectRegistry
	WikiHook        agentcore.LifecycleHook
	WikiStore       *knowledge.Store
	KnowledgeExt    agentcore.Extension
	Manifests       []agentcore.AgentManifest
	Provider        agentcore.Provider
	// MadyHome 是应用数据根目录（~/.mady），所有可写子资源从此派生。
	MadyHome string
	// WorkspaceDir 是解析后的 workspace 绝对路径（~/.mady/workspace 或 $WORKSPACE_DIR）。
	WorkspaceDir string
	// ManifestDir 是外部 manifest 覆盖目录（~/.mady/manifests 或 $MANIFEST_DIR），可为 ""。
	ManifestDir string
	// KnowledgeGraph 是实体-关系知识图谱，用于多跳推理遍历。
	// 启动时为空，由 wiki 导入或其他数据管线填充。
	KnowledgeGraph *kgwgraph.GraphStore
}

// setupFrameworkContext 执行三个入口共享的初始化逻辑：
//   - Provider 构建
//   - MadyHome 解析（~/.mady，任意 cwd 可用）
//   - Manifest 加载（go:embed 内置 + MADY_HOME/manifests 外部覆盖）
//   - Wiki 知识库加载（可选的 WIKI_PATH 环境变量）
//   - ProjectRegistry 初始化
func setupFrameworkContext(ctx context.Context) *frameworkContext {
	fc := &frameworkContext{}

	provider, err := agentconfig.BuildProvider()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mady: %v\n", err)
		os.Exit(1)
	}
	model := agentconfig.DefaultModel()

	// 解析应用数据根目录（~/.mady），确保任意 cwd 下资源定位一致。
	madyHome, err := util.MadyHome()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mady: 初始化数据目录失败: %v（将使用 cwd 相对路径）\n", err)
		madyHome = "" // 降级：后续 env / cwd 回退
	} else {
		fmt.Fprintf(os.Stderr, "mady: 数据目录 %s\n", madyHome)
	}
	fc.MadyHome = madyHome
	fc.Provider = provider

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

	// 知识检索：优先 SQLite backend（向量+FTS RRF），回退 wiki 内存库。
	fc.WikiStore, fc.WikiHook, fc.KnowledgeExt = loadWikiStore(fc.MadyHome)

	// Manifest 加载：go:embed 内置 + 外部覆盖。
	// 优先级：$MANIFEST_DIR > ~/.mady/manifests > 仅内置。
	// 内置 4 个 manifest 始终可用（embed 进二进制），外部目录可选。
	manifestDir := os.Getenv("MANIFEST_DIR")
	if manifestDir == "" && madyHome != "" {
		manifestDir = filepath.Join(madyHome, "manifests")
	}
	fc.ManifestDir = manifestDir
	mergeRes := agentcore.LoadManifests(manifestDir)
	fc.Manifests = mergeRes.Manifests

	// 醒目加载日志：区分内置 / 外部 / 覆盖 / 新增
	if mergeRes.EmbeddedCount > 0 {
		fmt.Fprintf(os.Stderr, "manifest: 已加载 %d 个内置 Agent（embed）\n", mergeRes.EmbeddedCount)
	}
	if mergeRes.ExternalCount > 0 {
		fmt.Fprintf(os.Stderr, "manifest: 从 %s 加载 %d 个外部 Agent", manifestDir, mergeRes.ExternalCount)
		if len(mergeRes.Overridden) > 0 {
			fmt.Fprintf(os.Stderr, "，覆盖 %d 个（%s）", len(mergeRes.Overridden), strings.Join(mergeRes.Overridden, ", "))
		}
		if len(mergeRes.Added) > 0 {
			fmt.Fprintf(os.Stderr, "，新增 %d 个（%s）", len(mergeRes.Added), strings.Join(mergeRes.Added, ", "))
		}
		fmt.Fprintln(os.Stderr)
	}
	for _, m := range fc.Manifests {
		fmt.Fprintf(os.Stderr, "  - %s (%s)\n", m.Name, m.Domain)
	}
	for _, e := range mergeRes.Errors {
		fmt.Fprintf(os.Stderr, "manifest: [警告] %s: %s\n", e.Path, e.Error)
	}
	if len(fc.Manifests) == 0 {
		fmt.Fprintf(os.Stderr, "manifest: 未加载任何 manifest（内置 embed 异常？）→ 将回退到单 Agent 模式\n")
	}

	// Skill 自动发现：扫描 $SKILL_DIR、$HOME/.agent、$PWD/.agent、~/.mady/skills。
	// 优先级：$SKILL_DIR > $HOME/.agent > $PWD/.agent > ~/.mady/skills。
	// 同名 skill 保留最先发现的。
	var skillPaths []string
	if sd := os.Getenv("SKILL_DIR"); sd != "" {
		skillPaths = append(skillPaths, sd)
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		skillPaths = append(skillPaths, filepath.Join(homeDir, ".agent"))
	}
	if cwd, err := os.Getwd(); err == nil {
		skillPaths = append(skillPaths, filepath.Join(cwd, ".agent"))
	}
	if madyHome != "" {
		skillPaths = append(skillPaths, filepath.Join(madyHome, "skills"))
	}
	loadedSkills, skillDiags, skillErr := skill.Load(skillPaths...)
	if skillErr != nil {
		fmt.Fprintf(os.Stderr, "skill: 加载失败: %v\n", skillErr)
	} else {
		fc.BaseConfig.SkillPaths = skillPaths
		fc.BaseConfig.AvailableSkills = loadedSkills
		fc.BaseConfig.SkillDiagnostics = skillDiags
		if len(loadedSkills) > 0 {
			var names []string
			for _, s := range loadedSkills {
				names = append(names, s.Name)
			}
			fmt.Fprintf(os.Stderr, "skill: 从 %d 个路径加载 %d 个 skill（%s）\n",
				len(skillPaths), len(loadedSkills), strings.Join(names, ", "))
		}
		if len(skillDiags) > 0 {
			for _, d := range skillDiags {
				fmt.Fprintf(os.Stderr, "skill: [警告] %s: %s\n", d.Path, d.Message)
			}
		}
	}

	// MCP 自动发现：扫描 $MCP_CONFIG、~/.mady/mcp.json、$PWD/.mcp.json、~/.claude.json。
	mcpExts, mcpWarnings := mcp.DiscoverMCPExtensions(ctx, madyHome)
	for _, w := range mcpWarnings {
		fmt.Fprintf(os.Stderr, "mcp: [警告] %v\n", w)
	}
	if len(mcpExts) > 0 {
		var names []string
		for _, ext := range mcpExts {
			names = append(names, ext.Name())
		}
		fmt.Fprintf(os.Stderr, "mcp: 已加载 %d 个 MCP 服务器（%s）\n",
			len(mcpExts), strings.Join(names, ", "))
		fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, mcpExts...)
	}

	// Workspace：$WORKSPACE_DIR > ~/.mady/workspace。
	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir == "" {
		if madyHome != "" {
			workspaceDir = filepath.Join(madyHome, "workspace")
		} else {
			workspaceDir = "./workspace" // 降级兜底
		}
	}
	// 确保 workspace 及 projects 子目录存在。
	// ProjectRegistry.Register 写入 registry.json 时依赖父目录已创建
	// （NewProjectRegistryOrEmpty 只 load 不 mkdir）。
	if err := util.EnsureDir(filepath.Join(workspaceDir, "projects")); err != nil {
		fmt.Fprintf(os.Stderr, "mady: 创建 workspace 目录失败: %v\n", err)
	}
	fc.WorkspaceDir = workspaceDir
	projectDir := filepath.Join(workspaceDir, "projects")
	fc.ProjectRegistry = domains.NewProjectRegistryOrEmpty(projectDir)

	// 注入 WorkspaceDir 到 BaseConfig，供领域工厂函数（如 AssistantAgentConfig）
	// 读取，避免工具沙箱硬编码 cwd 相对路径。
	fc.BaseConfig.WorkspaceDir = workspaceDir

	// ProjectDir = 用户当前 cwd，作为工具沙箱边界。
	// 领域工厂函数读取此字段设置工具 WorkingDir。
	// 案件模式在 applyPersistence 中覆盖为 RootPath。
	if cwd, err := os.Getwd(); err == nil {
		fc.BaseConfig.ProjectDir = cwd
	}

	// 初始化知识图谱（空存储，由 wiki import 或数据管线填充）。
	fc.KnowledgeGraph = kgwgraph.NewGraphStore()

	return fc
}

// buildReasoningRetriever 从框架上下文中构造 MultiSourceRetriever。
// 当知识图谱可用时创建完整适配链，否则返回 nil（Stage ② 跳过）。
func buildReasoningRetriever(fc *frameworkContext) *reasoning.MultiSourceRetriever {
	if fc.KnowledgeGraph == nil {
		return nil
	}
	adapter := kgwgraph.NewReasoningStoreAdapter(fc.KnowledgeGraph)
	walker := reasoning.NewReasoningWalker(adapter, nil)
	return reasoning.NewMultiSourceRetriever(walker, nil, nil)
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
//
// 运行模式切换（优先级由高到低）：
//
//	MADY_SINGLE_AGENT=1 → 单 Agent 模式（纯 LLM 对话，无路由）
//	MADY_ROUTER_MODE=1  → Router 多域路由模式（传统交接可见）
//	默认（有 Manifest）  → 集成模式（Chat Agent 统一入口，内部 Invisible Handoff）
//	默认（无 Manifest）  → 单 Agent 模式（降级）
func runTui(ctx context.Context) {
	fs := flag.NewFlagSet("mady tui", flag.ExitOnError)
	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "mady tui: %v\n", err)
		os.Exit(1)
	}

	fc := setupFrameworkContext(ctx)

	if err := theme.InitThemeFromEnv(); err != nil {
		log.Printf("theme init: %v", err)
	}

	ruleEngine, _ := rules.LoadEngineFromMadyHome()
	var ruleExt agentcore.Extension
	if ruleEngine != nil {
		ruleExt = rules.NewExtension(ruleEngine)
		log.Printf("rules: 已加载规则引擎（%d 条规则）", len(ruleEngine.AllRules()))
	}

	provider, err := agentconfig.BuildProvider()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mady tui: %v\n", err)
		os.Exit(1)
	}
	model := agentconfig.DefaultModel()

	useSingleAgent := os.Getenv("MADY_SINGLE_AGENT") == "1"
	useRouterMode := os.Getenv("MADY_ROUTER_MODE") == "1"
	useMultiDomain := !useSingleAgent && len(fc.Manifests) > 0
	useIntegratedMode := useMultiDomain && !useRouterMode

	sessionDir := os.Getenv("SESSION_DIR")
	if sessionDir == "" {
		if fc.MadyHome != "" {
			sessionDir = filepath.Join(fc.MadyHome, "sessions")
		} else {
			sessionDir = "./sessions"
		}
	}
	var agentStore *session.AgentStore
	fileStore, persistErr := session.NewFileStore(sessionDir)
	if persistErr != nil {
		log.Printf("session: %v (continuing without persistence)", persistErr)
	} else {
		agentStore = session.NewAgentStore(fileStore, fc.WorkspaceDir)
	}

	fileIndexExt := fileindex.NewExtension(fileindex.ExtensionConfig{
		FallbackDir: fc.BaseConfig.ProjectDir,
	})

	s := &tuiSession{
		ctx:               ctx,
		fc:                fc,
		provider:          provider,
		model:             model,
		providerName:      firstNonEmpty(os.Getenv("PROVIDER"), "deepseek"),
		planModel:         agentconfig.DefaultPlanModel,
		normalModel:       model,
		currentThinking:   agentconfig.ThinkingFromEnv(),
		useMultiDomain:    useMultiDomain,
		useIntegratedMode: useIntegratedMode,
		ruleExt:           ruleExt,
		fileIndexExt:      fileIndexExt,
		agentStore:        agentStore,
		checkpointSaver:   agentcore.NewMemoryCheckpointSaver(),
		currentThreadID:   "default",
		sessionDir:        sessionDir,
	}

	s.currentAgent = agentcore.New(s.buildAgentConfig())
	defer s.currentAgent.Close()

	s.currentThemeName = "dark"
	if name := theme.CurrentPalette().Semantic.Name; strings.Contains(strings.ToLower(name), "light") {
		s.currentThemeName = "light"
	}

	// Build the slash registry once; both handleSubmit and the autocomplete
	// menu read from it (single source of truth, no dual switch).
	s.slashReg = s.buildSlashRegistry()
	slashSuggestions := s.slashReg.Suggestions(s)

	var app *chat.ChatApp
	app = tui.NewChatApp(chat.ChatAppConfig{
		Title:                      fmt.Sprintf("mady · model=%s", model),
		ShowTurns:                  true,
		SuppressHandoffToolDisplay: useIntegratedMode,
		AltScreen:                  true,
		MouseMode:                  "auto",
		KittyKeyboardFlags:         1,
		ContextWindow:              fc.BaseConfig.ContextWindow,
		Context:                    ctx,
		OnInterrupt: func() {
			s.cancelMu.Lock()
			defer s.cancelMu.Unlock()
			if s.runCancel != nil {
				s.runCancel()
				s.runCancel = nil
			}
		},
		OnQuit: func() {
			if app != nil {
				_ = app.Stop()
			}
		},
		Providers: []core.AutocompleteProvider{
			&component.StaticProvider{
				TriggerStr:  "/",
				Suggestions: slashSuggestions,
			},
		},
		OnSubmit: func(_ context.Context, input string) {
			s.handleSubmit(input)
		},
	})
	s.app = app
	agentadapter.BindAgent(app, s.currentAgent)

	// Load user keymap overrides from ~/.mady/keymap.json (if present) into the
	// app's keybinding manager so the editor and chat honor customized keys.
	// Warnings (unknown tokens) are surfaced to the user but never fatal.
	if fc.MadyHome != "" {
		if warnings := loadKeymapOverrides(fc.MadyHome, app.Keybindings()); len(warnings) > 0 {
			for _, w := range warnings {
				log.Printf("keymap: %s", w)
			}
		}
	}

	app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.planMode, useMultiDomain, s.currentThinking))

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

func firstNonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}
