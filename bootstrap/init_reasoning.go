// Package bootstrap 提供所有 mady 入口（tui/serve/acp/desktop）共享的装配逻辑。
// 注意：bootstrap 是全局装配器，已知会跨层引用 domains/mcp/guardrails 等上层包。
// 这是设计上接受的"必要之恶"，不应被其他基础设施层包导入。
package bootstrap

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/doctmpl"
	domainEvidence "github.com/xujian519/mady/domains/evidence"
	"github.com/xujian519/mady/domains/reasoning"
	reasoningwiring "github.com/xujian519/mady/domains/reasoning/wiring"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/domains/writing"
	kgwgraph "github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/knowledge/knowledgeinit"
	ksqlite "github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/prompt"
	"github.com/xujian519/mady/retrieval/domain"
	rbrowser "github.com/xujian519/mady/retrieval/domain/browser"
	rsqlite "github.com/xujian519/mady/retrieval/domain/sqlite"
)

// InitReasoningAndTemplates 初始化推理引擎 retriever/LLM 客户端、文档模板仓库、
// 引用核验装配（CitationGate 留痕 store），以及专利新颖性分析的检索器。
func InitReasoningAndTemplates(fc *Context) {
	loadWorkflowManifests(fc.MadyHome)

	retriever := BuildReasoningRetriever(fc)
	var llmClient reasoning.LlmClient
	if fc.Provider != nil {
		llmClient = reasoning.NewLlmClientFromProvider(fc.Provider, agentconfig.DefaultModel())
	}
	runner := domains.SetupPatentDraftingEngine(retriever, llmClient)
	// HCL replan 闭环：bridge 接入真实五步推理引擎（反馈 → 模板/KG/LLM 重规划）。
	if fc.PlantaskBridge != nil {
		fc.PlantaskBridge.SetRunner(runner)
	}

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

	// 在线专利数据库检索器（ego-browser 驱动，实时检索 Google Patents /
	// CNIPA / Espacenet）。ego lite 未安装时工厂返回 nil 并被过滤，
	// 不影响现有本地检索行为。
	cfg := rbrowser.DefaultConfig()
	domains.SetupBrowserPatentRetrievers([]domain.DomainRetriever{
		rbrowser.NewGooglePatentsRetriever(*cfg),
		rbrowser.NewCNIPARetriever(*cfg),
		rbrowser.NewEspacenetRetriever(*cfg),
	})

	domains.SetupKnowledgeExtension(fc.KnowledgeExt)

	// 加载本地专利知识库（data/knowledge/ 下的 Markdown 文件）。
	// 静默跳过不存在的文件。
	if fc.WikiStore != nil {
		if err := knowledgeinit.InitPatentKnowledge(fc.WikiStore); err != nil {
			slog.Warn("patent knowledge init", "error", err)
		}
	}

	// 侵权分析知识检索器：从 KnowledgeExtension 适配为 infringement.KnowledgeRetriever。
	// 当 KnowledgeExt 不可用时为 nil，侵权分析降级为纯 LLM 知识。
	domains.SetupInfringementKnowledgeRetriever(
		NewInfringementKnowledgeRetriever(fc.KnowledgeExt),
	)

	domains.SetupClaimDraftingExtension(fc.Provider, agentconfig.DefaultModel())
	domains.SetupSpecDraftingExtension(fc.Provider)

	// 写作模式扩展：加载种子模式文件，注入 Agent 配置。
	writingStore := writing.NewPatternStore()
	if fc.MadyHome != "" {
		seedDirs := []string{
			filepath.Join(fc.MadyHome, "knowledge", "seed-patterns"),
			filepath.Join("domains", "writing", "seed-patterns"),
		}
		for _, dir := range seedDirs {
			count, err := writingStore.LoadSeedDir(dir)
			if err == nil && count > 0 {
				slog.Info("writing: loaded seed pattern files", "dir", dir, "count", count)
				break
			}
		}
	}
	domains.SetupWritingExtension(writingStore)

	domains.SetupRulesExtension(fc.RuleEngine)

	// 证据判断扩展：将 EvidenceDomainExtension 注入 Agent 配置。
	// ext 为 nil 时静默跳过，使 Agent 仍然可以正常启动。
	domains.SetupEvidenceExtension(domainEvidence.NewDomainExtension(nil))

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

	if err := os.MkdirAll(workflowDir, 0o750); err != nil {
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
