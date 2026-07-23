package main

import (
	"log"
	"os"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/permission"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/domains/rules"
	"github.com/xujian519/mady/knowledge/risk"
	"github.com/xujian519/mady/memory"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/tools"
)

// buildMemoryExtension 根据当前会话状态构建 MemoryExtension。
// 使用 WithSharedManager() 参数，确保 Dispose 时不关闭框架级共享 Manager。
// 当 fc.MemoryManager 为 nil 时返回 nil（记忆功能不可用）。
func (s *tuiSession) buildMemoryExtension() *memory.MemoryExtension {
	if s.fc.MemoryManager == nil {
		return nil
	}
	scope := memory.MemoryScope{
		UserID:    s.currentThreadID,
		SessionID: s.currentThreadID,
		AgentID:   s.detectAgentID(),
		ProjectID: s.detectProjectID(),
	}
	ext := memory.NewExtension(s.fc.MemoryManager, scope,
		memory.DefaultExtensionConfig(), memory.WithSharedManager())

	// 若框架级会话汇总器可用，注入到扩展实例（会话关闭时异步汇总）
	if s.fc.SessionSummarizer != nil {
		ext.SetSummarizer(s.fc.SessionSummarizer)
	}

	return ext
}

// injectMemoryExtension 将 s.memExt 注入到 agentcore.Config 的 Extensions 列表中。
// 当 memExt 为 nil 时直接返回原 cfg 不变。
func (s *tuiSession) injectMemoryExtension(cfg agentcore.Config) agentcore.Config {
	if s.memExt != nil {
		cfg.Extensions = append(cfg.Extensions, s.memExt)
	}
	return cfg
}

// buildAgentConfig constructs the agentcore.Config based on current session state.
func (s *tuiSession) buildAgentConfig() agentcore.Config {
	s.memExt = s.buildMemoryExtension()

	switch {
	case s.useIntegratedMode:
		base := s.fc.BaseConfig
		base.Name = "chat-agent"
		base.ModelConfig = agentcore.ModelConfig{
			Name:      "mady",
			Model:     s.model,
			Provider:  s.provider,
			Thinking:  cloneThinkingConfig(s.thinkingConfig()),
			Streaming: true,
		}
		s.applyPlanModeThinking(&base)
		if s.toolApprover != nil {
			base.Extensions = append(base.Extensions,
				permission.NewExtension(permission.ProjectAgentPolicy(), s.toolApprover))
		}
		return s.extendConfig(domains.IntegratedChatConfig(base))

	case s.useMultiDomain:
		if s.toolApprover != nil {
			base := s.fc.BaseConfig
			base.Extensions = append(base.Extensions,
				permission.NewExtension(permission.ProjectAgentPolicy(), s.toolApprover))
		}
		cfg := buildRouterConfig(s.fc.BaseConfig, s.fc.Manifests)
		cfg.Thinking = cloneThinkingConfig(s.thinkingConfig())
		s.applyPlanModeThinking(&cfg)
		return s.extendConfig(cfg)

	default:
		singleCfg := agentcore.Config{
			ModelConfig: agentcore.ModelConfig{
				Name:      "mady",
				Model:     s.model,
				Provider:  s.provider,
				Thinking:  cloneThinkingConfig(s.thinkingConfig()),
				Streaming: true,
			},
			SystemPrompt: defaultSystemPrompt,
			ExecutionConfig: agentcore.ExecutionConfig{
				MaxTurns:          25,
				ExecutionMode:     agentcore.ModeSerial,
				ValidateArguments: true,
			},
			CompactionConfig: agentcore.CompactionConfig{
				ContextWindow:    agentconfig.ResolveContextWindow(s.model),
				ReserveTokens:    32000,
				KeepRecentTokens: 4000,
			},
			RetryConfig: &agentcore.RetryConfig{
				MaxRetries:  3,
				BaseDelayMs: 1000,
				MaxDelayMs:  15000,
			},
			Lifecycle: s.fc.WikiHook,
		}
		s.applyPlanModeThinking(&singleCfg)
		return s.extendConfig(singleCfg)
	}
}

// applyPlanModeThinking 在计划模式下将 Thinking 级别提升至最高。
func (s *tuiSession) applyPlanModeThinking(cfg *agentcore.Config) {
	if !s.isPlanMode() {
		return
	}
	cfg.Model = s.planModel
	if cfg.Thinking == nil {
		cfg.Thinking = &agentcore.ThinkingConfig{Effort: agentcore.ThinkingEffortMax}
	} else {
		cfg.Thinking.Effort = agentcore.ThinkingEffortMax
	}
}

