package claimdrafting

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/graph"
)

// stateHasSkip 检查 state 中是否包含跳过标记（输入校验失败导致不进入撰写流程）。
func stateHasSkip(state graph.PregelState) bool {
	if v, ok := state[StateKeyOutput]; ok {
		if s, ok := v.(*DraftOutput); ok && s != nil && s.Claims == nil && len(s.Warnings) > 0 {
			return true
		}
	}
	return false
}

// extractInput 从 state 中提取 DraftInput。
func extractInput(state graph.PregelState) *DraftInput {
	if v, ok := state[StateKeyInput]; ok {
		if input, ok := v.(*DraftInput); ok {
			return input
		}
	}
	return nil
}

// =============================================================================
// 节点实现
// =============================================================================

// loadInputNode 从 PregelState 读取输入并验证，自动推断技术领域和撰写策略。
func loadInputNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		raw, ok := state[StateKeyInput]
		if !ok {
			state[StateKeyOutput] = &DraftOutput{
				Warnings: []string{"未提供输入数据"},
			}
			return state, nil
		}

		input, ok := raw.(*DraftInput)
		if !ok || input == nil {
			state[StateKeyOutput] = &DraftOutput{
				Warnings: []string{"输入数据格式无效"},
			}
			return state, nil
		}

		if input.TechDomain == "" {
			input.TechDomain = classifyDomain(*input)
		}
		if input.Strategy == "" {
			input.Strategy = StrategyProductOnly
		}

		state[StateKeyInput] = input
		return state, nil
	}
}

// classifyFeaturesNode 将特征分类为必要特征和可选特征。
// 必要特征进入独立权利要求，可选特征进入从属权利要求（金字塔型布局）。
func classifyFeaturesNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		essential, optional := classifyFeatures(input.Features, input.PFETriples)

		if len(essential) == 0 && len(input.Features) > 0 {
			essential = make([]Feature, len(input.Features))
			copy(essential, input.Features)
			optional = nil
		}

		state[StateKeyEssential] = essential
		state[StateKeyOptional] = optional
		return state, nil
	}
}

// draftPrimaryNode 撰写主独立权利要求（前序部分 + 特征部分）。
func draftPrimaryNode(builder *ClaimBuilder) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}
		essential, _ := state[StateKeyEssential].([]Feature)

		primary, err := builder.buildIndependent(*input, input.TechDomain, essential)
		if err != nil {
			return nil, fmt.Errorf("撰写独立权利要求失败: %w", err)
		}

		state[StateKeyPrimary] = primary
		return state, nil
	}
}

// draftParallelNode 根据撰写策略生成并列独立权利要求。
// 策略为 product_only 时跳过（不设置 StateKeyParallels）。
func draftParallelNode(builder *ClaimBuilder) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}
		essential, _ := state[StateKeyEssential].([]Feature)
		primary, ok := state[StateKeyPrimary].(Claim)
		if !ok {
			return state, nil
		}

		switch input.Strategy {
		case StrategyProductOnly:
			return state, nil

		case StrategyProductAndMethod:
			var parallels []Claim
			methodClaim := builder.buildParallelMethod(*input, input.TechDomain, primary.Number, essential)
			if methodClaim != nil {
				parallels = append(parallels, *methodClaim)
				if input.TechDomain == DomainSoftware {
					if d := builder.buildSoftwareApparatus(*input, input.TechDomain, primary.Number, methodClaim, essential); d != nil {
						parallels = append(parallels, *d)
					}
				}
			}
			if len(parallels) > 0 {
				state[StateKeyParallels] = parallels
			}

		case StrategyProductAndManufacturing:
			if p := builder.buildParallelManufacturing(*input, input.TechDomain, primary.Number, essential); p != nil {
				state[StateKeyParallels] = []Claim{*p}
			}

		case StrategyProductAndUse:
			if p := builder.buildParallelUse(*input, input.TechDomain, primary.Number); p != nil {
				state[StateKeyParallels] = []Claim{*p}
			}

		case StrategyPharmaUse:
			if p := builder.buildPharmaUse(*input, input.TechDomain, primary.Number); p != nil {
				state[StateKeyParallels] = []Claim{*p}
			}

		case StrategyMarkush:
			if p := builder.buildMarkush(*input, input.TechDomain, primary.Number, essential); p != nil {
				primary.ClaimType = ClaimTypeProduct
				state[StateKeyPrimary] = primary
				state[StateKeyParallels] = []Claim{*p}
			}
		}

		return state, nil
	}
}

