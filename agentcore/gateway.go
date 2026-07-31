package agentcore

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// This file implements the Gateway — a single per-turn decision entry point
// inspired by PilotDeck's Gateway. It orchestrates three concerns that
// previously ran as independent lifecycle hooks:
//
//  1. complexity classification (ReasoningRouter / DefaultClassifier)
//  2. token-budget evaluation (TokenBudgetManager)
//  3. health-aware model selection (FallbackRouter)
//
// The value of unifying them: classification happens ONCE per turn (the
// ReasoningRouter and FallbackRouter each classified independently when
// registered as separate hooks), and the budget state can now drive the
// reasoning strategy — specifically, a BudgetBlocking context clamps effort
// to Low so the model does not burn reasoning tokens against an
// already-overflowing context window.
//
// Gateway composes rather than replaces: model selection delegates to
// FallbackRouter.SelectModel, effort/budget policy reads ReasoningRouter's
// public Efforts/Budgets maps (or Gateway's own). Gateway implements
// LifecycleHook; register it INSTEAD OF the individual routers to get the
// unified, classify-once behavior.

// GatewayDecision is the unified per-turn decision produced by Gateway.
// It captures everything Gateway learned and chose for a turn in one struct,
// making the decision observable and traceable.
type GatewayDecision struct {
	// Turn is the AgentRunContext.Turn the decision was made for.
	Turn int64
	// Complexity is the single classification result reused across all
	// downstream concerns (model selection, effort mapping).
	Complexity Complexity
	// Budget is the token-budget snapshot. Zero-value when no
	// TokenBudgetManager / ContextWindow is configured.
	Budget BudgetSnapshot
	// Model is the selected model (empty when no FallbackRouter is
	// configured, meaning the caller's default model is kept).
	Model string
	// Effort is the reasoning effort mapped from Complexity, possibly
	// clamped by the budget state.
	Effort ThinkingEffort
	// BudgetClamp is true when Effort was forced to Low because the budget
	// state was BudgetBlocking.
	BudgetClamp bool
	// Reason is a compact, human-readable trace of the decision inputs.
	Reason string
}

// Gateway is the single decision entry point orchestrating complexity,
// budget, model selection, and reasoning effort. It implements LifecycleHook
// so it composes transparently with other hooks (register via WithLifecycle).
//
// Integration contract: when Gateway is registered as the model-call hook,
// do NOT also register FallbackRouter and ReasoningRouter as independent
// lifecycle hooks — Gateway holds and drives them. Registering both would
// double-classify and double-record health.
type Gateway struct {
	BaseLifecycleHook

	// Classifier is the single source of truth for complexity. Nil defaults
	// to DefaultClassifier at first use.
	Classifier ComplexityClassifier

	// BudgetManager evaluates the context budget. Optional; nil → Budget is
	// a zero-value snapshot and no clamping occurs.
	BudgetManager *TokenBudgetManager

	// ContextWindow is the token window BudgetManager is evaluated against.
	// Required for budget evaluation; <=0 disables it.
	ContextWindow int64

	// ToolDefinitions are included in the budget estimate (tools are static
	// per agent, so this is set once at construction).
	ToolDefinitions []ToolDefinition

	// Fallback drives health-aware model selection. Optional; nil → Model
	// is left empty (caller's default model is kept).
	Fallback *FallbackRouter

	// Reasoning provides per-complexity Effort and reasoning-token Budget
	// policy. Optional; when nil, Gateway.Efforts or the default-by-
	// complexity mapping is used.
	Reasoning *ReasoningRouter

	// StrategySelector, when non-nil with StrategyHintInjection=true,
	// appends a reasoning strategy hint (e.g. StepByStep) to the request's
	// system message — the same behavior as ReasoningStrategyRouter, but
	// driven by the single classification Gateway already performed rather
	// than a second Classify call. This lets Gateway fully replace
	// ReasoningStrategyRouter without losing strategy injection.
	StrategySelector *StrategySelector

	// Efforts is Gateway's own complexity→effort map, consulted when
	// Reasoning is nil. Leave nil for the default Low/Medium/High mapping.
	Efforts map[Complexity]ThinkingEffort

	// Decision, when set, is invoked with each GatewayDecision for tracing.
	Decision func(d GatewayDecision)

	// OnHighComplexity, when set, is invoked with the run context and the
	// current decision whenever the classification is ComplexityHigh.
	// Used by plantask AutoEnterPlanning (03-design §1.3): the callback
	// fires per model call; turn-based deduplication is the consumer's job.
	// This field is the agentcore-side injection point (no import of
	// bootstrap/domains), wired at assembly time.
	OnHighComplexity func(arc *AgentRunContext, d GatewayDecision)

	// mu guards lastDecision (written in BeforeModelCall, read via
	// LastDecision). The agent model-call path is single-threaded per turn;
	// the mutex only protects LastDecision() callers on other goroutines.
	mu           sync.RWMutex
	lastDecision GatewayDecision
}

