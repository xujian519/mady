package psychological

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// psychologicalHook 实现 AgentRunObserver
// 在 Agent 启动时分析用户输入并注入心理上下文
type psychologicalHook struct {
	config Config
}

// Compile-time interface assertion.
var _ agentcore.AgentRunObserver = (*psychologicalHook)(nil)

// NewLifecycleHook 创建心理引擎的 LifecycleHook
// 轻量模式：仅 BeforeAgentRun 中分析用户输入并前置系统消息
func NewLifecycleHook(cfg Config) agentcore.LifecycleHook { //nolint:staticcheck
	return agentcore.ObserversToHook(&psychologicalHook{config: cfg})
}

// BeforeAgentRun 在 Agent 启动时运行心理分析
func (h *psychologicalHook) BeforeAgentRun(ctx context.Context, arc *agentcore.AgentRunContext) error {
	if arc == nil || arc.Input == "" {
		return nil
	}

	result := ExecuteFullPipeline(arc.Input, &PipelineConfig{
		SkipDistortionDetection: h.config.SkipDistortionDetection,
	})

	contextBlock := BuildContextBlock(result)

	sysMsg := agentcore.Message{
		Role:    agentcore.RoleSystem,
		Content: contextBlock,
	}
	arc.Messages = append([]agentcore.Message{sysMsg}, arc.Messages...)
	return nil
}

func (h *psychologicalHook) AfterAgentRun(_ context.Context, _ *agentcore.AgentRunContext, _ string, _ error) {
}
