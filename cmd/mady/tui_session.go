package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/reasoning"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/knowledge/fileindex"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/tools"
	"github.com/xujian519/mady/tui/agentadapter"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// tuiSession holds the mutable state shared across TUI command handlers.
// All slash-command logic operates on this struct instead of capturing
// local variables in closures, making the code testable and readable.
type tuiSession struct {
	ctx context.Context
	fc  *frameworkContext

	// Provider/model config
	provider        agentcore.Provider
	model           string
	providerName    string
	planModel       string
	normalModel     string
	planMode        bool
	currentThinking *agentcore.ThinkingConfig

	// Mode flags
	useMultiDomain    bool
	useIntegratedMode bool

	// Extensions
	ruleExt      agentcore.Extension
	fileIndexExt *fileindex.Extension

	// Agent state
	currentAgent *agentcore.Agent
	runMu        sync.Mutex
	cancelMu     sync.Mutex
	runCancel    context.CancelFunc

	// Session persistence
	agentStore      *session.AgentStore
	checkpointSaver *agentcore.MemoryCheckpointSaver
	currentThreadID string
	sessionDir      string

	// Project/case context
	currentProject     *domains.ProjectRecord
	currentProjectMeta *domains.ProjectMeta
	currentFileIndex   *fileindex.FileIndex
	currentFileWatcher *fileindex.FileWatcher

	reviewMode bool

	// Approval gate state — persists human decisions (adopted/modified/rejected)
	// to accumulate real AdoptionRate data for evaluation (see P3 roadmap).
	approvalGate *domains.ApprovalGate

	app              *chat.ChatApp
	currentThemeName string

	// slashReg is the single source of truth for slash commands: both
	// handleSubmit (dispatch) and the autocomplete menu read from it.
	slashReg *Registry
}

