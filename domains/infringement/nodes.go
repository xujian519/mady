package infringement

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// JSON Schema constants.
const (
	jsTypeObject       = "object"
	jsTypeString       = "string"
	jsTypeArray        = "array"
	jsTypeBoolean      = "boolean"
	jsTypeNumber       = "number"
	jsFieldProperties  = "properties"
	jsFieldRequired    = "required"
	jsFieldLiteral     = "literal"
	jsFieldEquivalence = "equivalence"
	jsFieldTimeline    = "timeline"
	jsValHigh          = "high"
	jsValLow           = "low"
	jsFieldType        = "type"
	jsFieldEnum        = "enum"
	jsFieldItems       = "items"
	jsValMedium        = "medium"
)

// newInfringementAgent creates a uniformly-configured Agent node.
// All LLM nodes share Temperature=0.1 and MaxTurns=1.
func newInfringementAgent(provider agentcore.Provider, name, prompt string, schema map[string]any) *agentcore.Agent {
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:        name,
			Model:       "default",
			Provider:    provider,
			Temperature: 0.1,
		},
		SystemPrompt: prompt,
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 1,
		},
	}
	if schema != nil {
		cfg.ResponseFormat = agentcore.NewJSONSchemaResponseFormat(name, schema)
	}
	return agentcore.New(cfg)
}

// buildPerspectivePrompt wraps the neutral framework with perspective-specific instructions.
func buildPerspectivePrompt(framework, patenteePrompt, defendantPrompt string, perspective Perspective) string {
	var sb strings.Builder
	sb.WriteString(framework)
	sb.WriteString("\n\n")
	switch perspective {
	case PerspectivePatentee:
		sb.WriteString("## 分析视角：专利权人/原告\n\n")
		sb.WriteString(patenteePrompt)
	case PerspectiveDefendant:
		sb.WriteString("## 分析视角：被控侵权人/被告\n\n")
		sb.WriteString(defendantPrompt)
	default:
		// Unknown or empty perspective — inject both sides for neutral analysis.
		slog.Warn("infringement: unknown perspective, using neutral analysis", "perspective", string(perspective))
		sb.WriteString("## 分析视角：中立分析\n\n")
		sb.WriteString("请从客观中立角度进行分析，不要偏向专利权人或被控侵权人任何一方。\n")
	}
	return sb.String()
}

// extractInput safely extracts InfringementInput from Pregel state.
func extractInput(state graph.PregelState) *InfringementInput {
	v, ok := state[StateInput].(*InfringementInput)
	if !ok || v == nil {
		return nil
	}
	return v
}

// extractPerspective safely extracts the analysis perspective from Pregel state.
// Returns an error if the perspective key is missing or has the wrong type,
// since all infringement graph nodes require a perspective to produce correct output.
func extractPerspective(state graph.PregelState) (Perspective, error) {
	raw, ok := state[StatePerspective]
	if !ok {
		return "", fmt.Errorf("state missing required key %q", StatePerspective)
	}
	p, ok := raw.(Perspective)
	if !ok {
		return "", fmt.Errorf("state[%q] has unexpected type %T, expected Perspective", StatePerspective, raw)
	}
	return p, nil
}

// claimScopeNode returns the claim interpretation Pregel node.
func claimScopeNode(provider agentcore.Provider, frameworkProvider ArticleFrameworkProvider) graph.PregelNode {
	framework := defaultInfringementFramework
	if frameworkProvider != nil {
		framework = frameworkProvider.InfringementFramework()
	}
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, fmt.Errorf("claim_scope: input not available")
		}
		perspective, err := extractPerspective(state)
		if err != nil {
			return state, fmt.Errorf("claim_scope: %w", err)
		}

		patenteePrompt := "你是专利权人的法律顾问。请以最宽合理解释原则确定专利保护范围，在权利要求用语允许的范围内尽可能宽地解释保护范围。利用说明书和附图支持宽泛解释。"
		defendantPrompt := "你是被控侵权人的法律顾问。请以严格解释原则分析专利保护范围，识别权利要求中的限缩性用语，主张窄化解释。利用说明书中的具体实施方式限制权利要求的抽象含义。"

		prompt := buildPerspectivePrompt(framework, patenteePrompt, defendantPrompt, perspective)
		inputText := buildScopeInputText(input)

		agent := newInfringementAgent(provider, "infringement-claim-scope", prompt, claimScopeSchema())
		defer agent.Close()

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("claim_scope: %w", err)
		}

		var result ClaimScopeResult
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			return state, fmt.Errorf("claim_scope parse: %w", err)
		}
		state[StateClaimScope] = &result
		return state, nil
	}
}

