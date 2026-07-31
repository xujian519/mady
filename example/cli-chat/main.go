// Command cli-chat runs an interactive chat session with the Mady agent.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/loader"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/provider/chatcompat"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/skill"
	agentstore "github.com/xujian519/mady/store"
	"github.com/xujian519/mady/tui"
	"github.com/xujian519/mady/tui/agentadapter"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
	core "github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

type threadStore interface {
	CreateThread(ctx context.Context) (*session.ThreadSnapshot, error)
	BranchThread(ctx context.Context, key, entryID string) (*session.ThreadSnapshot, error)
	GetThread(ctx context.Context, key string) (*session.ThreadSnapshot, error)
}

// loadWikiStore initializes the knowledge store from a wiki directory.
// It returns the store and a retrieval hook, or nil if WIKI_PATH is not set.
func loadWikiStore(logger *slog.Logger) (*knowledge.Store, agentcore.LifecycleHook) { //nolint:staticcheck
	wikiPath := os.Getenv("WIKI_PATH")
	if wikiPath == "" {
		return nil, nil
	}
	store := knowledge.NewStore()
	wikiLoader := loader.NewWikiLoader(store, wikiPath)
	stats, err := wikiLoader.ImportWiki()
	if err != nil {
		logger.Warn("wiki: import failed", "err", err)
		return nil, nil
	}
	logger.Info("wiki: imported", "docs", stats.Imported, "chunks",
		store.Stats().TotalChunks, "elapsed", "ok")
	hook := store.RetrievalHook("patent", retrieval.RetrievalConfig{
		TopK:     5,
		MaxChars: 4000,
		Prefix:   "以下是知识库中检索到的相关专利法律信息，请参考使用：\n",
	})
	return store, hook
}

// envConfig holds all environment-derived configuration for the chat app.
type envConfig struct {
	logger           *slog.Logger
	llm              agentcore.Provider
	model            string
	thinking         *agentcore.ThinkingConfig
	availableSkills  []skill.Skill
	skillDiagnostics []skill.Diagnostic
	selectedSkills   []string
	mode             agentcore.ExecutionMode
	store            agentcore.Store
	wikiStore        *knowledge.Store
	wikiHook         agentcore.LifecycleHook
	providerName     string
}

// setupEnvironment 初始化日志、主题、LLM Provider 和技能加载。
func setupEnvironment() *envConfig {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn, // stderr — don't interfere with TUI rendering
	}))

	if err := theme.InitThemeFromEnv(); err != nil {
		log.Fatalf("theme: %v", err)
	}

	llm := buildProvider()
	model := util.EnvOrDefault("AGENT_MODEL", defaultModel())
	thinking := thinkingFromEnv()
	availableSkills, skillDiagnostics, err := loadSkillsFromEnv()
	if err != nil {
		log.Fatalf("load skills: %v", err)
	}
	selectedSkills := parseListEnv("AGENT_SKILLS")

	mode := agentcore.ModeSerial
	if os.Getenv("EXECUTION_MODE") == "parallel" {
		mode = agentcore.ModeParallel
	}

	var store agentcore.Store
	if dir := os.Getenv("STORE_DIR"); dir != "" {
		var err error
		store, err = agentstore.NewSnapshotStore(dir)
		if err != nil {
			log.Fatalf("create store: %v", err)
		}
	}

	wikiStore, wikiHook := loadWikiStore(logger)
	providerName := util.EnvOrDefault("PROVIDER", "deepseek")

	return &envConfig{
		logger:           logger,
		llm:              llm,
		model:            model,
		thinking:         thinking,
		availableSkills:  availableSkills,
		skillDiagnostics: skillDiagnostics,
		selectedSkills:   selectedSkills,
		mode:             mode,
		store:            store,
		wikiStore:        wikiStore,
		wikiHook:         wikiHook,
		providerName:     providerName,
	}
}

