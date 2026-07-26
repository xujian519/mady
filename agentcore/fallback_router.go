package agentcore

import (
	"context"
	"log/slog"
)

// ============================================================================
// FallbackRouter — 模型级联回退路由器
//
// 为每个复杂度等级定义一个候选模型链。当主模型连续失败（被健康检测器标记为
// 降级）时，自动选择链中下一个可用模型。同时支持 sticky 模式——在一个会话
// 内保持一致的模型选择，避免切换模型导致的行为漂移。
//
// 参考：PilotDeck Smart Router Fallback Chain + ProviderHealthTracker 设计
// ============================================================================

// FallbackConfig 定义模型回退链配置。
type FallbackConfig struct {
	// Candidates 按复杂度等级映射到候选模型列表（有序）。
	// 第一个元素为主模型，后续为回退模型。
	// 示例：
	//
	//	{
	//	  ComplexityLow:    ["gpt-4o-mini", "deepseek-v4-flash"],
	//	  ComplexityMedium: ["deepseek-v4-pro", "gpt-4o", "gemini-2.0-flash"],
	//	  ComplexityHigh:   ["claude-3.5-sonnet", "deepseek-v4-pro", "gpt-4o"],
	//	}
	Candidates map[Complexity][]string `json:"candidates,omitempty" yaml:"candidates,omitempty"`

	// StickySession 启用 sticky 模式后，同一会话内首次选择的模型将持续使用，
	// 避免模型切换导致的输出风格漂移。仅在模型降级时才切换。
	StickySession bool `json:"sticky_session,omitempty" yaml:"sticky_session,omitempty"`
}

// FallbackRouter 是一个 LifecycleHook，集成到 Agent 的 BeforeModelCall 中。
//
// 工作流程：
//  1. BeforeModelCall：根据复杂度选择最佳可用模型，设置到 mcc.Request.Model
//  2. AfterModelCall：根据响应记录健康状态
//  3. callModelWithFallback 中的 tryFallbackChain：呼叫成功后更新健康状态；
//     失败后通过 NextFallback() 获取回退链中的下一模型
//
// FallbackRouter 作为 ModelCallObserver 实现，通过 ObserversToHook 注册。
type FallbackRouter struct {
	HealthTracker *ProviderHealthTracker
	Config        FallbackConfig

	// Classifier 用于获取当前轮次的复杂度等级。
	// 如果为 nil，则使用 DefaultClassifier。
	Classifier ComplexityClassifier

	// stickyModel 缓存当前会话选择的模型 key = complexity 的字符串。
	// 启用 StickySession 时，首次选中的模型将持续使用。
	stickyModel map[string]string

	// fallbackIndex 追踪每个复杂度的回退进度（在 NextFallback 中使用）。
	fallbackIndex map[Complexity]int

	// lastComplexity 缓存最后一个 BeforeModelCall 中分类的复杂度，
	// 供 NextFallback 使用（那里无法获取 AgentRunContext）。
	lastComplexity Complexity

	// lastModel 缓存 BeforeModelCall 中选取的模型名。
	// AfterModelCall 中 mcc.Request 可能为 nil（成功路径），用此字段兜底。
	lastModel string
}

// NewFallbackRouter 创建一个 FallbackRouter。
// classifier 为 nil 时使用 DefaultClassifier。
// healthTracker 为 nil 时使用默认健康配置创建一个。
func NewFallbackRouter(cfg FallbackConfig, classifier ComplexityClassifier, healthTracker *ProviderHealthTracker) *FallbackRouter {
	if classifier == nil {
		classifier = NewDefaultClassifier()
	}
	if healthTracker == nil {
		healthTracker = NewProviderHealthTracker(nil)
	}
	return &FallbackRouter{
		HealthTracker: healthTracker,
		Config:        cfg,
		Classifier:    classifier,
		stickyModel:   make(map[string]string),
		fallbackIndex: make(map[Complexity]int),
	}
}

// --- ModelCallObserver 实现 ---

// BeforeModelCall 根据复杂度等级选择最佳可用模型。
// 选择策略：
//  1. 如果启用 StickySession 且已有选中模型，使用它
//  2. 从候选列表中选择第一个健康的模型
//  3. 如果全部降级，使用第一个候选（主模型）
func (r *FallbackRouter) BeforeModelCall(_ context.Context, arc *AgentRunContext, mcc *ModelCallContext) error {
	if mcc == nil || mcc.Request == nil || r.Config.Candidates == nil {
		return nil
	}

	// 获取当前复杂度并缓存
	input := latestUserInput(arc)
	c := r.Classifier.Classify(input, arc.Messages)
	r.lastComplexity = c

	// 获取候选模型列表
	candidates, ok := r.Config.Candidates[c]
	if !ok || len(candidates) == 0 {
		// 无此复杂度的回退配置，不干预——但记录默认模型
		r.lastModel = mcc.Request.Model
		return nil
	}

	// 选择模型
	selected := r.selectModel(c, candidates)
	r.lastModel = selected
	mcc.Request.Model = selected

	return nil
}

