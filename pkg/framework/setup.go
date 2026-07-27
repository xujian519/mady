// Package framework 提供所有 mady 入口（tui/serve/acp/desktop）共享的装配逻辑。
package framework

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/agentcore/filecheckpoint"
	"github.com/xujian519/mady/agentcore/planmode"
	"github.com/xujian519/mady/agentcore/tasklist"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/doctmpl"
	domainEvidence "github.com/xujian519/mady/domains/evidence"
	"github.com/xujian519/mady/domains/reasoning"
	reasoningwiring "github.com/xujian519/mady/domains/reasoning/wiring"
	"github.com/xujian519/mady/domains/rules"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/guardrails/guardian"
	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/fileindex"
	kgwgraph "github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/knowledge/loader"
	ksqlite "github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/mcp"
	"github.com/xujian519/mady/memory"
	"github.com/xujian519/mady/memory/compiler"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/lawcite"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/prompt"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/retrieval/domain"
	rsqlite "github.com/xujian519/mady/retrieval/domain/sqlite"
	"github.com/xujian519/mady/skill"
	"github.com/xujian519/mady/tools"
	"github.com/xujian519/mady/tracing"
)

// Mode 控制初始化是同步完成还是分两阶段（首帧必需 + 后台延迟）。
type Mode int

const (
	// ModeSync 全同步初始化，serve/acp/desktop 入口使用。
	ModeSync Mode = iota
	// ModeDeferred 首帧必需部分同步，其余后台延迟，TUI 入口使用。
	ModeDeferred
)

// Options 控制 Setup 的行为。
type Options struct {
	Mode    Mode
	CmdName string // "tui", "serve", "acp", "desktop"
}

// Context 封装入口之间共享的初始化资源。
type Context struct {
	BaseConfig      agentcore.Config
	ProjectRegistry *domains.ProjectRegistry
	// CaseIndex 是基于 SQLite 的案件索引库，替代 ProjectRegistry 的核心功能。
	CaseIndex    *domains.CaseIndex
	WikiHook     agentcore.LifecycleHook //nolint:staticcheck // legacy hook type retained for backward compat
	WikiStore    *knowledge.Store
	KnowledgeExt agentcore.Extension
	Manifests    []agentcore.AgentManifest
	Provider     agentcore.Provider
	MadyHome     string
	PromptStore  *prompt.PromptStore
	WorkspaceDir string
	ManifestDir  string
	// KnowledgeGraph 是实体-关系知识图谱，用于多跳推理遍历。
	KnowledgeGraph *kgwgraph.GraphStore
	// KnowledgeBackend 是已打开的 SQLite 知识库（FTS + 向量）。
	KnowledgeBackend knowledge.KnowledgeBackend
	// RuleEngine 是已加载的确定性规则引擎（domains/rules YAML）。
	RuleEngine *rules.Engine
	// WikiRoot 是 Obsidian wiki 根目录（~/.mady/knowledge/wiki 或 $WIKI_PATH）。
	WikiRoot       string
	MemoryManager  *memory.Manager
	MemoryCompiler *compiler.Compiler
	CompilerDBPath string
	// SessionSummarizer 是会话关闭时的异步汇总器。为 nil 时跳过汇总。
	SessionSummarizer *memory.SessionSummarizer
	PlanModeExt       *planmode.PlanModeExtension
	TracerFlush       func(context.Context) error
	GuardianExt       *guardian.GuardianExtension
	EvidenceExt       *evidence.EvidenceExtension
	FileCheckpointExt *filecheckpoint.FileCheckpointExtension
	Deferred          *DeferredInit
}

// pluginToolExtension wraps a single *agentcore.Tool into an Extension.
type pluginToolExtension struct {
	agentcore.BaseLifecycleHook
	tool *agentcore.Tool
}

func (e *pluginToolExtension) Name() string { return "plugin-tool" }
func (e *pluginToolExtension) Init(_ context.Context, _ *agentcore.Agent) error {
	return nil
}
func (e *pluginToolExtension) Dispose() error { return nil }
func (e *pluginToolExtension) BuildTools() []*agentcore.Tool {
	if e.tool == nil {
		return nil
	}
	return []*agentcore.Tool{e.tool}
}

// caseFileReader implements domains.FileContentReader by wrapping fileindex.FileReader
// with an os.ReadFile fallback.
type CaseFileReader struct{}