// agentConfigs holds the three agent configurations used in this example.
type agentConfigs struct {
	coordinatorCfg    agentcore.Config
	weatherSpecialist agentcore.Config
	mathSpecialist    agentcore.Config
}

// buildAgentConfigs 构建天气专家、数学专家和协调 Agent 的配置。
func buildAgentConfigs(env *envConfig) *agentConfigs {
	weatherSpecialist := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:      "weather_specialist",
			Model:     env.model,
			Provider:  env.llm,
			Thinking:  env.thinking,
			Streaming: true,
		},
		SystemPrompt: "你是一个天气查询专家。使用 get_weather 工具简洁地回答天气相关问题。",
		Tools:        []*agentcore.Tool{weatherTool()},
	}

	mathSpecialist := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:      "math_specialist",
			Model:     env.model,
			Provider:  env.llm,
			Thinking:  env.thinking,
			Streaming: true,
		},
		SystemPrompt: "你是一个数学计算专家。使用 calculator 工具求解数学问题，回答需简洁。",
		Tools:        []*agentcore.Tool{calculatorTool()},
	}

	coordinatorCfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:      "coordinator",
			Model:     env.model,
			Provider:  env.llm,
			Thinking:  env.thinking,
			Streaming: true,
		},
		SystemPrompt: strings.Join([]string{
			"你是一个协调 Agent。",
			"天气相关问题，委派给 weather_specialist。",
			"数学计算问题，委派给 math_specialist。",
			"一般对话直接回答。",
			"你可以在同一轮调用多个专家。",
		}, " "),
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          10,
			ExecutionMode:     env.mode,
			Concurrency:       5,
			ValidateArguments: true,
			SteeringMode:      agentcore.SteeringAll,
			FollowUpMode:      agentcore.SteeringAll,
			GlobalBefore:      []agentcore.BeforeHook{agentcore.LoggingBeforeHook(env.logger)},
			GlobalAfter:       []agentcore.AfterHook{agentcore.LoggingAfterHook(env.logger)},
			Middleware:        []agentcore.Middleware{agentcore.TimeoutMiddleware(30 * time.Second)},
		},
		CompactionConfig: agentcore.CompactionConfig{
			ContextWindow:    128000,
			ReserveTokens:    32000,
			KeepRecentTokens: 4000,
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills: env.availableSkills,
			SelectedSkills:  env.selectedSkills,
		},
		Store:     env.store,
		Lifecycle: env.wikiHook,
		RetryConfig: &agentcore.RetryConfig{
			MaxRetries:  3,
			BaseDelayMs: 1000,
			MaxDelayMs:  15000,
		},
		Handoffs: []agentcore.HandoffConfig{
			{
				Name:        "weather_specialist",
				Description: "Handles weather-related questions for any city or location",
				Mode:        agentcore.HandoffDelegate,
				AgentConfig: weatherSpecialist,
			},
			{
				Name:        "math_specialist",
				Description: "Handles math calculations and arithmetic",
				Mode:        agentcore.HandoffDelegate,
				AgentConfig: mathSpecialist,
			},
		},
	}

	return &agentConfigs{
		coordinatorCfg:    coordinatorCfg,
		weatherSpecialist: weatherSpecialist,
		mathSpecialist:    mathSpecialist,
	}
}

