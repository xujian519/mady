package domains

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// findGateway 遍历 Lifecycle（可能是单 hook 或 LifecycleChain）找到 *Gateway。
// 用于验证 UnifiedAgentConfig 确实把 Gateway 注入了生命周期链。
//
// 参数用 any 而非 agentcore.LifecycleHook：LifecycleHook 接口已废弃
// （staticcheck SA1019），建议改用细粒度 Observer 接口；但 cfg.Lifecycle
// 字段类型现状仍是该接口，测试 helper 用 any 接收其值即可绕过显式引用。
func findGateway(t *testing.T, lc any) *agentcore.Gateway {
	t.Helper()
	if lc == nil {
		return nil
	}
	if g, ok := lc.(*agentcore.Gateway); ok {
		return g
	}
	if chain, ok := lc.(agentcore.LifecycleChain); ok {
		for _, h := range chain {
			// LifecycleChain 元素本身可能也是包装过的；递归一层足够
			// 覆盖 AppendLifecycle 的扁平化结构。
			if g, ok := h.(*agentcore.Gateway); ok {
				return g
			}
		}
	}
	return nil
}

// --- Gateway 已注入 Lifecycle ---

func TestUnifiedAgentConfig_GatewayInjected(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := UnifiedAgentConfig(base)

	g := findGateway(t, cfg.Lifecycle)
	if g == nil {
		t.Fatal("UnifiedAgentConfig 必须把 Gateway 注入 Lifecycle（替换原 ReasoningStrategyRouter）")
	}
	if g.Reasoning == nil {
		t.Error("Gateway.Reasoning 未配置（effort/budget map 缺失）")
	}
	if g.StrategySelector == nil {
		t.Error("Gateway.StrategySelector 未配置（策略注入职责丢失）")
	}
	if !g.StrategySelector.StrategyHintInjection {
		t.Error("StrategyHintInjection 必须启用以保留原行为")
	}
}

// --- 不再有裸 ReasoningStrategyRouter 注册（避免双触发） ---

func TestUnifiedAgentConfig_NoStandaloneStrategyRouter(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := UnifiedAgentConfig(base)

	// 遍历整条 LifecycleChain，确认没有 *ReasoningStrategyRouter。
	// Gateway 已吸收其职责，重复注册会导致每轮两次 Classify。
	if chain, ok := cfg.Lifecycle.(agentcore.LifecycleChain); ok {
		for _, h := range chain {
			if _, ok := h.(*agentcore.ReasoningStrategyRouter); ok {
				t.Error("LifecycleChain 中仍存在裸 ReasoningStrategyRouter，会与 Gateway 双触发")
			}
		}
	}
}

// --- FallbackRouter 已接入（候选链可空，结构就位） ---

func TestUnifiedAgentConfig_FallbackRouterWired(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := UnifiedAgentConfig(base)

	g := findGateway(t, cfg.Lifecycle)
	if g == nil {
		t.Fatal("Gateway not injected")
	}
	if g.Fallback == nil {
		t.Fatal("Gateway.Fallback 未配置：模型回退链结构缺失")
	}
	// 候选链默认为空（用户未配置备选模型），SelectModel 返回空字符串，
	// Gateway 保留 mcc.Request.Model 不变 —— 这是安全的 no-op。
	if sel := g.Fallback.SelectModel(agentcore.ComplexityHigh); sel != "" {
		t.Errorf("空候选链应返回空（no-op），got %q", sel)
	}
	// Config.FallbackRouter 必须指向同一实例，供 callModelWithFallback 使用。
	if cfg.FallbackRouter != g.Fallback {
		t.Error("cfg.FallbackRouter 与 Gateway.Fallback 不是同一实例，回退链断链")
	}
}

// --- 候选链配置生效（接入后用户可配置备选模型） ---

func TestUnifiedAgentConfig_FallbackCandidatesEffective(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := UnifiedAgentConfig(base)

	g := findGateway(t, cfg.Lifecycle)
	if g == nil {
		t.Fatal("Gateway not injected")
	}
	// 模拟用户配置候选链 —— 接入后这条路径立即生效。
	g.Fallback.Config.Candidates = map[agentcore.Complexity][]string{
		agentcore.ComplexityHigh: {"primary-model", "backup-model"},
	}

	mcc := &agentcore.ModelCallContext{Request: &agentcore.ProviderRequest{Model: "default"}}
	arc := &agentcore.AgentRunContext{Input: "请深度分析这个专利的创造性侵权问题"}
	if err := g.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall: %v", err)
	}
	if mcc.Request.Model != "primary-model" {
		t.Errorf("候选链应选中主模型 primary-model，got %q", mcc.Request.Model)
	}
}

// --- 预算钳制在 blocking 时触发（核心价值验证） ---

