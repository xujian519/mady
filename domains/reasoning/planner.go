package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// factsInPromptLimit limits facts serialized in the LLM prompt to prevent
// context window overflow. With ~250 bytes per fact, 50 facts ≈ 12KB.
// This is the default when no context window is configured.
const defaultFactsInPrompt = 50

// computeFactsInPromptLimit derives a fact limit from the configured context
// window. It reserves 80% of the window for facts and assumes ~250 bytes per
// fact, clamped to a sane [10, 200] range.
func computeFactsInPromptLimit(contextWindow int64) int {
	if contextWindow <= 0 {
		return defaultFactsInPrompt
	}
	const bytesPerFact = 250
	maxBytes := contextWindow / 5
	n := int(maxBytes / bytesPerFact)
	if n < 10 {
		return 10
	}
	if n > 200 {
		return 200
	}
	return n
}

// Planner generates a Plan (Stage ③) from facts and rules.
//
// Three generation paths (tried in order):
//   - Template path: for simple intents, looks up a pre-built Plan template
//     registered via RegisterTemplate.
//   - KG topology path: when no template matches, attempts to generate a
//     Plan from the knowledge graph's WorkflowTopology (if a TopologyExtractor
//     is configured).
//   - LLM path: for complex/ambiguous intents, calls the LLM to generate a
//     Plan, optionally with multi-hypothesis branching.
type Planner struct {
	llm           LlmClient
	templates     map[string]Plan // key: "<CaseType>_<PlanIntent>"
	contextWindow int64
	topologyExt   *TopologyExtractor // optional — enables KG topology path
	mu            sync.RWMutex
}

// NewPlanner creates a Planner with an optional LLM client.
// llm may be nil for template-only operation.
func NewPlanner(llm LlmClient) *Planner {
	return &Planner{
		llm:       llm,
		templates: make(map[string]Plan),
	}
}

// WithTopologyExtractor attaches a topology extractor. When set and no
// hand-written template matches, the Planner attempts to generate a Plan
// from the knowledge graph's edge topology for the given case type.
func (p *Planner) WithTopologyExtractor(ext *TopologyExtractor) *Planner {
	p.topologyExt = ext
	return p
}

// SetContextWindow configures the model context window used to compute the
// number of facts included in LLM prompts.
func (p *Planner) SetContextWindow(ctxWindow int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.contextWindow = ctxWindow
}

func (p *Planner) factsInPromptLimit() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return computeFactsInPromptLimit(p.contextWindow)
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
			plan.UsedRules = ruleIDsFromConstraints(bb.ConfirmedRuleConstraints())
			bb.SetPlanV2(*plan)
			return plan, nil
		}
		// No hand-written template — try KG topology path.
		if plan := p.generateFromTopology(ctx, bb, intent); plan != nil {
			return plan, nil
		}
	}

	// Complex intent with LLM available — delegate to LLM.
	// detectPlanIntent currently returns PlanIntentChain for patentability/
	// invalidation cases to keep Phase 1 deterministic. ReAct/MultiHypothesis
	// are only exercised when the caller explicitly requests them (e.g. tests
	// or future runner promotion). This branch is the explicit LLM planning
	// entry point for those advanced intents.
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

// llmGenerate calls the LLM to generate a Plan from the blackboard state.
// The LLM is instructed to produce a structured JSON plan with steps,
// hypotheses when applicable, and reasoning trace. On any error (LLM failure,
// JSON parse failure), it falls back to buildFallbackPlan.
func (p *Planner) llmGenerate(ctx context.Context, bb *FactBlackboard, intent PlanIntent) (*Plan, error) {
	if p.llm == nil {
		slog.Warn("planner: LLM client is nil, using fallback plan", "case", bb.CaseID, "intent", intent)
		return p.buildFallbackPlan(bb, intent), nil
	}

	prompt := p.buildLLMPrompt(ctx, bb, intent)
	resp, err := p.llm.Chat(ctx, []LlmMessage{{Role: "user", Content: prompt}})
	if err != nil {
		slog.Warn("planner: LLM call failed, using fallback plan", "err", err)
		return p.buildFallbackPlan(bb, intent), nil
	}

	plan, err := p.parsePlanResponse(resp, bb, intent)
	if err != nil {
		slog.Warn("planner: LLM response parse failed, using fallback plan", "err", err)
		return p.buildFallbackPlan(bb, intent), nil
	}

	plan.UsedFacts = factIDs(bb.ActiveFacts())
	plan.UsedRules = ruleIDsFromConstraints(bb.ConfirmedRuleConstraints())
	bb.SetPlanV2(*plan)
	return plan, nil
}

