package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/permission"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/reasoning"
	reasoningsqlite "github.com/xujian519/mady/domains/reasoning/sqlite"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/knowledge/fileindex"
	"github.com/xujian519/mady/memory"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
	"github.com/xujian519/mady/workflows/patent"

	// graph 包用于 PregelState 构建（斜杠命令直接调用工作流）
	"github.com/xujian519/mady/graph"
)

// tuiSession holds the mutable state shared across TUI command handlers.
// All slash-command logic operates on this struct instead of capturing
// local variables in closures, making the code testable and readable.
type tuiSession struct {
	ctx context.Context
	fc  *frameworkContext

	// Provider/model config
	provider     agentcore.Provider
	model        string
	providerName string
	planModel    string
	normalModel  string

	// Mode flags
	useMultiDomain    bool
	useIntegratedMode bool

	// Extensions
	writingExt   agentcore.Extension
	fileIndexExt *fileindex.Extension
	memExt       *memory.MemoryExtension

	// Agent state
	agentMu           sync.RWMutex
	currentAgent      *agentcore.Agent
	agentInitInFlight bool
	agentInitErr      string
	shuttingDown      bool
	runMu             sync.Mutex
	cancelMu          sync.Mutex
	runCancel         context.CancelFunc

	// Session persistence
	agentStore      *session.AgentStore
	checkpointSaver *agentcore.MemoryCheckpointSaver
	currentThreadID string
	sessionDir      string
	workflowStore   reasoning.CheckpointStore

	// Project/case context
	currentProject     *domains.ProjectRecord
	currentProjectMeta *domains.ProjectMeta
	currentFileIndex   *fileindex.FileIndex
	currentFileWatcher *fileindex.FileWatcher

	// Approval gate state
	approvalGate *domains.ApprovalGate

	// toolApprover is the interactive tool-call approval controller.
	toolApprover *permission.TUIChannelApprover

	app *chat.ChatApp

	// slashReg is the single source of truth for slash commands.
	slashReg *Registry

	// store is the single source of truth for settings.
	store *SettingsStore
}

// --- Simple accessors ---

func (s *tuiSession) isPlanMode() bool   { return s.store.Get(SettingKeyPlan) == "on" }
func (s *tuiSession) isReviewMode() bool { return s.store.Get(SettingKeyReview) == "on" }
func (s *tuiSession) themeName() string  { return s.store.Get(SettingKeyTheme) }

func (s *tuiSession) applyThinkingConfig(cfg *agentcore.ThinkingConfig) {
	val := "default"
	if cfg != nil {
		switch cfg.Display {
		case agentcore.ThinkingDisplaySummarized:
			val = "summarized"
		case agentcore.ThinkingDisplayOmitted:
			val = "omitted"
		}
	}
	if err := s.store.Set(SettingKeyThinking, val, SettingsScopeGlobal); err != nil {
		log.Printf("settings: persist thinking: %v", err)
	}
}

func (s *tuiSession) thinkingConfig() *agentcore.ThinkingConfig {
	switch s.store.Get(SettingKeyThinking) {
	case "summarized":
		return &agentcore.ThinkingConfig{Display: agentcore.ThinkingDisplaySummarized}
	case "omitted":
		return &agentcore.ThinkingConfig{Display: agentcore.ThinkingDisplayOmitted}
	default:
		return nil
	}
}

func (s *tuiSession) detectAgentID() string {
	switch {
	case s.useIntegratedMode:
		return "chat-agent"
	case s.useMultiDomain:
		return "router"
	default:
		return "single"
	}
}

func (s *tuiSession) detectProjectID() string {
	if s.currentProject != nil {
		return s.currentProject.ProjectID
	}
	return ""
}

// --- Slash command handlers ---

