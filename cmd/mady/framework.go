package main

// 本文件负责共享框架装配：frameworkContext 封装 tui/serve/acp 三个入口
// 共用的初始化资源，setupFrameworkContext 执行 Provider 构建、MadyHome
// 解析、Manifest 加载（go:embed 内置 + 外部覆盖）、Skill/MCP 自动发现、
// workspace 与 ProjectRegistry 初始化；并含 reasoning 多源召回器、
// Router 配置的装配及通用装配辅助函数。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/doctmpl"
	"github.com/xujian519/mady/domains/reasoning"
	reasoningwiring "github.com/xujian519/mady/domains/reasoning/wiring"
	"github.com/xujian519/mady/domains/rules"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/knowledge"
	kgwgraph "github.com/xujian519/mady/knowledge/graph"
	ksqlite "github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/mcp"
	"github.com/xujian519/mady/memory"
	"github.com/xujian519/mady/memory/compiler"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/retrieval/domain"
	rsqlite "github.com/xujian519/mady/retrieval/domain/sqlite"
	"github.com/xujian519/mady/skill"
	"github.com/xujian519/mady/tools"
)

// pluginToolExtension wraps a single *agentcore.Tool into an Extension
// for registration into the agent's tool chain. This is a lightweight
// adapter that makes the run_plugin tool available as a standard Extension
// without modifying the tools package's ExtensionConfig.
type pluginToolExtension struct {
	agentcore.BaseLifecycleHook
	tool *agentcore.Tool
}

func (e *pluginToolExtension) Name() string                                     { return "plugin-tool" }
func (e *pluginToolExtension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }
func (e *pluginToolExtension) Dispose() error                                   { return nil }
func (e *pluginToolExtension) BuildTools() []*agentcore.Tool {
	if e.tool == nil {
		return nil
	}
	return []*agentcore.Tool{e.tool}
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
	// KnowledgeBackend 是已打开的 SQLite 知识库（FTS + 向量），
	// 供 reasoning Stage ② 规则召回等子系统复用，避免重复打开数据库。
	KnowledgeBackend knowledge.KnowledgeBackend
	// RuleEngine 是已加载的确定性规则引擎（domains/rules YAML），
	// 供 reasoning Stage ② 的第四路（RuleSourceRules）召回复用。
	RuleEngine *rules.Engine
	// WikiRoot 是 Obsidian wiki 根目录（~/.mady/knowledge/wiki 或 $WIKI_PATH），
	// 供 reasoning Stage ② 的 patent-cards 经验召回复用。可为 ""。
	WikiRoot string
	// MemoryManager 是长期记忆系统的核心协调器。
	// 所有入口（tui/serve/acp）共享同一个 Manager 实例。
	MemoryManager *memory.Manager
	// MemoryCompiler 是策略学习编译器，通过 ε-greedy 探索策略选择最佳执行路径。
	// 与 MemoryManager 不同，CompilerExtension 无 scope 依赖，直接注册到 BaseConfig。
	MemoryCompiler *compiler.Compiler
	// SessionSummarizer 是会话关闭时的异步汇总器。为 nil 时跳过汇总。
	SessionSummarizer *memory.SessionSummarizer
}

const startupMCPDiscoveryTimeout = 1500 * time.Millisecond

func withStartupDiscoveryTimeout(ctx context.Context, cmdName string) context.Context {
	if os.Getenv("MADY_MCP_DISCOVERY_TIMEOUT_MS") != "" {
		return ctx
	}
	switch cmdName {
	case "tui", "serve":
		return mcp.WithDiscoveryTimeout(ctx, startupMCPDiscoveryTimeout)
	default:
		return ctx
	}
}

