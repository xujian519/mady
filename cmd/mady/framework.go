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

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/doctmpl"
	"github.com/xujian519/mady/domains/reasoning"
	reasoningwiring "github.com/xujian519/mady/domains/reasoning/wiring"
	"github.com/xujian519/mady/domains/rules"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/knowledge"
	kgwgraph "github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/mcp"
	"github.com/xujian519/mady/memory"
	"github.com/xujian519/mady/memory/compiler"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/skill"
	"github.com/xujian519/mady/tools"
)

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

	// Skill 自动发现：扫描 $SKILL_DIR、$HOME/.agent、$PWD/.agent、~/.mady/skills、
	// ~/.agents/skills/。
	// 优先级：$SKILL_DIR > $HOME/.agent > $PWD/.agent > ~/.mady/skills > ~/.agents/skills。
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
	if homeDir, err := os.UserHomeDir(); err == nil {
		skillPaths = append(skillPaths, filepath.Join(homeDir, ".agents", "skills"))
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
	// MadyHome() 最终回退会调 filepath.Abs("./.mady")，故此处不会出现 cwd 相对路径。
	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir == "" {
		if madyHome != "" {
			workspaceDir = filepath.Join(madyHome, "workspace")
		} else {
			// 不可达兜底：MadyHome() 仅在 filepath.Abs 自身失败时返错。
			// 走 ResolveDataDir 以保证最终路径仍经过 filepath.Abs 规范化。
			dir, err := util.ResolveDataDir("workspace")
			if err != nil {
				fmt.Fprintf(os.Stderr, "mady: 解析 workspace 目录失败，回退为空串: %v\n", err)
			}
			workspaceDir = dir
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

	// 内置工具扩展：为所有 Agent 提供基础文件工具（read/edit/write_file/ls/grep/find/glob/view）
	// 和网络工具（web_search/web_fetch）。领域 Agent 工厂函数（AssistantAgentConfig 等）
	// 在此基础之上叠加领域特定配置（沙箱、禁用列表等）。
	// 不启用沙箱：BaseConfig 是共享基础，沙箱由领域工厂函数按需开启。
	// 不启用危险工具：bash/git/browser/execute_code/process/computer_use 默认关闭。
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

	// 插件系统：从 plugins/ 目录发现并加载 patent/legal 工作流插件。
	// 为所有 Agent 注册 run_plugin 工具，使其可按名称调用插件工作流。
	// 扫描当前工作目录和 MadyHome 下的 plugins 目录。
	pluginSearchDirs := []string{}
	if cwd, err := os.Getwd(); err == nil {
		pluginSearchDirs = append(pluginSearchDirs, filepath.Join(cwd, "plugins"))
	}
	if madyHome != "" {
		pluginSearchDirs = append(pluginSearchDirs, filepath.Join(madyHome, "plugins"))
	}
	pluginManager, err := agentcore.NewPluginManager(fc.Provider, nil, pluginSearchDirs...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "plugin: 初始化插件管理器失败: %v（run_plugin 工具不可用）\n", err)
	} else {
		plugins := pluginManager.Plugins()
		if len(plugins) > 0 {
			var names []string
			for _, p := range plugins {
				names = append(names, p.Name)
			}
			fmt.Fprintf(os.Stderr, "plugin: 已加载 %d 个插件（%s）\n", len(plugins), strings.Join(names, ", "))
			// 将 run_plugin 工具注册到 BaseConfig 的内建工具列表中。
			// 通过嵌入一个仅含此工具的轻量 Extension 实现。
			pluginTool := pluginManager.RunPluginTool()
			fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, &pluginToolExtension{tool: pluginTool})
		} else {
			fmt.Fprintf(os.Stderr, "plugin: 未发现任何插件（搜索路径: %v）\n", pluginSearchDirs)
		}
	}

	// 长期记忆系统：优先 SQLite 持久化，回退 InMemoryStore。
	// SQLite 文件位于 MADY_HOME/memory.db，所有 Agent 共享同一个存储后端，
	// 支持跨会话持久化和向量检索。
	memoryDB := filepath.Join(madyHome, "memory.db")

	// 1. 构建 Embedder（向量语义检索）。
	// 通过环境变量 EMBEDDING_BASE_URL / EMBEDDING_API_KEY / EMBEDDING_MODEL 配置。
	// 未配置时 embedder 为 nil，Recall 自动降级为关键词匹配。
	var embedder retrieval.Embedder
	if embURL := os.Getenv("EMBEDDING_BASE_URL"); embURL != "" {
		embModel := os.Getenv("EMBEDDING_MODEL")
		if embModel == "" {
			embModel = "bge-m3" // 默认 BGE-M3，中英文效果好
		}
		embKey := os.Getenv("EMBEDDING_API_KEY")
		embedder = retrieval.NewAPIEmbedder(embURL, embKey, embModel)
		fmt.Fprintf(os.Stderr, "memory: Embedding 已启用 (model: %s, dims: %d)\n",
			embModel, embedder.Dimensions())
	} else {
		fmt.Fprintf(os.Stderr, "memory: 未配置 EMBEDDING_BASE_URL，使用关键词检索\n")
	}

	// 2. 构建 MemoryStore（优先 SQLite + embedding）。
	var memoryStore memory.MemoryStore
	var storeOpts []memory.SQLiteOption
	if embedder != nil {
		storeOpts = append(storeOpts, memory.WithSQLiteEmbedder(embedder))
	}
	if madyHome != "" {
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

	// 3. 构建 Extractor（LLM 原子事实提取）。
	// 通过环境变量 MADY_MEMORY_AUTO_EXTRACT 控制（默认关闭，保持向后兼容）。
	var extractor *memory.Extractor
	managerCfg := memory.DefaultManagerConfig()
	if os.Getenv("MADY_MEMORY_AUTO_EXTRACT") == "1" {
		if fc.Provider != nil {
			model := agentconfig.DefaultModel()
			llmExtractor := memory.NewProviderExtractor(fc.Provider, model)
			extractor = memory.NewExtractor(llmExtractor, memory.DefaultExtractorConfig())
			managerCfg.AutoExtract = true
			fmt.Fprintf(os.Stderr, "memory: LLM 事实提取已启用 (model: %s)\n", model)
		} else {
			fmt.Fprintf(os.Stderr, "memory: MADY_MEMORY_AUTO_EXTRACT=1 但 Provider 不可用，跳过\n")
		}
	}

	fc.MemoryManager = memory.NewManager(memoryStore, extractor, nil, managerCfg)
	// 在 BaseConfig 中注册 MemoryExtension 供所有入口复用。
	// MemoryScope 在入口层填充（因 UserID/SessionID 仅在会话上下文中可知）。
	// 此处仅创建共享存储与管理器，扩展实例由入口层（tui/serve/acp）按需创建。
	fc.MemoryManager.LogStats(context.Background())
	fmt.Fprintf(os.Stderr, "memory: 长期记忆系统已就绪\n")

	// 4. 构建 BM25 索引并注入 Retriever（启用混合检索：稠密向量 + BM25 稀疏 + RRF 融合）。
	// 仅 SQLite 持久化存储支持 BM25（需全量扫描构建索引）。
	if sqliteStore, ok := memoryStore.(*memory.SQLiteMemoryStore); ok {
		if bm25Idx, err := sqliteStore.BuildBM25Index(context.Background()); err == nil {
			fc.MemoryManager.SetBM25Index(bm25Idx)
			fmt.Fprintf(os.Stderr, "memory: BM25 混合检索已启用（%d 条索引）\n", bm25Idx.Size())
		} else {
			fmt.Fprintf(os.Stderr, "memory: BM25 索引构建失败: %v（退化为纯稠密检索）\n", err)
		}
	}

	// 策略学习编译器：注册 CompilerExtension 到 BaseConfig 供所有入口复用。
	// 与 MemoryExtension 不同，CompilerExtension 无 scope 依赖（策略选择只依赖 goal 文本），
	// 因此直接注册到 BaseConfig.Extensions，无需入口层按需创建。
	fc.MemoryCompiler = compiler.NewCompiler(compiler.Config{
		ExplorationRate: 5, // 5% ε-greedy 探索率
		MaxTraces:       1000,
	})
	fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions,
		compiler.NewExtension(fc.MemoryCompiler))
	fmt.Fprintf(os.Stderr, "compiler: 策略学习系统已就绪（%d 个预设策略）\n",
		len(fc.MemoryCompiler.Strategies()))

	// 会话汇总器：当 Provider 可用且 MADY_MEMORY_AUTO_EXTRACT=1 时启用。
	// 在会话关闭时从 Session 层提取长期事实存入 LongTerm 层。
	if fc.Provider != nil && os.Getenv("MADY_MEMORY_AUTO_EXTRACT") == "1" {
		fc.SessionSummarizer = memory.NewSessionSummarizer(fc.Provider, agentconfig.DefaultModel())
		fmt.Fprintf(os.Stderr, "memory: 会话汇总器已启用\n")
	}

	// 初始化知识图谱（空存储，由 wiki import 或数据管线填充）。
	fc.KnowledgeGraph = kgwgraph.NewGraphStore()

	// 专利撰写推理引擎注入。
	// 构建 retriever（可能为 nil，FiveStepRunner 内部可降级）和 LLM 客户端，
	// 使 PatentAgentConfig 创建的所有 Agent 实例均可调用 run_five_step_workflow 工具。
	retriever := buildReasoningRetriever(fc)
	var llmClient reasoning.LlmClient
	if fc.Provider != nil {
		llmClient = reasoning.NewLlmClientFromProvider(fc.Provider, agentconfig.DefaultModel())
	}
	domains.SetupPatentDraftingEngine(retriever, llmClient)

	// 文档模板仓库：加载内嵌模板 + 用户自定义模板
	userTmplDir := filepath.Join(fc.MadyHome, "doc-templates")
	store, err := doctmpl.NewTemplateStore(userTmplDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctmpl: 加载模板仓库失败: %v（模板工具不可用）\n", err)
	} else {
		domains.SetupDocTemplateStore(store)
	}

	// 引用核验装配注入：使 PatentAgentConfig 中的 CitationGate 运行在
	// P2b Strict 模式（带留痕 store）。Source 为 nil 时 Gate 退回 S1 静态表。
	approvalDB := filepath.Join(fc.WorkspaceDir, "approvals.db")
	var citationStore domains.ApprovalStore
	if store, err := sqlitestore.NewApprovalStore(approvalDB); err == nil {
		citationStore = store
	} else {
		fmt.Fprintf(os.Stderr, "citation: 打开留痕数据库失败 %s: %v（降级为内存存储）\n", approvalDB, err)
		citationStore = domains.NewMemoryApprovalStore()
	}
	domains.SetupCitationWiring(domains.CitationWiring{
		Source: nil, // nil → 退回 S1 静态表（zero-dep 默认源）
		Store:  citationStore,
	})

	return fc
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