func (CaseFileReader) ReadText(path string) string {
	dir := filepath.Dir(path)
	reader := fileindex.NewFileReader(dir)
	if result, err := reader.ReadProjectFile(context.Background(), filepath.Base(path)); err == nil {
		return result.Content
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// docxRendererAdapter 适配 doctmpl.Renderer 的 Render(md, meta) 签名
// 到 disclosure.DOCXConverter 的 Render(md) 签名，忽略 meta 参数。
type docxRendererAdapter struct {
	docx doctmpl.Renderer
}

func (a docxRendererAdapter) Render(markdownBody string) ([]byte, error) {
	return a.docx.Render(markdownBody, doctmpl.RenderMeta{})
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

// CwdPartitionName returns a short, filesystem-safe identifier for a working
// directory. It uses the first 16 hex chars of SHA-256 so names are stable
// across restarts and avoid special-character issues on any platform.
func CwdPartitionName(cwd string) string {
	sum := sha256.Sum256([]byte(cwd))
	return hex.EncodeToString(sum[:])[:16]
}

// Setup 执行所有入口共享的初始化逻辑：
//   - Provider 构建
//   - MadyHome 解析（~/.mady，任意 cwd 可用）
//   - Manifest 加载（go:embed 内置 + MADY_HOME/manifests 外部覆盖）
//   - Wiki 知识库加载（可选的 WIKI_PATH 环境变量）
//   - ProjectRegistry 初始化
//
// 当 opts.Mode == ModeDeferred 时，重型初始化（MCP/memory/reasoning/plugins等）
// 注册到 fc.Deferred 延迟队列而非立即执行，由调用方在初始化完成后
// 通过 fc.Deferred.StartAll() 在后台完成。
func Setup(ctx context.Context, opts Options) (*Context, error) {
	ctx = withStartupDiscoveryTimeout(ctx, opts.CmdName)
	fc := &Context{}

	// === 首帧必需：立即执行 ===

	provider, err := agentconfig.BuildProvider()
	if err != nil {
		return nil, fmt.Errorf("构建 Provider 失败: %w", err)
	}
	model := agentconfig.DefaultModel()

	madyHome, err := util.MadyHome()
	if err != nil {
		slog.Warn("初始化数据目录失败，将使用 cwd 相对路径", "error", err)
		madyHome = ""
	} else {
		slog.Info("数据目录", "path", madyHome)
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

	// 模型级联回退候选链。
	if fbCfg := LoadFallbackConfig(); fbCfg != nil {
		fc.BaseConfig.FallbackConfig = fbCfg
	}

	// OpenTelemetry 分布式追踪。
	if tracingMode := os.Getenv("MADY_TRACING"); tracingMode != "" {
		switch tracingMode {
		case "stdout":
			tracer, flushFn, err := tracing.NewStdoutTracer("mady")
			if err != nil {
				slog.Error("初始化 stdout tracer 失败", "error", err)
			} else {
				fc.BaseConfig.Tracer = tracer
				fc.TracerFlush = flushFn
				slog.Info("stdout tracer 已启用")
			}
		default:
			slog.Warn("未知 tracing 模式", "mode", tracingMode)
		}
	}

	// Manifest 加载。
	LoadManifests(fc)

	// 用户自定义风格目录。
	if fc.MadyHome != "" {
		domains.AddStylePath(filepath.Join(fc.MadyHome, "styles"))
	}

	// === 后台延迟阶段 ===
	deferBackground := (opts.Mode == ModeDeferred)

	if deferBackground {
		fc.Deferred = NewDeferredInit()
		registerDeferredTasks(ctx, fc)
	} else {
		executeSyncRemaining(ctx, fc)
	}

	return fc, nil
}

// registerDeferredTasks 注册所有非关键初始化任务到 fc.Deferred。
func registerDeferredTasks(ctx context.Context, fc *Context) {
	fc.Deferred.Add("wikistore", func() error {
		fc.WikiStore, fc.WikiHook, fc.KnowledgeExt, fc.KnowledgeBackend = LoadWikiStore(fc.MadyHome)
		return nil
	})
	fc.Deferred.Add("wikiroot", func() error {
		fc.WikiRoot = ResolveWikiRoot(fc.MadyHome)
		return nil
	})

	fc.Deferred.Add("rules", func() error {
		engine, err := rules.LoadEngineFromMadyHome()
		if err != nil {
			return err
		}
		fc.RuleEngine = engine
		if engine != nil {
			slog.Info("已加载规则引擎", "count", len(engine.AllRules()))
		}
		return nil
	})

	fc.Deferred.Add("skills", func() error {
		DiscoverSkills(fc)
		return nil
	})

	fc.Deferred.Add("mcp", func() error {
		DiscoverMCP(ctx, fc)
		return nil
	})

	fc.Deferred.Add("workspace", func() error {
		InitWorkspace(fc)
		return nil
	})

	fc.Deferred.Add("tools", func() error {
		BuildBaseTools(fc)
		return nil
	})

	fc.Deferred.Add("plugins", func() error {
		InitPlugins(fc)
		return nil
	})

	fc.Deferred.Add("memory", func() error {
		InitMemorySystem(fc)
		return nil
	})

	fc.Deferred.Add("reasoning", func() error {
		InitReasoningAndTemplates(fc)
		return nil
	})
}

// executeSyncRemaining 同步执行所有剩余初始化（serve/acp 入口使用）。
func executeSyncRemaining(ctx context.Context, fc *Context) {
	fc.WikiStore, fc.WikiHook, fc.KnowledgeExt, fc.KnowledgeBackend = LoadWikiStore(fc.MadyHome)
	fc.WikiRoot = ResolveWikiRoot(fc.MadyHome)

	fc.RuleEngine, _ = rules.LoadEngineFromMadyHome()
	if fc.RuleEngine != nil {
		slog.Info("已加载规则引擎", "count", len(fc.RuleEngine.AllRules()))
	}

	DiscoverSkills(fc)
	DiscoverMCP(ctx, fc)
	InitWorkspace(fc)
	BuildBaseTools(fc)
	InitPlugins(fc)
	InitMemorySystem(fc)
	InitReasoningAndTemplates(fc)
}

// LoadManifests 加载 go:embed 内置 + 外部覆盖的 AgentManifest 到 fc。
func LoadManifests(fc *Context) {
	manifestDir := os.Getenv("MANIFEST_DIR")
	if manifestDir == "" && fc.MadyHome != "" {
		manifestDir = filepath.Join(fc.MadyHome, "manifests")
	}
	fc.ManifestDir = manifestDir
	mergeRes := agentcore.LoadManifests(manifestDir)
	fc.Manifests = mergeRes.Manifests

	if mergeRes.EmbeddedCount > 0 {
		slog.Info("已加载内置 Agent（embed）", "count", mergeRes.EmbeddedCount)
	}
	if mergeRes.ExternalCount > 0 {
		attrs := []any{"count", mergeRes.ExternalCount, "from", manifestDir}
		if len(mergeRes.Overridden) > 0 {
			attrs = append(attrs, "overridden", strings.Join(mergeRes.Overridden, ", "))
		}
		if len(mergeRes.Added) > 0 {
			attrs = append(attrs, "added", strings.Join(mergeRes.Added, ", "))
		}
		slog.Info("加载外部 Agent", attrs...)
	}
	for _, m := range fc.Manifests {
		slog.Info("  - manifest", "name", m.Name, "domain", m.Domain)
	}
	for _, e := range mergeRes.Errors {
		slog.Warn("manifest 加载警告", "path", e.Path, "error", e.Error)
	}
	if len(fc.Manifests) == 0 {
		slog.Warn("未加载任何 manifest，将回退到单 Agent 模式")
	}
}

// DiscoverSkills 扫描多路径 SKILL.md 并注册到 BaseConfig。
func DiscoverSkills(fc *Context) {
	var skillPaths []string
	if sd := os.Getenv("SKILL_DIR"); sd != "" {
		skillPaths = append(skillPaths, sd)
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		skillPaths = append(skillPaths, filepath.Join(homeDir, ".agent"))
	}
	if cwd, err := os.Getwd(); err == nil {
		skillPaths = append(skillPaths, filepath.Join(cwd, ".agent"), filepath.Join(cwd, "skills"))
	}
	if fc.MadyHome != "" {
		skillPaths = append(skillPaths, filepath.Join(fc.MadyHome, "skills"))
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		skillPaths = append(skillPaths, filepath.Join(homeDir, ".agents", "skills"))
	}
	loadedSkills, skillDiags, skillErr := skill.Load(skillPaths...)
	if skillErr != nil {
		slog.Error("skill 加载失败", "error", skillErr)
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
		slog.Info("加载 skill", "path_count", len(skillPaths), "skill_count", len(loadedSkills), "names", strings.Join(names, ", "))
	}
	if len(skillDiags) > 0 {
		for _, d := range skillDiags {
			slog.Warn("skill 诊断", "path", d.Path, "message", d.Message)
		}
	}
}

// DiscoverMCP 自动发现并注册 MCP 扩展到 BaseConfig。
func DiscoverMCP(ctx context.Context, fc *Context) {
	mcpExts, mcpWarnings := mcp.DiscoverMCPExtensions(ctx, fc.MadyHome)
	for _, w := range mcpWarnings {
		slog.Warn("MCP", "warning", w)
	}
	if len(mcpExts) > 0 {
		var names []string
		for _, ext := range mcpExts {
			names = append(names, ext.Name())
		}
		slog.Info("已加载 MCP 服务器", "count", len(mcpExts), "names", strings.Join(names, ", "))
		fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, mcpExts...)
	}
}

// InitWorkspace 解析 workspace 目录、创建 projects 子目录、初始化 ProjectRegistry。
func InitWorkspace(fc *Context) {
	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir == "" {
		if fc.MadyHome != "" {
			workspaceDir = filepath.Join(fc.MadyHome, "workspace")
		} else {
			dir, err := util.ResolveDataDir("workspace")
			if err != nil {
				slog.Error("解析 workspace 目录失败，回退为空串", "error", err)
			}
			workspaceDir = dir
		}
	}
	if err := util.EnsureDir(filepath.Join(workspaceDir, "projects")); err != nil {
		slog.Error("创建 workspace 目录失败", "error", err)
	}
	fc.WorkspaceDir = workspaceDir
	fc.ProjectRegistry = domains.NewProjectRegistryOrEmpty(filepath.Join(workspaceDir, "projects"))
	if ci, err := domains.NewCaseIndex(filepath.Join(workspaceDir, "cases.db")); err != nil {
		slog.Error("案件索引库初始化失败，回退到 ProjectRegistry", "error", err)
	} else {
		fc.CaseIndex = ci
	}
	fc.BaseConfig.WorkspaceDir = workspaceDir
	if cwd, err := os.Getwd(); err == nil {
		fc.BaseConfig.ProjectDir = cwd
	}
}

// BuildBaseTools 为所有 Agent 注册基础文件工具和网络工具。
func BuildBaseTools(fc *Context) {
	toolWorkingDir := fc.BaseConfig.ProjectDir
	if toolWorkingDir == "" {
		toolWorkingDir = fc.BaseConfig.WorkspaceDir
	}
	baseTools := tools.NewExtension(tools.ExtensionConfig{
		WorkingDir: toolWorkingDir,
	})
	fc.FileCheckpointExt = filecheckpoint.NewExtension(toolWorkingDir)
	fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions,
		baseTools,
		fc.FileCheckpointExt,
	)

	fc.PlanModeExt = planmode.NewExtension(planmode.Policy{})
	fc.EvidenceExt = evidence.NewExtension()
	fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions,
		fc.PlanModeExt,
		fc.EvidenceExt,
		domainEvidence.NewDomainExtension(nil),
		domains.NewDeadlineCalculatorExtension(),
	)

	if taskDir, err := util.ResolveDataDir("sessions"); err == nil {
		taskDir = TasklistDirForCWD(taskDir, fc.BaseConfig.ProjectDir)
		if taskExt, err := tasklist.NewExtension(taskDir); err == nil {
			fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, taskExt)
		}
	}

	// 审计日志扩展：当 MADY_HOME 可用时启用（可通过空 MadyHome 禁用）。
	if auditBase, err := util.MadyHome(); err == nil {
		if auditExt, err := domains.NewAuditExtension(auditBase, "mady-agent"); err == nil && auditExt != nil {
			fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, auditExt)
			slog.Info("审计日志已启用", "base", auditBase)
		}
	}

	if os.Getenv("MADY_EGOLITE") == "1" {
		if egoExt, err := tools.NewEgoLiteExtension(tools.EgoLiteConfig{
			Enabled:  true,
			TaskName: "mady-agent",
		}); err == nil {
			fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, egoExt)
		}
	}

	if os.Getenv("MADY_GUARDIAN") == "1" && fc.Provider != nil {
		session := guardian.NewSession(guardian.Config{
			Provider: fc.Provider,
			Model:    agentconfig.DefaultModel(),
		})
		fc.GuardianExt = guardian.NewExtension(session)
		fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, fc.GuardianExt)
		slog.Info("AI 安全审查已启用（熔断器保护）")
	}
}