// setupFrameworkContext 执行三个入口共享的初始化逻辑：
//   - Provider 构建
//   - MadyHome 解析（~/.mady，任意 cwd 可用）
//   - Manifest 加载（go:embed 内置 + MADY_HOME/manifests 外部覆盖）
//   - Wiki 知识库加载（可选的 WIKI_PATH 环境变量）
//   - ProjectRegistry 初始化
func setupFrameworkContext(ctx context.Context, cmdName string) *frameworkContext {
	ctx = withStartupDiscoveryTimeout(ctx, cmdName)
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
			ContextWindow:    agentconfig.ResolveContextWindow(model),
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
	fc.WikiStore, fc.WikiHook, fc.KnowledgeExt, fc.KnowledgeBackend = loadWikiStore(fc.MadyHome)

	// Wiki 根目录：供 reasoning Stage ② 的 patent-cards 经验召回读取。
	// 优先 $WIKI_PATH，否则 $MADY_HOME/knowledge/wiki（通常软链接到外部语料）。
	// 仅当目录存在时赋值，避免 SkillRuleReader 指向空路径。
	fc.WikiRoot = resolveWikiRoot(fc.MadyHome)

	// 确定性规则引擎：从 $MADY_HOME/knowledge/rules 加载 YAML（通常软链接到外部语料）。
	// 供 reasoning Stage ② 第四路（RuleSourceRules）召回 + chat agent 的 search_rules 工具。
	fc.RuleEngine, _ = rules.LoadEngineFromMadyHome()
	if fc.RuleEngine != nil {
		fmt.Fprintf(os.Stderr, "rules: 已加载规则引擎（%d 条规则）\n", len(fc.RuleEngine.AllRules()))
	}

	loadManifests(fc)
	discoverSkills(fc)
	discoverMCP(ctx, fc)
	initWorkspace(fc)
	buildBaseTools(fc)
	initPlugins(fc)
	initMemorySystem(fc)
	initReasoningAndTemplates(fc)

	return fc
}

// loadManifests 加载 go:embed 内置 + 外部覆盖的 AgentManifest 到 fc。
// 优先级：$MANIFEST_DIR > ~/.mady/manifests > 仅内置。
func loadManifests(fc *frameworkContext) {
	manifestDir := os.Getenv("MANIFEST_DIR")
	if manifestDir == "" && fc.MadyHome != "" {
		manifestDir = filepath.Join(fc.MadyHome, "manifests")
	}
	fc.ManifestDir = manifestDir
	mergeRes := agentcore.LoadManifests(manifestDir)
	fc.Manifests = mergeRes.Manifests

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
}

// discoverSkills 扫描多路径 SKILL.md 并注册到 BaseConfig。
// 优先级：$SKILL_DIR > $HOME/.agent > $PWD/.agent > ~/.mady/skills > ~/.agents/skills。
func discoverSkills(fc *frameworkContext) {
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
	if fc.MadyHome != "" {
		skillPaths = append(skillPaths, filepath.Join(fc.MadyHome, "skills"))
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		skillPaths = append(skillPaths, filepath.Join(homeDir, ".agents", "skills"))
	}
	loadedSkills, skillDiags, skillErr := skill.Load(skillPaths...)
	if skillErr != nil {
		fmt.Fprintf(os.Stderr, "skill: 加载失败: %v\n", skillErr)
		return
	}
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

// discoverMCP 自动发现并注册 MCP 扩展到 BaseConfig。
// 扫描 $MCP_CONFIG、~/.mady/mcp.json、$PWD/.mcp.json、~/.claude.json。
func discoverMCP(ctx context.Context, fc *frameworkContext) {
	mcpExts, mcpWarnings := mcp.DiscoverMCPExtensions(ctx, fc.MadyHome)
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
}

// initWorkspace 解析 workspace 目录、创建 projects 子目录、初始化 ProjectRegistry。
// 优先级：$WORKSPACE_DIR > ~/.mady/workspace。
func initWorkspace(fc *frameworkContext) {
	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir == "" {
		if fc.MadyHome != "" {
			workspaceDir = filepath.Join(fc.MadyHome, "workspace")
		} else {
			dir, err := util.ResolveDataDir("workspace")
			if err != nil {
				fmt.Fprintf(os.Stderr, "mady: 解析 workspace 目录失败，回退为空串: %v\n", err)
			}
			workspaceDir = dir
		}
	}
	if err := util.EnsureDir(filepath.Join(workspaceDir, "projects")); err != nil {
		fmt.Fprintf(os.Stderr, "mady: 创建 workspace 目录失败: %v\n", err)
	}
	fc.WorkspaceDir = workspaceDir
	fc.ProjectRegistry = domains.NewProjectRegistryOrEmpty(filepath.Join(workspaceDir, "projects"))
	fc.BaseConfig.WorkspaceDir = workspaceDir
	if cwd, err := os.Getwd(); err == nil {
		fc.BaseConfig.ProjectDir = cwd
	}
}

// buildBaseTools 为所有 Agent 注册基础文件工具和网络工具。
// 危险工具（bash/git/browser/execute_code/process/computer_use）默认关闭。
func buildBaseTools(fc *frameworkContext) {
	toolWorkingDir := fc.BaseConfig.ProjectDir
	if toolWorkingDir == "" {
		toolWorkingDir = fc.BaseConfig.WorkspaceDir
	}
	baseTools := tools.NewExtension(tools.ExtensionConfig{
		WorkingDir: toolWorkingDir,
		DisableTools: []string{
			tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
			tools.ToolBrowser, tools.ToolExecuteCode, tools.ToolProcess, tools.ToolComputerUse,
		},
	})
	fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, baseTools)
}