// 拆分到 3 个函数后仍因多 Agent 协调和交互循环而保持高复杂度。作为教学示例，
// 集中展示比分散到多个文件更易读。
//
//nolint:gocognit // 原因：示例程序的 main 包含完整的 Agent 编排流程（初始化/配置/循环），
func main() {
	env := setupEnvironment()
	ac := buildAgentConfigs(env)

	var busy atomic.Bool
	var currentAgent atomic.Pointer[agentcore.Agent]
	var currentThreadID string

	// appPtr is used inside OnSubmit before the app is created (Go two-phase
	// init pattern — closures capture the pointer, not the value).
	var appPtr *chat.ChatApp

	createThread := func(ctx context.Context) (string, *session.ThreadSnapshot, error) {
		if env.store == nil {
			return "", nil, nil
		}
		if ts, ok := env.store.(threadStore); ok {
			thread, err := ts.CreateThread(ctx)
			if err != nil {
				return "", nil, err
			}
			return thread.Info.ID, thread, nil
		}
		return fmt.Sprintf("thread-%d", time.Now().UnixMilli()), nil, nil
	}

	loadAgentForThread := func(ctx context.Context, threadID string) (*agentcore.Agent, error) {
		agent := agentcore.New(ac.coordinatorCfg)
		if threadID == "" || env.store == nil {
			return agent, nil
		}
		hasState, err := storeHasKey(ctx, env.store, threadID)
		if err != nil {
			return nil, err
		}
		if !hasState {
			return agent, nil
		}
		if err := agent.LoadState(ctx, threadID); err != nil {
			return nil, err
		}
		return agent, nil
	}

	renderThread := func(thread *session.ThreadSnapshot) {
		if thread == nil || appPtr == nil {
			return
		}
		appPtr.History().Clear()
		for _, item := range thread.Transcript {
			appPtr.History().Append(chatMessageFromAgentMessage(item.Message))
		}
	}

	switchConversation := func(ctx context.Context, threadID string, snapshot *session.ThreadSnapshot) error {
		agent, err := loadAgentForThread(ctx, threadID)
		if err != nil {
			return err
		}
		if prev := currentAgent.Load(); prev != nil {
			prev.Close()
		}
		currentAgent.Store(agent)
		currentThreadID = threadID
		if appPtr != nil {
			agentadapter.BindAgent(appPtr, agent)
			if snapshot != nil {
				renderThread(snapshot)
			} else if ts, ok := env.store.(threadStore); ok && threadID != "" {
				thread, err := ts.GetThread(ctx, threadID)
				if err != nil {
					return err
				}
				renderThread(thread)
			} else {
				appPtr.History().Clear()
			}
		}
		return nil
	}

	startNewConversation := func(ctx context.Context) error {
		threadID, snapshot, err := createThread(ctx)
		if err != nil {
			return err
		}
		return switchConversation(ctx, threadID, snapshot)
	}

	saveCurrentConversation := func(ctx context.Context) error {
		ag := currentAgent.Load()
		if ag == nil || env.store == nil || currentThreadID == "" {
			return nil
		}
		return ag.SaveState(ctx, currentThreadID)
	}

	applyThinking := func(cfg *agentcore.Config, thinking *agentcore.ThinkingConfig) {
		cfg.Thinking = cloneThinkingConfig(thinking)
		for i := range cfg.Handoffs {
			cfg.Handoffs[i].AgentConfig.Thinking = cloneThinkingConfig(thinking)
		}
	}
	applyThinking(&ac.weatherSpecialist, env.thinking)
	applyThinking(&ac.mathSpecialist, env.thinking)
	applyThinking(&ac.coordinatorCfg, env.thinking)

	slashSuggestions := make([]core.Suggestion, 0, 12+len(env.availableSkills))
	slashSuggestions = append(slashSuggestions,
		core.Suggestion{InsertText: "/help", Label: "/help", Description: "Show keybindings"},
		core.Suggestion{InsertText: "/clear", Label: "/clear", Description: "Start a fresh conversation"},
		core.Suggestion{InsertText: "/new", Label: "/new", Description: "Start a fresh conversation"},
		core.Suggestion{InsertText: "/branch", Label: "/branch", Description: "Branch the current thread"},
		core.Suggestion{InsertText: "/thinking", Label: "/thinking", Description: "Show or change thinking mode"},
		core.Suggestion{InsertText: "/thinking summarized", Label: "/thinking summarized", Description: "Show summarized reasoning blocks"},
		core.Suggestion{InsertText: "/thinking omitted", Label: "/thinking omitted", Description: "Hide visible reasoning blocks"},
		core.Suggestion{InsertText: "/thinking effort medium", Label: "/thinking effort medium", Description: "Set reasoning effort"},
		core.Suggestion{InsertText: "/thinking budget -1", Label: "/thinking budget -1", Description: "Use dynamic thinking budget"},
		core.Suggestion{InsertText: "/skill:", Label: "/skill:", Description: "Explicitly invoke a loaded skill"},
		core.Suggestion{InsertText: "/save", Label: "/save", Description: "Persist the current thread"},
		core.Suggestion{InsertText: "/quit", Label: "/quit", Description: "Exit application"},
	)
	for _, item := range env.availableSkills {
		slashSuggestions = append(slashSuggestions, core.Suggestion{
			InsertText:  "/skill:" + item.Name + " ",
			Label:       "/skill:" + item.Name,
			Description: item.Description,
		})
	}

	reloadCurrentAgent := func(_ context.Context) {
		var snap *agentcore.StateSnapshot
		if ag := currentAgent.Load(); ag != nil {
			s := ag.State().Snapshot()
			snap = &s
		}

		agent := agentcore.New(ac.coordinatorCfg)
		if snap != nil {
			agent.State().Restore(*snap)
		}
		if prev := currentAgent.Load(); prev != nil {
			prev.Close()
		}
		currentAgent.Store(agent)
		if appPtr != nil {
			agentadapter.BindAgent(appPtr, agent)
			if snap != nil {
				appPtr.History().Clear()
				for _, msg := range snap.Messages {
					appPtr.History().Append(chatMessageFromAgentMessage(msg))
				}
			}
		}
	}

	printThinkingStatus := func() {
		appPtr.PrintSystem("thinking: " + formatThinkingConfig(ac.coordinatorCfg.Thinking))
	}

	handleSubmit := func(_ context.Context, input string) {
		trimmed := strings.TrimSpace(input)
		if trimmed == "" {
			return
		}
		if cmd, ok := skill.ParseCommand(trimmed); ok {
			if _, found := skill.FindByName(ac.coordinatorCfg.AvailableSkills, cmd.Name); !found {
				appPtr.PrintError(fmt.Errorf("unknown skill %q", cmd.Name))
				return
			}
		}
		if strings.HasPrefix(trimmed, "/thinking") {
			if busy.Load() {
				appPtr.PrintSystem("(busy — please wait for the current reply)")
				return
			}
			next, changed, err := parseThinkingCommand(trimmed, ac.coordinatorCfg.Thinking)
			if err != nil {
				appPtr.PrintError(err)
				return
			}
			if !changed {
				printThinkingStatus()
				return
			}
			applyThinking(&ac.weatherSpecialist, next)
			applyThinking(&ac.mathSpecialist, next)
			applyThinking(&ac.coordinatorCfg, next)
			reloadCurrentAgent(context.Background())
			printThinkingStatus()
			return
		}
		switch trimmed {
		case "/help":
			appPtr.ToggleKeyHelp()
			return
		case "/clear", "/new":
			if busy.Load() {
				appPtr.PrintSystem("(busy — please wait for the current reply)")
				return
			}
			if err := startNewConversation(context.Background()); err != nil {
				appPtr.PrintError(err)
				return
			}
			if currentThreadID != "" {
				appPtr.PrintSystem(fmt.Sprintf("started new thread: %s", currentThreadID))
			} else {
				appPtr.PrintSystem("started new conversation")
			}
			return
		case "/branch":
			if busy.Load() {
				appPtr.PrintSystem("(busy — please wait for the current reply)")
				return
			}
			ts, ok := env.store.(threadStore)
			if !ok || currentThreadID == "" {
				appPtr.PrintSystem("branching requires a session-backed store and an active thread")
				return
			}
			if err := saveCurrentConversation(context.Background()); err != nil {
				appPtr.PrintError(fmt.Errorf("save current thread: %w", err))
				return
			}
			thread, err := ts.BranchThread(context.Background(), currentThreadID, "")
			if err != nil {
				appPtr.PrintError(err)
				return
			}
			if err := switchConversation(context.Background(), thread.Info.ID, thread); err != nil {
				appPtr.PrintError(err)
				return
			}
			appPtr.PrintSystem(fmt.Sprintf("branched thread %s -> %s", thread.Info.ParentSession, thread.Info.ID))
			return
		case "/save":
			if env.store == nil {
				appPtr.PrintSystem("nothing to save (no store configured or no session yet)")
				return
			}
			if err := saveCurrentConversation(context.Background()); err != nil {
				appPtr.PrintError(fmt.Errorf("save state: %w", err))
			} else {
				if currentThreadID != "" {
					appPtr.PrintSystem(fmt.Sprintf("thread saved: %s", currentThreadID))
				} else {
					appPtr.PrintSystem("conversation saved")
				}
			}
			return
		case "/quit", "exit":
			_ = appPtr.Stop()
			return
		}

		if busy.Load() {
			appPtr.PrintSystem("(busy — please wait for the current reply)")
			return
		}

		agent := currentAgent.Load()
		if agent == nil {
			if err := startNewConversation(context.Background()); err != nil {
				appPtr.PrintError(err)
				return
			}
			agent = currentAgent.Load()
		}
		if agent == nil {
			appPtr.PrintError(fmt.Errorf("failed to initialize conversation"))
			return
		}

		busy.Store(true)
		go func() { //nolint:gosec // G118: TUI goroutine, no request context (example code)
			defer busy.Store(false)
			_, runErr := agent.Run(context.Background(), trimmed)
			// Agent error is already printed via ChatEventAgentError event,
			// so we only handle non-agent errors (e.g. save failure) here.
			if saveErr := saveCurrentConversation(context.Background()); saveErr != nil {
				if runErr != nil {
					appPtr.PrintSystem(fmt.Sprintf("Conversation saved with errors: %v", saveErr))
				} else {
					appPtr.PrintError(fmt.Errorf("save thread: %w", saveErr))
				}
			}
		}()
	}

	app := tui.NewChatApp(chat.ChatAppConfig{
		Title: fmt.Sprintf(
			"mady · provider=%s model=%s mode=%s",
			env.providerName, env.model, env.mode,
		),
		ShowTimings: true,
		ShowTurns:   true,
		AltScreen:   true,
		MouseMode:   "auto",
		Providers: []core.AutocompleteProvider{
			&component.StaticProvider{
				TriggerStr:  "/",
				Suggestions: slashSuggestions,
			},
		},
		OnSubmit: handleSubmit,
	})
	appPtr = app

	theme.SetOnSemanticThemeChange(func() {
		app.History().SetTheme(chat.DefaultChatHistoryTheme())
	})
	themePath := strings.TrimSpace(os.Getenv("TUI_THEME"))
	if themePath == "" {
		themePath = strings.TrimSpace(os.Getenv("AGENT_TUI_THEME"))
	}
	var themeStop func()
	if themePath != "" && os.Getenv("TUI_THEME_WATCH") != "0" {
		themeStop = theme.StartSemanticThemeWatcher(themePath, 0, nil)
	}
	defer func() {
		if themeStop != nil {
			themeStop()
		}
	}()

	app.PrintSystem(strings.Join([]string{
		"Welcome! Type a question below.",
		"Slash commands: /help /clear /new /branch /thinking /skill:name /save /quit.",
		"Try \"What's the weather in Tokyo and what is 7 * 8?\"",
	}, "\n"))
	for _, diag := range env.skillDiagnostics {
		app.PrintSystem(fmt.Sprintf("skill warning: %s (%s)", diag.Message, diag.Path))
	}
	if len(env.availableSkills) > 0 {
		names := make([]string, 0, len(env.availableSkills))
		for _, item := range env.availableSkills {
			names = append(names, item.Name)
		}
		app.PrintSystem("available skills: " + strings.Join(names, ", "))
	}

	if env.wikiStore != nil {
		stats := env.wikiStore.Stats()
		app.PrintSystem(fmt.Sprintf("wiki knowledge: %d docs, %d chunks (RAG: patent/legal)", stats.TotalDocs, stats.TotalChunks))
	}

	if err := startNewConversation(context.Background()); err != nil {
		if themeStop != nil {
			themeStop()
		}
		log.Fatalf("start conversation: %v", err) //nolint:gocritic // exitAfterDefer: themeStop() manually called above
	}
	if currentThreadID != "" {
		app.PrintSystem(fmt.Sprintf("active thread: %s", currentThreadID))
	}
	printThinkingStatus()

	// Ctrl+/ opens the keybindings overlay. Esc closes it.
	app.Keybindings().Register("app.help", terminal.KeybindingDef{
		DefaultKeys: []terminal.KeyID{"ctrl+/"},
		Description: "Toggle keybindings help",
	})
	app.Host().AddChild(hotkeyRouter{app: app})

	// 中观 Mady 启动欢迎语 — Madhyamaka-inspired startup banner.
	app.PrintSystem(strings.Join([]string{
		``,
		`╭──────────────────────────────────────────────────╮`,
		`│                                                  │`,
		`│                Mady  中 观 智 能 体                │`,
		`│              Madhyamaka AI Agent                 │`,
		`│                                                  │`,
		`│       不生亦不灭    不常亦不断                     │`,
		`│       不一亦不异    不来亦不出                     │`,
		`│                                                  │`,
		`│         离 于 二 边 ， 行 于 中 道                  │`,
		`│                                                  │`,
		`│     provider = ` + env.providerName,
		`╰──────────────────────────────────────────────────╯`,
	}, "\n") + "\n")
	app.PrintSystem(fmt.Sprintf("boot time: %s\n", time.Now().Format("2006-01-02 15:04:05")))

	if err := app.Start(); err != nil {
		if themeStop != nil {
			themeStop()
		}
		log.Fatalf("start tui: %v", err)
	}
	<-app.Done()
}