// draftDependentsNode 根据独立权利要求和可选特征构建从属权利要求（金字塔型布局）。
func draftDependentsNode(builder *ClaimBuilder) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}
		primary, ok := state[StateKeyPrimary].(Claim)
		if !ok {
			return state, nil
		}
		optional, _ := state[StateKeyOptional].([]Feature)

		indClaims := []Claim{primary}
		if parallels, ok := state[StateKeyParallels].([]Claim); ok {
			indClaims = append(indClaims, parallels...)
		}

		deps := builder.buildDependents(indClaims, *input, optional)
		state[StateKeyDependents] = deps
		return state, nil
	}
}

// collectAllClaims 从 state 收集所有权利要求（独立+从属，平面列表）。
func collectAllClaims(state graph.PregelState) []Claim {
	primary, _ := state[StateKeyPrimary].(Claim)
	parallels, _ := state[StateKeyParallels].([]Claim)
	dependents, _ := state[StateKeyDependents].([]Claim)

	var all []Claim
	all = append(all, primary)
	all = append(all, parallels...)
	all = append(all, dependents...)

	return all
}

// validateNode 运行规则引擎验证所有生成的 claims。
func validateNode(engine *RuleEngine) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil || engine == nil {
			return state, nil
		}

		allClaims := collectAllClaims(state)
		// 验证结果由 scoreNode 内部调用 engine.Validate 并存入 ScoreReport
		_ = engine.Validate(allClaims, *input)

		return state, nil
	}
}

// scoreNode 运行质量评分器对生成的 claims 进行多维度评分。
func scoreNode(scorer *ClaimScorer) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil || scorer == nil {
			return state, nil
		}

		allClaims := collectAllClaims(state)
		report := scorer.Score(allClaims, *input)
		state[StateKeyScore] = report
		return state, nil
	}
}

// finalizeNode 从 state 各 key 组装最终的 DraftOutput。
func finalizeNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		allClaims := collectAllClaims(state)
		if len(allClaims) == 0 {
			state[StateKeyOutput] = &DraftOutput{
				Warnings: []string{"未生成任何权利要求"},
			}
			return state, nil
		}

		claimSet := buildClaimSet(allClaims)

		output := &DraftOutput{
			Claims:    claimSet,
			Timestamp: timestamp(),
			InputMeta: struct {
				Domain       TechDomain `json:"domain"`
				ClaimType    ClaimType  `json:"claim_type"`
				FeatureCount int        `json:"feature_count"`
			}{
				Domain:       input.TechDomain,
				FeatureCount: len(input.Features),
			},
		}

		if len(allClaims) > 0 {
			output.InputMeta.ClaimType = allClaims[0].ClaimType
		}

		if report, ok := state[StateKeyScore].(*ScoreReport); ok && report != nil {
			output.Score = report.OverallScore
			for _, v := range report.Violations {
				if v.Severity == SeverityWarning || v.Severity == SeverityInfo {
					output.Warnings = append(output.Warnings, "["+v.RuleName+"] "+v.Message)
				}
			}
		}

		state[StateKeyOutput] = output
		return state, nil
	}
}

// buildClaimSet 将 claims 平面列表按类型拆分为 ClaimSet。
func buildClaimSet(allClaims []Claim) *ClaimSet {
	cs := &ClaimSet{}
	for _, c := range allClaims {
		if c.Kind == "dependent" {
			cs.DependentClaims = append(cs.DependentClaims, c)
		} else {
			cs.IndependentClaims = append(cs.IndependentClaims, c)
		}
	}
	return cs
}