// tasklistDirForCWD 按启动工作目录对 tasklist 存储目录分区。
func TasklistDirForCWD(baseDir, cwd string) string {
	if cwd == "" {
		return filepath.Join(baseDir, "tasks")
	}
	return filepath.Join(baseDir, "by-cwd", CwdPartitionName(cwd), "tasks")
}

// InitPlugins 从 plugins/ 目录发现并加载工作流插件。
func InitPlugins(fc *Context) {
	pluginSearchDirs := []string{}
	if cwd, err := os.Getwd(); err == nil {
		pluginSearchDirs = append(pluginSearchDirs, filepath.Join(cwd, "plugins"))
	}
	if fc.MadyHome != "" {
		pluginSearchDirs = append(pluginSearchDirs, filepath.Join(fc.MadyHome, "plugins"))
	}
	pluginManager, err := agentcore.NewPluginManager(fc.Provider, nil, pluginSearchDirs...)
	if err != nil {
		slog.Error("初始化插件管理器失败，run_plugin 工具不可用", "error", err)
		return
	}
	plugins := pluginManager.Plugins()
	if len(plugins) > 0 {
		var names []string
		for _, p := range plugins {
			names = append(names, p.Name)
		}
		slog.Info("已加载插件", "count", len(plugins), "names", strings.Join(names, ", "))
		pluginTool := pluginManager.RunPluginTool()
		fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, &pluginToolExtension{tool: pluginTool})
	} else {
		slog.Info("未发现任何插件", "search_dirs", fmt.Sprintf("%v", pluginSearchDirs))
	}
}

