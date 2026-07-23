package infringement

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// BuildGraph constructs and compiles the 10-node infringement analysis Pregel graph.
//
// Node topology:
//
//	load_input → claim_scope → feature_decomposition → literal_infringement
//	  → equivalence → infringement_verdict → defense_review
//	  → remedy_assessment → strategy → conclude → __end__
func BuildGraph(
	provider agentcore.Provider,
	frameworkProvider ArticleFrameworkProvider,
) (*graph.CompiledPregelGraph, error) {
	if provider == nil {
		return nil, fmt.Errorf("infringement: provider is required")
	}

	pg := graph.NewPregelGraph()

	nodes := map[string]graph.PregelNode{
		"load_input":            loadInputNode(),
		"claim_scope":           claimScopeNode(provider, frameworkProvider),
		"feature_decomposition": featureDecompositionNode(provider),
		"literal_infringement":  literalInfringementNode(provider),
		"equivalence":           equivalenceNode(provider, frameworkProvider),
		"infringement_verdict":  infringementVerdictNode(provider),
		"defense_review":        defenseReviewNode(provider, frameworkProvider),
		"remedy_assessment":     remedyAssessmentNode(provider, frameworkProvider),
		"strategy":              strategyNode(provider),
		"conclude":              concludeNode(),
	}

	for name, node := range nodes {
		if err := pg.AddNode(name, node); err != nil {
			return nil, fmt.Errorf("infringement: add node %q: %w", name, err)
		}
	}

	edges := [][2]string{
		{"load_input", "claim_scope"},
		{"claim_scope", "feature_decomposition"},
		{"feature_decomposition", "literal_infringement"},
		{"literal_infringement", "equivalence"},
		{"equivalence", "infringement_verdict"},
		{"infringement_verdict", "defense_review"},
		{"defense_review", "remedy_assessment"},
		{"remedy_assessment", "strategy"},
		{"strategy", "conclude"},
		{"conclude", graph.PregelEnd},
	}
	for _, e := range edges {
		if err := pg.AddEdge(e[0], e[1]); err != nil {
			return nil, fmt.Errorf("infringement: add edge %q→%q: %w", e[0], e[1], err)
		}
	}

	return pg.Compile("load_input", 100)
}

func stateHasSkip(state graph.PregelState) bool {
	skipped, _ := state[StateSkipped].(bool)
	return skipped
}

func loadInputNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		input, ok := state[StateInput].(*InfringementInput)
		if !ok || input == nil {
			state[StateSkipped] = true
			state[StateOutput] = &InfringementOutput{
				Verdict:    InfringementVerdict{Conclusion: "error", KeyFindings: []string{"输入无效或为空"}},
				Disclaimer: "分析无法完成: 输入数据缺失",
			}
			return state, nil
		}
		if input.PatentClaims == "" || input.AccusedProduct == "" {
			state[StateSkipped] = true
			state[StateOutput] = &InfringementOutput{
				Verdict: InfringementVerdict{
					Conclusion:  "error",
					KeyFindings: []string{"权利要求文本和被控产品描述均为必填项"},
				},
				Disclaimer: "分析无法完成: 必要输入缺失",
			}
			return state, nil
		}
		state[StatePerspective] = input.Perspective
		return state, nil
	}
}

func concludeNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if _, exists := state[StateOutput]; exists {
			return state, nil
		}
		output := &InfringementOutput{
			Disclaimer: "本分析基于AI辅助生成，不构成法律意见。侵权判定应由具有管辖权的法院或专利管理部门最终确定。分析结果仅供参考。",
		}
		if v, ok := state[StateClaimScope].(*ClaimScopeResult); ok {
			output.ClaimScope = *v
		}
		if v, ok := state[StateFeatureMapping].([]FeatureComparison); ok {
			output.FeatureMapping = v
		}
		if v, ok := state[StateLiteralResult].(*LiteralResult); ok {
			output.LiteralResult = *v
		}
		if v, ok := state[StateEquivalenceResult].(*EquivalenceResult); ok {
			output.EquivalenceResult = *v
		}
		if v, ok := state[StateVerdict].(*InfringementVerdict); ok {
			output.Verdict = *v
		}
		if v, ok := state[StateDefenseAnalysis].([]DefenseAssessment); ok {
			output.DefenseAnalysis = v
		}
		if v, ok := state[StateRemedyAssessment].(*RemedyResult); ok {
			output.RemedyAssessment = *v
		}
		if v, ok := state[StateStrategy].(*StrategyResult); ok {
			output.StrategyAdvice = *v
		}
		scorer := NewInfringementScorer(nil)
		scores := scorer.Score(output)
		output.Confidence = scores.Composite
		output.Verdict.RiskLevel = scores.RiskLevel
		state[StateOutput] = output
		return state, nil
	}
}
