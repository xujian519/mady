package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/permission"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/reasoning"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/knowledge/fileindex"
	"github.com/xujian519/mady/memory"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/tui/chat"
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
	pendingStore domains.PendingStore

	// Graph checkpoint store (SQLite-backed, for Pregel/DAG graph persistence)
	graphCheckpointStore *sqlitestore.SQLiteGraphCheckpointStore

	// Event logger (SQLite-backed, persists lifecycle events for audit trail)
	eventLogger *agentcore.EventLogger

	// toolApprover is the interactive tool-call approval controller.
	toolApprover *permission.TUIChannelApprover

	app *chat.ChatApp

	// slashReg is the single source of truth for slash commands.
	slashReg *Registry

	// store is the single source of truth for settings.
	store *SettingsStore

	// cancelAutoWatch cancels the system appearance watcher goroutine.
	cancelAutoWatch func()
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
	return "mady-agent"
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
			// 建议数量少时直接提示即可，多时打开命令中心便于浏览
			if len(suggestions) <= 2 {
				return
			}
		}
		// 打开命令中心并预填错误命令名，让用户筛选/浏览所有可用命令
		filter := strings.TrimPrefix(trimmed, "/")
		if sp := strings.IndexByte(filter, ' '); sp >= 0 {
			filter = filter[:sp]
		}
		s.app.PrintSystem(fmt.Sprintf("已打开命令中心 — 输入「%s」搜索类似命令", filter))
		s.openCommandCenter(filter)
		return
	}
	s.submitInput(trimmed)
}
