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
	"github.com/xujian519/mady/domains/reasoning"
	reasoningwiring "github.com/xujian519/mady/domains/reasoning/wiring"
	"github.com/xujian519/mady/domains/rules"
	"github.com/xujian519/mady/knowledge"
	kgwgraph "github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/mcp"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/skill"
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