// buildAgentConfig constructs the agentcore.Config based on current session state.
// It replaces the former buildCfg closure inside runTui.
func (s *tuiSession) buildAgentConfig() agentcore.Config {
	switch {
	case s.useIntegratedMode:
		base := s.fc.BaseConfig
		base.Name = "chat-agent"
		base.ModelConfig = agentcore.ModelConfig{
			Name:      "mady",
			Model:     s.model,
			Provider:  s.provider,
			Thinking:  cloneThinkingConfig(s.currentThinking),
			Streaming: true,
		}
		if s.planMode {
			base.Model = s.planModel
			if base.Thinking == nil {
				base.Thinking = &agentcore.ThinkingConfig{Effort: agentcore.ThinkingEffortMax}
			} else {
				base.Thinking.Effort = agentcore.ThinkingEffortMax
			}
		}
		cfg := domains.IntegratedChatConfig(base)
		if s.fc.WikiHook != nil {
			cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, s.fc.WikiHook)
		}
		if s.fc.KnowledgeExt != nil {
			cfg.Extensions = append(cfg.Extensions, s.fc.KnowledgeExt)
		}
		if s.ruleExt != nil {
			cfg.Extensions = append(cfg.Extensions, s.ruleExt)
		}
		cfg.Extensions = append(cfg.Extensions, s.fileIndexExt)
		return s.applyPersistence(cfg)

	case s.useMultiDomain:
		cfg := buildRouterConfig(s.fc.BaseConfig, s.fc.Manifests)
		cfg.Thinking = cloneThinkingConfig(s.currentThinking)
		if s.planMode {
			cfg.Model = s.planModel
			if cfg.Thinking == nil {
				cfg.Thinking = &agentcore.ThinkingConfig{Effort: agentcore.ThinkingEffortMax}
			} else {
				cfg.Thinking.Effort = agentcore.ThinkingEffortMax
			}
		}
		if s.fc.WikiHook != nil {
			cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, s.fc.WikiHook)
		}
		if s.fc.KnowledgeExt != nil {
			cfg.Extensions = append(cfg.Extensions, s.fc.KnowledgeExt)
		}
		if s.ruleExt != nil {
			cfg.Extensions = append(cfg.Extensions, s.ruleExt)
		}
		cfg.Extensions = append(cfg.Extensions, s.fileIndexExt)
		return s.applyPersistence(cfg)

	default:
		effectiveModel := s.model
		effectiveThinking := cloneThinkingConfig(s.currentThinking)
		if s.planMode {
			effectiveModel = s.planModel
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
				Provider:  s.provider,
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
			Lifecycle: s.fc.WikiHook,
		}
		if s.fc.KnowledgeExt != nil {
			singleCfg.Extensions = append(singleCfg.Extensions, s.fc.KnowledgeExt)
		}
		if s.ruleExt != nil {
			singleCfg.Extensions = append(singleCfg.Extensions, s.ruleExt)
		}
		singleCfg.Extensions = append(singleCfg.Extensions, s.fileIndexExt)
		return s.applyPersistence(singleCfg)
	}
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
	if s.currentProject != nil {
		cfg.WorkspaceDir = s.currentProject.RootPath
		cfg.ProjectDir = s.currentProject.RootPath
		cfg.SystemPrompt += formatProjectContext(s.currentProject, s.currentProjectMeta)

		retriever := buildReasoningRetriever(s.fc)
		var llmClient reasoning.LlmClient
		if s.provider != nil {
			llmClient = reasoning.NewLlmClientFromProvider(s.provider, s.model)
		}
		runner := reasoning.NewWorkflowRunner(
			s.currentProject.ProjectID,
			mapMatterTypeToCaseType(s.currentProjectMeta),
			s.currentProject.Domain,
			retriever,
			llmClient,
		)
		cfg.Tools = append(cfg.Tools, reasoning.AsWorkflowTool(runner))
	} else if cfg.ProjectDir != "" {
		cfg.SystemPrompt += fmt.Sprintf(
			"\n\n【当前工作目录】\n你正在「%s」目录下工作。可以使用文件工具（read、ls、grep、find、write_file 等）读取和分析该目录中的文件。用户提到的相对路径默认基于此目录。",
			cfg.ProjectDir,
		)
	}

	if s.reviewMode {
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

// rebuildAgent recreates the current agent from the latest config and rebinds it to the UI.
func (s *tuiSession) rebuildAgent() {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	prev := s.currentAgent
	s.currentAgent = agentcore.New(s.buildAgentConfig())
	prev.Close()
	agentadapter.BindAgent(s.app, s.currentAgent)
}

// submitInput sends user input to the current agent asynchronously.
// The agent runs in a separate goroutine to avoid blocking the TUI event loop.
func (s *tuiSession) submitInput(input string) {
	agent := s.currentAgent
	store := s.agentStore
	threadID := s.currentThreadID
	go func() {
		s.runMu.Lock()
		defer s.runMu.Unlock()

		runCtx, cancel := context.WithCancel(s.ctx)
		s.cancelMu.Lock()
		s.runCancel = cancel
		s.cancelMu.Unlock()
		defer func() {
			s.cancelMu.Lock()
			s.runCancel = nil
			s.cancelMu.Unlock()
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

// resumeIfInterrupted continues the agent from an interrupt point (e.g. the
// disclosure review_gate) by calling agent.Resume, which preserves the
// interrupted runLoop's state. Returns true when a resume was initiated.
//
// This is the hard-interrupt recovery path, distinct from submitInput: when a
// Pregel tool node returns InterruptError, the agent loop exits and only
// Resume() can pick it up — submitInput would instead start a fresh turn and
// lose the in-flight tool context. Callers (/approve) should try this first
// and fall back to submitInput only when the agent is not interrupted (the
// ApprovalGate keyword-triggered soft-interrupt case).
func (s *tuiSession) resumeIfInterrupted() bool {
	agent := s.currentAgent
	if agent == nil || agent.Interrupted() == nil {
		return false
	}
	store := s.agentStore
	threadID := s.currentThreadID
	go func() {
		s.runMu.Lock()
		defer s.runMu.Unlock()

		runCtx, cancel := context.WithCancel(s.ctx)
		s.cancelMu.Lock()
		s.runCancel = cancel
		s.cancelMu.Unlock()
		defer func() {
			s.cancelMu.Lock()
			s.runCancel = nil
			s.cancelMu.Unlock()
		}()

		if _, err := agent.Resume(runCtx); err != nil {
			log.Printf("[mady] agent resume failed: %v", err)
			return
		}
		if store == nil {
			return
		}
		if err := agent.SaveState(context.Background(), threadID); err != nil {
			log.Printf("[mady] save state: %v", err)
		}
	}()
	return true
}

// handleSubmit processes user input from the TUI, dispatching slash commands
// via the slash registry or forwarding plain text to the agent.
func (s *tuiSession) handleSubmit(input string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return
	}

	if cmd := s.slashReg.Lookup(trimmed, s); cmd != nil {
		cmd.Handler(slashCtx{s: s, input: trimmed})
		return
	}

	if strings.HasPrefix(trimmed, "/") {
		s.app.PrintSystem(fmt.Sprintf("未知命令: %s（输入 / 查看可用命令）", trimmed))
		return
	}
	s.submitInput(trimmed)
}

func (s *tuiSession) handleThinkingCommand(trimmed string) {
	next, changed, err := parseThinkingCommand(trimmed, s.currentThinking)
	if err != nil {
		s.app.PrintError(err)
		return
	}
	if !changed {
		s.app.PrintSystem("推理配置: " + formatThinkingConfig(s.currentThinking))
		return
	}
	s.currentThinking = next
	s.runMu.Lock()
	prev := s.currentAgent
	s.currentAgent = agentcore.New(s.buildAgentConfig())
	prev.Close()
	agentadapter.BindAgent(s.app, s.currentAgent)
	s.app.PrintSystem("推理配置已更新: " + formatThinkingConfig(s.currentThinking))
	mdl := s.normalModel
	if s.planMode {
		mdl = s.planModel
	}
	s.app.UpdateStatusBar(s.providerName, mdl, statusBarModeLabel(s.planMode, s.useMultiDomain, s.currentThinking))
	s.runMu.Unlock()
}

func (s *tuiSession) handleThemeCommand(trimmed string) {
	switch trimmed {
	case "/theme":
		s.app.PrintSystem("当前主题: " + s.currentThemeName)
	case "/theme dark":
		theme.SetSemanticTheme(theme.DefaultSemanticDark(), theme.DetectColorMode())
		s.app.History().SetTheme(chat.DefaultChatHistoryTheme())
		s.currentThemeName = "dark"
		s.app.PrintSystem("已切换深色主题")
	case "/theme light":
		theme.SetSemanticTheme(theme.DefaultSemanticLight(), theme.DetectColorMode())
		s.app.History().SetTheme(chat.DefaultChatHistoryTheme())
		s.currentThemeName = "light"
		s.app.PrintSystem("已切换浅色主题")
	}
}

func (s *tuiSession) handleCaseCommand(trimmed string) {
	args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/case"))
	switch args {
	case "", "list":
		records := s.fc.ProjectRegistry.List()
		if len(records) == 0 {
			s.app.PrintSystem("暂无已注册案件。使用 mady serve 或 ProjectRegistry API 注册案件。")
			return
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "已注册案件（%d）：\n", len(records))
		for i, rec := range records {
			marker := "  "
			if s.currentProject != nil && rec.ProjectID == s.currentProject.ProjectID {
				marker = "→ "
			}
			fmt.Fprintf(&sb, "%s%d. %s（%s）[%s]\n", marker, i+1, rec.Alias, rec.ProjectID, rec.Domain)
		}
		if s.currentProject == nil {
			sb.WriteString("\n使用 /case <ID或别名> 切换案件")
		}
		s.app.PrintSystem(sb.String())

	case "info":
		if s.currentProject == nil {
			s.app.PrintSystem("当前未选择案件。使用 /case 查看可用案件。")
			return
		}
		s.app.PrintSystem(formatProjectInfo(s.currentProject, s.currentProjectMeta))

	case "off", "clear":
		if s.currentProject == nil {
			s.app.PrintSystem("当前未选择案件")
			return
		}
		oldName := s.currentProject.Alias
		s.currentProject = nil
		s.closeFileResources()
		s.currentProjectMeta = nil
		s.rebuildAgent()
		s.app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.planMode, s.useMultiDomain, s.currentThinking))
		s.app.PrintSystem(fmt.Sprintf("已清除案件上下文（%s）", oldName))

	default:
		records := s.fc.ProjectRegistry.List()
		var matched *domains.ProjectRecord
		for i := range records {
			if strings.Contains(records[i].ProjectID, args) || strings.Contains(records[i].Alias, args) {
				matched = &records[i]
				break
			}
		}
		if matched == nil {
			s.app.PrintSystem(fmt.Sprintf("未找到匹配 '%s' 的案件。使用 /case 查看可用案件。", args))
			return
		}
		s.switchToProject(matched)
	}
}

func (s *tuiSession) switchToProject(matched *domains.ProjectRecord) {
	s.currentProject = matched
	s.currentProjectMeta = nil
	s.closeFileResources()
	s.fileIndexExt.SetFileIndex(nil)

	wsDir := s.fc.WorkspaceDir
	if wsDir == "" {
		wsDir = filepath.Join(os.TempDir(), "mady-fileindex")
	}
	dbPath := filepath.Join(wsDir, "projects", matched.ProjectID, "fileindex.db")

	if fi, err := fileindex.OpenFileIndex(matched.RootPath, dbPath); err == nil {
		_ = fi.Refresh(context.Background())
		s.currentFileIndex = fi
		s.fileIndexExt.SetFileIndex(fi)
		wcfg := fileindex.FileWatcherConfig{}
		s.currentFileWatcher = fileindex.NewFileWatcher(fi, wcfg)
		if err := s.currentFileWatcher.Start(context.Background()); err != nil {
			log.Printf("filewatcher: start: %v (continuing without)", err)
			s.currentFileWatcher = nil
		}
	}

	if meta, err := s.fc.ProjectRegistry.LoadMeta(matched.ProjectID); err == nil {
		s.currentProjectMeta = meta
	}
	s.rebuildAgent()
	s.app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.planMode, s.useMultiDomain, s.currentThinking))
	s.app.PrintSystem(fmt.Sprintf("已切换到案件: %s（%s）\n工作目录: %s\n⚖ 已启用五阶段法律推理工具（run_five_step_workflow）", matched.Alias, matched.ProjectID, matched.RootPath))
}

