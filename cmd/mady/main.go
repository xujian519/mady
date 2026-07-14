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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xujian519/mady/acp"
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
	"github.com/xujian519/mady/server"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/skill"
	"github.com/xujian519/mady/tools"
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
		baseURL = "http://127.0.0.1:8000/v1"
	}
	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		return nil
	}
	model := os.Getenv("OMLX_EMBED_MODEL")
	if model == "" {
		model = "bge-m3-mlx-8bit"
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
		baseURL = "http://127.0.0.1:8000/v1"
	}
	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		return nil
	}
	model := os.Getenv("OMLX_RERANK_MODEL")
	if model == "" {
		model = "Qwen3-Reranker-4B-4bit-MLX"
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

	provider := agentconfig.BuildProvider()
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
// 模式优先级（从高到低）：
//
//	MADY_SINGLE_AGENT=1 → 单 Agent 模式（纯 LLM 对话，无路由）
//	MADY_ROUTER_MODE=1  → Router 多域路由模式（传统交接可见）
//	默认（有 Manifest）  → 集成模式（Chat Agent 统一入口，内部 Invisible Handoff）
//	默认（无 Manifest）  → 单 Agent 模式（降级）
func runTui(ctx context.Context) {
	fs := flag.NewFlagSet("mady tui", flag.ExitOnError)
	_ = fs.Parse(os.Args[2:])

	fc := setupFrameworkContext(ctx)

	if err := theme.InitThemeFromEnv(); err != nil {
		log.Printf("theme init: %v", err)
	}

	// 加载规则引擎（专利法律规则 / OA 解析 / 反套话）。
	// 目录不存在时静默失败，不阻塞启动。
	ruleEngine, _ := rules.LoadEngineFromMadyHome()
	var ruleExt agentcore.Extension
	if ruleEngine != nil {
		ruleExt = rules.NewExtension(ruleEngine)
		log.Printf("rules: 已加载规则引擎（%d 条规则）", len(ruleEngine.AllRules()))
	}

	provider := agentconfig.BuildProvider()
	model := agentconfig.DefaultModel()

	// planMode 切换高质量推理模式（/plan）。
	// planMode = true 时使用 planModel（deepseek-v4-pro）+ 最大推理强度。
	planMode := false
	planModel := "deepseek-v4-pro"
	normalModel := model
	// providerName is the provider identifier used in status bar display.
	providerName := os.Getenv("PROVIDER")
	if providerName == "" {
		providerName = "deepseek"
	}

	currentThinking := agentconfig.ThinkingFromEnv()

	// 运行模式切换（优先级由高到低）：
	//   MADY_SINGLE_AGENT=1 → 单 Agent 模式（纯 LLM 对话，无路由）
	//   MADY_ROUTER_MODE=1  → Router 模式（传统多域路由，交接可见）
	//   默认（有 Manifest）  → 集成模式（Chat Agent 内部路由，交接不可见）
	//   默认（无 Manifest）  → 单 Agent 模式（降级）
	useSingleAgent := os.Getenv("MADY_SINGLE_AGENT") == "1"
	useRouterMode := os.Getenv("MADY_ROUTER_MODE") == "1"
	useMultiDomain := !useSingleAgent && len(fc.Manifests) > 0

	// 集成模式：Chat Agent 内置路由（默认且推荐）
	useIntegratedMode := useMultiDomain && !useRouterMode

	// runMu 防止快速输入时多个 goroutine 并发调用 Agent.Run。
	var runMu sync.Mutex
	// runCancel 用于中断正在执行的 Agent 运行。
	var runCancel context.CancelFunc
	var cancelMu sync.Mutex

	// 会话持久化：JSONL 文件存储，TUI 模式自动保存对话到磁盘。
	// 优先级：$SESSION_DIR > $MADY_HOME/sessions > ~/.mady/sessions。
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
	checkpointSaver := agentcore.NewMemoryCheckpointSaver()
	currentThreadID := "default"

	// 案件上下文：切换案件时更新，buildCfg 注入到 Agent 的 WorkspaceDir + SystemPrompt。
	var currentProject *domains.ProjectRecord
	var currentProjectMeta *domains.ProjectMeta
	// 文件索引：案件切换时打开/刷新，buildCfg 注入 fileindex Extension。
	var currentFileIndex *fileindex.FileIndex
	var currentFileWatcher *fileindex.FileWatcher
	fileIndexExt := fileindex.NewExtension(fileindex.ExtensionConfig{
		FallbackDir: fc.BaseConfig.ProjectDir,
	})

	// 审核关卡：启用后在关键决策点（专利结论/法律意见/风险评估等）插入人工审核提示。
	reviewMode := false

	buildCfg := func() agentcore.Config {
		applyPersistence := func(cfg agentcore.Config) agentcore.Config {
			if agentStore != nil {
				cfg.Store = agentStore
			}
			cfg.Checkpoint = &agentcore.CheckpointSettings{
				Saver:    checkpointSaver,
				ThreadID: currentThreadID,
			}
			if currentProject != nil {
				cfg.WorkspaceDir = currentProject.RootPath
				cfg.ProjectDir = currentProject.RootPath
				cfg.SystemPrompt += formatProjectContext(currentProject, currentProjectMeta)
				// 注入五阶段法律推理工具，让 Agent 能调用深度可验证推理。
				// 从框架上下文中构建检索器和 LLM 客户端。
				retriever := buildReasoningRetriever(fc)
				var llmClient reasoning.LlmClient
				if provider != nil {
					llmClient = reasoning.NewLlmClientFromProvider(provider, model)
				}
				runner := reasoning.NewWorkflowRunner(
					currentProject.ProjectID,
					mapMatterTypeToCaseType(currentProjectMeta),
					currentProject.Domain,
					retriever,
					llmClient,
				)
				cfg.Tools = append(cfg.Tools, reasoning.AsWorkflowTool(runner))
			} else if cfg.ProjectDir != "" {
				// 无案件模式：告知 Agent 当前工作目录，使其知晓可以读取哪些文件。
				// 工具沙箱已限制文件操作在此目录范围内。
				cfg.SystemPrompt += fmt.Sprintf(
					"\n\n【当前工作目录】\n你正在「%s」目录下工作。可以使用文件工具（read、ls、grep、find、write_file 等）读取和分析该目录中的文件。用户提到的相对路径默认基于此目录。",
					cfg.ProjectDir,
				)
			}

			if reviewMode {
				gate := domains.NewApprovalGate(domains.DefaultApprovalConfig())
				cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, gate)
			}

			// 配置 vision_analyze 工具：使用实际 LLM provider 而非占位符。
			for _, ext := range cfg.Extensions {
				if te, ok := ext.(*tools.Extension); ok {
					te.WithVision(provider, model)
				}
			}

			return cfg
		}
		switch {
		case useIntegratedMode:
			// 集成模式：Chat Agent 内置路由，交接不可见
			base := fc.BaseConfig
			base.Name = "chat-agent"
			base.ModelConfig = agentcore.ModelConfig{
				Name:      "mady",
				Model:     model,
				Provider:  provider,
				Thinking:  cloneThinkingConfig(currentThinking),
				Streaming: true,
			}
			if planMode {
				base.Model = planModel
				if base.Thinking == nil {
					base.Thinking = &agentcore.ThinkingConfig{Effort: agentcore.ThinkingEffortMax}
				} else {
					base.Thinking.Effort = agentcore.ThinkingEffortMax
				}
			}
			cfg := domains.IntegratedChatConfig(base)
			if fc.WikiHook != nil {
				cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, fc.WikiHook)
			}
			if fc.KnowledgeExt != nil {
				cfg.Extensions = append(cfg.Extensions, fc.KnowledgeExt)
			}
			if ruleExt != nil {
				cfg.Extensions = append(cfg.Extensions, ruleExt)
			}
			cfg.Extensions = append(cfg.Extensions, fileIndexExt)
			return applyPersistence(cfg)

		case useMultiDomain:
			// Router 模式：传统多域路由，交接可见
			cfg := buildRouterConfig(fc.BaseConfig, fc.Manifests)
			cfg.Thinking = cloneThinkingConfig(currentThinking)
			if planMode {
				cfg.Model = planModel
				if cfg.Thinking == nil {
					cfg.Thinking = &agentcore.ThinkingConfig{Effort: agentcore.ThinkingEffortMax}
				} else {
					cfg.Thinking.Effort = agentcore.ThinkingEffortMax
				}
			}
			if fc.WikiHook != nil {
				cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, fc.WikiHook)
			}
			if fc.KnowledgeExt != nil {
				cfg.Extensions = append(cfg.Extensions, fc.KnowledgeExt)
			}
			if ruleExt != nil {
				cfg.Extensions = append(cfg.Extensions, ruleExt)
			}
			return applyPersistence(cfg)

		default:
			// 单 Agent 模式：根据 planMode 选择模型和推理配置。
			effectiveModel := model
			effectiveThinking := cloneThinkingConfig(currentThinking)
			if planMode {
				effectiveModel = planModel
				if effectiveThinking == nil {
					effectiveThinking = &agentcore.ThinkingConfig{Effort: agentcore.ThinkingEffortMax}
				} else {
					effectiveThinking.Effort = agentcore.ThinkingEffortMax
				}
			}

			singleCfg := agentcore.Config{
				ModelConfig: agentcore.ModelConfig{
					Name:      "mady",
					Model:     effectiveModel,
					Provider:  provider,
					Thinking:  effectiveThinking,
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
			if fc.KnowledgeExt != nil {
				singleCfg.Extensions = append(singleCfg.Extensions, fc.KnowledgeExt)
			}
			if ruleExt != nil {
				singleCfg.Extensions = append(singleCfg.Extensions, ruleExt)
			}
			return applyPersistence(singleCfg)
		}
	}

	currentAgent := agentcore.New(buildCfg())
	defer currentAgent.Close()

	currentThemeName := "dark"
	if name := theme.CurrentPalette().Semantic.Name; strings.Contains(strings.ToLower(name), "light") {
		currentThemeName = "light"
	}

	slashSuggestions := []core.Suggestion{
		{InsertText: "/help", Label: "/help", Description: "显示快捷键"},
		{InsertText: "/clear", Label: "/clear", Description: "开始新对话"},
		{InsertText: "/new", Label: "/new", Description: "开始新对话"},
		{InsertText: "/branch", Label: "/branch", Description: "从当前对话创建分支"},
		{InsertText: "/thinking", Label: "/thinking", Description: "查看或修改推理模式"},
		{InsertText: "/thinking summarized", Label: "/thinking summarized", Description: "显示推理摘要"},
		{InsertText: "/thinking omitted", Label: "/thinking omitted", Description: "隐藏推理块"},
		{InsertText: "/thinking effort medium", Label: "/thinking effort medium", Description: "设置推理强度"},
		{InsertText: "/thinking budget -1", Label: "/thinking budget -1", Description: "动态推理预算"},
		{InsertText: "/skill:", Label: "/skill:", Description: "显式调用技能"},
		{InsertText: "/save", Label: "/save", Description: "显示会话保存信息"},
		{InsertText: "/theme", Label: "/theme", Description: "切换主题"},
		{InsertText: "/theme dark", Label: "/theme dark", Description: "深色主题"},
		{InsertText: "/theme light", Label: "/theme light", Description: "浅色主题"},
		{InsertText: "/copy", Label: "/copy", Description: "复制最后一条回复"},
		{InsertText: "/export", Label: "/export", Description: "导出当前对话为 Markdown"},
		{InsertText: "/case", Label: "/case", Description: "查看或切换案件"},
		{InsertText: "/deadline", Label: "/deadline", Description: "显示当前案件期限"},
		{InsertText: "/review", Label: "/review", Description: "切换审核关卡（关键内容人工确认）"},
		{InsertText: "/approve", Label: "/approve", Description: "确认AI输出，继续执行（审核模式下）"},
		{InsertText: "/reject", Label: "/reject", Description: "拒绝AI输出，请求修改（审核模式下）"},
		{InsertText: "/plan", Label: "/plan", Description: "切换计划模式（高质量推理）"},
		{InsertText: "/quit", Label: "/quit", Description: "退出"},
	}

	if useMultiDomain {
		slashSuggestions = append(slashSuggestions,
			core.Suggestion{InsertText: "/mode", Label: "/mode", Description: "显示当前 Agent 模式"},
		)
	}

	var app *chat.ChatApp
	app = tui.NewChatApp(chat.ChatAppConfig{
		Title:                      fmt.Sprintf("mady · model=%s", model),
		ShowTurns:                  true,
		SuppressHandoffToolDisplay: useIntegratedMode,
		AltScreen:                  true,
		MouseMode:                  "auto",
		KittyKeyboardFlags:         1, // disambiguate only; flag 8 via MADY_KITTY_FLAGS env var
		Context:                    ctx,
		OnInterrupt: func() {
			cancelMu.Lock()
			defer cancelMu.Unlock()
			if runCancel != nil {
				runCancel()
				runCancel = nil
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
				runMu.Lock()
				prev := currentAgent
				currentAgent = agentcore.New(buildCfg())
				prev.Close()
				agentadapter.BindAgent(app, currentAgent)
				app.PrintSystem("推理配置已更新: " + formatThinkingConfig(currentThinking))
				mdl := normalModel
				if planMode {
					mdl = planModel
				}
				app.UpdateStatusBar(providerName, mdl, statusBarModeLabel(planMode, useMultiDomain, currentThinking))
				runMu.Unlock()
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

			// /case 命令族：案件上下文管理。
			if trimmed == "/case" || strings.HasPrefix(trimmed, "/case ") {
				args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/case"))
				switch args {
				case "", "list":
					records := fc.ProjectRegistry.List()
					if len(records) == 0 {
						app.PrintSystem("暂无已注册案件。使用 mady serve 或 ProjectRegistry API 注册案件。")
						return
					}
					var sb strings.Builder
					fmt.Fprintf(&sb, "已注册案件（%d）：\n", len(records))
					for i, rec := range records {
						marker := "  "
						if currentProject != nil && rec.ProjectID == currentProject.ProjectID {
							marker = "→ "
						}
						fmt.Fprintf(&sb, "%s%d. %s（%s）[%s]\n", marker, i+1, rec.Alias, rec.ProjectID, rec.Domain)
					}
					if currentProject == nil {
						sb.WriteString("\n使用 /case <ID或别名> 切换案件")
					}
					app.PrintSystem(sb.String())
					return
				case "info":
					if currentProject == nil {
						app.PrintSystem("当前未选择案件。使用 /case 查看可用案件。")
						return
					}
					app.PrintSystem(formatProjectInfo(currentProject, currentProjectMeta))
					return
				case "off", "clear":
					if currentProject == nil {
						app.PrintSystem("当前未选择案件")
						return
					}
					oldName := currentProject.Alias
					currentProject = nil
					if currentFileWatcher != nil {
						currentFileWatcher.Stop()
						currentFileWatcher = nil
					}
					if currentFileIndex != nil {
						currentFileIndex.Close()
						currentFileIndex = nil
						fileIndexExt.SetFileIndex(nil)
						fileIndexExt.SetFallbackDir(fc.BaseConfig.ProjectDir)
					}
					currentProjectMeta = nil
					runMu.Lock()
					prev := currentAgent
					currentAgent = agentcore.New(buildCfg())
					prev.Close()
					agentadapter.BindAgent(app, currentAgent)
					runMu.Unlock()
					app.UpdateStatusBar(providerName, normalModel, statusBarModeLabel(planMode, useMultiDomain, currentThinking))
					app.PrintSystem(fmt.Sprintf("已清除案件上下文（%s）", oldName))
					return
				default:
					records := fc.ProjectRegistry.List()
					var matched *domains.ProjectRecord
					for i := range records {
						if strings.Contains(records[i].ProjectID, args) || strings.Contains(records[i].Alias, args) {
							matched = &records[i]
							break
						}
					}
					if matched == nil {
						app.PrintSystem(fmt.Sprintf("未找到匹配 '%s' 的案件。使用 /case 查看可用案件。", args))
						return
					}
					currentProject = matched
					currentProjectMeta = nil

					// Always close old resources before switching (independent of meta load).
					if currentFileWatcher != nil {
						currentFileWatcher.Stop()
						currentFileWatcher = nil
					}
					if currentFileIndex != nil {
						currentFileIndex.Close()
						currentFileIndex = nil
					}
					fileIndexExt.SetFileIndex(nil)

					// Determine database path with WorkspaceDir fallback.
					wsDir := fc.WorkspaceDir
					if wsDir == "" {
						wsDir = filepath.Join(os.TempDir(), "mady-fileindex")
					}
					dbPath := filepath.Join(wsDir, "projects", matched.ProjectID, "fileindex.db")

					// Open/refresh FileIndex for the selected project (independent of meta load).
					if fi, err := fileindex.OpenFileIndex(matched.RootPath, dbPath); err == nil {
						_ = fi.Refresh(context.Background())
						currentFileIndex = fi
						fileIndexExt.SetFileIndex(fi)
						// Start file watcher for incremental updates.
						wcfg := fileindex.FileWatcherConfig{}
						currentFileWatcher = fileindex.NewFileWatcher(fi, wcfg)
						if err := currentFileWatcher.Start(context.Background()); err != nil {
							log.Printf("filewatcher: start: %v (continuing without)", err)
							currentFileWatcher = nil
						}
					}

					// Load meta if available (non-fatal on failure).
					if meta, err := fc.ProjectRegistry.LoadMeta(matched.ProjectID); err == nil {
						currentProjectMeta = meta
					}
					runMu.Lock()
					prev := currentAgent
					currentAgent = agentcore.New(buildCfg())
					prev.Close()
					agentadapter.BindAgent(app, currentAgent)
					runMu.Unlock()
					app.UpdateStatusBar(providerName, normalModel, statusBarModeLabel(planMode, useMultiDomain, currentThinking))
					app.PrintSystem(fmt.Sprintf("已切换到案件: %s（%s）\n工作目录: %s\n⚖ 已启用五阶段法律推理工具（run_five_step_workflow）", matched.Alias, matched.ProjectID, matched.RootPath))
					return
				}
			}

			// /deadline 命令：显示当前案件期限。
			if trimmed == "/deadline" {
				if currentProjectMeta == nil || len(currentProjectMeta.Deadlines) == 0 {
					app.PrintSystem("当前案件无期限信息。使用 /case 选择案件。")
					return
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, "案件 %s 的期限：\n", currentProject.Alias)
				for _, d := range currentProjectMeta.Deadlines {
					mark := "  "
					if d.Reminded {
						mark = "✓ "
					}
					fmt.Fprintf(&sb, "%s%s: %s\n", mark, d.Type, d.DueDate)
				}
				app.PrintSystem(sb.String())
				return
			}

			// submitAgentInput 向当前 Agent 提交输入（供 /approve /reject 复用）。
			// 异步执行：Agent.Run 在独立 goroutine 中运行，避免阻塞 TUI 事件循环。
			// 按值捕获 agent/store/threadID 以消除 TOCTOU 窗口——goroutine 创建后
			// /clear /plan 等命令可能替换 currentAgent。
			submitAgentInput := func(input string) {
				agent := currentAgent
				store := agentStore
				threadID := currentThreadID
				go func() {
					runMu.Lock()
					defer runMu.Unlock()

					runCtx, cancel := context.WithCancel(ctx)
					cancelMu.Lock()
					runCancel = cancel
					cancelMu.Unlock()
					defer func() {
						cancelMu.Lock()
						runCancel = nil
						cancelMu.Unlock()
					}()

					if _, err := agent.Run(runCtx, input); err != nil {
						log.Printf("[mady] agent run failed: %v", err)
						return
					}
					if store == nil {
						return
					}
					if err := agent.SaveState(context.Background(), threadID); err != nil {
						log.Printf("[mady] save state: %v", err)
					}
				}()
			}

			switch trimmed {
			case "/help":
				app.ToggleKeyHelp()
				return
			case "/clear", "/new":
				if agentStore != nil {
					currentThreadID = fmt.Sprintf("tui-%d", time.Now().UnixNano())
				}
				runMu.Lock()
				prev := currentAgent
				currentAgent = agentcore.New(buildCfg())
				prev.Close()
				agentadapter.BindAgent(app, currentAgent)
				runMu.Unlock()
				app.History().Clear()
				app.PrintSystem("已开始新对话")
				return
			case "/branch":
				if agentStore == nil {
					app.PrintSystem("会话持久化未启用，无法分支")
					return
				}
				snap, err := agentStore.GetThread(context.Background(), currentThreadID)
				if err != nil || len(snap.Messages) == 0 {
					app.PrintSystem("当前会话为空，无法分支")
					return
				}
				var lastEntryID string
				if len(snap.Transcript) > 0 {
					lastEntryID = snap.Transcript[len(snap.Transcript)-1].EntryID
				}
				branched, err := agentStore.BranchThread(context.Background(), currentThreadID, lastEntryID)
				if err != nil {
					app.PrintError(fmt.Errorf("分支失败: %w", err))
					return
				}
				oldID := currentThreadID
				currentThreadID = branched.Info.ID
				runMu.Lock()
				prev := currentAgent
				currentAgent = agentcore.New(buildCfg())
				prev.Close()
				agentadapter.BindAgent(app, currentAgent)
				runMu.Unlock()
				app.History().Clear()
				for _, msg := range branched.Messages {
					switch msg.Role {
					case agentcore.RoleUser:
						app.History().Append(chat.ChatMessage{Role: chat.RoleUser, Text: msg.Content})
					case agentcore.RoleAssistant:
						app.History().Append(chat.ChatMessage{Role: chat.RoleAssistant, Text: msg.Content})
					}
				}
				app.PrintSystem(fmt.Sprintf("已从 %s 创建分支 → %s（%d 条消息）", oldID, currentThreadID, len(branched.Messages)))
				return
			case "/save":
				if agentStore != nil {
					threads, _ := agentStore.ListThreads(context.Background())
					msg := fmt.Sprintf("✅ 会话已自动保存到 %s（当前线程: %s", sessionDir, currentThreadID)
					if len(threads) > 0 {
						msg += fmt.Sprintf("，共 %d 个线程", len(threads))
					}
					msg += "）"
					app.PrintSystem(msg)
				} else {
					app.PrintSystem("⚠ 会话持久化未启用（session 目录创建失败）")
				}
				return
			case "/skill:":
				app.PrintSystem("mady tui 简化版未加载技能，请使用 example/cli-chat 配合 SKILL_DIRS")
				return
			case "/copy":
				msgs := app.History().Messages()
				for i := len(msgs) - 1; i >= 0; i-- {
					if msgs[i].Role == chat.RoleAssistant && msgs[i].Text != "" {
						go func(text string) {
							if err := chat.CopyToClipboard(text); err != nil {
								app.PrintError(err)
							} else {
								truncated := text
								if core.VisibleWidth(truncated) > 60 {
									truncated = core.TruncateToWidth(truncated, 57, "...")
								}
								app.PrintSystem("📋 已复制: " + truncated)
							}
						}(msgs[i].Text)
						return
					}
				}
				app.PrintSystem("没有可复制的助手回复")
				return

			case "/export":
				msgs := app.History().Messages()
				if len(msgs) == 0 {
					app.PrintSystem("当前对话为空，无法导出")
					return
				}
				exportPath := strings.TrimSpace(strings.TrimPrefix(trimmed, "/export"))
				if exportPath == "" {
					exportDir := "exports"
					if fc.MadyHome != "" {
						exportDir = filepath.Join(fc.MadyHome, "exports")
					}
					_ = os.MkdirAll(exportDir, 0o755)
					exportPath = filepath.Join(exportDir, fmt.Sprintf("export-%s.md", time.Now().Format("20060102-150405")))
				}
				exportContent := formatExportMarkdown(msgs, currentThreadID, currentProject)
				if err := os.WriteFile(exportPath, []byte(exportContent), 0o644); err != nil {
					app.PrintError(fmt.Errorf("导出失败: %w", err))
					return
				}
				app.PrintSystem(fmt.Sprintf("📄 已导出到 %s（%d 条消息）", exportPath, len(msgs)))
				return

			case "/review":
				reviewMode = !reviewMode
				runMu.Lock()
				prev := currentAgent
				currentAgent = agentcore.New(buildCfg())
				prev.Close()
				runMu.Unlock()
				agentadapter.BindAgent(app, currentAgent)
				app.UpdateStatusBar(providerName, normalModel, statusBarModeLabel(planMode, useMultiDomain, currentThinking))
				if reviewMode {
					app.PrintSystem("⚖ 审核关卡已启用 — 专利结论/法律意见/风险评估将插入人工审核提示")
					if currentProject != nil {
						ct := currentProject.CaseType
						if ct == "" {
							ct = "未分类"
						}
						app.PrintSystem(fmt.Sprintf("📁 当前案件: %s (%s)", currentProject.Alias, currentProject.ProjectID))
						app.PrintSystem(fmt.Sprintf("   📋 案件类型: %s", ct))
					}
					app.PrintSystem("   📌 触发关键词: 专利结论、侵权判断、法律意见、风险评估、最终建议")
					app.PrintSystem("   💡 使用 /approve 确认 /reject 拒绝/取消")
				} else {
					app.PrintSystem("⚖ 审核关卡已关闭")
				}
				return

			case "/approve":
				if !reviewMode {
					app.PrintSystem("⚠ 审核关卡未启用。使用 /review 开启")
					return
				}
				app.PrintSystem("✅ 已确认 — Agent 将继续执行")
				submitAgentInput("确认")
				return

			case "/reject":
				if !reviewMode {
					app.PrintSystem("⚠ 审核关卡未启用。使用 /review 开启")
					return
				}
				app.PrintSystem("❌ 已拒绝 — Agent 将根据您的反馈调整")
				submitAgentInput("拒绝，请根据审核意见修改后重新输出")
				return

			case "/plan":
				planMode = !planMode
				runMu.Lock()
				prev := currentAgent
				currentAgent = agentcore.New(buildCfg())
				prev.Close()
				runMu.Unlock()
				agentadapter.BindAgent(app, currentAgent)
				mdl := normalModel
				if planMode {
					mdl = planModel
				}
				app.UpdateStatusBar(providerName, mdl, statusBarModeLabel(planMode, useMultiDomain, currentThinking))
				if planMode {
					app.PrintSystem("🧠 计划模式已启用 · 模型: " + planModel + " · 推理强度: max")
				} else {
					app.PrintSystem("⚡ 已切回普通模式 · 模型: " + normalModel)
				}
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

			submitAgentInput(trimmed)
		},
	})
	agentadapter.BindAgent(app, currentAgent)

	app.UpdateStatusBar(providerName, normalModel, statusBarModeLabel(planMode, useMultiDomain, currentThinking))

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

	fc := setupFrameworkContext(ctx)

	err := acp.RunServer(ctx, acp.RunOptions{
		Provider:   agentconfig.BuildProvider(),
		Model:      agentconfig.DefaultModel(),
		Thinking:   agentconfig.ThinkingFromEnv(),
		Lifecycle:  fc.WikiHook,
		Extensions: extSlice(fc.KnowledgeExt),
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
		fmt.Fprintf(os.Stderr, "flag: %v\n", err)
		return
	}

	fc := setupFrameworkContext(ctx)

	// Build Router config from manifests (or use hardcoded fallback).
	cfg := buildRouterConfig(fc.BaseConfig, fc.Manifests)

	// Attach wiki retrieval hook if available.
	if fc.WikiHook != nil {
		cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, fc.WikiHook)
	}
	if fc.KnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, fc.KnowledgeExt)
	}

	// Session persistence via JSONL file store.
	// 优先级：$SESSION_DIR > ~/.mady/sessions。
	sessionDir := os.Getenv("SESSION_DIR")
	if sessionDir == "" {
		if fc.MadyHome != "" {
			sessionDir = filepath.Join(fc.MadyHome, "sessions")
		} else {
			sessionDir = "./sessions" // 降级兜底
		}
	}
	fileStore, err := session.NewFileStore(sessionDir)
	if err != nil {
		log.Printf("session: %v (continuing without persistence)", err)
	} else {
		// 修复：使用 fc.WorkspaceDir 而非硬编码 "./workspace"，
		// 确保与 ProjectRegistry、AgentStore 共用同一 workspace。
		cfg.Store = session.NewAgentStore(fileStore, fc.WorkspaceDir)
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
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		return
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

// statusBarModeLabel 生成状态栏的模式标签（中文友好）。
func statusBarModeLabel(planMode, useMultiDomain bool, thinking *agentcore.ThinkingConfig) string {
	if planMode {
		return "🧠 计划"
	}
	label := "集成"
	if useMultiDomain {
		label = "多域路由"
	}
	if thinking != nil && thinking.IncludeThoughts {
		if thinking.Effort != "" && thinking.Effort != agentcore.ThinkingEffortDefault {
			label += " · 推理" + string(thinking.Effort)
		} else {
			label += " · 推理"
		}
	}
	return label
}

func formatProjectContext(rec *domains.ProjectRecord, meta *domains.ProjectMeta) string {
	s := "\n\n---\n## 当前案件上下文\n"
	s += fmt.Sprintf("- 案件: %s（%s）\n", rec.Alias, rec.ProjectID)
	s += fmt.Sprintf("- 领域: %s\n", rec.Domain)
	if meta != nil {
		if meta.MatterType != "" {
			s += fmt.Sprintf("- 事项类型: %s\n", meta.MatterType)
		}
		if meta.ClientName != "" {
			s += fmt.Sprintf("- 客户: %s\n", meta.ClientName)
		}
		if len(meta.Deadlines) > 0 {
			s += "- 期限:\n"
			for _, d := range meta.Deadlines {
				s += fmt.Sprintf("  - %s: %s\n", d.Type, d.DueDate)
			}
		}
	}
	s += fmt.Sprintf("- 工作目录: %s\n", rec.RootPath)
	return s
}

func formatProjectInfo(rec *domains.ProjectRecord, meta *domains.ProjectMeta) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "案件: %s\n", rec.Alias)
	fmt.Fprintf(&sb, "ID: %s\n", rec.ProjectID)
	fmt.Fprintf(&sb, "领域: %s\n", rec.Domain)
	fmt.Fprintf(&sb, "状态: %s\n", rec.Status)
	fmt.Fprintf(&sb, "工作目录: %s\n", rec.RootPath)
	fmt.Fprintf(&sb, "注册时间: %s\n", rec.RegisteredAt.Format("2006-01-02"))
	if meta != nil {
		if meta.MatterType != "" {
			fmt.Fprintf(&sb, "事项类型: %s\n", meta.MatterType)
		}
		if meta.ClientName != "" {
			fmt.Fprintf(&sb, "客户: %s\n", meta.ClientName)
		}
		if len(meta.Deadlines) > 0 {
			sb.WriteString("期限:\n")
			for _, d := range meta.Deadlines {
				mark := ""
				if d.Reminded {
					mark = "✓ "
				}
				fmt.Fprintf(&sb, "  %s%s: %s\n", mark, d.Type, d.DueDate)
			}
		}
	}
	return sb.String()
}

