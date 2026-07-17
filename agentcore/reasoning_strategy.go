package agentcore

import "context"

// ============================================================================
// ReasoningStrategy — turns a complexity classification into a concrete
// reasoning approach, selecting both a strategy hint and a step framework.
// ============================================================================

// ReasoningStrategy describes a reasoning approach the model should follow.
type ReasoningStrategy int

const (
	// StrategyDefault lets the model choose its own approach.
	StrategyDefault ReasoningStrategy = iota
	// StrategyStepByStep instructs chain-of-thought reasoning.
	StrategyStepByStep
	// StrategyStructuredAnalysis provides a structured framework for multi-dim analysis.
	StrategyStructuredAnalysis
	// StrategyDebate simulates multiple perspectives debating the problem.
	StrategyDebate
	// StrategyTreeOfThoughts explores multiple reasoning branches.
	StrategyTreeOfThoughts
	// StrategyVerifiedThinking requires verification after each step.
	StrategyVerifiedThinking
	// StrategyFirstPrinciples instructs first-principles reasoning.
	StrategyFirstPrinciples
)

func (s ReasoningStrategy) String() string {
	switch s {
	case StrategyDefault:
		return "default"
	case StrategyStepByStep:
		return "step_by_step"
	case StrategyStructuredAnalysis:
		return "structured_analysis"
	case StrategyDebate:
		return "debate"
	case StrategyTreeOfThoughts:
		return "tree_of_thoughts"
	case StrategyVerifiedThinking:
		return "verified_thinking"
	case StrategyFirstPrinciples:
		return "first_principles"
	}
	return "unknown"
}

// StrategyHint returns a system prompt fragment that hints at the reasoning
// approach without being prescriptive. The hint is appended to the system prompt.
func (s ReasoningStrategy) StrategyHint() string {
	switch s {
	case StrategyStepByStep:
		return "\n\n推理策略：请按步骤逐步推理，每步完成后标注【步骤N/总步数】。分析过程要完整、可追溯。" +
			"\n\nReasoning approach: Reason step by step. Mark each step as 【Step N/Total】. Keep analysis complete and traceable."

	case StrategyStructuredAnalysis:
		return "\n\n推理策略：请使用结构化分析框架：\n" +
			"1. 问题分解：将复杂问题拆分为子问题\n" +
			"2. 事实收集：列出已知事实和假设\n" +
			"3. 法律分析：逐条适用相关法条\n" +
			"4. 结论推导：从分析中得出明确结论\n\n" +
			"Reasoning approach: Use structured analysis: decompose, collect facts, apply rules, derive conclusion."

	case StrategyDebate:
		return "\n\n推理策略：请模拟多方辩论。对同一问题从不同角度提出论点并进行反驳，" +
			"最后综合各方观点得出最合理的结论。每个论点需标注论据来源。\n\n" +
			"Reasoning approach: Simulate multi-perspective debate. Present arguments from different angles, " +
			"rebut each, and synthesize the most reasonable conclusion."

	case StrategyTreeOfThoughts:
		return "\n\n推理策略：请使用思维树方法。对关键决策点，探索至少两个不同的推理分支，" +
			"评估每个分支的合理性后再选择最优路径继续推理。\n\n" +
			"Reasoning approach: Use tree-of-thoughts. At key decision points, explore at least two branches, " +
			"evaluate each, then continue along the most promising path."

	case StrategyVerifiedThinking:
		return "\n\n推理策略：请在每步推理后进行自我验证。检查逻辑一致性、法条适用正确性、" +
			"事实与结论的因果关系。如果发现错误，回溯修正。\n\n" +
			"Reasoning approach: Self-verify after each step. Check logical consistency, " +
			"correct application of rules, and causal links. Backtrack on errors."

	case StrategyFirstPrinciples:
		return "\n\n推理策略：请从第一性原理出发。不受已有结论或常见做法的约束，" +
			"回归到最基本的法律原则和事实本身，重建论证链。\n\n" +
			"Reasoning approach: Start from first principles. Avoid anchoring on existing conclusions. " +
			"Rebuild the argument chain from basic legal principles and facts."

	default:
		return ""
	}
}

// FrameworkStep describes a single step in a reasoning framework.
type FrameworkStep struct {
	// ID is a unique step identifier.
	ID string `json:"id"`
	// Instruction is the step instruction for the model.
	Instruction string `json:"instruction"`
	// OutputHint hints the expected output format for this step.
	OutputHint string `json:"output_hint,omitempty"`
}

// Framework is a sequence of reasoning steps organized by complexity.
type Framework struct {
	// Name identifies the framework.
	Name string `json:"name"`
	// Steps is the ordered list of reasoning steps.
	Steps []FrameworkStep `json:"steps"`
}