// InitMemorySystem 初始化长期记忆系统。
func InitMemorySystem(fc *Context) {
	memoryDB := filepath.Join(fc.MadyHome, "memory.db")

	var embedder retrieval.Embedder
	if embURL := os.Getenv("EMBEDDING_BASE_URL"); embURL != "" {
		embModel := os.Getenv("EMBEDDING_MODEL")
		if embModel == "" {
			embModel = "bge-m3"
		}
		embKey := os.Getenv("EMBEDDING_API_KEY")
		embedder = retrieval.NewAPIEmbedder(embURL, embKey, embModel)
		slog.Info("Embedding 已启用", "model", embModel, "dims", embedder.Dimensions())
	} else {
		slog.Info("未配置 EMBEDDING_BASE_URL，使用关键词检索")
	}

	var memoryStore memory.MemoryStore
	var storeOpts []memory.SQLiteOption
	if embedder != nil {
		storeOpts = append(storeOpts, memory.WithSQLiteEmbedder(embedder))
	}
	if fc.MadyHome != "" {
		ms, err := memory.NewSQLiteMemoryStore(memoryDB, storeOpts...)
		if err != nil {
			slog.Error("打开 SQLite 存储失败，降级为 InMemoryStore", "path", memoryDB, "error", err)
			memoryStore = memory.NewInMemoryStore(memory.WithEmbedder(embedder))
		} else {
			slog.Info("SQLite 持久化存储已加载", "path", memoryDB)
			memoryStore = ms
		}
	} else {
		memoryStore = memory.NewInMemoryStore(memory.WithEmbedder(embedder))
	}

	var extractor *memory.Extractor
	managerCfg := memory.DefaultManagerConfig()
	if os.Getenv("MADY_MEMORY_AUTO_EXTRACT") == "1" {
		if fc.Provider != nil {
			model := agentconfig.DefaultModel()
			extractor = memory.NewExtractor(memory.NewProviderExtractor(fc.Provider, model), memory.DefaultExtractorConfig())
			managerCfg.AutoExtract = true
			slog.Info("LLM 事实提取已启用", "model", model)
		} else {
			slog.Warn("MADY_MEMORY_AUTO_EXTRACT=1 但 Provider 不可用，跳过")
		}
	}

	fc.MemoryManager = memory.NewManager(memoryStore, extractor, nil, managerCfg)
	fc.MemoryManager.LogStats(context.Background())
	slog.Info("长期记忆系统已就绪")

	if sqliteStore, ok := memoryStore.(*memory.SQLiteMemoryStore); ok {
		if bm25Idx, err := sqliteStore.BuildBM25Index(context.Background()); err == nil {
			fc.MemoryManager.SetBM25Index(bm25Idx)
			slog.Info("BM25 混合检索已启用", "index_size", bm25Idx.Size())
		} else {
			slog.Error("BM25 索引构建失败，退化为纯稠密检索", "error", err)
		}
	}

	fc.MemoryCompiler = compiler.NewCompiler(compiler.Config{
		ExplorationRate: 5,
		MaxTraces:       1000,
	})
	compilerDB := filepath.Join(fc.MadyHome, "compiler.json")
	if err := fc.MemoryCompiler.Load(compilerDB); err != nil {
		slog.Warn("加载持久化状态失败，使用默认策略", "error", err)
	} else {
		slog.Info("策略学习系统已就绪", "strategies", len(fc.MemoryCompiler.Strategies()))
	}
	fc.CompilerDBPath = compilerDB
	fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, compiler.NewExtension(fc.MemoryCompiler))

	if fc.Provider != nil && os.Getenv("MADY_MEMORY_AUTO_EXTRACT") == "1" {
		fc.SessionSummarizer = memory.NewSessionSummarizer(fc.Provider, agentconfig.DefaultModel())
		slog.Info("会话汇总器已启用")
	}

	fc.KnowledgeGraph = kgwgraph.NewGraphStore()
}