// mapMatterTypeToCaseType 将案件事项类型映射到 reasoning 工作流的 CaseType。
func mapMatterTypeToCaseType(meta *domains.ProjectMeta) reasoning.CaseType {
	if meta == nil || meta.MatterType == "" {
		return reasoning.CaseGeneralLegal
	}
	m := strings.ToLower(meta.MatterType)
	switch {
	case strings.Contains(m, "无效"):
		return reasoning.CaseInvalidation
	case strings.Contains(m, "自由实施") || strings.Contains(m, "fto"):
		return reasoning.CaseFTO
	case strings.Contains(m, "新颖性"):
		return reasoning.CaseNoveltySearch
	case strings.Contains(m, "专利性") || strings.Contains(m, "创造性"):
		return reasoning.CasePatentability
	case strings.Contains(m, "侵权"):
		return reasoning.CaseInfringement
	case strings.Contains(m, "审查意见") || strings.Contains(m, "oa") || strings.Contains(m, "答复"):
		return reasoning.CaseRejection
	case strings.Contains(m, "复审"):
		return reasoning.CaseReexamination
	case strings.Contains(m, "撰写") || strings.Contains(m, "申请"):
		return reasoning.CaseDrafting
	default:
		return reasoning.CaseGeneralLegal
	}
}