// featureDecompositionNode returns the feature decomposition Pregel node.
func featureDecompositionNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, fmt.Errorf("feature_decomposition: input not available")
		}
		perspective, err := extractPerspective(state)
		if err != nil {
			return state, fmt.Errorf("feature_decomposition: %w", err)
		}

		prompt := "请将权利要求和被控产品/方法分别分解为独立的技术特征列表。每个特征应是一个完整的技术限定。特征的粒度应适中。\n\n## 权利要求文本\n" + input.PatentClaims + "\n\n## 被控产品/方法描述\n" + input.AccusedProduct

		if perspective == PerspectivePatentee {
			prompt += "\n\n从专利权人角度，将权利要求特征分解得尽可能全面细致。"
		} else {
			prompt += "\n\n从被控侵权人角度，注意特征粒度的合理划分，为后续差异化比对留出空间。"
		}

		agent := newInfringementAgent(provider, "infringement-feature-decomp", prompt, featureDecompSchema())
		defer agent.Close()

		output, err := agent.Run(ctx, toInputText(input))
		if err != nil {
			return state, fmt.Errorf("feature_decomposition: %w", err)
		}

		var out struct {
			ClaimFeatures   []string `json:"claim_features"`
			ProductFeatures []string `json:"product_features"`
		}
		if err := json.Unmarshal([]byte(output), &out); err != nil {
			return state, fmt.Errorf("feature_decomposition parse: %w", err)
		}
		state[StateClaimFeatures] = out.ClaimFeatures
		state[StateProductFeatures] = out.ProductFeatures
		return state, nil
	}
}

// literalInfringementNode returns the all-elements rule Pregel node.
func literalInfringementNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, fmt.Errorf("literal_infringement: input not available")
		}
		perspective, err := extractPerspective(state)
		if err != nil {
			return state, fmt.Errorf("literal_infringement: %w", err)
		}
		claimFeatures, _ := state[StateClaimFeatures].([]string)
		productFeatures, _ := state[StateProductFeatures].([]string)
		scope, _ := state[StateClaimScope].(*ClaimScopeResult)

		prompt := "## 任务：全面覆盖原则比对\n\n逐项比对权利要求的每个技术特征与被控产品的对应特征。\n被控方案包含全部特征→字面侵权成立；缺少任一特征→不成立（进入等同分析）；额外特征不影响认定。\n\n"
		if scope != nil {
			prompt += "### 保护范围\n" + scope.InterpretedScope + "\n\n"
		}
		prompt += "### 权利要求特征\n" + joinLines(claimFeatures)
		prompt += "\n### 被控产品特征\n" + joinLines(productFeatures)

		if perspective == PerspectivePatentee {
			prompt += "\n\n从专利权人角度：论证每个特征如何被被控产品覆盖。上位概念向下覆盖具体实施方式。"
		} else {
			prompt += "\n\n从被控侵权人角度：寻找每个可能的特征差异。注意上位概念与具体实施方式的精确对应。"
		}

		agent := newInfringementAgent(provider, "infringement-literal", prompt, literalSchema())
		defer agent.Close()

		output, err := agent.Run(ctx, toInputText(input))
		if err != nil {
			return state, fmt.Errorf("literal_infringement: %w", err)
		}

		var out struct {
			AllElementsMet  bool                `json:"all_elements_met"`
			FeatureMapping  []FeatureComparison `json:"feature_mapping"`
			MissingFeatures []string            `json:"missing_features"`
			ExtraFeatures   []string            `json:"extra_features"`
		}
		if err := json.Unmarshal([]byte(output), &out); err != nil {
			return state, fmt.Errorf("literal_infringement parse: %w", err)
		}
		state[StateLiteralResult] = &LiteralResult{
			AllElementsMet:  out.AllElementsMet,
			MissingFeatures: out.MissingFeatures,
			ExtraFeatures:   out.ExtraFeatures,
		}
		state[StateFeatureMapping] = out.FeatureMapping
		return state, nil
	}
}