func (s *tuiSession) handleSubmit(input string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return
	}

	// Check for pending tool approval request.
	if req := s.toolApprover.PollPending(); req != nil {
		switch strings.ToLower(trimmed) {
		case "y", "yes", "n", "no":
			if trimmed == "y" || trimmed == "yes" {
				s.toolApprover.Respond(permission.DecisionAllow)
				s.app.PrintSystem("✅ 已允许执行: " + req.ToolName)
			} else {
				s.toolApprover.Respond(permission.DecisionDeny)
				s.app.PrintSystem("❌ 已拒绝执行: " + req.ToolName)
			}
		default:
			s.app.PrintSystem("输入 y (允许) 或 n (拒绝) 以回应审批请求")
		}
		return
	}

	if cmd := s.slashReg.Lookup(trimmed, s); cmd != nil {
		cmd.Handler(slashCtx{s: s, input: trimmed})
		return
	}

	if strings.HasPrefix(trimmed, "/") {
		suggestions := s.slashReg.Suggest(trimmed, s)
		if len(suggestions) > 0 {
			quoted := make([]string, len(suggestions))
			for i, n := range suggestions {
				quoted[i] = "/" + n
			}
			s.app.PrintSystem(fmt.Sprintf("未知命令: %s — 你是不是想输入 %s？",
				trimmed, strings.Join(quoted, " 或 ")))
		} else {
			s.app.PrintSystem(fmt.Sprintf("未知命令: %s（输入 / 查看可用命令）", trimmed))
		}
		return
	}
	s.submitInput(trimmed)
}

func (s *tuiSession) handleThinkingCommand(trimmed string) {
	next, changed, err := parseThinkingCommand(trimmed, s.thinkingConfig())
	if err != nil {
		s.app.PrintError(err)
		return
	}
	if !changed {
		s.app.PrintSystem("推理配置: " + formatThinkingConfig(s.thinkingConfig()))
		return
	}
	s.applyThinkingConfig(next)
	s.rebuildAgent()
	s.app.PrintSystem("推理配置已更新: " + formatThinkingConfig(s.thinkingConfig()))
	mdl := s.normalModel
	if s.isPlanMode() {
		mdl = s.planModel
	}
	s.app.UpdateStatusBar(s.providerName, mdl, statusBarModeLabel(s.isPlanMode(), s.useMultiDomain, s.thinkingConfig()))
}

func (s *tuiSession) handleThemeCommand(trimmed string) {
	switch trimmed {
	case "/theme":
		s.app.PrintSystem("当前主题: " + s.themeName())

	case "/theme light":
		theme.SetSemanticTheme(theme.DefaultSemanticLight(), theme.DetectColorMode())
		s.app.History().SetTheme(chat.DefaultChatHistoryTheme())
		if err := s.store.Set(SettingKeyTheme, "light", SettingsScopeGlobal); err != nil {
			log.Printf("settings: persist theme: %v", err)
		}
		s.app.PrintSystem("已切换浅色主题")

	case "/theme dark":
		theme.SetSemanticTheme(theme.DefaultMadyDark(), theme.DetectColorMode())
		s.app.History().SetTheme(chat.DefaultChatHistoryTheme())
		if err := s.store.Set(SettingKeyTheme, "dark", SettingsScopeGlobal); err != nil {
			log.Printf("settings: persist theme: %v", err)
		}
		s.app.PrintSystem("已切换深色主题")
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
		s.app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.isPlanMode(), s.useMultiDomain, s.thinkingConfig()))
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
	s.app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.isPlanMode(), s.useMultiDomain, s.thinkingConfig()))
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
	if err := os.WriteFile(exportPath, []byte(exportContent), 0o600); err != nil {
		s.app.PrintError(fmt.Errorf("导出失败: %w", err))
		return
	}
	s.app.PrintSystem(fmt.Sprintf("📄 已导出到 %s（%d 条消息）", exportPath, len(msgs)))
}