// hotkeyRouter is a zero-size Component that captures global hotkeys.
type hotkeyRouter struct {
	app *chat.ChatApp
}

func (h hotkeyRouter) Render(int64) []string { return nil }
func (h hotkeyRouter) HandleInput(data string) {
	if terminal.MatchesKey(data, "ctrl+/") {
		h.app.ToggleKeyHelp()
	}
}
func (hotkeyRouter) Invalidate() {}

// --- provider setup ---

func buildProvider() agentcore.Provider {
	providerType := util.EnvOrDefault("PROVIDER", "deepseek")

	// Unified chatcompat provider — all supported backends use the OpenAI
	// Chat Completions compatible protocol (DeepSeek, Zhipu GLM, Kimi, etc.).
	apiKey := os.Getenv("API_KEY")
	baseURL := os.Getenv("BASE_URL")

	switch providerType {
	case "deepseek":
		if apiKey == "" {
			apiKey = os.Getenv("DEEPSEEK_API_KEY")
		}
		if baseURL == "" {
			baseURL = "https://api.deepseek.com/v1"
		}
	case "zhipu":
		if apiKey == "" {
			apiKey = os.Getenv("ZHIPU_API_KEY")
		}
		if baseURL == "" {
			baseURL = "https://open.bigmodel.cn/api/coding/paas/v4"
		}
	case "kimi":
		if apiKey == "" {
			apiKey = os.Getenv("KIMI_API_KEY")
		}
		if baseURL == "" {
			baseURL = "https://api.moonshot.cn/v1"
		}
	default:
		// Generic OpenAI-compatible provider
		if baseURL == "" {
			baseURL = os.Getenv("OPENAI_BASE_URL")
		}
	}

	if apiKey == "" {
		log.Fatal("API_KEY (or provider-specific env var) is required")
	}
	return chatcompat.New(chatcompat.Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
}

func loadSkillsFromEnv() ([]skill.Skill, []skill.Diagnostic, error) {
	paths := parsePathListEnv("SKILL_DIRS")
	if len(paths) == 0 {
		return nil, nil, nil
	}
	return skill.Load(paths...)
}

func parsePathListEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, string(filepath.ListSeparator))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func parseListEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func defaultModel() string {
	switch util.EnvOrDefault("PROVIDER", "deepseek") {
	case "deepseek":
		return "deepseek-chat"
	case "zhipu":
		return "glm-5.2"
	case "kimi":
		return "kimi-k2-0905-preview"
	default:
		return os.Getenv("MODEL")
	}
}

