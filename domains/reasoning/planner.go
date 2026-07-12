package reasoning

import (
	"context"
	"fmt"
	"sync"
)

// Planner generates a Plan (Stage ③) from facts and rules.
//
// Two generation paths:
//   - Template path: for simple intents, looks up a pre-built Plan template,
//     fills in UsedFacts/UsedRules from the blackboard, and returns it.
//   - LLM path: for complex/ambiguous intents, calls the LLM to generate a
//     Plan, optionally with multi-hypothesis branching.
type Planner struct {
	llm       LlmClient
	templates map[string]Plan // key: "<CaseType>_<PlanIntent>"
	mu        sync.RWMutex
}

// NewPlanner creates a Planner with an optional LLM client.
// llm may be nil for template-only operation.
func NewPlanner(llm LlmClient) *Planner {
	return &Planner{
		llm:       llm,
		templates: make(map[string]Plan),
	}
}

// RegisterTemplate adds or replaces a Plan template.
func (p *Planner) RegisterTemplate(caseType CaseType, intent PlanIntent, plan Plan) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.templates[templateKey(caseType, intent)] = plan
}

// GeneratePlan produces a Plan from the blackboard state.
//
// It detects the intent, then either returns a filled-in template (simple
// intents) or delegates to the LLM (complex intents). If the LLM is nil and
// no template matches, it falls back to a minimal Plan with a single chain step.
func (p *Planner) GeneratePlan(ctx context.Context, bb *FactBlackboard, intent PlanIntent) (*Plan, error) {
	// Try template first for simple intents.
	if intent == PlanIntentSimple || intent == PlanIntentChain {
		if plan := p.lookupTemplate(bb.CaseType, intent); plan != nil {
			plan.UsedFacts = factIDs(bb.ActiveFacts())
			plan.UsedRules = ruleIDsFromConstraints(bb.RuleConstraints())
			bb.SetPlanV2(*plan)
			return plan, nil
		}
	}

	// Complex intent with LLM available — delegate to LLM.
	if p.llm != nil && (intent == PlanIntentReAct || intent == PlanIntentMultiHypothesis) {
		return p.llmGenerate(ctx, bb, intent)
	}

	// Fallback: build a minimal single-step chain Plan.
	return p.buildFallbackPlan(bb, intent), nil
}

// lookupTemplate returns a copy of the matching template, or nil.
func (p *Planner) lookupTemplate(caseType CaseType, intent PlanIntent) *Plan {
	p.mu.RLock()
	defer p.mu.RUnlock()
	tpl, ok := p.templates[templateKey(caseType, intent)]
	if !ok {
		return nil
	}
	cp := clonePlan(tpl)
	return &cp
}

// llmGenerate delegates Plan generation to the LLM (not yet implemented).
func (p *Planner) llmGenerate(_ context.Context, bb *FactBlackboard, intent PlanIntent) (*Plan, error) {
	// TODO(Phase 2): Implement LLM-driven Plan generation.
	return p.buildFallbackPlan(bb, intent), nil
}

// buildFallbackPlan creates a minimal single-step chain Plan.
func (p *Planner) buildFallbackPlan(bb *FactBlackboard, intent PlanIntent) *Plan {
	pid := fmt.Sprintf("plan_%s_%s", bb.CaseID, intent)
	plan := &Plan{
		PlanID:   pid,
		Intent:   intent,
		CaseType: bb.CaseType,
		Steps: []PlanStep{{
			Order:       1,
			Description: "执行推理分析",
			Strategy:    StrategyChain,
		}},
		UsedFacts: factIDs(bb.ActiveFacts()),
		UsedRules: ruleIDsFromConstraints(bb.RuleConstraints()),
	}
	bb.SetPlanV2(*plan)
	return plan
}

// defaultPatentPlanTemplates returns the built-in Plan templates for patent scenarios.
func defaultPatentPlanTemplates() map[string]Plan {
	tpl := make(map[string]Plan)

	// Patent Novelty Search: parse → search → analyze → conclude.
	tpl[templateKey(CaseNoveltySearch, PlanIntentChain)] = Plan{
		PlanID:   "tpl_novelty_chain",
		Intent:   PlanIntentChain,
		CaseType: CaseNoveltySearch,
		Steps: []PlanStep{
			{Order: 1, Description: "解析技术交底书，提取技术特征", Strategy: StrategyChain},
			{Order: 2, Description: "检索现有技术文献", Strategy: StrategyReact},
			{Order: 3, Description: "逐项对比技术特征与现有技术", Strategy: StrategyChain},
			{Order: 4, Description: "生成新颖性分析结论", Strategy: StrategyChain},
		},
	}

	// Patent Patentability: broader analysis with inventive step.
	tpl[templateKey(CasePatentability, PlanIntentChain)] = Plan{
		PlanID:   "tpl_patentability_chain",
		Intent:   PlanIntentChain,
		CaseType: CasePatentability,
		Steps: []PlanStep{
			{Order: 1, Description: "解析技术交底书", Strategy: StrategyChain},
			{Order: 2, Description: "检索现有技术", Strategy: StrategyReact},
			{Order: 3, Description: "新颖性比对", Strategy: StrategyChain},
			{Order: 4, Description: "创造性分析（显而易见性判断）", Strategy: StrategyMultiHypothesis},
			{Order: 5, Description: "生成可专利性综合报告", Strategy: StrategyChain},
		},
	}

	return tpl
}

// --- helpers ---

func templateKey(caseType CaseType, intent PlanIntent) string {
	return string(caseType) + "_" + string(intent)
}

func factIDs(facts []FactEntry) []string {
	ids := make([]string, 0, len(facts))
	for _, f := range facts {
		ids = append(ids, f.ID)
	}
	return ids
}

func ruleIDsFromConstraints(constraints []RuleConstraint) []string {
	ids := make([]string, 0, len(constraints))
	for _, c := range constraints {
		ids = append(ids, c.ArticleID)
	}
	return ids
}

func clonePlan(p Plan) Plan {
	cp := p
	if p.Steps != nil {
		cp.Steps = make([]PlanStep, len(p.Steps))
		copy(cp.Steps, p.Steps)
	}
	if p.Hypotheses != nil {
		cp.Hypotheses = make([]PlanHypothesis, len(p.Hypotheses))
		copy(cp.Hypotheses, p.Hypotheses)
	}
	if p.UsedFacts != nil {
		cp.UsedFacts = make([]string, len(p.UsedFacts))
		copy(cp.UsedFacts, p.UsedFacts)
	}
	if p.UsedRules != nil {
		cp.UsedRules = make([]string, len(p.UsedRules))
		copy(cp.UsedRules, p.UsedRules)
	}
	return cp
}
