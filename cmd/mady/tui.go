package main

// 本文件实现 `mady tui` 子命令入口：交互式终端对话的启动装配
// （运行模式判定、会话持久化、SettingsStore、主题、slash 注册表、
// ChatApp 构建与 Agent 绑定）。会话交互逻辑见 tui_session.go，
// 辅助函数见 tui_helpers.go，slash 命令见 slash_registry.go。

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/permission"
	"github.com/xujian519/mady/domains/rules"
	"github.com/xujian519/mady/domains/writing"
	"github.com/xujian519/mady/knowledge/fileindex"
	"github.com/xujian519/mady/knowledge/risk"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/tui"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// loadWritingPatterns 加载写作模式种子文件并返回 PatternStore。
func loadWritingPatterns(madyHome string) *writing.PatternStore {
	store := writing.NewPatternStore()
	if madyHome == "" {
		return store
	}
	// 种子模式文件位于 $MADY_HOME/knowledge/seed-patterns/ 或项目目录。
	seedDirs := []string{
		filepath.Join(madyHome, "knowledge", "seed-patterns"),
		filepath.Join("domains", "writing", "seed-patterns"),
	}
	for _, dir := range seedDirs {
		count, err := store.LoadSeedDir(dir)
		if err == nil && count > 0 {
			log.Printf("writing: loaded %d seed pattern files from %s", count, dir)
			return store
		}
	}
	log.Println("writing: no seed patterns found, using empty store")
	return store
}

// defaultSystemPrompt 仅在多域 manifest 全部加载失败时的最终兜底。
// 正常情况下 mady 通过 go:embed 内置的 4 个领域 manifest 进入多域路由模式，
// 不会用到这个提示词。
const defaultSystemPrompt = "你是 Mady 智能助手，一个能力完备的通用 AI 代理。" +
	"你可以调用工具、检索知识、多步推理。请用简洁清晰的中文回答用户。"