// --- tool definitions ---

func weatherTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "get_weather",
		Description: "Get the current weather for a given city",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{
					"type":        "string",
					"description": "City name, e.g. Tokyo",
				},
			},
			"required":             []string{"location"},
			"additionalProperties": false,
		},
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Location string `json:"location"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			return map[string]any{
				"location":    p.Location,
				"temperature": "22°C",
				"condition":   "sunny",
				"humidity":    "45%",
			}, nil
		},
	}
}

func calculatorTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "calculator",
		Description: "Evaluate a simple math expression",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{
					"type":        "string",
					"description": "Math expression, e.g. 7*8",
				},
			},
			"required":             []string{"expression"},
			"additionalProperties": false,
		},
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Expression string `json:"expression"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			return map[string]string{
				"expression": p.Expression,
				"result":     "56",
				"note":       "stub calculator",
			}, nil
		},
	}
}

func storeHasKey(ctx context.Context, store agentcore.Store, key string) (bool, error) {
	return store.Has(ctx, key)
}

func chatMessageFromAgentMessage(msg agentcore.Message) chat.ChatMessage {
	role := chat.RoleSystem
	switch msg.Role {
	case agentcore.RoleUser:
		role = chat.RoleUser
	case agentcore.RoleAssistant:
		role = chat.RoleAssistant
	case agentcore.RoleTool:
		role = chat.RoleTool
	case agentcore.RoleSystem:
		role = chat.RoleSystem
	}
	text := msg.Content
	if text == "" && len(msg.ToolCalls) > 0 {
		text = fmt.Sprintf("tool calls: %d", len(msg.ToolCalls))
	}
	return chat.ChatMessage{
		Role: role,
		Text: text,
	}
}