// equivalenceNode returns the doctrine of equivalents Pregel node.
func equivalenceNode(provider agentcore.Provider, frameworkProvider ArticleFrameworkProvider) graph.PregelNode {
	framework := defaultInfringementFramework
	if frameworkProvider != nil {
		framework = frameworkProvider.InfringementFramework()
	}
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, fmt.Errorf("equivalence: input not available")
		}
		perspective, err := extractPerspective(state)
		if err != nil {
			return state, fmt.Errorf("equivalence: %w", err)
		}
		literal, _ := state[StateLiteralResult].(*LiteralResult)
		if literal == nil {
			return state, fmt.Errorf("equivalence: literal result not available")
		}

		patenteePrompt := "从专利权人角度：对于每个不匹配的特征，论证被控方案通过等同方式实现了相同的手段/功能/效果。反驳禁止反悔和捐献规则的适用。"
		defendantPrompt := "从被控侵权人角度：论证差异具有实质性——手段/功能/效果至少一项不同。积极援引禁止反悔原则和捐献规则限制等同范围。"

		prompt := buildPerspectivePrompt(framework, patenteePrompt, defendantPrompt, perspective)
		prompt += fmt.Sprintf("\n\n### 字面比对\n- 全部匹配: %v\n- 缺失特征: %v\n\n### 审查历史\n%s",
			literal.AllElementsMet, literal.MissingFeatures, truncateText(input.ProsecutionHistory, 2000))

		agent := newInfringementAgent(provider, "infringement-equivalence", prompt, equivalenceSchema())
		defer agent.Close()

		output, err := agent.Run(ctx, toInputText(input))
		if err != nil {
			return state, fmt.Errorf("equivalence: %w", err)
		}

		var out struct {
			EquivalentFeatures []EquivalenceAssessment `json:"equivalent_features"`
			EstoppelApplied    bool                    `json:"estoppel_applied"`
			EstoppelDetails    string                  `json:"estoppel_details"`
			DedicationApplied  bool                    `json:"dedication_applied"`
			DedicationDetails  string                  `json:"dedication_details"`
		}
		if err := json.Unmarshal([]byte(output), &out); err != nil {
			return state, fmt.Errorf("equivalence parse: %w", err)
		}
		state[StateEquivalenceResult] = &EquivalenceResult{
			EquivalentFeatures: out.EquivalentFeatures,
			EstoppelApplied:    out.EstoppelApplied,
			EstoppelDetails:    out.EstoppelDetails,
			DedicationApplied:  out.DedicationApplied,
			DedicationDetails:  out.DedicationDetails,
		}
		return state, nil
	}
}