// buildLLMPrompt constructs the LLM prompt for plan generation.
func (p *Planner) buildLLMPrompt(ctx context.Context, bb *FactBlackboard, intent PlanIntent) string {
	var sb strings.Builder
	sb.WriteString("你是一名资深专利分析专家。请根据以下案件信息和分析意图，生成一个结构化的执行计划。\n\n")

	fmt.Fprintf(&sb, "案件类型: %s\n", bb.CaseType)
	fmt.Fprintf(&sb, "案件ID: %s\n", bb.CaseID)
	fmt.Fprintf(&sb, "技术领域: %s\n", bb.TechnicalField)
	fmt.Fprintf(&sb, "分析意图: %s\n\n", intent)

	limit := p.factsInPromptLimit()
	// List active facts (limit to top N by confidence to avoid context overflow).
	facts := bb.ActiveFacts()
	fmt.Fprintf(&sb, "已收集事实（共 %d 条，展示前 %d 条）:\n", len(facts), limit)
	shown := 0
	for _, f := range facts {
		if !f.IsDiscarded() {
			fmt.Fprintf(&sb, "- [%s] [%s] %s (置信度: %.0f%%)\n", f.ID, f.Category, truncate(f.Content, 200), f.Confidence*100)
			shown++
			if shown >= limit {
				break
			}
		}
	}
	sb.WriteString("\n")

	// List rule constraints.
	constraints := bb.ConfirmedRuleConstraints()
	fmt.Fprintf(&sb, "适用规则（共 %d 条）:\n", len(constraints))
	for _, r := range constraints {
		fmt.Fprintf(&sb, "- [%s] [%s] %s\n", r.ArticleID, r.Requirement, r.Description)
	}
	sb.WriteString("\n")

	// KG topology-derived recommended step order (if available).
	if p.topologyExt != nil && p.topologyExt.HasStore() {
		topology, err := p.topologyExt.ExtractByCaseType(ctx, bb.CaseType, 2, 10)
		if err == nil && len(topology.Steps) > 0 {
			fmt.Fprintf(&sb, "知识图谱推荐步骤顺序（基于 %d 条审查指南规则）:\n", len(topology.Steps))
			for i, s := range topology.Steps {
				fmt.Fprintf(&sb, "  %d. [%s] %s\n", i+1, s.Relation, s.Name)
				if s.Content != "" {
					fmt.Fprintf(&sb, "     规则摘要: %s\n", truncate(s.Content, 150))
				}
			}
			sb.WriteString("\n")
		}
	}

	// Instruction for plan generation.
	sb.WriteString("请生成一个执行计划，格式为 JSON，结构如下：\n")
	sb.WriteString(`{
  "plan_id": "plan_<case_id>_<intent>_<timestamp>",
  "steps": [
    {
      "order": 1,
      "description": "步骤描述",
      "strategy": "chain",
      "expected_output": "预期产出描述"
    }
  ],
  "hypotheses": [
    {
      "id": "pro",
      "label": "主张方标签",
      "thesis": "核心论点"
    }
  ],
  "llm_reasoning": "解释为什么选择这些步骤和分析策略"
}
`)
	sb.WriteString("\nstrategy 可选值：chain（链式推理）、react（观察-思考-行动循环）、multi_hypothesis（正反方辩论+裁判）\n")
	sb.WriteString("hypotheses 仅在 intent 为 multi_hypothesis 时需要提供。\n")
	sb.WriteString("\n要求:\n")
	sb.WriteString("1. steps 至少包含 3 个步骤：信息分析 → 深度推理 → 结论生成\n")
	sb.WriteString("2. 对于创造性评估或无效分析，在步骤中适当使用 multi_hypothesis 策略进行正反方辩论\n")
	sb.WriteString("3. strategy 可选值：chain（链式推理）, react（观察-思考-行动循环）, multi_hypothesis（正反方辩论+裁判）\n")
	sb.WriteString("4. hypotheses 仅在意图为 multi_hypothesis 时需要提供\n")
	sb.WriteString("5. 只返回 JSON，不要用 markdown 代码块包裹\n")
	sb.WriteString("6. llm_reasoning 用中文简要说明你的规划思路\n")

	return sb.String()
}

