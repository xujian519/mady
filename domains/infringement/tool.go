package infringement

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// ToolName is the agentcore.Tool name for infringement analysis.
const ToolName = "evaluate_infringement"

// NewInfringementTool creates the agentcore.Tool that wraps the Pregel sub-graph.
// This replaces the old workflows/patent infringement tool.
func NewInfringementTool(
	provider agentcore.Provider,
	frameworkProvider ArticleFrameworkProvider,
	knowledgeRetriever KnowledgeRetriever,
) (*agentcore.Tool, error) {
	compiledGraph, err := BuildGraph(provider, frameworkProvider)
	if err != nil {
		return nil, fmt.Errorf("infringement: build graph: %w", err)
	}

	return &agentcore.Tool{
		Name:        ToolName,
		Description: "专利侵权判定分析——从原告或被告视角，评估专利权有效性、侵权成立可能性、抗辩可行性、损害赔偿和诉讼策略。支持全面覆盖原则、等同原则、禁止反悔原则、捐献规则、现有技术抗辩、先用权抗辩、合法来源抗辩等全流程分析。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patent_claims":       map[string]any{"type": "string", "description": "专利权利要求书文本"},
				"patent_spec":         map[string]any{"type": "string", "description": "专利说明书文本（可选）"},
				"prosecution_history": map[string]any{"type": "string", "description": "审查历史/案卷（可选）"},
				"accused_product":     map[string]any{"type": "string", "description": "被控侵权产品/方法的技术描述"},
				"perspective":         map[string]any{"type": "string", "enum": []string{"patentee", "defendant"}, "description": "分析视角：patentee=专利权人/原告，defendant=被控侵权人/被告"},
				"patent_type":         map[string]any{"type": "string", "enum": []string{"invention", "utility_model"}, "description": "专利类型：invention=发明，utility_model=实用新型"},
				"prior_art_refs":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "已知现有技术文献引用（可选）"},
			},
			"required": []string{"patent_claims", "accused_product", "perspective", "patent_type"},
		},
		ReadOnly: true,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var raw map[string]any
			if err := json.Unmarshal(args, &raw); err != nil {
				return nil, fmt.Errorf("infringement: parse args: %w", err)
			}

			input := &InfringementInput{
				PatentClaims:       getString(raw, "patent_claims"),
				PatentSpec:         getString(raw, "patent_spec"),
				ProsecutionHistory: getString(raw, "prosecution_history"),
				AccusedProduct:     getString(raw, "accused_product"),
				Perspective:        Perspective(getString(raw, "perspective")),
				PatentType:         PatentType(getString(raw, "patent_type")),
				PriorArtRefs:       getStringSlice(raw, "prior_art_refs"),
			}

			if knowledgeRetriever != nil {
				guidelines, _ := knowledgeRetriever.SearchGuidelines(ctx, "专利侵权判定 全面覆盖原则 等同原则")
				input.GuidelineRefs = guidelines
				cases, _ := knowledgeRetriever.SearchSimilarCases(ctx, input.PatentClaims)
				input.SimilarCases = cases
			}

			initialState := graph.PregelState{
				StateInput:       input,
				StatePerspective: input.Perspective,
			}
			resultState, err := compiledGraph.Run(ctx, initialState)
			if err != nil {
				return nil, fmt.Errorf("infringement graph execution: %w", err)
			}

			output, ok := resultState[StateOutput].(*InfringementOutput)
			if !ok {
				return nil, fmt.Errorf("infringement: unexpected output type")
			}

			return output, nil
		},
	}, nil
}

func getString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func getStringSlice(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