// runTui launches the interactive terminal chat.
//
// 运行模式切换（优先级由高到低）：
//
//	MADY_SINGLE_AGENT=1 → 单 Agent 模式（纯 LLM 对话，无路由）
//	MADY_ROUTER_MODE=1  → Router 多域路由模式（传统交接可见）
//	默认（有 Manifest）  → 集成模式（Chat Agent 统一入口，内部 Invisible Handoff）
//	默认（无 Manifest）  → 单 Agent 模式（降级）
func runTui(ctx context.Context) {
	fs := flag.NewFlagSet("mady tui", flag.ExitOnError)
	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "mady tui: %v\n", err)
		os.Exit(1)
	}

	fc := setupFrameworkContext(ctx, "tui")

	if err := theme.InitThemeFromEnv(); err != nil {
		log.Printf("theme init: %v", err)
	}

	// 复用 setupFrameworkContext 已加载的规则引擎（避免重复 LoadEngineFromMadyHome）。
	ruleEngine := fc.RuleEngine
	var ruleExt agentcore.Extension
	if ruleEngine != nil {
		ruleExt = rules.NewExtension(ruleEngine)
	}

	// 风险扫描扩展（依赖知识库中的判决文书数据）。
	var riskExt agentcore.Extension
	if fc.WikiStore != nil {
		riskExt = risk.NewExtension(fc.WikiStore, risk.DefaultScannerConfig())
	}

	// 写作模式扩展。
	var writingExt agentcore.Extension
	if patternStore := loadWritingPatterns(fc.MadyHome); patternStore != nil {
		writingExt = writing.NewExtension(patternStore)
	}

	provider, err := agentconfig.BuildProvider()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mady tui: %v\n", err)
		os.Exit(1)
	}
	model := agentconfig.DefaultModel()

	useSingleAgent := os.Getenv("MADY_SINGLE_AGENT") == "1"
	useRouterMode := os.Getenv("MADY_ROUTER_MODE") == "1"
	useMultiDomain := !useSingleAgent && len(fc.Manifests) > 0
	useIntegratedMode := useMultiDomain && !useRouterMode

	sessionDir := os.Getenv("SESSION_DIR")
	if sessionDir == "" {
		if fc.MadyHome != "" {
			sessionDir = filepath.Join(fc.MadyHome, "sessions")
		} else {
			// 不可达兜底：MadyHome() 仅在 filepath.Abs 自身失败时返错。
			// 走 ResolveDataDir 以保证最终路径仍经过 filepath.Abs 规范化。
			dir, err := util.ResolveDataDir("sessions")
			if err != nil {
				log.Printf("resolve sessions dir: %v (falling back to empty)", err)
			}
			sessionDir = dir
		}
	}
	var agentStore *session.AgentStore
	fileStore, persistErr := session.NewFileStore(sessionDir)
	if persistErr != nil {
		log.Printf("session: %v (continuing without persistence)", persistErr)
	} else {
		agentStore = session.NewAgentStore(fileStore, fc.WorkspaceDir)
	}

	fileIndexExt := fileindex.NewExtension(fileindex.ExtensionConfig{
		FallbackDir: fc.BaseConfig.ProjectDir,
	})

	s := &tuiSession{
		ctx:               ctx,
		fc:                fc,
		provider:          provider,
		model:             model,
		providerName:      firstNonEmpty(os.Getenv("PROVIDER"), "deepseek"),
		planModel:         agentconfig.DefaultPlanModel,
		normalModel:       model,
		useMultiDomain:    useMultiDomain,
		useIntegratedMode: useIntegratedMode,
		ruleExt:           ruleExt,
		riskExt:           riskExt,
		writingExt:        writingExt,
		fileIndexExt:      fileIndexExt,
		toolApprover:      permission.NewTUIChannelApprover(),
		agentStore:        agentStore,
		checkpointSaver:   agentcore.NewMemoryCheckpointSaver(),
		currentThreadID:   "default",
		sessionDir:        sessionDir,
	}

	// 初始化 SettingsStore（~/.mady/settings.json），优先于其他操作
	if homeDir, err := os.UserHomeDir(); err == nil {
		store, storeErr := NewSettingsStore(filepath.Join(homeDir, ".mady", "settings.json"))
		if storeErr == nil {
			s.store = store
		}
	}
	if s.store == nil {
		s.store, _ = NewSettingsStore("")
	}

	// 从 store 读取持久化的主题并应用（首次启动使用 mady-dark 默认值）。
	// 直接调 SetSemanticTheme 而不是 handleThemeCommand，因为此时 s.app 尚未初始化。
	applyStoredTheme(s)

	// 同步当前终端检测到的主题到 store（仅在首次启动时）
	if name := theme.CurrentPalette().Semantic.Name; strings.Contains(strings.ToLower(name), "light") {
		if err := s.store.Set(SettingKeyTheme, "light", SettingsScopeGlobal); err != nil {
			log.Printf("settings: persist theme: %v", err)
		}
		theme.SetSemanticTheme(theme.DefaultSemanticLight(), theme.DetectColorMode())
	}

	// Build the slash registry once; both handleSubmit and the autocomplete
	// menu read from it (single source of truth, no dual switch).
	s.slashReg = s.buildSlashRegistry()
	slashSuggestions := s.slashReg.Suggestions(s)

	var app *chat.ChatApp
	app = tui.NewChatApp(chat.ChatAppConfig{
		Title:                      fmt.Sprintf("mady · model=%s", model),
		ShowTurns:                  true,
		SuppressHandoffToolDisplay: useIntegratedMode,
		AltScreen:                  true,
		MouseMode:                  "auto",
		KittyKeyboardFlags:         1,
		ContextWindow:              fc.BaseConfig.ContextWindow,
		Context:                    ctx,
		OnInterrupt: func() {
			s.cancelMu.Lock()
			defer s.cancelMu.Unlock()
			if s.runCancel != nil {
				s.runCancel()
				s.runCancel = nil
			}
		},
		OnQuit: func() {
			if app != nil {
				_ = app.Stop()
			}
		},
		Providers: []core.AutocompleteProvider{
			&component.StaticProvider{
				TriggerStr:  "/",
				Suggestions: slashSuggestions,
			},
		},
		OnSubmit: func(_ context.Context, input string) {
			s.handleSubmit(input)
		},
	})
	s.app = app
	// Agent 在 app.Start() 之后创建并绑定，避免阻塞首帧渲染。

	// 现在 s.app 已就绪，通过 handler 应用持久化主题（同时更新 History 主题 + 状态栏）
	s.handleThemeCommand("/theme " + s.store.Get(SettingKeyTheme))

	// Load user keymap overrides from ~/.mady/keymap.json (if present) into the
	// app's keybinding manager so the editor and chat honor customized keys.
	// Warnings (unknown tokens) are surfaced to the user but never fatal.
	if fc.MadyHome != "" {
		if warnings := loadKeymapOverrides(fc.MadyHome, app.Keybindings()); len(warnings) > 0 {
			for _, w := range warnings {
				log.Printf("keymap: %s", w)
			}
		}
	}

	// Phase 4.4: install sidebar when the terminal is wide enough
	app.SetSidebar(s.buildSidebar())

	app.UpdateStatusBar(s.providerName, s.normalModel, statusBarModeLabel(s.isPlanMode(), useMultiDomain, s.thinkingConfig()))

	modeInfo := "单 Agent 模式"
	if useMultiDomain {
		modeInfo = "多域路由模式"
	}
	app.PrintSystem(fmt.Sprintf("Mady 中观智能体已启动（%s）。正在初始化 Agent，请稍候… 输入 / 查看命令，Ctrl+C 退出。", modeInfo))
	if fc.WikiStore != nil {
		st := fc.WikiStore.Stats()
		app.PrintSystem(fmt.Sprintf("wiki 知识库: %d 文档, %d 分块 (RAG: patent)", st.TotalDocs, st.TotalChunks))
	}

	// 先启动 TUI 渲染，再在后台初始化 Agent，避免 agentcore.New 阻塞首帧。
	// 启动后到 Agent 就绪前会经过显式“初始化中”状态；submitInput、/mode
	// 等入口统一读取该状态，而不是依赖裸 nil 判断。
	if err := app.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		return
	}
	s.initializeAgentAsync()

	<-app.Done()
	if agent := s.shutdownAgent(); agent != nil {
		agent.Close()
	}
}

// applyStoredTheme reads the persisted theme from the store and applies it to
// the terminal palette. Called early in startup before s.app exists, so it uses
// SetSemanticTheme directly rather than going through handleThemeCommand.
func applyStoredTheme(s *tuiSession) {
	name := s.store.Get(SettingKeyTheme)
	switch name {
	case "light":
		theme.SetSemanticTheme(theme.DefaultSemanticLight(), theme.DetectColorMode())
	case "dark":
		theme.SetSemanticTheme(theme.DefaultMadyDark(), theme.DetectColorMode())
	default:
		theme.SetSemanticTheme(theme.DefaultSemanticLight(), theme.DetectColorMode())
	}
}

func firstNonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}