// initPlugins 从 plugins/ 目录发现并加载工作流插件。
// 为所有 Agent 注册 run_plugin 工具。
func initPlugins(fc *frameworkContext) {
	pluginSearchDirs := []string{}
	if cwd, err := os.Getwd(); err == nil {
		pluginSearchDirs = append(pluginSearchDirs, filepath.Join(cwd, "plugins"))
	}
	if fc.MadyHome != "" {
		pluginSearchDirs = append(pluginSearchDirs, filepath.Join(fc.MadyHome, "plugins"))
	}
	pluginManager, err := agentcore.NewPluginManager(fc.Provider, nil, pluginSearchDirs...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "plugin: 初始化插件管理器失败: %v（run_plugin 工具不可用）\n", err)
		return
	}
	plugins := pluginManager.Plugins()
	if len(plugins) > 0 {
		var names []string
		for _, p := range plugins {
			names = append(names, p.Name)
		}
		fmt.Fprintf(os.Stderr, "plugin: 已加载 %d 个插件（%s）\n", len(plugins), strings.Join(names, ", "))
		pluginTool := pluginManager.RunPluginTool()
		fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, &pluginToolExtension{tool: pluginTool})
	} else {
		fmt.Fprintf(os.Stderr, "plugin: 未发现任何插件（搜索路径: %v）\n", pluginSearchDirs)
	}
}