// infringementVerdictNode returns the verdict synthesis Pregel node.
func infringementVerdictNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		perspective, err := extractPerspective(state)
		if err != nil {
			return state, fmt.Errorf("infringement_verdict: %w", err)
		}
		literal, _ := state[StateLiteralResult].(*LiteralResult)
		equiv, _ := state[StateEquivalenceResult].(*EquivalenceResult)
		if literal == nil || equiv == nil {
			return state, fmt.Errorf("infringement_verdict: literal or equivalence result not available")
		}

		prompt := fmt.Sprintf("## 侵权判定综合结论\n\n字面侵权: 全部匹配=%v, 缺失=%v\n等同认定: 等同特征数=%d, 禁止反悔=%v, 捐献规则=%v\n\n请给出结论(infringed/not_infringed/uncertain)、可能性(0-1)、判定依据、核心发现、风险等级。",
			literal.AllElementsMet, literal.MissingFeatures, len(equiv.EquivalentFeatures), equiv.EstoppelApplied, equiv.DedicationApplied)

		if perspective == PerspectivePatentee {
			prompt += "\n\n从专利权人角度：综合评估己方论证强度，识别薄弱环节。"
		} else {
			prompt += "\n\n从被控侵权人角度：评估被认定侵权的风险等级，识别抗辩方向。"
		}

		agent := newInfringementAgent(provider, "infringement-verdict", prompt, verdictSchema())
		defer agent.Close()

		output, err := agent.Run(ctx, "{}")
		if err != nil {
			return state, fmt.Errorf("infringement_verdict: %w", err)
		}

		var out struct {
			Conclusion  string   `json:"conclusion"`
			Likelihood  float64  `json:"likelihood"`
			Basis       []string `json:"basis"`
			KeyFindings []string `json:"key_findings"`
			RiskLevel   string   `json:"risk_level"`
		}
		if err := json.Unmarshal([]byte(output), &out); err != nil {
			return state, fmt.Errorf("infringement_verdict parse: %w", err)
		}
		state[StateVerdict] = &InfringementVerdict{
			Conclusion:  out.Conclusion,
			Likelihood:  out.Likelihood,
			Basis:       out.Basis,
			KeyFindings: out.KeyFindings,
			RiskLevel:   out.RiskLevel,
		}

		// Run deterministic rule engine checks alongside the LLM analysis.
		// The rule engine provides structured violation detection covering
		// all-elements, equivalence-three-part-test, prosecution-history-estoppel,
		// dedication-rule, and other legally-grounded rules.
		claimFeats, _ := state[StateClaimFeatures].([]string)
		prodFeats, _ := state[StateProductFeatures].([]string)
		featMapping, _ := state[StateFeatureMapping].([]FeatureComparison)
		prosecutionHistoryStr, _ := state["prosecution_history"].(string)
		priorArtStrs, _ := state["prior_art_refs"].([]string)

		ruleInput := &RuleCheckInput{
			ClaimFeatures:      claimFeats,
			ProductFeatures:    prodFeats,
			FeatureMapping:     featMapping,
			LiteralResult:      literal,
			EquivalenceResult:  equiv,
			ProsecutionHistory: prosecutionHistoryStr,
			PriorArtRefs:       priorArtStrs,
		}
		engine := NewRuleEngine()
		violations := engine.Check(ctx, ruleInput)
		if len(violations) > 0 {
			state[StateRuleCheckResults] = violations
		}
		return state, nil
	}
}

// defenseReviewNode returns the defense analysis Pregel node.
func defenseReviewNode(provider agentcore.Provider, frameworkProvider ArticleFrameworkProvider) graph.PregelNode {
	framework := defaultDefensesFramework
	if frameworkProvider != nil {
		framework = frameworkProvider.DefensesFramework()
	}
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, fmt.Errorf("defense_review: input not available")
		}
		perspective, err := extractPerspective(state)
		if err != nil {
			return state, fmt.Errorf("defense_review: %w", err)
		}
		verdict, _ := state[StateVerdict].(*InfringementVerdict)
		if verdict == nil {
			return state, fmt.Errorf("defense_review: verdict not available")
		}

		patenteePrompt := "从专利权人角度：预判被告可能提出的抗辩，分析每个抗辩的弱点，为庭审准备反驳策略。"
		defendantPrompt := "从被控侵权人角度：构建多层抗辩体系——确定首选抗辩策略和备用策略，按可行性排序。需要哪些证据支持？"

		prompt := buildPerspectivePrompt(framework, patenteePrompt, defendantPrompt, perspective)
		prompt += fmt.Sprintf("\n\n### 案情\n- 侵权结论: %s (%.0f%%)\n- 现有技术: %v\n- 许可: %v\n\n请逐一评估现有技术/先用权/合法来源/权利用尽/权利冲突抗辩。",
			verdict.Conclusion, verdict.Likelihood*100, input.PriorArtRefs, input.LicenseInfo)

		agent := newInfringementAgent(provider, "infringement-defense", prompt, defenseSchema())
		defer agent.Close()

		output, err := agent.Run(ctx, toInputText(input))
		if err != nil {
			return state, fmt.Errorf("defense_review: %w", err)
		}

		var out struct {
			Defenses []DefenseAssessment `json:"defenses"`
		}
		if err := json.Unmarshal([]byte(output), &out); err != nil {
			return state, fmt.Errorf("defense_review parse: %w", err)
		}
		state[StateDefenseAnalysis] = out.Defenses
		return state, nil
	}
}