func thinkingFromEnv() *agentcore.ThinkingConfig {
	includeRaw := strings.TrimSpace(os.Getenv("THINKING_INCLUDE_THOUGHTS"))
	displayRaw := strings.TrimSpace(os.Getenv("THINKING_DISPLAY"))
	effortRaw := strings.TrimSpace(os.Getenv("THINKING_EFFORT"))
	budgetRaw := strings.TrimSpace(os.Getenv("THINKING_BUDGET"))
	if includeRaw == "" && displayRaw == "" && effortRaw == "" && budgetRaw == "" {
		return nil
	}

	cfg := &agentcore.ThinkingConfig{}
	if includeRaw != "" {
		if v, err := strconv.ParseBool(includeRaw); err == nil {
			cfg.IncludeThoughts = v
		}
	}
	if displayRaw != "" {
		cfg.Display = agentcore.ThinkingDisplay(strings.ToLower(displayRaw))
	}
	if effortRaw != "" {
		cfg.Effort = agentcore.ThinkingEffort(strings.ToLower(effortRaw))
	}
	if budgetRaw != "" {
		if v, err := strconv.ParseInt(budgetRaw, 10, 64); err == nil {
			cfg.Budget = v
		}
	}
	return cfg
}

func cloneThinkingConfig(cfg *agentcore.ThinkingConfig) *agentcore.ThinkingConfig {
	if cfg == nil {
		return nil
	}
	cp := *cfg
	return &cp
}