func formatExportMarkdown(msgs []chat.ChatMessage, threadID string, project *domains.ProjectRecord) string {
	var b strings.Builder
	b.WriteString("# Mady 对话记录\n\n")
	fmt.Fprintf(&b, "**导出时间**: %s  \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "**会话ID**: %s  \n", threadID)
	if project != nil {
		fmt.Fprintf(&b, "**案件**: %s (%s)  \n", project.Alias, project.ProjectID)
	}
	b.WriteString("\n---\n\n")
	for _, msg := range msgs {
		switch msg.Role {
		case chat.RoleUser:
			b.WriteString("## 👤 用户\n\n")
		case chat.RoleAssistant:
			b.WriteString("## 🤖 助手\n\n")
		case chat.RoleSystem:
			b.WriteString("## 💬 系统\n\n")
		case chat.RoleTool:
			label := "## 🔧 工具"
			if msg.Meta != "" {
				label += " (" + msg.Meta + ")"
			}
			b.WriteString(label + "\n\n")
		case chat.RoleError:
			b.WriteString("## ❌ 错误\n\n")
		default:
			continue
		}
		if msg.Text != "" {
			b.WriteString(msg.Text)
			b.WriteString("\n\n")
		}
		b.WriteString("---\n\n")
	}
	return b.String()
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
		return agentcore.CloneThinkingConfig(current), false, nil
	}

	next := agentcore.CloneThinkingConfig(current)
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