// handleReviewCommandEx implements /review [on|off|status].
func (s *tuiSession) handleReviewCommandEx(sub string) {
	switch sub {
	case "on":
		if s.isReviewMode() {
			s.app.PrintSystem("⚖ 审核关卡已在启用状态")
			return
		}
		if err := s.store.Set(SettingKeyReview, "on", SettingsScopeGlobal); err != nil {
			log.Printf("settings: persist review: %v", err)
		}
	case "off":
		if !s.isReviewMode() {
			s.app.PrintSystem("⚖ 审核关卡已在关闭状态")
			return
		}
		if err := s.store.Set(SettingKeyReview, "off", SettingsScopeGlobal); err != nil {
			log.Printf("settings: persist review: %v", err)
		}
	default:
		status := "关闭"
		if s.isReviewMode() {
			status = "启用"
		}
		s.app.PrintSystem(fmt.Sprintf("⚖ 审核关卡: %s  |  使用 /review on 或 /review off 切换", status))
		return
	}

	s.rebuildAgent()
	s.app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.isPlanMode(), s.useMultiDomain, s.thinkingConfig()))
	if s.isReviewMode() {
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

// handlePlanCommandEx implements /plan [on|off|status].
func (s *tuiSession) handlePlanCommandEx(sub string) {
	switch sub {
	case "on":
		if s.isPlanMode() {
			s.app.PrintSystem(fmt.Sprintf("🧠 计划模式已在启用状态 · 模型: %s", s.planModel))
			return
		}
		if err := s.store.Set(SettingKeyPlan, "on", SettingsScopeGlobal); err != nil {
			log.Printf("settings: persist plan: %v", err)
		}
	case "off":
		if !s.isPlanMode() {
			s.app.PrintSystem(fmt.Sprintf("⚡ 已在普通模式 · 模型: %s", s.normalModel))
			return
		}
		if err := s.store.Set(SettingKeyPlan, "off", SettingsScopeGlobal); err != nil {
			log.Printf("settings: persist plan: %v", err)
		}
	default:
		status := "关闭（普通模式）"
		mdl := s.normalModel
		if s.isPlanMode() {
			status = "启用"
			mdl = s.planModel
		}
		s.app.PrintSystem(fmt.Sprintf("🧠 计划模式: %s · 模型: %s  |  使用 /plan on 或 /plan off 切换", status, mdl))
		return
	}

	s.rebuildAgent()
	mdl := s.normalModel
	if s.isPlanMode() {
		mdl = s.planModel
	}
	s.app.UpdateStatusBar(s.providerName, mdl, statusBarModeLabel(s.isPlanMode(), s.useMultiDomain, s.thinkingConfig()))
	if s.isPlanMode() {
		s.app.PrintSystem("🧠 计划模式已启用 · 模型: " + s.planModel + " · 推理强度: max")
	} else {
		s.app.PrintSystem("⚡ 已切回普通模式 · 模型: " + s.normalModel)
	}
}

// --- Settings ---

func (s *tuiSession) handleSettingsReset() {
	if err := s.store.Reset(); err != nil {
		s.app.PrintError(fmt.Errorf("settings reset failed: %w", err))
		return
	}
	s.rebuildAgent()
	mdl := s.normalModel
	if s.isPlanMode() {
		mdl = s.planModel
	}
	s.app.UpdateStatusBar(s.providerName, mdl, statusBarModeLabel(s.isPlanMode(), s.useMultiDomain, s.thinkingConfig()))
	s.app.PrintSystem("✅ 设置已恢复默认值")
	for k, v := range s.store.Export() {
		s.app.PrintSystem(fmt.Sprintf("  %s = %s", k, v))
	}
}

// --- Approval store ---

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

// openWorkflowCheckpointStore 打开 SQLite 工作流检查点存储。
// 参照 openApprovalStore 模式，使用 WorkspaceDir 作为基准路径。
// 返回错误时调用方应回退到内存存储。
func (s *tuiSession) openWorkflowCheckpointStore() (reasoning.CheckpointStore, error) {
	base := s.fc.WorkspaceDir
	if base == "" {
		base = filepath.Join(os.TempDir(), "mady")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("workflow checkpoint: mkdir %s: %w", base, err)
	}
	dbPath := filepath.Join(base, "workflow_checkpoints.db")
	store, err := reasoningsqlite.NewCheckpointStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("workflow checkpoint: open %s: %w", dbPath, err)
	}
	return store, nil
}