func compactThinkingConfig(cfg *agentcore.ThinkingConfig) *agentcore.ThinkingConfig {
	if cfg == nil {
		return nil
	}
	if !cfg.IncludeThoughts &&
		cfg.Display == agentcore.ThinkingDisplayDefault &&
		cfg.Effort == agentcore.ThinkingEffortDefault &&
		cfg.Budget == 0 {
		return nil
	}
	return cfg
}

func formatThinkingConfig(cfg *agentcore.ThinkingConfig) string {
	if cfg == nil {
		return "default"
	}
	parts := []string{
		"display=" + string(cfg.NormalizedDisplay()),
	}
	if cfg.Effort != "" {
		parts = append(parts, "effort="+string(cfg.Effort))
	}
	if cfg.Budget != 0 {
		parts = append(parts, fmt.Sprintf("budget=%d", cfg.Budget))
	}
	parts = append(parts, fmt.Sprintf("include_thoughts=%t", cfg.IncludeThoughts))
	return strings.Join(parts, " ")
}

func parseThinkingCommand(input string, current *agentcore.ThinkingConfig) (*agentcore.ThinkingConfig, bool, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) <= 1 {
		return cloneThinkingConfig(current), false, nil
	}

	next := cloneThinkingConfig(current)
	if next == nil {
		next = &agentcore.ThinkingConfig{}
	}

	switch strings.ToLower(fields[1]) {
	case "reset":
		return nil, true, nil
	case "on", "summarized":
		next.IncludeThoughts = true
		next.Display = agentcore.ThinkingDisplaySummarized
		return compactThinkingConfig(next), true, nil
	case "off", "omitted":
		next.IncludeThoughts = false
		next.Display = agentcore.ThinkingDisplayOmitted
		return compactThinkingConfig(next), true, nil
	case "effort":
		if len(fields) < 3 {
			return nil, false, fmt.Errorf("usage: /thinking effort <low|medium|high|max|default>")
		}
		switch strings.ToLower(fields[2]) {
		case "default", "reset":
			next.Effort = agentcore.ThinkingEffortDefault
		case "low", "medium", "high", "max":
			next.Effort = agentcore.ThinkingEffort(strings.ToLower(fields[2]))
		default:
			return nil, false, fmt.Errorf("invalid thinking effort %q", fields[2])
		}
		return compactThinkingConfig(next), true, nil
	case "budget":
		if len(fields) < 3 {
			return nil, false, fmt.Errorf("usage: /thinking budget <n|default>")
		}
		if strings.EqualFold(fields[2], "default") || strings.EqualFold(fields[2], "reset") {
			next.Budget = 0
			return compactThinkingConfig(next), true, nil
		}
		v, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, false, fmt.Errorf("invalid thinking budget %q", fields[2])
		}
		next.Budget = v
		return compactThinkingConfig(next), true, nil
	case "include":
		if len(fields) < 3 {
			return nil, false, fmt.Errorf("usage: /thinking include <true|false>")
		}
		v, err := strconv.ParseBool(fields[2])
		if err != nil {
			return nil, false, fmt.Errorf("invalid thinking include value %q", fields[2])
		}
		next.IncludeThoughts = v
		if next.Display == agentcore.ThinkingDisplayDefault {
			if v {
				next.Display = agentcore.ThinkingDisplaySummarized
			} else {
				next.Display = agentcore.ThinkingDisplayOmitted
			}
		}
		return compactThinkingConfig(next), true, nil
	default:
		return nil, false, fmt.Errorf("usage: /thinking [on|off|summarized|omitted|effort <...>|budget <...>|include <true|false>|reset]")
	}
}