func (s *tuiSession) closeFileResources() {
	if s.currentFileWatcher != nil {
		s.currentFileWatcher.Stop()
		s.currentFileWatcher = nil
	}
	if s.currentFileIndex != nil {
		s.currentFileIndex.Close()
		s.currentFileIndex = nil
		s.fileIndexExt.SetFileIndex(nil)
		s.fileIndexExt.SetFallbackDir(s.fc.BaseConfig.ProjectDir)
	}
}

func (s *tuiSession) handleDeadlineCommand() {
	if s.currentProjectMeta == nil || len(s.currentProjectMeta.Deadlines) == 0 {
		s.app.PrintSystem("当前案件无期限信息。使用 /case 选择案件。")
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "案件 %s 的期限：\n", s.currentProject.Alias)
	for _, d := range s.currentProjectMeta.Deadlines {
		mark := "  "
		if d.Reminded {
			mark = "✓ "
		}
		fmt.Fprintf(&sb, "%s%s: %s\n", mark, d.Type, d.DueDate)
	}
	s.app.PrintSystem(sb.String())
}

func (s *tuiSession) handleClearCommand() {
	if s.agentStore != nil {
		s.currentThreadID = fmt.Sprintf("tui-%d", time.Now().UnixNano())
	}
	s.rebuildAgent()
	s.app.History().Clear()
	s.app.PrintSystem("已开始新对话")
}

