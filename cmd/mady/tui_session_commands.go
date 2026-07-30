package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/knowledge/fileindex"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

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
	s.app.UpdateStatusBar(s.providerName, mdl, statusBarModeLabel(s.isPlanMode(), s.thinkingConfig()))
}

func (s *tuiSession) handleThemeCommand(trimmed string) {
	switch {
	case trimmed == "/theme":
		current := s.themeName()
		if current == "" {
			current = "auto"
		}
		s.app.PrintSystem("当前主题: " + current)
		// List available themes on a second line.
		names := theme.ThemeNames()
		s.app.PrintSystem("可用主题: " + strings.Join(names, ", "))
	case trimmed == "/theme list":
		names := theme.ThemeNames()
		s.app.PrintSystem("可用主题: " + strings.Join(names, ", "))
		current := s.themeName()
		if current == "" {
			current = "auto"
		}
		s.app.PrintSystem("当前: " + current)

	case strings.HasPrefix(trimmed, "/theme "):
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "/theme "))
		if name == "" {
			s.app.PrintSystem("当前主题: " + s.themeName())
			return
		}
		if info := theme.ThemeInfoByName(name); info != nil {
			if err := theme.SetThemeByName(name); err != nil {
				log.Printf("theme: apply %s: %v", name, err)
			}
			s.app.History().SetTheme(chat.DefaultChatHistoryTheme())
			if err := s.store.Set(SettingKeyTheme, name, SettingsScopeGlobal); err != nil {
				log.Printf("settings: persist theme: %v", err)
			}
			s.app.PrintSystem("已切换主题: " + info.Display)

			// Manage system appearance watcher.
			if name == "auto" {
				if s.cancelAutoWatch == nil {
					s.cancelAutoWatch = theme.StartAutoThemeWatcher()
					s.app.PrintSystem("已启用系统外观跟随 — macOS 切换深色/浅色时自动切换")
				}
			} else {
				if s.cancelAutoWatch != nil {
					s.cancelAutoWatch()
					s.cancelAutoWatch = nil
				}
			}
		} else {
			s.app.PrintSystem("未知主题: " + name + "。可用主题: " + strings.Join(theme.ThemeNames(), ", "))
		}

	default:
		s.app.PrintSystem("当前主题: " + s.themeName())
	}
}

// detectCaseFromCWD checks if the current working directory is associated with
// a known case and loads its context. When no case is found, automatically
// creates a transient project context from CWD so the agent always has a
// working directory — no manual case registration needed.
func (s *tuiSession) detectCaseFromCWD() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd, _ = os.UserHomeDir()
		if cwd == "" {
			return ""
		}
	}
	// 先尝试通过 CaseIndex 查找
	if s.fc.CaseIndex != nil {
		cases, _ := s.fc.CaseIndex.FindByPath(context.Background(), cwd)
		if len(cases) > 0 {
			rec := cases[0]
			return s.loadCaseContext(&rec)
		}
	}
	// 回退到 ProjectRegistry（旧机制）
	if s.fc.ProjectRegistry != nil {
		records := s.fc.ProjectRegistry.List()
		for _, rec := range records {
			if rec.RootPath == cwd || strings.HasPrefix(cwd, rec.RootPath+string(filepath.Separator)) {
				r := rec
				return s.switchToProject(&r)
			}
		}
	}
	// 未关联已知案件 → 自动以 CWD 创建瞬态项目上下文
	dirName := filepath.Base(cwd)
	pr := &domains.ProjectRecord{
		ProjectID:    fmt.Sprintf("cwd-%d", time.Now().UnixNano()),
		Domain:       "",
		Alias:        dirName,
		RootPath:     cwd,
		Status:       domains.StatusActive,
		RegisteredAt: time.Now(),
		LastAccessed: time.Now(),
	}
	s.applyProjectContext(pr, nil)
	return fmt.Sprintf("工作目录: %s", cwd)
}

// applyProjectContext sets the project record + metadata, opens file index,
// rebuilds the agent, and updates the status bar. It is the single shared
// post-assignment sequence for all three entry paths (CWD fallback, CaseIndex
// match, ProjectRegistry match).
func (s *tuiSession) applyProjectContext(pr *domains.ProjectRecord, meta *domains.ProjectMeta) {
	s.currentProject = pr
	s.currentProjectMeta = meta
	s.openFileIndexForPath(pr.RootPath, pr.ProjectID)
	s.rebuildAgent()
	s.app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.isPlanMode(), s.thinkingConfig()))
}

// loadCaseContext sets the session's case context from a CaseRecord.
// Returns a status message for the caller to display.
func (s *tuiSession) loadCaseContext(rec *domains.CaseRecord) string {
	pr := rec.ToProjectRecord()
	meta := rec.ToProjectMeta()
	s.applyProjectContext(&pr, &meta)
	return fmt.Sprintf("已加载案件: %s（%s）\n工作目录: %s", rec.DisplayLabel(), rec.PrimaryIdentity(), pr.RootPath)
}

// switchToProject sets the session's case context from a ProjectRecord.
// Returns a status message for the caller to display.
func (s *tuiSession) switchToProject(matched *domains.ProjectRecord) string {
	s.closeFileResources()

	meta, err := s.fc.ProjectRegistry.LoadMeta(s.ctx, matched.ProjectID)
	if err != nil {
		meta = nil
	}
	s.applyProjectContext(matched, meta)
	return fmt.Sprintf("已切换到案件: %s（%s）\n工作目录: %s\n⚖ 已启用五阶段法律推理工具（run_five_step_workflow）", matched.Alias, matched.ProjectID, matched.RootPath)
}