// InitReasoningAndTemplates 初始化推理引擎 retriever/LLM 客户端、文档模板仓库、
// 引用核验装配（CitationGate 留痕 store），以及专利新颖性分析的检索器。
func InitReasoningAndTemplates(fc *Context) {
	loadWorkflowManifests(fc.MadyHome)

	retriever := BuildReasoningRetriever(fc)
	var llmClient reasoning.LlmClient
	if fc.Provider != nil {
		llmClient = reasoning.NewLlmClientFromProvider(fc.Provider, agentconfig.DefaultModel())
	}
	domains.SetupPatentDraftingEngine(retriever, llmClient)

	var patentRetriever domain.DomainRetriever
	if fc.KnowledgeBackend != nil {
		if store, ok := fc.KnowledgeBackend.(*ksqlite.SQLiteStore); ok {
			patentRetriever = rsqlite.NewPatentDomainRetriever(store)
		} else {
			slog.Debug("patent retriever disabled: KnowledgeBackend is not *ksqlite.SQLiteStore",
				"type", fmt.Sprintf("%T", fc.KnowledgeBackend))
		}
	} else {
		slog.Debug("patent retriever disabled: KnowledgeBackend is nil")
	}
	domains.SetupPatentRetriever(patentRetriever)

	domains.SetupKnowledgeExtension(fc.KnowledgeExt)

	domains.SetupClaimDraftingExtension(fc.Provider, agentconfig.DefaultModel())
	domains.SetupSpecDraftingExtension(fc.Provider)

	domains.SetupRulesExtension(fc.RuleEngine)

	userTmplDir := filepath.Join(fc.MadyHome, "doc-templates")
	store, err := doctmpl.NewTemplateStore(userTmplDir)
	if err != nil {
		slog.Error("加载模板仓库失败，模板工具不可用", "error", err)
	} else {
		domains.SetupDocTemplateStore(store)

		// 将 doctmpl 的 DOCX 渲染器注入 disclosure 包的 DOCX 导出接口，
		// 使技术交底书分析报告等文档具备 DOCX 格式输出能力。
		if docxRenderer, ok := store.RendererRegistry().Get(doctmpl.FormatDOCX); ok {
			disclosure.SetDOCXConverter(docxRendererAdapter{docx: docxRenderer})
			slog.Info("DOCX 导出渲染器已接入 disclosure 包")
		}
	}

	promptDir := filepath.Join(fc.MadyHome, "prompt-templates")
	fc.PromptStore, err = prompt.NewPromptStore(promptDir)
	if err != nil {
		slog.Error("加载提示词模板仓库失败，prompt:// 引用不可用", "error", err)
	} else {
		domains.SetupPromptStore(fc.PromptStore)
		slog.Info("已加载提示词模板", "count", fc.PromptStore.Count())
	}

	approvalDB := filepath.Join(fc.WorkspaceDir, "approvals.db")
	var citationStore domains.ApprovalStore
	if store, err := sqlitestore.NewApprovalStore(approvalDB); err == nil {
		citationStore = store
	} else {
		slog.Error("打开留痕数据库失败，降级为内存存储", "path", approvalDB, "error", err)
		citationStore = domains.NewMemoryApprovalStore()
	}

	citationSource := BuildCitationSource(fc.WikiRoot)
	domains.SetupCitationWiring(domains.CitationWiring{
		Source: citationSource,
		Store:  citationStore,
	})
}