func (s *tuiSession) handleBranchCommand() {
	if s.agentStore == nil {
		s.app.PrintSystem("会话持久化未启用，无法分支")
		return
	}
	snap, err := s.agentStore.GetThread(context.Background(), s.currentThreadID)
	if err != nil || len(snap.Messages) == 0 {
		s.app.PrintSystem("当前会话为空，无法分支")
		return
	}
	var lastEntryID string
	if len(snap.Transcript) > 0 {
		lastEntryID = snap.Transcript[len(snap.Transcript)-1].EntryID
	}
	branched, err := s.agentStore.BranchThread(context.Background(), s.currentThreadID, lastEntryID)
	if err != nil {
		s.app.PrintError(fmt.Errorf("分支失败: %w", err))
		return
	}
	oldID := s.currentThreadID
	s.currentThreadID = branched.Info.ID
	s.rebuildAgent()
	s.app.History().Clear()
	for _, msg := range branched.Messages {
		switch msg.Role {
		case agentcore.RoleUser:
			s.app.History().Append(chat.ChatMessage{Role: chat.RoleUser, Text: msg.Content})
		case agentcore.RoleAssistant:
			s.app.History().Append(chat.ChatMessage{Role: chat.RoleAssistant, Text: msg.Content})
		}
	}
	s.app.PrintSystem(fmt.Sprintf("已从 %s 创建分支 → %s（%d 条消息）", oldID, s.currentThreadID, len(branched.Messages)))
}