// parsePlanResponse attempts to parse an LLM response into a Plan.
// It locates the first '{' and last '}' in the response to extract JSON,
// handling explanatory text before/after the JSON block.
func (p *Planner) parsePlanResponse(resp string, bb *FactBlackboard, intent PlanIntent) (*Plan, error) {
	// Locate JSON object boundaries — robust against LLM wrapping output
	// in explanatory text or markdown code fences.
	start := strings.Index(resp, "{")
	end := strings.LastIndex(resp, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON object found in response")
	}
	raw := resp[start : end+1]

	var plan Plan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, fmt.Errorf("unmarshal plan: %w", err)
	}

	// Fill in required fields.
	plan.CaseType = bb.CaseType
	plan.Intent = intent
	if plan.PlanID == "" {
		plan.PlanID = fmt.Sprintf("plan_%s_%s", bb.CaseID, intent)
	}
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("plan has no steps")
	}
	for i := range plan.Steps {
		if plan.Steps[i].Order == 0 {
			plan.Steps[i].Order = i + 1
		}
	}
	return &plan, nil
}

// generateFromTopology attempts to build a Plan from the knowledge graph's
// WorkflowTopology. It returns nil when no topology extractor is configured,
// the KG has no matching GuidelineRule nodes, or the topology cannot produce
// a valid Plan. Callers fall through to the LLM or fallback path.
func (p *Planner) generateFromTopology(ctx context.Context, bb *FactBlackboard, intent PlanIntent) *Plan {
	if p.topologyExt == nil || !p.topologyExt.HasStore() || bb == nil {
		return nil
	}

	topology, err := p.topologyExt.ExtractByCaseType(ctx, bb.CaseType, 2, 10)
	if err != nil || topology == nil || len(topology.Steps) == 0 {
		return nil
	}

	steps := make([]PlanStep, 0, len(topology.Steps))
	for i, ts := range topology.Steps {
		// Convert topology-level dependency indices (0-indexed) to PlanStep
		// DependsOn references (1-indexed Order numbers).
		var dependsOn []string
		if i < len(topology.Dependencies) && len(topology.Dependencies[i]) > 0 {
			for _, depIdx := range topology.Dependencies[i] {
				depOrder := depIdx + 1
				dependsOn = append(dependsOn, fmt.Sprintf("step_%d", depOrder))
			}
		}
		step := PlanStep{
			Order:         i + 1,
			Strategy:      ts.Strategy,
			DependsOn:     dependsOn,
			RequiredRules: []string{ts.ArticleID},
		}
		// Build a natural-language description from the KG node.
		switch ts.Relation {
		case WorkflowRelCites:
			step.Description = fmt.Sprintf("核查 %s — 依据 %s", ts.NodeType, ts.Name)
			if ts.Content != "" {
				step.Description += "：" + truncate(ts.Content, 100)
			}
		case WorkflowRelApplies:
			step.Description = fmt.Sprintf("对比 %s — %s 适用分析", ts.Name, bb.CaseType)
		case WorkflowRelRelatedTo:
			step.Description = fmt.Sprintf("关联审查 %s — %s", ts.NodeType, ts.Name)
		default:
			step.Description = fmt.Sprintf("分析 %s — %s", ts.NodeType, ts.Name)
		}
		step.ExpectedOutput = fmt.Sprintf("%s 分析结论", ts.NodeType)
		steps = append(steps, step)
	}

	if len(steps) == 0 {
		return nil
	}

	pid := fmt.Sprintf("plan_%s_%s_kg", bb.CaseID, intent)
	plan := &Plan{
		PlanID:   pid,
		Intent:   intent,
		CaseType: bb.CaseType,
		Steps:    steps,
	}
	plan.UsedFacts = factIDs(bb.ActiveFacts())
	plan.UsedRules = ruleIDsFromConstraints(bb.ConfirmedRuleConstraints())
	bb.SetPlanV2(*plan)
	return plan
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
		UsedRules: ruleIDsFromConstraints(bb.ConfirmedRuleConstraints()),
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