// NewGateway builds a Gateway with a default classifier. Configure
// BudgetManager / Fallback / Reasoning on the returned value before use.
func NewGateway(classifier ComplexityClassifier) *Gateway {
	return &Gateway{Classifier: classifier}
}

func (g *Gateway) classifier() ComplexityClassifier {
	if g.Classifier == nil {
		return NewDefaultClassifier()
	}
	return g.Classifier
}

// Decide computes the unified per-turn decision. It is pure with respect to
// any ProviderRequest: it does not mutate the request, only reads the
// conversation from arc. Call BeforeModelCall to apply the decision.
func (g *Gateway) Decide(arc *AgentRunContext) GatewayDecision {
	d := GatewayDecision{Turn: turnOf(arc)}
	if arc != nil {
		d.Complexity = g.classifier().Classify(latestUserInput(arc), arc.Messages)
	}

	if g.BudgetManager != nil && g.ContextWindow > 0 {
		var msgs []Message
		if arc != nil {
			msgs = arc.Messages
		}
		d.Budget = g.BudgetManager.Evaluate(msgs, g.ToolDefinitions, g.ContextWindow)
	}

	if g.Fallback != nil {
		d.Model = g.Fallback.SelectModel(d.Complexity)
	}

	d.Effort = g.effortFor(d.Complexity)
	// Budget-driven strategy: when the context is near overflow, clamp
	// reasoning effort to Low. Burning reasoning tokens against an
	// already-blocking context risks worsening overflow; TieredEngine's
	// compaction (snipRatio 0.6) should have fired earlier, so this is a
	// backstop that also signals the clamp in the decision trace.
	if d.Budget.State == BudgetBlocking {
		d.Effort = ThinkingEffortLow
		d.BudgetClamp = true
	}

	d.Reason = g.explain(d)
	return d
}

// BeforeModelCall applies the unified decision to mcc.Request: model
// selection, reasoning effort, and (when not clamped) the per-complexity
// reasoning-token budget. Caches the decision for LastDecision() and fires
// the optional Decision tracing callback.
func (g *Gateway) BeforeModelCall(_ context.Context, arc *AgentRunContext, mcc *ModelCallContext) error {
	d := g.Decide(arc)

	if mcc != nil && mcc.Request != nil {
		if d.Model != "" {
			mcc.Request.Model = d.Model
		}
		applyGatewayEffort(mcc.Request, d.Effort)
		// Apply the per-complexity reasoning token budget only when not
		// clamped: a clamped Low effort with a large thinking budget is
		// contradictory.
		if !d.BudgetClamp {
			if b, ok := g.reasoningBudgetFor(d.Complexity); ok {
				setGatewayThinkingBudget(mcc.Request, b)
			}
		}
		// Strategy hint injection — reuses d.Complexity (no second Classify),
		// so Gateway replaces ReasoningStrategyRouter without behavioral loss.
		g.injectStrategyHint(mcc.Request, d.Complexity)
	}

	g.mu.Lock()
	g.lastDecision = d
	g.mu.Unlock()

	if g.Decision != nil {
		g.Decision(d)
	}
	if d.Complexity == ComplexityHigh && g.OnHighComplexity != nil {
		g.OnHighComplexity(arc, d)
	}
	if d.Budget.State != BudgetOK && d.Budget.MaxContextTokens > 0 {
		slog.Info("gateway: budget-aware decision",
			"turn", d.Turn, "complexity", d.Complexity,
			"budget_state", d.Budget.State,
			"model", d.Model, "effort", d.Effort,
			"clamped", d.BudgetClamp)
	}
	return nil
}

// AfterModelCall delegates health recording to the FallbackRouter. Because
// SelectModel (called in Decide, which BeforeModelCall invokes) already
// cached fr.lastModel, the router attributes the result to the correct model
// even though the success path passes mcc.Request == nil.
func (g *Gateway) AfterModelCall(ctx context.Context, arc *AgentRunContext, mcc *ModelCallContext) {
	if g.Fallback != nil {
		g.Fallback.AfterModelCall(ctx, arc, mcc)
	}
}