func (s *tuiSession) handleSaveCommand() {
	if s.agentStore != nil {
		threads, _ := s.agentStore.ListThreads(context.Background())
		msg := fmt.Sprintf("✅ 会话已自动保存到 %s（当前线程: %s", s.sessionDir, s.currentThreadID)
		if len(threads) > 0 {
			msg += fmt.Sprintf("，共 %d 个线程", len(threads))
		}
		msg += "）"
		s.app.PrintSystem(msg)
	} else {
		s.app.PrintSystem("⚠ 会话持久化未启用（session 目录创建失败）")
	}
}

func (s *tuiSession) handleCopyCommand() {
	msgs := s.app.History().Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == chat.RoleAssistant && msgs[i].Text != "" {
			go func(text string) {
				if err := chat.CopyToClipboard(text); err != nil {
					s.app.PrintError(err)
				} else {
					truncated := text
					if core.VisibleWidth(truncated) > 60 {
						truncated = core.TruncateToWidth(truncated, 57, "...")
					}
					s.app.PrintSystem("📋 已复制: " + truncated)
				}
			}(msgs[i].Text)
			return
		}
	}
	s.app.PrintSystem("没有可复制的助手回复")
}

func (s *tuiSession) handleExportCommand(trimmed string) {
	msgs := s.app.History().Messages()
	if len(msgs) == 0 {
		s.app.PrintSystem("当前对话为空，无法导出")
		return
	}
	exportPath := strings.TrimSpace(strings.TrimPrefix(trimmed, "/export"))
	if exportPath == "" {
		exportDir := "exports"
		if s.fc.MadyHome != "" {
			exportDir = filepath.Join(s.fc.MadyHome, "exports")
		}
		_ = os.MkdirAll(exportDir, 0o755)
		exportPath = filepath.Join(exportDir, fmt.Sprintf("export-%s.md", time.Now().Format("20060102-150405")))
	}
	exportContent := formatExportMarkdown(msgs, s.currentThreadID, s.currentProject)
	if err := os.WriteFile(exportPath, []byte(exportContent), 0o644); err != nil {
		s.app.PrintError(fmt.Errorf("导出失败: %w", err))
		return
	}
	s.app.PrintSystem(fmt.Sprintf("📄 已导出到 %s（%d 条消息）", exportPath, len(msgs)))
}