// AfterModelCall 根据调用结果更新健康状态——这是健康记录的**唯一**路径。
// 注意：成功路径中 mcc.Request 可能为 nil（由 runAfterModelCall 传入），
// 此时回退到 BeforeModelCall 中缓存的 lastModel。
// callModelWithFallback / tryFallbackChain 不再显式调用 RecordSuccess，
// 避免健康计数翻倍。
func (r *FallbackRouter) AfterModelCall(_ context.Context, _ *AgentRunContext, mcc *ModelCallContext) {
	if mcc == nil {
		return
	}
	// 获取模型名：优先从 Request，其次从缓存
	model := r.lastModel
	if mcc.Request != nil && mcc.Request.Model != "" {
		model = mcc.Request.Model
	}
	if model == "" {
		return
	}

	if mcc.Err != nil {
		nonRetryable := !IsRetryableError(mcc.Err)
		r.HealthTracker.RecordFailure(model, nonRetryable)
		slog.Debug("fallback_router: model failure recorded",
			"model", model,
			"non_retryable", nonRetryable,
			"err", mcc.Err,
		)
	} else if mcc.Response != nil {
		r.HealthTracker.RecordSuccess(model)
	}
}

// --- 公共方法 ---

// NextFallback 返回当前复杂度的下一个回退模型。
// 当主模型呼叫失败且重试耗尽时由 Agent 调用。
// 返回空字符串表示已无可用回退。
func (r *FallbackRouter) NextFallback(currentModel string) string {
	c := r.lastComplexity
	candidates, ok := r.Config.Candidates[c]
	if !ok || len(candidates) <= 1 {
		return ""
	}

	idx := r.fallbackIndex[c]
	if idx >= len(candidates)-1 {
		return "" // 已尝试所有候选
	}

	// 前进到下一个候选
	idx++
	r.fallbackIndex[c] = idx
	next := candidates[idx]

	slog.Info("fallback_router: switching to fallback model",
		"complexity", c,
		"from", currentModel,
		"to", next,
		"attempt", idx,
	)

	// 更新 sticky 缓存
	if r.Config.StickySession {
		r.stickyModel[c.String()] = next
	}

	return next
}

// SelectModel 选择给定复杂度下的最佳可用模型，并更新内部 sticky /
// lastComplexity / lastModel 状态。它是分类无关的选模型入口，供 Gateway
// 在已经分类过一次后调用，避免 BeforeModelCall 里的重复分类。
//
// 返回空字符串表示该复杂度未配置候选链——调用方应保留原模型不变。
func (r *FallbackRouter) SelectModel(c Complexity) string {
	if r.Config.Candidates == nil {
		return ""
	}
	candidates, ok := r.Config.Candidates[c]
	if !ok || len(candidates) == 0 {
		return ""
	}
	r.lastComplexity = c
	selected := r.selectModel(c, candidates)
	r.lastModel = selected
	return selected
}

// Reset resets the fallback indices and sticky state.
// Useful for session boundaries or testing.
func (r *FallbackRouter) Reset() {
	r.fallbackIndex = make(map[Complexity]int)
	r.stickyModel = make(map[string]string)
}

// RecordFallbackResult 记录回退链中的单次模型调用结果。
// err 为 nil 表示成功，非 nil 表示失败。
// 此方法封装了对 HealthTracker 的访问，避免调用方直接操作内部字段。
//
// tryFallbackChain 使用此方法代替直接访问 HealthTracker。
// 成功由 AfterModelCall 统一记录，此方法仅记录失败。
func (r *FallbackRouter) RecordFallbackResult(model string, err error) {
	if err == nil {
		return // 成功由 AfterModelCall 统一记录
	}
	nonRetryable := !IsRetryableError(err)
	r.HealthTracker.RecordFailure(model, nonRetryable)
}

// --- 内部方法 ---

// selectModel 从候选列表中选择最佳可用模型。
func (r *FallbackRouter) selectModel(c Complexity, candidates []string) string {
	// 1. StickySession: 复用已选模型
	if r.Config.StickySession {
		if selected, ok := r.stickyModel[c.String()]; ok {
			// 检查 sticky 模型是否健康
			if r.HealthTracker.IsHealthy(selected) {
				return selected
			}
			// sticky 模型已降级，需要切换
			slog.Debug("fallback_router: sticky model degraded, switching",
				"complexity", c, "model", selected)
		}
	}

	// 2. 选择第一个健康的候选模型
	for _, model := range candidates {
		if r.HealthTracker.IsHealthy(model) {
			if r.Config.StickySession {
				r.stickyModel[c.String()] = model
			}
			return model
		}
	}

	// 3. 全部降级 → 回退到主模型（第一个候选）
	// 这样即使降级期内调用可能失败，但模型恢复正常后能立刻接住
	primary := candidates[0]
	if r.Config.StickySession {
		r.stickyModel[c.String()] = primary
	}
	return primary
}
