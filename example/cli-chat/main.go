// Command cli-chat runs an interactive chat session with the Mady agent.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/skill"
	"github.com/xujian519/mady/tui"
	"github.com/xujian519/mady/tui/agentadapter"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
	core "github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

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
				// 参数非法→界面提示后返回，命令不生效。
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
				// 新建失败→界面提示后返回，保持当前会话。
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
				// 保存失败→界面提示后返回，不产生半完成的分支。
				appPtr.PrintError(fmt.Errorf("save current thread: %w", err))
				return
			}
			thread, err := ts.BranchThread(context.Background(), currentThreadID, "")
			if err != nil {
				// 分支失败→界面提示后返回，保持当前线程。
				appPtr.PrintError(err)
				return
			}
			if err := switchConversation(context.Background(), thread.Info.ID, thread); err != nil {
				// 切换失败→界面提示后返回，新分支已建但未激活。
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
				// 初始化失败→界面提示后返回，等待用户重试输入。
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
