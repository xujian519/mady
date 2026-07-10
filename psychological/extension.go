package psychological

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// Config 心理引擎配置
type Config struct {
	SDTConfig              *SDTTrackerConfig    // SDT 追踪器配置
	StoreDir                string               // 持久化目录
	SessionID               string               // 会话标识符，用于隔离不同会话的 SDT 状态（默认 "default"）
	EnableLLM               bool                 // 是否启用 LLM 二次验证
	LLMVerifier             DistortionLLMVerifier // LLM 验证器
	SkipDistortionDetection bool                 // 跳过认知扭曲检测
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{SessionID: "default"}
}

// Extension 实现 agentcore.Extension 接口
// 同时实现 ToolProvider, SystemPromptProvider, TransformContextProvider
type Extension struct {
	config    Config
	tracker   *SDTTracker
	store     *Store
	lastInput string
	mu        sync.Mutex
}

// NewExtension 创建心理引擎扩展
func NewExtension(cfg Config) *Extension {
	if cfg.SessionID == "" {
		cfg.SessionID = "default"
	}
	return &Extension{
		config:  cfg,
		tracker: NewSDTTracker(cfg.SDTConfig),
	}
}

// Name 返回扩展名称
func (e *Extension) Name() string { return "psychological" }

// Init 扩展初始化，加载持久化的 SDT 状态
func (e *Extension) Init(ctx context.Context, agent *agentcore.Agent) error {
	if e.config.StoreDir != "" {
		store, err := NewStore(e.config.StoreDir)
		if err != nil {
			return fmt.Errorf("psychological: create store: %w", err)
		}
		e.store = store
		// 尝试恢复 SDT 状态
		if data, err := store.LoadSDTState(e.config.SessionID); err == nil && data != nil {
			e.tracker.RestoreState(data.SDTState, data.RoundCount)
		}
	}
	return nil
}

// Dispose 扩展清理
func (e *Extension) Dispose() error { return nil }

// SystemPromptSuffix 实现 SystemPromptProvider — 添加心理感知基础指令
func (e *Extension) SystemPromptSuffix() string {
	return `【心理感知能力】
你具备感知用户情绪状态的能力。系统会自动分析用户消息的心理信号。
当收到【当前感知的用户心理状态】信息块时：
- 根据主导情绪调整语气（负面→温和共情，正面→积极共鸣）
- 遵循【对话策略】的指引调整沟通方式
- 不要直接提及"你的VAD值是..."等原始数据，而是自然内化这些信息
- 察觉到严重认知扭曲时，以温和方式引导而非直接指出
- 心理分析结果仅在每轮对话开始时提供一次，请据此调整本轮所有回复`
}

// TransformContext 实现 TransformContextProvider — 分析用户消息并注入心理上下文
func (e *Extension) TransformContext(ctx context.Context, msgs []agentcore.Message) []agentcore.Message {
	// 找到最新的用户消息（同时记录其索引用于精确定位插入）
	lastUserIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == agentcore.RoleUser {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		return msgs
	}
	lastUserMsg := msgs[lastUserIdx].Content

	// 避免重复分析同一条消息
	e.mu.Lock()
	if lastUserMsg == e.lastInput {
		e.mu.Unlock()
		return msgs
	}
	e.lastInput = lastUserMsg
	e.mu.Unlock()

	// 执行管道
	result := ExecuteFullPipeline(lastUserMsg, &PipelineConfig{
		SDTTracker:              e.tracker,
		LLMVerifier:             e.config.LLMVerifier,
		SkipDistortionDetection: e.config.SkipDistortionDetection,
	})

	// 持久化
	if e.store != nil {
		_ = e.store.SaveSDTState(e.config.SessionID, e.tracker.GetState(), e.tracker.RoundCount())
	}

	// 构建上下文块，在最新用户消息之前精确插入一次
	contextBlock := BuildContextBlock(result)
	sysMsg := agentcore.Message{
		Role:    agentcore.RoleSystem,
		Content: contextBlock,
	}

	var out []agentcore.Message
	for i, msg := range msgs {
		if i == lastUserIdx {
			out = append(out, sysMsg)
		}
		out = append(out, msg)
	}
	return out
}

// Tools 实现 ToolProvider — 注册心理分析工具
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		{
			Name:        "analyze_emotion",
			Description: "只读分析用户输入的心理状态，返回完整的情绪分析结果（VAD、OCC 情绪、认知扭曲、SDT 需求、推荐策略）。注意：此工具仅分析传入的文本，不会更新对话的心理追踪状态",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "要分析的文本",
					},
				},
				"required": []string{"text"},
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				var params struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return nil, err
				}
				// 使用独立的临时追踪器，避免影响对话的 SDT 状态
				result := ExecuteFullPipeline(params.Text, &PipelineConfig{
					SDTTracker:              NewSDTTracker(e.config.SDTConfig),
					LLMVerifier:             e.config.LLMVerifier,
					SkipDistortionDetection: e.config.SkipDistortionDetection,
				})
				return result, nil
			},
		},
		{
			Name:        "emotion_status",
			Description: "查看当前对话的 SDT 心理需求状态和情绪轨迹（自主性、胜任感、归属感、动机水平）",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				state := e.tracker.GetState()
				return map[string]any{
					"sdt_state":   state,
					"round_count": e.tracker.RoundCount(),
					"lowest_need": e.tracker.LowestNeed(),
				}, nil
			},
		},
	}
}