func (s *tuiSession) handleReviewCommand() {
	s.reviewMode = !s.reviewMode
	s.runMu.Lock()
	prev := s.currentAgent
	s.currentAgent = agentcore.New(s.buildAgentConfig())
	prev.Close()
	s.runMu.Unlock()
	agentadapter.BindAgent(s.app, s.currentAgent)
	s.app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.planMode, s.useMultiDomain, s.currentThinking))
	if s.reviewMode {
		s.app.PrintSystem("⚖ 审核关卡已启用 — 专利结论/法律意见/风险评估将插入人工审核提示")
		if s.currentProject != nil {
			ct := s.currentProject.CaseType
			if ct == "" {
				ct = "未分类"
			}
			s.app.PrintSystem(fmt.Sprintf("📁 当前案件: %s (%s)", s.currentProject.Alias, s.currentProject.ProjectID))
			s.app.PrintSystem(fmt.Sprintf("   📋 案件类型: %s", ct))
		}
		s.app.PrintSystem("   📌 触发关键词: 专利结论、侵权判断、法律意见、风险评估、最终建议")
		s.app.PrintSystem("   💡 使用 /approve 确认 /reject 拒绝/取消")
	} else {
		s.app.PrintSystem("⚖ 审核关卡已关闭")
	}
}

func (s *tuiSession) handlePlanCommand() {
	s.planMode = !s.planMode
	s.runMu.Lock()
	prev := s.currentAgent
	s.currentAgent = agentcore.New(s.buildAgentConfig())
	prev.Close()
	s.runMu.Unlock()
	agentadapter.BindAgent(s.app, s.currentAgent)
	mdl := s.normalModel
	if s.planMode {
		mdl = s.planModel
	}
	s.app.UpdateStatusBar(s.providerName, mdl, statusBarModeLabel(s.planMode, s.useMultiDomain, s.currentThinking))
	if s.planMode {
		s.app.PrintSystem("🧠 计划模式已启用 · 模型: " + s.planModel + " · 推理强度: max")
	} else {
		s.app.PrintSystem("⚡ 已切回普通模式 · 模型: " + s.normalModel)
	}
}

// openApprovalStore creates a SQLite-backed ApprovalStore in the workspace
// directory. The store persists human approval decisions (adopted/modified/
// rejected) so that real AdoptionRate data accumulates across sessions —
// the foundation for P3 expert blind testing and Golden Benchmark regression.
func (s *tuiSession) openApprovalStore() (domains.ApprovalStore, error) {
	base := s.fc.WorkspaceDir
	if base == "" {
		base = filepath.Join(os.TempDir(), "mady")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("approval store: mkdir %s: %w", base, err)
	}
	dbPath := filepath.Join(base, "approvals.db")
	store, err := sqlitestore.NewApprovalStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("approval store: open %s: %w", dbPath, err)
	}
	return store, nil
}

// recordApprovalDecision persists the human operator's verdict on the last
// gated Agent output. Called from /approve (adopted) and /reject (rejected).
// For /approve followed by edits, the modified output can be passed via the
// modifiedOutput parameter (used by a future /modify command).
func (s *tuiSession) recordApprovalDecision(decision domains.ApprovalDecision, modifiedOutput, feedback string) {
	if s.approvalGate == nil {
		return
	}
	caseID := ""
	if s.currentProject != nil {
		caseID = s.currentProject.ProjectID
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	// originalOutput="" lets the gate use its saved lastTriggeredOutput.
	if err := s.approvalGate.RecordDecision(ctx, s.currentThreadID, caseID, "review", "", decision, modifiedOutput, feedback); err != nil {
		log.Printf("approval: record decision: %v", err)
	}
}
