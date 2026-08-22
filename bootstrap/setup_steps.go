// Package bootstrap 提供所有 mady 入口（tui/serve/acp/desktop）共享的装配逻辑。
// 注意：bootstrap 是全局装配器，已知会跨层引用 domains/mcp/guardrails 等上层包。
// 这是设计上接受的"必要之恶"，不应被其他基础设施层包导入。
package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/agentcore/filecheckpoint"
	"github.com/xujian519/mady/agentcore/planmode"
	"github.com/xujian519/mady/agentcore/plantask"
	"github.com/xujian519/mady/agentcore/tasklist"
	"github.com/xujian519/mady/bootstrap/agentconfig"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/deadline"
	domainEvidence "github.com/xujian519/mady/domains/evidence"
	"github.com/xujian519/mady/domains/rules"
	"github.com/xujian519/mady/guardrails/guardian"
	kgwgraph "github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/mcp"
	"github.com/xujian519/mady/memory"
	"github.com/xujian519/mady/memory/compiler"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/skill"
	"github.com/xujian519/mady/tools"
)

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
		skillPaths = append(skillPaths, filepath.SplitList(sd)...)
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		skillPaths = append(skillPaths, filepath.Join(homeDir, ".agent"))
	}
	if cwd, err := os.Getwd(); err == nil {
		skillPaths = append(skillPaths, filepath.Join(cwd, ".agent"), filepath.Join(cwd, "skills"), filepath.Join(cwd, "plugins"))
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
		names := make([]string, 0, len(mcpExts))
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

	fc.PlanModeExt = planmode.NewExtension(planmode.Policy{
		AllowedTools: []string{
			"plan_submit", "plan_approve", "plan_reject", "plan_revise",
			"workflow_interrupt", "workflow_resume", "workflow_feedback",
		},
	})
	fc.EvidenceExt = evidence.NewExtension()
	fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions,
		fc.PlanModeExt,
		fc.EvidenceExt,
		domainEvidence.NewDomainExtension(newEvidenceRuleIndex()),
		deadline.NewCalculatorExtension(),
	)

	if taskDir, err := util.ResolveDataDir("sessions"); err == nil {
		taskDir = TasklistDirForCWD(taskDir, fc.BaseConfig.ProjectDir)
		if taskExt, err := tasklist.NewExtension(taskDir); err == nil {
			fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, taskExt)
		}
		// PlanTask HCL 扩展：会话 + 计划任务镜像存储，门控复用 PlanModeExt。
		ptSessionsDir := filepath.Join(taskDir, "plantask", "sessions")
		ptTasksDir := filepath.Join(taskDir, "plantask", "tasks")
		if sessStore, sErr := plantask.NewFileStore(ptSessionsDir); sErr == nil {
			if taskStore, tErr := tasklist.NewFileStore(ptTasksDir); tErr == nil {
				expiry := plantask.DefaultExpirySettings()
				bridge := NewPlantaskBridge(taskStore)
				if ptExt, eErr := plantask.NewExtension(plantask.Config{
					Store:         sessStore,
					TaskStore:     taskStore,
					Gate:          fc.PlanModeExt,
					DefaultExpiry: &expiry,
					Replanner:     bridge,
					// 02-spec §N4：连续 2 轮 High 自动进入规划态。
					AutoEnter: plantask.AutoEnterConfig{Rounds: 2},
				}); eErr == nil {
					fc.PlantaskExt = ptExt
					fc.PlantaskBridge = bridge
					fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions, ptExt)
				} else {
					slog.Warn("PlanTask 扩展初始化失败，跳过", "error", eErr)
				}
			}
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

// TasklistDirForCWD returns the tasklist storage directory partitioned by the current working directory.
func TasklistDirForCWD(baseDir, cwd string) string {
	if cwd == "" {
		return filepath.Join(baseDir, "tasks")
	}
	return filepath.Join(baseDir, "by-cwd", CwdPartitionName(cwd), "tasks")
}

// InitPlugins 从 plugins/ 目录发现并加载工作流插件。
func InitPlugins(fc *Context) {
	// 插件功能暂未完成，跳过初始化。
	_ = fc
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
		// BM25 索引构建在后台异步完成，避免阻塞启动流程。
		// 用户可在索引完成前先使用纯稠密检索或关键词检索。
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("[PANIC] bootstrap: BM25 index build panicked", "panic", r)
				}
			}()
			bmCtx, bmCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer bmCancel()
			if bm25Idx, err := sqliteStore.BuildBM25Index(bmCtx); err == nil {
				fc.MemoryManager.SetBM25Index(bm25Idx)
				slog.Info("BM25 混合检索已启用", "index_size", bm25Idx.Size())
			} else {
				slog.Error("BM25 索引构建失败，退化为纯稠密检索", "error", err)
			}
		}()
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