func (s *tuiSession) recordApprovalDecision(decision domains.ApprovalDecision, modifiedOutput, feedback string) {
	if s.approvalGate == nil {
		return
	}
	caseID := ""
	if s.currentProject != nil {
		caseID = s.currentProject.ProjectID
	}
	triggerKeyword := "review"
	originalOutput := ""
	if agent := s.getCurrentAgent(); agent != nil {
		if ir := agent.Interrupted(); ir != nil {
			if gate, ok := ir.Data["gate"].(string); ok && gate != "" {
				triggerKeyword = gate
			}
			originalOutput = ir.Reason
			if len(ir.Data) > 0 {
				if data, err := json.Marshal(ir.Data); err == nil {
					originalOutput += "\n" + string(data)
				}
			}
		}
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	if err := s.approvalGate.RecordDecision(ctx, s.currentThreadID, caseID, triggerKeyword, originalOutput, decision, modifiedOutput, feedback); err != nil {
		log.Printf("approval: record decision: %v", err)
	}
}

// persistSlashMessages 将斜杠命令的用户输入和 Pregel 输出写入 AgentStore JSONL，
// 确保分析结果不因 TUI 重启而丢失。
//
// 若持久化未启用（agentStore == nil），静默跳过；错误仅记录日志，不阻塞显示。
func (s *tuiSession) persistSlashMessages(inputLine, outputText string) {
	if s.agentStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	// 加载当前线程已有消息准备追加。
	existing, err := s.agentStore.Load(ctx, s.currentThreadID)
	if err != nil {
		// Load 在首次使用空线程时返回 StatusIdle + 空 Messages，不会报错。
		log.Printf("[mady] load thread for slash persistence: %v", err)
		return
	}

	msgs := existing.Messages
	msgs = append(msgs,
		agentcore.Message{Role: agentcore.RoleUser, Content: inputLine},
		agentcore.Message{Role: agentcore.RoleAssistant, Content: outputText},
	)

	snap := agentcore.StateSnapshot{
		Status:     agentcore.StatusFinished,
		Messages:   msgs,
		Turn:       existing.Turn + 1,
		TotalUsage: existing.TotalUsage,
	}

	if err := s.agentStore.Save(ctx, s.currentThreadID, snap); err != nil {
		log.Printf("[mady] persist slash result: %v", err)
	}
}

// handleNoveltySlash 处理 /novelty <描述> 斜杠命令——直接运行新颖性分析 Pregel 图，
// 绕过 LLM 意图分类，结果输出到聊天面板并经 AgentStore 持久化。
func (s *tuiSession) handleNoveltySlash(ctx slashCtx) {
	// 提取 /novelty 之后的描述文本。
	description := strings.TrimSpace(strings.TrimPrefix(ctx.input, "/novelty"))
	description = strings.Trim(description, `"'`)
	if description == "" {
		s.app.PrintSystem("用法: /novelty <发明描述>\n" +
			"示例: /novelty \"一种基于深度学习的图像识别方法，包括卷积神经网络...\"")
		return
	}

	opts := []patent.GraphOption{}
	if retriever := domains.GetPatentRetriever(); retriever != nil {
		opts = append(opts, patent.WithRetriever(retriever))
	}
	compiled, err := patent.BuildNoveltyGraphWithRulesWithOpts(opts...)
	if err != nil {
		s.app.PrintError(fmt.Errorf("新颖性分析引擎初始化失败: %w", err))
		return
	}

	state, err := compiled.Run(s.ctx, graph.PregelState{
		patent.StateInput: description,
	})
	if err != nil {
		s.app.PrintError(fmt.Errorf("新颖性分析执行失败: %w", err))
		return
	}

	output := state.GetString(patent.StateOutput)
	if output == "" {
		s.app.PrintSystem("分析完成但未能生成输出结果。")
		return
	}

	// 先持久化再显示：确保即使写入失败也不阻断用户体验。
	s.persistSlashMessages(ctx.input, output)
	s.app.PrintSystem(output)
}

// handleOASlash 处理 /oa <OA通知书文本> 斜杠命令——直接运行 OA 答复起草 Pregel 图。
func (s *tuiSession) handleOASlash(ctx slashCtx) {
	oaText := strings.TrimSpace(strings.TrimPrefix(ctx.input, "/oa"))
	oaText = strings.Trim(oaText, `"'`)
	if oaText == "" {
		s.app.PrintSystem("用法: /oa <OA通知书文本>\n" +
			"示例: /oa \"审查员认为权利要求1不具备专利法第22条第2款规定的新颖性\"")
		return
	}

	compiled, err := patent.BuildOAResponseGraph()
	if err != nil {
		s.app.PrintError(fmt.Errorf("OA 答复引擎初始化失败: %w", err))
		return
	}

	state, err := compiled.Run(s.ctx, graph.PregelState{
		patent.OAStateInput: oaText,
	})
	if err != nil {
		s.app.PrintError(fmt.Errorf("OA 答复生成失败: %w", err))
		return
	}

	output := state.GetString(patent.OAStateOutput)
	if output == "" {
		s.app.PrintSystem("OA 答复生成完成但未能生成输出结果。")
		return
	}

	// 先持久化再显示。
	s.persistSlashMessages(ctx.input, output)
	s.app.PrintSystem(output)
}