// loadWorkflowManifests 从 $MADY_HOME/workflows/ 加载 YAML workflow manifest。
func loadWorkflowManifests(madyHome string) {
	workflowDir := filepath.Join(madyHome, "workflows")

	if err := os.MkdirAll(workflowDir, 0755); err != nil {
		slog.Warn("无法创建 workflow manifest 目录，使用内置默认值", "dir", workflowDir, "error", err)
		return
	}

	store := reasoning.GlobalWorkflowStore()

	if err := store.LoadDir(workflowDir); err == nil {
		ids := store.List()
		slog.Info("workflow manifest 已从 YAML 加载",
			"dir", workflowDir, "count", len(ids), "manifests", ids)
		return
	}

	defaults := reasoning.DefaultManifests()
	seeded := 0
	for _, m := range defaults {
		filename := filepath.Join(workflowDir, m.ID+".yaml")
		if _, statErr := os.Stat(filename); statErr == nil {
			continue
		}
		data, err := yaml.Marshal(map[string]any{"workflow_manifest": m})
		if err != nil {
			slog.Warn("workflow manifest 序列化失败", "id", m.ID, "error", err)
			continue
		}
		if err := os.WriteFile(filename, data, 0600); err != nil {
			slog.Warn("无法写入 workflow manifest 模板", "path", filename, "error", err)
			continue
		}
		seeded++
	}

	if seeded > 0 {
		slog.Info("已生成 workflow manifest YAML 模板", "dir", workflowDir, "count", seeded)
	} else {
		slog.Debug("workflow manifest: 已有 YAML 文件，跳过模板生成", "dir", workflowDir)
	}

	if err := store.LoadDir(workflowDir); err != nil {
		slog.Warn("workflow manifest YAML 加载失败，使用内置默认值", "dir", workflowDir, "error", err)
	} else {
		ids := store.List()
		slog.Info("workflow manifest 已从 YAML 加载", "dir", workflowDir, "count", len(ids), "manifests", ids)
	}
}

