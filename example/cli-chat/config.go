package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"strings"
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
		// wiki 导入失败→记警告并跳过，聊天主流程不依赖 wiki。
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