// initMemorySystem 初始化长期记忆系统，含 Embedder、MemoryStore（优先 SQLite）、
// BM25 混合检索、策略学习编译器、会话汇总器。
func initMemorySystem(fc *frameworkContext) {
	memoryDB := filepath.Join(fc.MadyHome, "memory.db")

	// 1. 构建 Embedder。
	var embedder retrieval.Embedder
	if embURL := os.Getenv("EMBEDDING_BASE_URL"); embURL != "" {
		embModel := os.Getenv("EMBEDDING_MODEL")
		if embModel == "" {
			embModel = "bge-m3"
		}
		embKey := os.Getenv("EMBEDDING_API_KEY")
		embedder = retrieval.NewAPIEmbedder(embURL, embKey, embModel)
		fmt.Fprintf(os.Stderr, "memory: Embedding 已启用 (model: %s, dims: %d)\n",
			embModel, embedder.Dimensions())
	} else {
		fmt.Fprintf(os.Stderr, "memory: 未配置 EMBEDDING_BASE_URL，使用关键词检索\n")
	}

	// 2. 构建 MemoryStore。
	var memoryStore memory.MemoryStore
	var storeOpts []memory.SQLiteOption
	if embedder != nil {
		storeOpts = append(storeOpts, memory.WithSQLiteEmbedder(embedder))
	}
	if fc.MadyHome != "" {
		ms, err := memory.NewSQLiteMemoryStore(memoryDB, storeOpts...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "memory: 打开 SQLite 存储失败 %s: %v（降级为 InMemoryStore）\n", memoryDB, err)
			memoryStore = memory.NewInMemoryStore(memory.WithEmbedder(embedder))
		} else {
			fmt.Fprintf(os.Stderr, "memory: SQLite 持久化存储已加载（%s）\n", memoryDB)
			memoryStore = ms
		}
	} else {
		memoryStore = memory.NewInMemoryStore(memory.WithEmbedder(embedder))
	}

	// 3. 构建 Extractor。
	var extractor *memory.Extractor
	managerCfg := memory.DefaultManagerConfig()
	if os.Getenv("MADY_MEMORY_AUTO_EXTRACT") == "1" {
		if fc.Provider != nil {
			model := agentconfig.DefaultModel()
			extractor = memory.NewExtractor(memory.NewProviderExtractor(fc.Provider, model), memory.DefaultExtractorConfig())
			managerCfg.AutoExtract = true
			fmt.Fprintf(os.Stderr, "memory: LLM 事实提取已启用 (model: %s)\n", model)
		} else {
			fmt.Fprintf(os.Stderr, "memory: MADY_MEMORY_AUTO_EXTRACT=1 但 Provider 不可用，跳过\n")
		}
	}

	fc.MemoryManager = memory.NewManager(memoryStore, extractor, nil, managerCfg)
	fc.MemoryManager.LogStats(context.Background())
	fmt.Fprintf(os.Stderr, "memory: 长期记忆系统已就绪\n")

	// 4. BM25 混合检索。
	if sqliteStore, ok := memoryStore.(*memory.SQLiteMemoryStore); ok {
		if bm25Idx, err := sqliteStore.BuildBM25Index(context.Background()); err == nil {
			fc.MemoryManager.SetBM25Index(bm25Idx)
			fmt.Fprintf(os.Stderr, "memory: BM25 混合检索已启用（%d 条索引）\n", bm25Idx.Size())
		} else {
			fmt.Fprintf(os.Stderr, "memory: BM25 索引构建失败: %v（退化为纯稠密检索）\n", err)
		}
	}

	// 5. 策略学习编译器。
	fc.MemoryCompiler = compiler.NewCompiler(compiler.Config{
		ExplorationRate: 5,
		MaxTraces:       1000,
	})
	fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, compiler.NewExtension(fc.MemoryCompiler))
	fmt.Fprintf(os.Stderr, "compiler: 策略学习系统已就绪（%d 个预设策略）\n",
		len(fc.MemoryCompiler.Strategies()))

	// 6. 会话汇总器。
	if fc.Provider != nil && os.Getenv("MADY_MEMORY_AUTO_EXTRACT") == "1" {
		fc.SessionSummarizer = memory.NewSessionSummarizer(fc.Provider, agentconfig.DefaultModel())
		fmt.Fprintf(os.Stderr, "memory: 会话汇总器已启用\n")
	}

	fc.KnowledgeGraph = kgwgraph.NewGraphStore()
}

// initReasoningAndTemplates 初始化推理引擎 retriever/LLM 客户端、文档模板仓库、
// 引用核验装配（CitationGate 留痕 store），以及专利新颖性分析的检索器。
func initReasoningAndTemplates(fc *frameworkContext) {
	retriever := buildReasoningRetriever(fc)
	var llmClient reasoning.LlmClient
	if fc.Provider != nil {
		llmClient = reasoning.NewLlmClientFromProvider(fc.Provider, agentconfig.DefaultModel())
	}
	domains.SetupPatentDraftingEngine(retriever, llmClient)

	// 专利新颖性分析现有技术检索器：从已打开的知识库构建 PatentDomainRetriever。
	// 配置后 analyze_patent_novelty 工具的 search 节点将使用本地专利知识库的 FTS5
	// 检索返回专利文献作为证据，替代默认的占位文本。
	var patentRetriever domain.DomainRetriever
	if fc.KnowledgeBackend != nil {
		if store, ok := fc.KnowledgeBackend.(*ksqlite.SQLiteStore); ok {
			patentRetriever = rsqlite.NewPatentDomainRetriever(store)
		}
	}
	domains.SetupPatentRetriever(patentRetriever)

	userTmplDir := filepath.Join(fc.MadyHome, "doc-templates")
	store, err := doctmpl.NewTemplateStore(userTmplDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctmpl: 加载模板仓库失败: %v（模板工具不可用）\n", err)
	} else {
		domains.SetupDocTemplateStore(store)
	}

	approvalDB := filepath.Join(fc.WorkspaceDir, "approvals.db")
	var citationStore domains.ApprovalStore
	if store, err := sqlitestore.NewApprovalStore(approvalDB); err == nil {
		citationStore = store
	} else {
		fmt.Fprintf(os.Stderr, "citation: 打开留痕数据库失败 %s: %v（降级为内存存储）\n", approvalDB, err)
		citationStore = domains.NewMemoryApprovalStore()
	}
	domains.SetupCitationWiring(domains.CitationWiring{
		Source: nil,
		Store:  citationStore,
	})
}