// extendConfig 为配置注入共享扩展：WikiHook、知识图谱、规则引擎、风险评估、
// 写作辅助、文件索引、以及记忆扩展和持久化。
//
// 扩展从 s.fc 按需读取（而非预构建），确保 deferred 初始化完成后
// rebuildAgent() 能获取到最新装配的扩展实例。
func (s *tuiSession) extendConfig(cfg agentcore.Config) agentcore.Config {
	if s.fc.WikiHook != nil {
		cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, s.fc.WikiHook)
	}
	if s.fc.KnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, s.fc.KnowledgeExt)
	}
	// 规则引擎扩展：从 fc.RuleEngine 按需构建（可能由 deferred 任务填充）。
	if s.fc.RuleEngine != nil {
		cfg.Extensions = append(cfg.Extensions, rules.NewExtension(s.fc.RuleEngine))
	}
	// 风险扫描扩展：从 fc.WikiStore 按需构建（可能由 deferred 任务填充）。
	if s.fc.WikiStore != nil {
		cfg.Extensions = append(cfg.Extensions, risk.NewExtension(s.fc.WikiStore, risk.DefaultScannerConfig()))
	}
	if s.writingExt != nil {
		cfg.Extensions = append(cfg.Extensions, s.writingExt)
	}
	cfg.Extensions = append(cfg.Extensions, s.fileIndexExt)
	// 案件管理扩展：AI 内部工具（list_cases/sync_case/focus_case 等），用户不可见。
	if s.fc.CaseIndex != nil {
		cwd, _ := os.Getwd()
		cfg.Extensions = append(cfg.Extensions, domains.NewCaseExtension(s.fc.CaseIndex, cwd, caseFileReader{}))
	}
	return s.injectMemoryExtension(s.applyPersistence(cfg))
}

// applyPersistence injects session store, checkpoint, project context,
// review gate, and vision config into the given agent config.
func (s *tuiSession) applyPersistence(cfg agentcore.Config) agentcore.Config {
	if s.agentStore != nil {
		cfg.Store = s.agentStore
	}
	cfg.Checkpoint = &agentcore.CheckpointSettings{
		Saver:    s.checkpointSaver,
		ThreadID: s.currentThreadID,
	}
	// currentProject 由 detectCaseFromCWD 确保始终非空（匹配已知案件或自动从 CWD 创建）。
	cfg.WorkspaceDir = s.currentProject.RootPath
	cfg.ProjectDir = s.currentProject.RootPath
	cfg.SystemPrompt += formatProjectContext(s.currentProject, s.currentProjectMeta)

	retriever := buildReasoningRetriever(s.fc)
	var llmClient reasoning.LlmClient
	if s.provider != nil {
		llmClient = reasoning.NewLlmClientFromProvider(s.provider, s.model)
	}
	// 仅在已有领域元数据时才注入五步推理工作流工具
	if s.currentProjectMeta != nil && s.currentProjectMeta.MatterType != "" {
		runner := reasoning.NewWorkflowRunner(
			s.currentProject.ProjectID,
			mapMatterTypeToCaseType(s.currentProjectMeta),
			s.currentProject.Domain,
			retriever,
			llmClient,
		)
		// Confirmation gate: when review mode is on, Stage ② interrupts for
		// human rule confirmation. The workflow checkpoint store (lazily
		// initialized) persists the interruption point for resumption.
		if s.isReviewMode() {
			runner.SetRequireRuleConfirmation(true)
			if s.workflowStore == nil {
				// 优先使用 SQLite 持久化存储，失败时回退到内存。
				store, err := s.openWorkflowCheckpointStore()
				if err != nil {
					log.Printf("工作流检查点：SQLite 不可用，回退到内存存储: %v", err)
					s.workflowStore = reasoning.NewMemoryCheckpointStore()
				} else {
					s.workflowStore = store
				}
			}
		}
		cfg.Tools = append(cfg.Tools, reasoning.AsWorkflowToolWithCheckpoint(runner, s.workflowStore))
	}

	if s.isReviewMode() {
		var gate *domains.ApprovalGate
		// Wire up SQLite persistence so human decisions (adopted/modified/rejected)
		// are recorded for AdoptionRate evaluation (roadmap P3). Falls back to
		// in-memory store if the SQLite store cannot be opened.
		if store, err := s.openApprovalStore(); err == nil {
			gate = domains.NewApprovalGate(domains.DefaultApprovalConfig(), domains.WithApprovalStore(store))
		} else {
			gate = domains.NewApprovalGate(domains.DefaultApprovalConfig(), domains.WithApprovalStore(domains.NewMemoryApprovalStore()))
		}
		s.approvalGate = gate
		cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, gate)
	} else {
		s.approvalGate = nil
	}

	for _, ext := range cfg.Extensions {
		if te, ok := ext.(*tools.Extension); ok {
			te.WithVision(s.provider, s.model)
		}
	}

	return cfg
}