func TestUnifiedAgentConfig_BudgetBlockingClampsEffort(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	base.ContextWindow = 100 // 极小窗口，构造 blocking 场景
	cfg := UnifiedAgentConfig(base)

	g := findGateway(t, cfg.Lifecycle)
	if g == nil {
		t.Fatal("Gateway not injected")
	}
	if g.BudgetManager == nil {
		t.Fatal("ContextWindow>0 时必须配置 BudgetManager")
	}
	if g.ContextWindow != 100 {
		t.Errorf("ContextWindow 未透传，got %d", g.ContextWindow)
	}

	// 构造超大消息触发 blocking。
	mcc := &agentcore.ModelCallContext{Request: &agentcore.ProviderRequest{
		Messages: []agentcore.Message{{Role: agentcore.RoleUser, Content: strings.Repeat("a", 1000)}},
	}}
	arc := &agentcore.AgentRunContext{
		Input:    "x",
		Messages: []agentcore.Message{{Role: agentcore.RoleUser, Content: strings.Repeat("a", 1000)}},
	}
	if err := g.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall: %v", err)
	}
	d := g.LastDecision()
	if d.Budget.State != agentcore.BudgetBlocking {
		t.Fatalf("预期 blocking，got %s (%s)", d.Budget.State, d.Budget)
	}
	if !d.BudgetClamp {
		t.Error("blocking 时必须 BudgetClamp=true")
	}
	if mcc.Request.Thinking == nil || mcc.Request.Thinking.Effort != agentcore.ThinkingEffortLow {
		t.Errorf("blocking 时 effort 必须钳制为 Low，got %+v", mcc.Request.Thinking)
	}
}

// --- 预算未配置时不钳制（向后兼容） ---

func TestUnifiedAgentConfig_NoContextWindow_NoBudgetManager(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{} // ContextWindow=0
	cfg := UnifiedAgentConfig(base)

	g := findGateway(t, cfg.Lifecycle)
	if g == nil {
		t.Fatal("Gateway not injected")
	}
	if g.BudgetManager != nil {
		t.Error("ContextWindow=0 时不应配置 BudgetManager（向后兼容）")
	}
}

// --- 策略注入经 Gateway 保留（不丢失原 ReasoningStrategyRouter 行为） ---

func TestUnifiedAgentConfig_StrategyHintInjectionPreserved(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := UnifiedAgentConfig(base)

	g := findGateway(t, cfg.Lifecycle)
	if g == nil {
		t.Fatal("Gateway not injected")
	}

	originalSys := "你是专利助手。"
	mcc := &agentcore.ModelCallContext{Request: &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: originalSys},
			{Role: agentcore.RoleUser, Content: "分析专利CN12345的新颖性"},
		},
	}}
	arc := &agentcore.AgentRunContext{
		Input: "分析专利CN12345的新颖性",
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "分析专利CN12345的新颖性"},
		},
	}
	if err := g.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall: %v", err)
	}
	sysMsg := mcc.Request.Messages[0]
	if len(sysMsg.Content) <= len(originalSys) {
		t.Errorf("策略 hint 未注入到系统消息：got %q", sysMsg.Content)
	}
	if !strings.HasPrefix(sysMsg.Content, originalSys) {
		t.Errorf("注入应追加而非替换：got %q", sysMsg.Content)
	}
}

// --- 只分类一次（核心保证：跨 Gateway 全部职责） ---

func TestUnifiedAgentConfig_ClassifyOncePerTurn(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := UnifiedAgentConfig(base)

	g := findGateway(t, cfg.Lifecycle)
	if g == nil {
		t.Fatal("Gateway not injected")
	}

	calls := 0
	// 包装 Classifier 计数。Gateway 内部在 Decide 里调用一次。
	original := g.Classifier
	g.Classifier = &countingClassifierDomains{
		inner:  original,
		onCall: func() { calls++ },
	}

	mcc := &agentcore.ModelCallContext{Request: &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: "sys"},
			{Role: agentcore.RoleUser, Content: "分析专利创造性"},
		},
	}}
	arc := &agentcore.AgentRunContext{Input: "分析专利创造性"}
	if err := g.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatalf("BeforeModelCall: %v", err)
	}
	if calls != 1 {
		t.Errorf("Gateway 必须每轮只分类一次（驱动 effort + 模型 + 策略注入），got %d 次", calls)
	}
}

// countingClassifierDomains 包装一个 Classifier 并在每次 Classify 时回调。
type countingClassifierDomains struct {
	inner  agentcore.ComplexityClassifier
	onCall func()
}

func (c *countingClassifierDomains) Classify(input string, messages []agentcore.Message) agentcore.Complexity {
	c.onCall()
	return c.inner.Classify(input, messages)
}