// BuildReasoningRetriever 从框架上下文中构造 MultiSourceRetriever。
func BuildReasoningRetriever(fc *Context) *reasoning.MultiSourceRetriever {
	if fc.KnowledgeGraph == nil && fc.KnowledgeBackend == nil && fc.WikiRoot == "" && fc.RuleEngine == nil {
		return nil
	}
	var walker *reasoning.ReasoningWalker
	var kgAdapter *kgwgraph.ReasoningStoreAdapter
	if fc.KnowledgeGraph != nil {
		adapter := kgwgraph.NewReasoningStoreAdapter(fc.KnowledgeGraph)
		kgAdapter = adapter
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
	retriever := reasoning.NewMultiSourceRetriever(walker, vs, sr, re)

	// 连接 IPC 审查标准源：使 retriever 在 Stage ② 规则获取中能查询 IPC 分类对应的审查标准。
	if ipcAdapter, err := reasoning.NewIPCStandardAdapter(); err == nil {
		retriever.WithIPCSource(ipcAdapter)
		slog.Info("IPC 审查标准源已接入推理检索器")
	} else {
		slog.Debug("IPC 审查标准源不可用，跳过", "error", err)
	}

	// 连接知识图谱拓扑提取器：使 retriever 在 Stage ② 中可以通过 KG 拓扑生成排序后的工作流步骤。
	if kgAdapter != nil {
		topoExt := reasoning.NewTopologyExtractor(kgAdapter)
		retriever.WithTopologyExtractor(topoExt)
		slog.Info("知识图谱拓扑提取器已接入推理检索器")
	}

	return retriever
}

// ExtSlice wraps a single Extension into a slice, returning nil for nil input.
func ExtSlice(ext agentcore.Extension) []agentcore.Extension {
	if ext == nil {
		return nil
	}
	return []agentcore.Extension{ext}
}

// AgentThinking 将 agentconfig.ThinkingConfig 转换为 agentcore.ThinkingConfig。
func AgentThinking(cfg *agentconfig.ThinkingConfig) *agentcore.ThinkingConfig {
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

// LoadFallbackConfig 从 agentconfig 读取模型级联回退候选链。
func LoadFallbackConfig() *agentcore.FallbackConfig {
	ac := agentconfig.LoadOrDefault()
	if ac.Fallback == nil || len(ac.Fallback.Candidates) == 0 {
		return nil
	}
	candidates := make(map[agentcore.Complexity][]string, len(ac.Fallback.Candidates))
	for level, models := range ac.Fallback.Candidates {
		var c agentcore.Complexity
		switch strings.ToLower(level) {
		case "low":
			c = agentcore.ComplexityLow
		case "medium":
			c = agentcore.ComplexityMedium
		case "high":
			c = agentcore.ComplexityHigh
		default:
			slog.Debug("framework: ignoring unknown fallback complexity level", "level", level)
			continue
		}
		candidates[c] = models
	}
	if len(candidates) == 0 {
		return nil
	}
	return &agentcore.FallbackConfig{
		Candidates:    candidates,
		StickySession: ac.Fallback.StickySession,
	}
}

// BuildCitationSource 从 wiki 拆分法条文件构建 S2 知识源索引，与 S1
// 内嵌静态表组合为复合源（CompositeCitationSource）。
func BuildCitationSource(wikiRoot string) guardrails.CitationSource {
	s1 := guardrails.DefaultCitationSource()

	if wikiRoot == "" {
		return s1
	}
	legalDir := filepath.Join(wikiRoot, "legal")
	idx, err := loader.BuildLawArticleIndex(legalDir)
	if err != nil {
		slog.Error("构建 S2 法条索引失败，降级为仅 S1 静态表", "error", err)
		return s1
	}
	slog.Info("S2 法条索引已加载", "articles", idx.ArticleCount())

	s2 := guardrails.CitationSourceFuncs{
		TopicsFunc: func(s lawcite.Statute, article int) ([]string, bool) {
			if s != lawcite.StatutePatentLaw {
				return nil, false
			}
			return idx.Topics(article)
		},
		MaxArticleFunc: func(s lawcite.Statute) int {
			if s != lawcite.StatutePatentLaw {
				return 0
			}
			return idx.MaxArticle()
		},
	}
	return guardrails.CompositeCitationSource(s1, s2)
}

// ============================================================
// Knowledge 装配函数
// ============================================================

// LoadWikiStore initializes the knowledge retrieval system.
func LoadWikiStore(madyHome string) (*knowledge.Store, agentcore.LifecycleHook, agentcore.Extension, knowledge.KnowledgeBackend) { //nolint:staticcheck // legacy hook type retained for backward compat
	embedder := BuildEmbedder()
	backend, knowledgeDBPath := LoadKnowledgeBackend(madyHome)
	if backend != nil {
		ext := knowledge.NewExtension(nil, nil, "patent", knowledge.DefaultKnowledgeExtConfig())
		ext.WithBackend(backend, embedder)
		if reranker := BuildReranker(); reranker != nil {
			ext.WithReranker(reranker)
			slog.Info("knowledge: cross-encoder rerank enabled")
		}
		if ws := openWritableStore(madyHome, embedder, knowledgeDBPath); ws != nil {
			ext.WithWritableStore(ws)
		}

		if store, ok := backend.(*ksqlite.SQLiteStore); ok {
			dbDir := filepath.Dir(knowledgeDBPath)

			lawsPath := filepath.Join(dbDir, "laws-full-local.db")
			if _, err := os.Stat(lawsPath); os.IsNotExist(err) {
				lawsPath = filepath.Join(dbDir, "laws-full.db")
			}
			if _, err := os.Stat(lawsPath); err == nil {
				if err := store.OpenLawsDB(lawsPath); err != nil {
					slog.Error("knowledge: laws-full.db open failed", "error", err)
				} else {
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
					mode := "FTS5"
					if !store.HasLawFTS() {
						mode = "LIKE"
					}
					lawsLabel := filepath.Base(lawsPath)
					slog.Info("knowledge: laws-full.db active", "file", lawsLabel, "mode", mode)
				}
			}

			if gs, err := store.LoadGraph(); err != nil {
				slog.Error("knowledge: graph load failed", "error", err)
			} else if gs.NodeCount() > 0 {
				enhancer := kgwgraph.NewGraphEnhancer(gs, kgwgraph.DefaultEnhanceConfig())
				ext.WithGraph(enhancer)
				typeCounts := gs.NodeTypeCounts()
				lawCount := typeCounts[kgwgraph.NodeLawArticle]
				caseCount := typeCounts[kgwgraph.NodeCase] + typeCounts[kgwgraph.NodeJudgment]
				ipcCount := typeCounts[kgwgraph.NodeIPC]
				evidenceCount := typeCounts[kgwgraph.NodeEvidence]
				slog.Info("knowledge: 图谱已加载",
					"nodes", gs.NodeCount(), "edges", gs.EdgeCount(),
					"law", lawCount, "case", caseCount, "ipc", ipcCount, "evidence", evidenceCount)
			}
		}

		hook := ext.BackendHook(retrieval.RetrievalConfig{ //nolint:staticcheck // legacy backend hook retained for backward compat
			TopK:          5,
			MaxChars:      4000,
			TriggerPolicy: retrieval.TriggerSmart,
			Prefix:        "以下是从知识库中检索到的相关法条、判例和审查指南。请在回答时优先参考这些信息，并核实引用的法条编号与检索结果一致：\n",
		})
		if hook != nil {
			return nil, hook, ext, backend
		}
	}

	wikiPath := os.Getenv("WIKI_PATH")
	if wikiPath == "" {
		return nil, nil, nil, nil
	}
	store := knowledge.NewStore()
	wikiLoader := loader.NewWikiLoader(store, wikiPath)
	stats, err := wikiLoader.ImportWiki()
	if err != nil {
		slog.Error("wiki import failed", "error", err)
		return nil, nil, nil, nil
	}
	slog.Info("wiki imported", "docs", stats.Imported, "chunks", store.Stats().TotalChunks)
	hook := store.RetrievalHook("patent", retrieval.RetrievalConfig{
		TopK:          5,
		MaxChars:      4000,
		TriggerPolicy: retrieval.TriggerSmart,
		Prefix:        "以下是从知识库中检索到的相关法条、判例和审查指南。请在回答时优先参考这些信息，并核实引用的法条编号与检索结果一致：\n",
	})
	return store, hook, nil, nil
}

// ResolveWikiRoot resolves the Obsidian wiki root for patent-cards access.
func ResolveWikiRoot(madyHome string) string {
	if p := os.Getenv("WIKI_PATH"); p != "" {
		p = filepath.Clean(p)
		if info, err := os.Stat(p); err == nil && info.IsDir() { // #nosec G703 -- path cleaned above
			return p
		}
	}
	if madyHome == "" {
		return ""
	}
	candidate := filepath.Join(madyHome, "knowledge", "wiki")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() { // #nosec G703 -- joined from trusted MadyHome
		return candidate
	}
	return ""
}

// BuildEmbedder creates an APIEmbedder from environment variables.
func BuildEmbedder() retrieval.Embedder {
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

// BuildReranker creates a ModelReranker from environment variables.
func BuildReranker() retrieval.QueryReranker {
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

// LoadKnowledgeBackend opens the SQLite knowledge database read-only.
func LoadKnowledgeBackend(madyHome string) (knowledge.KnowledgeBackend, string) {
	dbDir := os.Getenv("KNOWLEDGE_DB_DIR")
	if dbDir == "" {
		if madyHome != "" {
			dbDir = filepath.Join(madyHome, "knowledge")
		} else {
			return nil, ""
		}
	}
	dbPath := filepath.Join(dbDir, "knowledge.db")
	if _, err := os.Stat(dbPath); err != nil { // #nosec G703 -- dbDir resolved from MadyHome or cleaned env
		return nil, ""
	}
	store, err := ksqlite.NewSQLiteStore(dbPath)
	if err != nil {
		slog.Error("knowledge: failed to open SQLite store", "error", err)
		return nil, ""
	}
	if err := store.PreloadVectors(); err != nil {
		slog.Warn("knowledge: vector preload failed, using SQL batch fallback", "error", err)
	} else {
		stats := store.Stats()
		slog.Info("knowledge: SQLite backend active", "path", dbPath)
		slog.Info("knowledge: stats",
			"docs", stats.Documents, "chunks", stats.Chunks,
			"embeddings", stats.Embeddings, "dims", stats.Dim, "vector_mb", stats.VectorMemoryMB)
	}
	return store, dbPath
}

// openWritableStore opens or creates the user database (user.db).
func openWritableStore(madyHome string, embedder retrieval.Embedder, knowledgeDBPath string) *ksqlite.WritableStore {
	if embedder == nil {
		return nil
	}
	userDBPath := os.Getenv("USER_DB_PATH")
	if userDBPath == "" {
		if madyHome == "" {
			return nil
		}
		userDBPath = filepath.Join(madyHome, "knowledge", "user.db")
	}
	if dir := filepath.Dir(userDBPath); dir != "" {
		dir = filepath.Clean(dir)
		if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G703 -- cleaned above
			slog.Error("knowledge: user.db dir create failed", "error", err)
			return nil
		}
	}
	ws, err := ksqlite.OpenWritable(userDBPath, embedder, knowledgeDBPath)
	if err != nil {
		slog.Error("knowledge: user.db open failed", "error", err)
		return nil
	}
	slog.Info("knowledge: user.db writable store active", "path", userDBPath)
	return ws
}