// LastDecision returns the most recent decision applied via BeforeModelCall,
// safe for concurrent readers. Returns a zero-value GatewayDecision before
// the first BeforeModelCall.
func (g *Gateway) LastDecision() GatewayDecision {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.lastDecision
}

// --- internal helpers ---

// effortFor resolves the reasoning effort for a complexity, consulting
// ReasoningRouter.Efforts, then Gateway.Efforts, then a default mapping that
// matches NewReasoningRouter.
func (g *Gateway) effortFor(c Complexity) ThinkingEffort {
	if g.Reasoning != nil {
		if effort, ok := g.Reasoning.Efforts[c]; ok && effort != ThinkingEffortDefault {
			return effort
		}
	}
	if g.Efforts != nil {
		if effort, ok := g.Efforts[c]; ok && effort != ThinkingEffortDefault {
			return effort
		}
	}
	switch c {
	case ComplexityLow:
		return ThinkingEffortLow
	case ComplexityMedium:
		return ThinkingEffortMedium
	case ComplexityHigh:
		return ThinkingEffortHigh
	}
	return ThinkingEffortDefault
}

// reasoningBudgetFor returns the per-complexity reasoning-token budget from
// ReasoningRouter.Budgets when configured.
func (g *Gateway) reasoningBudgetFor(c Complexity) (int64, bool) {
	if g.Reasoning != nil {
		if b, ok := g.Reasoning.Budgets[c]; ok {
			return b, true
		}
	}
	return 0, false
}

func (g *Gateway) explain(d GatewayDecision) string {
	parts := make([]string, 0, 5)
	parts = append(parts, fmt.Sprintf("complexity=%s", d.Complexity))
	if d.Budget.MaxContextTokens > 0 {
		parts = append(parts, fmt.Sprintf("budget=%s", d.Budget.State))
	}
	if d.Model != "" {
		parts = append(parts, fmt.Sprintf("model=%s", d.Model))
	}
	parts = append(parts, fmt.Sprintf("effort=%s", d.Effort))
	if d.BudgetClamp {
		parts = append(parts, "clamped=low(blocking)")
	}
	return strings.Join(parts, " ")
}

func turnOf(arc *AgentRunContext) int64 {
	if arc == nil {
		return 0
	}
	return arc.Turn
}

// applyGatewayEffort sets the reasoning effort on req, initializing the
// ThinkingConfig if absent. A ThinkingEffortDefault value is a no-op so an
// unmapped complexity does not clobber an explicitly configured effort.
func applyGatewayEffort(req *ProviderRequest, effort ThinkingEffort) {
	if req == nil || effort == ThinkingEffortDefault {
		return
	}
	if req.Thinking == nil {
		req.Thinking = &ThinkingConfig{}
	}
	req.Thinking.Effort = effort
}

// setGatewayThinkingBudget sets the reasoning-token budget on req.
func setGatewayThinkingBudget(req *ProviderRequest, budget int64) {
	if req == nil {
		return
	}
	if req.Thinking == nil {
		req.Thinking = &ThinkingConfig{}
	}
	req.Thinking.Budget = budget
}

// injectStrategyHint appends the strategy hint for the given complexity to
// the request's system message, cloning the messages slice first to avoid
// mutating the slice captured by other observers. This is the Gateway-side
// equivalent of ReasoningStrategyRouter's injection, reusing the already-
// classified complexity instead of calling Classify a second time.
func (g *Gateway) injectStrategyHint(req *ProviderRequest, c Complexity) {
	if g.StrategySelector == nil || !g.StrategySelector.StrategyHintInjection {
		return
	}
	hint := g.StrategySelector.StrategyHint(c)
	if hint == "" {
		return
	}
	orig := req.Messages
	cloned := make([]Message, len(orig))
	for i, msg := range orig {
		cloned[i] = msg.Clone()
	}
	injected := false
	for i, msg := range cloned {
		if msg.Role == RoleSystem {
			cloned[i].Content = msg.Content + hint
			injected = true
			break
		}
	}
	if !injected {
		cloned = append([]Message{{Role: RoleSystem, Content: strings.TrimSpace(hint)}}, cloned...)
	}
	req.Messages = cloned
}