// remedyAssessmentNode returns the remedy assessment Pregel node.
func remedyAssessmentNode(provider agentcore.Provider, frameworkProvider ArticleFrameworkProvider) graph.PregelNode {
	framework := defaultRemediesFramework
	if frameworkProvider != nil {
		framework = frameworkProvider.RemediesFramework()
	}
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		perspective, err := extractPerspective(state)
		if err != nil {
			return state, fmt.Errorf("remedy_assessment: %w", err)
		}
		verdict, _ := state[StateVerdict].(*InfringementVerdict)
		if verdict == nil {
			return state, fmt.Errorf("remedy_assessment: verdict not available")
		}

		patenteePrompt := "从专利权人角度：构建最大化赔偿模型，论证禁令必要性。"
		defendantPrompt := "从被控侵权人角度：量化最大风险敞口，论证专利贡献率分割以减少赔偿基数。"

		prompt := buildPerspectivePrompt(framework, patenteePrompt, defendantPrompt, perspective)
		prompt += fmt.Sprintf("\n\n- 侵权结论: %s (%.0f%%)\n- 风险等级: %s\n\n请给出损害赔偿估算、临时/永久禁令分析和惩罚性赔偿风险评估。",
			verdict.Conclusion, verdict.Likelihood*100, verdict.RiskLevel)

		agent := newInfringementAgent(provider, "infringement-remedy", prompt, remedySchema())
		defer agent.Close()

		output, err := agent.Run(ctx, "{}")
		if err != nil {
			return state, fmt.Errorf("remedy_assessment: %w", err)
		}

		var out RemedyResult
		if err := json.Unmarshal([]byte(output), &out); err != nil {
			return state, fmt.Errorf("remedy_assessment parse: %w", err)
		}
		state[StateRemedyAssessment] = &out
		return state, nil
	}
}

// strategyNode returns the litigation strategy Pregel node.
func strategyNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		perspective, err := extractPerspective(state)
		if err != nil {
			return state, fmt.Errorf("strategy: %w", err)
		}
		verdict, _ := state[StateVerdict].(*InfringementVerdict)
		if verdict == nil {
			return state, fmt.Errorf("strategy: verdict not available")
		}

		prompt := fmt.Sprintf("## 诉讼策略建议\n\n侵权结论: %s (%.0f%%), 风险: %s\n\n", verdict.Conclusion, verdict.Likelihood*100, verdict.RiskLevel)

		if perspective == PerspectivePatentee {
			prompt += "作为专利权人的策略顾问：给出证据保全、临时禁令时机、管辖选择、损害赔偿举证策略。"
		} else {
			prompt += "作为被控侵权人的策略顾问：给出无效宣告路径、诉讼中止策略、现有技术检索方向、和解建议区间。"
		}

		agent := newInfringementAgent(provider, "infringement-strategy", prompt, strategySchema())
		defer agent.Close()

		output, err := agent.Run(ctx, "{}")
		if err != nil {
			return state, fmt.Errorf("strategy: %w", err)
		}

		var out StrategyResult
		if err := json.Unmarshal([]byte(output), &out); err != nil {
			return state, fmt.Errorf("strategy parse: %w", err)
		}
		state[StateStrategy] = &out
		return state, nil
	}
}