// buildReasoningRetriever 从框架上下文中构造 MultiSourceRetriever。
// 当任一规则源可用时创建适配链，否则返回 nil（Stage ② 跳过）。
//
// 四路规则召回的装配（对齐 design-rule-acquisition-stage.md 权威性分层）：
//   - Rules 路（RuleSourceRules）：确定性规则引擎 YAML（权威性最高 0.95），依赖 RuleEngine
//   - KG 路（RuleSourceKG）：知识图谱多跳遍历，依赖 KnowledgeGraph
//   - Vector 路（RuleSourceVector）：FTS 全文检索，依赖 KnowledgeBackend
//   - Skill 路（RuleSourceSkill）：wiki patent-cards 经验召回，依赖 WikiRoot
func buildReasoningRetriever(fc *frameworkContext) *reasoning.MultiSourceRetriever {
	if fc.KnowledgeGraph == nil && fc.KnowledgeBackend == nil && fc.WikiRoot == "" && fc.RuleEngine == nil {
		return nil
	}
	var walker *reasoning.ReasoningWalker
	if fc.KnowledgeGraph != nil {
		adapter := kgwgraph.NewReasoningStoreAdapter(fc.KnowledgeGraph)
		walker = reasoning.NewReasoningWalker(adapter, nil)
	}
	var vs reasoning.RuleVectorStore
	if fc.KnowledgeBackend != nil {
		vs = reasoningwiring.NewVectorRuleStore(fc.KnowledgeBackend)
	}
	var sr reasoning.RuleSkillReader
	if fc.WikiRoot != "" {
		sr = reasoningwiring.NewSkillRuleReader(fc.WikiRoot)
	}
	var re reasoning.RuleEngineSource
	if fc.RuleEngine != nil {
		re = reasoningwiring.NewRuleEngineAdapter(fc.RuleEngine)
	}
	return reasoning.NewMultiSourceRetriever(walker, vs, sr, re)
}

// buildRouterConfig 根据可用的 Manifest 构建 Router Agent 配置。
// 有 Manifest 时使用声明式注册，没有时回退到硬编码 RouterConfig。
func buildRouterConfig(base agentcore.Config, manifests []agentcore.AgentManifest) agentcore.Config {
	if len(manifests) > 0 {
		return domains.RouterConfigFromManifests(base, manifests)
	}
	return domains.RouterConfig(base)
}

// extSlice wraps a single Extension into a slice, returning nil for nil input.
func extSlice(ext agentcore.Extension) []agentcore.Extension {
	if ext == nil {
		return nil
	}
	return []agentcore.Extension{ext}
}

// agentThinking 将 agentconfig.ThinkingConfig 转换为 agentcore.ThinkingConfig。
func agentThinking(cfg *agentconfig.ThinkingConfig) *agentcore.ThinkingConfig {
	if cfg == nil {
		return nil
	}
	return &agentcore.ThinkingConfig{
		IncludeThoughts: cfg.IncludeThoughts,
		Display:         agentcore.ThinkingDisplay(cfg.Display),
		Effort:          agentcore.ThinkingEffort(cfg.Effort),
		Budget:          cfg.Budget,
	}
}