// openFileIndexForPath opens a file index for the given root path and project ID.
// Both parameters are explicit — this method does not read s.currentProject.
func (s *tuiSession) openFileIndexForPath(rootPath, projectID string) {
	wsDir := s.fc.WorkspaceDir
	if wsDir == "" {
		wsDir = filepath.Join(os.TempDir(), "mady-fileindex")
	}
	dbPath := filepath.Join(wsDir, "projects", projectID, "fileindex.db")

	if fi, err := fileindex.OpenFileIndex(rootPath, dbPath); err == nil {
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
}

func (s *tuiSession) closeFileResources() {
	if s.currentFileWatcher != nil {
		s.currentFileWatcher.Stop()
		s.currentFileWatcher = nil
	}
	if s.currentFileIndex != nil {
		if err := s.currentFileIndex.Close(); err != nil {
			log.Printf("close FileIndex: %v", err)
		}
		s.currentFileIndex = nil
		s.fileIndexExt.SetFileIndex(nil)
	}
}

func (s *tuiSession) handleDeadlineCommand() {
	if s.currentProjectMeta == nil || len(s.currentProjectMeta.Deadlines) == 0 {
		s.app.PrintSystem("当前案件无期限信息。")
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
		s.persistActiveSession()
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
	s.persistActiveSession()
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
		s.persistActiveSession()
		threads, _ := s.agentStore.ListThreads(context.Background())
		msg := fmt.Sprintf("✅ 已自动保存（当前线程: %s", s.currentThreadID)
		if len(threads) > 0 {
			msg += fmt.Sprintf("，共 %d 个线程", len(threads))
		}
		msg += "）"
		s.app.PrintSystem(msg)
	} else {
		s.app.PrintSystem("⚠ 会话持久化未启用")
	}
}

// persistActiveSession saves the current thread ID to settings so it can be
// restored on next TUI startup. Errors are non-critical and silently ignored.
func (s *tuiSession) persistActiveSession() {
	if s.store != nil && s.currentThreadID != "" {
		_ = s.store.Set(SettingKeyLastSession, s.currentThreadID, SettingsScopeGlobal)
	}
}

// handleSessionNameCommand assigns or displays the current session name.
// The EntrySessionInfo entry type already exists in the session store but
// has no TUI command to write it. This fills that gap.
func (s *tuiSession) handleSessionNameCommand(input string) {
	name := strings.TrimSpace(strings.TrimPrefix(input, "/session"))
	name = strings.TrimSpace(name)
	if name == "" {
		// Show current session info.
		s.app.PrintSystem(fmt.Sprintf("当前线程: %s（在 %s）", s.currentThreadID, s.sessionDir))
		return
	}
	if s.agentStore == nil {
		s.app.PrintSystem("会话持久化未启用")
		return
	}
	// Store session name via the agent store.
	if err := s.agentStore.SetThreadName(context.Background(), s.currentThreadID, name); err != nil {
		s.app.PrintError(fmt.Errorf("保存会话名失败: %w", err))
		return
	}
	s.app.PrintSystem(fmt.Sprintf("✅ 当前线程已命名为: %s", name))
}

// handleSessionsCommand lists all stored sessions with their IDs, names,
// message counts, and last-updated timestamps.
func (s *tuiSession) handleSessionsCommand() { //nolint:unused // kept for test coverage; replaced by interactive selector
	if s.agentStore == nil {
		s.app.PrintSystem("会话持久化未启用")
		return
	}
	ctx := context.Background()
	threads, err := s.agentStore.ListThreads(ctx)
	if err != nil {
		s.app.PrintError(fmt.Errorf("读取会话列表失败: %w", err))
		return
	}
	if len(threads) == 0 {
		s.app.PrintSystem("无已保存的会话")
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "📋 会话列表（共 %d 个）：\n", len(threads))
	for _, t := range threads {
		mark := " "
		if t.ID == s.currentThreadID {
			mark = "→"
		}
		name := t.Name
		if name == "" {
			name = t.ID
			if len(name) > 20 {
				name = name[:20] + "…"
			}
		}
		fmt.Fprintf(&b, "  %s %s (%d 条消息, %s)\n",
			mark, name, t.MessageCount, t.UpdatedAt.Format("01-02 15:04"))
	}
	b.WriteString("\n使用 /session <名称> 为当前线程命名")
	s.app.PrintSystem(b.String())
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
		_ = os.MkdirAll(exportDir, util.DefaultDirPerm)
		exportPath = filepath.Join(exportDir, fmt.Sprintf("export-%s.md", time.Now().Format("20060102-150405")))
	}
	exportContent := formatExportMarkdown(msgs, s.currentThreadID, s.currentProject)
	if err := os.WriteFile(exportPath, []byte(exportContent), util.DefaultFilePerm); err != nil {
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
	s.app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.isPlanMode(), s.thinkingConfig()))
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
	s.app.UpdateStatusBar(s.providerName, mdl, statusBarModeLabel(s.isPlanMode(), s.thinkingConfig()))
	if s.isPlanMode() {
		s.app.PrintSystem("🧠 计划模式已启用 · 模型: " + s.planModel + " · 推理强度: max")
	} else {
		s.app.PrintSystem("⚡ 已切回普通模式 · 模型: " + s.normalModel)
	}
}
