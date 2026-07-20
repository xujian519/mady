package psychological

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// Extension 实现 agentcore.Extension 接口
// 同时实现 SystemPromptProvider, TransformContextProvider, ToolProvider
type Extension struct {
	config    Config
	lastInput string
	mu        sync.Mutex
}

// NewExtension 创建心理引擎扩展
func NewExtension(cfg Config) *Extension {
	return &Extension{config: cfg}
}

// Name 返回扩展名称
func (e *Extension) Name() string { return "psychological" }

// Init 扩展初始化
func (e *Extension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }

// Dispose 扩展清理
func (e *Extension) Dispose() error { return nil }

// SystemPromptSuffix 实现 SystemPromptProvider — 添加心理感知基础指令
func (e *Extension) SystemPromptSuffix() string {
	return `【心理感知能力】
你具备感知用户情绪状态的能力。系统会自动分析用户消息的心理信号。
当收到【当前感知的用户心理状态】信息块时：
- 根据主导情绪调整语气（负面→温和共情，正面→积极共鸣）
- 遵循【对话策略】的指引调整沟通方式
- 不要直接提及原始情绪数据，而是自然内化这些信息
- 心理分析结果仅在每轮对话开始时提供一次，请据此调整本轮所有回复`
}

// TransformContext 实现 TransformContextProvider — 分析用户消息并注入心理上下文
func (e *Extension) TransformContext(ctx context.Context, msgs []agentcore.Message) []agentcore.Message {
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

	e.mu.Lock()
	if lastUserMsg == e.lastInput {
		e.mu.Unlock()
		return msgs
	}
	e.lastInput = lastUserMsg
	e.mu.Unlock()

	result := ExecuteFullPipeline(lastUserMsg, &PipelineConfig{
		SkipDistortionDetection: e.config.SkipDistortionDetection,
	})

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
			Description: "分析用户输入的心理状态，返回情绪分析结果（VAD、主导情绪、推荐策略）",
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
			Func: func(_ context.Context, args json.RawMessage) (any, error) {
				var params struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return nil, err
				}
				return ExecuteFullPipeline(params.Text, nil), nil
			},
		},
	}
}
