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
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/permission"
	"github.com/xujian519/mady/domains/writing"
	"github.com/xujian519/mady/knowledge/fileindex"
	"github.com/xujian519/mady/pkg/agentconfig"
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

	// 写作模式扩展：本地独立加载，不依赖延迟队列。
	var writingExt agentcore.Extension
	if patternStore := loadWritingPatterns(fc.MadyHome); patternStore != nil {
		writingExt = writing.NewExtension(patternStore)
	}

	// 复用 setupFrameworkContext 已构建的 Provider，避免重复调用 agentconfig.BuildProvider()。
	provider := fc.Provider
	model := agentconfig.DefaultModel()

	useSingleAgent := os.Getenv("MADY_SINGLE_AGENT") == "1"
	useRouterMode := os.Getenv("MADY_ROUTER_MODE") == "1"
	useMultiDomain := !useSingleAgent && len(fc.Manifests) > 0
	useIntegratedMode := useMultiDomain && !useRouterMode

	// === 存储预检（Sprint 1B） ===
	// 在构建 tuiSession 前完成所有存储探测，结果写入 s.storageProbes。
	var storageProbes []StorageProbeResult

	// Session 持久化探针
	sessionDir := os.Getenv("SESSION_DIR")
	sessionProbe := probeSessionDir(sessionDir, fc.MadyHome, fc.WorkspaceDir)
	storageProbes = append(storageProbes, sessionProbe)

	// 如果 session 可写，构建 agentStore（供后续 tuiSession 使用）。
	var agentStore *session.AgentStore
	if sessionProbe.Unavailable {
		log.Printf("session: %s (continuing without persistence)", sessionProbe.Message)
	} else {
		fileStore, err := session.NewFileStore(sessionProbe.ResolvedDir)
		if err != nil {
			log.Printf("session: %v (continuing without persistence)", err)
		} else {
			agentStore = session.NewAgentStore(fileStore, fc.WorkspaceDir)
		}
	}

	// Settings 探针
	homeDir, _ := os.UserHomeDir()
	settingsProbe := probeSettingsStore(homeDir)
	storageProbes = append(storageProbes, settingsProbe)

	// Approval store 探针
	approvalProbe := probeApprovalStore(fc.WorkspaceDir, fc.MadyHome)
	storageProbes = append(storageProbes, approvalProbe)

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
		writingExt:        writingExt,
		fileIndexExt:      fileIndexExt,
		toolApprover:      permission.NewTUIChannelApprover(),
		agentStore:        agentStore,
		checkpointSaver:   agentcore.NewMemoryCheckpointSaver(),
		currentThreadID:   "default",
		sessionDir:        sessionProbe.ResolvedDir,
	}

	// 初始化 SettingsStore
	if settingsProbe.Unavailable {
		log.Printf("settings: %s (using transient store)", settingsProbe.Message)
		s.store, _ = NewSettingsStore("")
	} else {
		store, storeErr := NewSettingsStore(settingsProbe.Path)
		if storeErr == nil {
			s.store = store
		} else {
			s.store, _ = NewSettingsStore("")
		}
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

	// 设置状态栏：包含 provider/model/mode 和存储降级状态。
	modeLabel := statusBarModeLabel(s.isPlanMode(), useMultiDomain, s.thinkingConfig())
	degTag := storageDegradationTag(storageProbes)
	if degTag != "" {
		modeLabel += " · " + degTag
	}
	app.UpdateStatusBar(s.providerName, s.normalModel, modeLabel)

	// 输出存储降级提示到聊天区（仅在有降级时）。
	for _, p := range storageProbes {
		if p.Unavailable {
			app.PrintSystem("⚠ " + p.UserMessage)
		}
	}
	// 结构化欢迎信息：品牌标识 + 命令速查 + 当前上下文。
	projectLabel := "无"
	if s.currentProject != nil {
		projectLabel = s.currentProject.Alias
	}
	app.PrintWelcome(s.providerName, s.normalModel, modeLabel, projectLabel)

	// 先启动 TUI 渲染，再在后台初始化 Agent 和延迟任务。
	if err := app.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		return
	}
	// TUI 已进入 alternate screen 模式；此后所有 stderr 输出都会泄漏
	// 到 TUI 显示区。重定向 log/slog/os.Stderr 到日志文件以阻止泄漏。
	// 启动失败的错误（上方）仍可正常输出到真实终端。
	stderrCleanup := redirectStderrToFile(fc.MadyHome)
	defer stderrCleanup()
	s.initializeAgentAsync()
	if fc.Deferred != nil {
		fc.Deferred.StartAll(ctx)
	}

	<-app.Done()
	if agent := s.shutdownAgent(); agent != nil {
		agent.Close()
	}

	// 后台延迟任务如果有错误，汇总输出到日志（TUI 已关闭，用户可查看）。
	if fc.Deferred != nil && fc.Deferred.HasErrors() {
		log.Printf("[mady] deferred init errors:\n%s", fc.Deferred.ErrorSummary())
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

// redirectStderrToFile 将 log/slog/os.Stderr 输出重定向到日志文件，
// 防止 TUI alternate screen 模式下日志/警告泄漏到终端显示区。
//
// 覆盖三个输出路径：
//   - log.Printf / log.Println → log.SetOutput
//   - slog.Warn / slog.Error   → slog.SetDefault (只保留 >=Warn 级别)
//   - fmt.Fprintf(os.Stderr,…) → os.Stderr 变量替换
//
// 返回的 cleanup 函数应在 TUI exit 后调用以恢复原始 stderr。
// madyHome 为空时返回 no-op 函数。
func redirectStderrToFile(madyHome string) func() {
	if madyHome == "" {
		return func() {}
	}

	logsDir := filepath.Join(madyHome, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		log.Printf("logs: cannot create %s: %v (stderr not redirected)", logsDir, err)
		return func() {}
	}

	logPath := filepath.Join(logsDir, "mady.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("logs: cannot open %s: %v (stderr not redirected)", logPath, err)
		return func() {}
	}

	origStderr := os.Stderr
	os.Stderr = logFile
	log.SetOutput(logFile)
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelWarn, // TUI 运行时只保留 >=Warn 级别
	})))

	// 记录日志重定向信息到文件
	now := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(logFile, "\n--- mady tui started at %s ---\n", now)

	return func() {
		// TUI 已退出 alternate screen，恢复原始 stderr 输出
		os.Stderr = origStderr
		log.SetOutput(origStderr)
		slog.SetDefault(slog.New(slog.NewTextHandler(origStderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})))
		logFile.Close()
	}
}
