package domains

import (
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/doomloop"
)

// defaultDoomLoopHook 返回领域统一的死循环检测 hook。
// 各领域 Agent 共享同一组默认阈值；如需领域定制阈值，
// 在共享 hook 之外另接 doomloop 覆盖（见 assistant.go 示例）。
func defaultDoomLoopHook() agentcore.LifecycleHook {
	return doomloop.New(
		doomloop.WithToolCallLoop(5),
		doomloop.WithTextRepetition(4),
		doomloop.WithCycleLength(2),
		doomloop.WithEmptyResultMax(5),
		doomloop.WithCircuitBreaker(100),
		doomloop.WithCompactionMax(5),
	).AsHook()
}

// newDefaultGateway 构造领域统一的 Gateway（PilotDeck 风格决策入口），
// 替换各领域原先各自注册的 ReasoningStrategyRouter。一次分类同时驱动：
//   - 推理 effort/budget（原 ReasoningRouter 职责）
//   - 策略 hint 注入到系统消息（原 ReasoningStrategyRouter 职责）
//   - 模型回退链选择（FallbackRouter，候选链默认空，安全 no-op）
//   - token 预算评估与 blocking 钳制（仅 ContextWindow>0 时启用）
//
// cfg 用于读取 ContextWindow 和 Tools（纳入预算估算）。
// 返回的 Gateway 已配置完毕；调用方负责 appendLifecycle 注册，
// 并将 gateway.Fallback 赋给 cfg.FallbackRouter（供 callModelWithFallback）。
//
// 接入契约：注册 Gateway 后不得再单独注册 ReasoningRouter /
// ReasoningStrategyRouter / FallbackRouter，否则会重复分类与重复健康计数。
func newDefaultGateway(cfg agentcore.Config) *agentcore.Gateway {
	gateway := agentcore.NewGateway(agentcore.NewDefaultClassifier())
	gateway.Reasoning = agentcore.NewReasoningRouter(nil) // effort/budget map
	gateway.StrategySelector = agentcore.NewDefaultStrategySelector()
	gateway.Fallback = agentcore.NewFallbackRouter(agentcore.FallbackConfig{}, nil, nil)
	if cfg.ContextWindow > 0 {
		gateway.BudgetManager = agentcore.NewTokenBudgetManager(agentcore.DefaultBudgetConfig())
		gateway.ContextWindow = cfg.ContextWindow
		// 静态工具定义纳入预算估算（Tool → ToolDefinition）。
		defs := make([]agentcore.ToolDefinition, 0, len(cfg.Tools))
		for _, t := range cfg.Tools {
			if t != nil {
				defs = append(defs, t.Definition())
			}
		}
		gateway.ToolDefinitions = defs
	}
	return gateway
}