// DefaultFrameworks returns a set of reasoning frameworks mapped to
// complexity levels.
func DefaultFrameworks() map[Complexity]Framework {
	return map[Complexity]Framework{
		ComplexityLow: {
			Name: "quick_answer",
			Steps: []FrameworkStep{
				{ID: "understand", Instruction: "理解用户问题：明确用户需要的具体信息", OutputHint: "问题理解确认"},
				{ID: "answer", Instruction: "直接回答：提供简洁、准确的信息", OutputHint: "答案"},
				{ID: "verify", Instruction: "验证答案准确性和完整性", OutputHint: "验证确认"},
			},
		},
		ComplexityMedium: {
			Name: "structured_analysis",
			Steps: []FrameworkStep{
				{ID: "decompose", Instruction: "问题分解：将复杂问题拆分为子问题", OutputHint: "子问题列表"},
				{ID: "collect", Instruction: "收集相关信息：列出已知事实、相关法条和证据", OutputHint: "信息清单"},
				{ID: "analyze", Instruction: "逐项分析每个子问题，适用相关法律依据", OutputHint: "分析结果"},
				{ID: "conclude", Instruction: "综合各子问题分析，形成完整结论", OutputHint: "最终结论"},
			},
		},
		ComplexityHigh: {
			Name: "deep_reasoning",
			Steps: []FrameworkStep{
				{ID: "decompose", Instruction: "问题分解：将复杂问题拆分为可管理的子问题", OutputHint: "问题树"},
				{ID: "research", Instruction: "信息收集：查找相关法条、判例、技术标准", OutputHint: "法条和事实清单"},
				{ID: "analyze", Instruction: "多维度分析：从法律、技术、事实各角度分析", OutputHint: "多维度分析矩阵"},
				{ID: "synthesize", Instruction: "综合判断：权衡各方观点，形成判断", OutputHint: "权衡分析"},
				{ID: "verify", Instruction: "验证结论：检查逻辑一致性和法条适用正确性", OutputHint: "验证报告"},
				{ID: "conclude", Instruction: "给出最终结论和行动建议", OutputHint: "最终结论和建议"},
			},
		},
	}
}

// ============================================================================
// StrategySelector selects a reasoning strategy based on complexity.
// ============================================================================

// StrategySelector maps complexity to a reasoning strategy and framework.
type StrategySelector struct {
	// StrategyMap maps complexity to a reasoning strategy.
	StrategyMap map[Complexity]ReasoningStrategy `json:"strategy_map"`
	// Frameworks maps complexity to a reasoning framework.
	Frameworks map[Complexity]Framework `json:"frameworks,omitempty"`
	// StrategyHintInjection, when true, appends the strategy hint to the
	// system prompt via BeforeModelCall.
	StrategyHintInjection bool `json:"strategy_hint_injection"`
}

// NewDefaultStrategySelector returns a StrategySelector with sensible defaults.
func NewDefaultStrategySelector() *StrategySelector {
	return &StrategySelector{
		StrategyMap: map[Complexity]ReasoningStrategy{
			ComplexityLow:    StrategyStepByStep,
			ComplexityMedium: StrategyStructuredAnalysis,
			ComplexityHigh:   StrategyVerifiedThinking,
		},
		Frameworks:            DefaultFrameworks(),
		StrategyHintInjection: true,
	}
}

// SelectStrategy returns the reasoning strategy for the given complexity.
func (s *StrategySelector) SelectStrategy(c Complexity) ReasoningStrategy {
	if s.StrategyMap == nil {
		return StrategyDefault
	}
	strategy, ok := s.StrategyMap[c]
	if !ok {
		return StrategyDefault
	}
	return strategy
}

// GetFramework returns the reasoning framework for the given complexity.
func (s *StrategySelector) GetFramework(c Complexity) Framework {
	if s.Frameworks == nil {
		return Framework{}
	}
	fw, ok := s.Frameworks[c]
	if !ok {
		return Framework{}
	}
	return fw
}

// StrategyHint returns the hint for the strategy matching the complexity.
func (s *StrategySelector) StrategyHint(c Complexity) string {
	strategy := s.SelectStrategy(c)
	return strategy.StrategyHint()
}

// ============================================================================
// ReasoningStrategyRouter — combined router with strategy selection
// ============================================================================

// ReasoningStrategyRouter extends ReasoningRouter with strategy selection,
// framework steps, and optional system prompt injection.
type ReasoningStrategyRouter struct {
	*ReasoningRouter
	Selector *StrategySelector
}

// NewReasoningStrategyRouter creates a combined reasoning router with strategy
// selection and framework support.
func NewReasoningStrategyRouter(classifier ComplexityClassifier, selector *StrategySelector) *ReasoningStrategyRouter {
	if selector == nil {
		selector = NewDefaultStrategySelector()
	}
	return &ReasoningStrategyRouter{
		ReasoningRouter: NewReasoningRouter(classifier),
		Selector:        selector,
	}
}

// BeforeModelCall extends the base ReasoningRouter's BeforeModelCall with
// strategy hint injection into the system prompt.
func (r *ReasoningStrategyRouter) BeforeModelCall(ctx context.Context, arc *AgentRunContext, mcc *ModelCallContext) error {
	if mcc == nil || mcc.Request == nil {
		return nil
	}

	// Run base router (thinking effort/budget).
	if err := r.ReasoningRouter.BeforeModelCall(ctx, arc, mcc); err != nil {
		return err
	}

	// Inject strategy hint if enabled.
	if r.Selector != nil && r.Selector.StrategyHintInjection {
		input := latestUserInput(arc)
		c := r.Classifier.Classify(input, arc.Messages)
		hint := r.Selector.StrategyHint(c)

		if hint != "" {
			// Find the system message, make a copy, and append the hint.
			// Avoid in-place mutation of mcc.Request.Messages[i] so other
			// AfterModelCall observers see an unmodified request.
			for i, msg := range mcc.Request.Messages {
				if msg.Role == RoleSystem {
					cp := msg
					cp.Content += hint
					mcc.Request.Messages[i] = cp
					break
				}
			}
		}
	}

	return nil
}
