package disclosure

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/prompt"
)

// JSON Schema field constants used across novelty schema construction.
const (
	jsType                 = "type"
	jsString               = "string"
	jsObject               = "object"
	jsArray                = "array"
	jsProperties           = "properties"
	jsDescription          = "description"
	jsRequired             = "required"
	jsEnum                 = "enum"
	jsItems                = "items"
	jsAdditionalProperties = "additionalProperties"
)

// novelty schema field name constants.
const (
	noveltyFieldFeatureID          = "feature_id"
	noveltyFieldNoveltyStatus      = "novelty_status"
	noveltyFieldConfidence         = "confidence"
	noveltyFieldReasoning          = "reasoning"
	noveltyFieldSimilarPriorArt    = "similar_prior_art"
	noveltyFieldCitedEvidenceIDs   = "cited_evidence_ids"
	noveltyFieldConclusion         = "conclusion"
	noveltyFieldFeatureAssessments = "feature_assessments"
	noveltyFieldOverallConfidence  = "overall_confidence"
)

// novelty status value constants.
const (
	noveltyStatusLikelyNovel   = "likely_novel"
	noveltyStatusPossiblyKnown = "possibly_known"
	noveltyStatusLikelyKnown   = "likely_known"
	noveltyStatusUnclear       = "unclear"
)

// confidence level constants.
const (
	confHigh   = "high"
	confMedium = "medium"
	confLow    = "low"
)

// model config constants.
const modelDefault = "default"

// coverage status constants.
const (
	coverageNone    = "none"
	coveragePartial = "partial"
	coverageFull    = "full"
)

// getMaxParallelism 返回 per-feature LLM 调用的最大并行度。
// 环境变量 MADY_NOVELTY_PARALLEL 可覆盖默认值 3。
func getMaxParallelism() int {
	v := os.Getenv("MADY_NOVELTY_PARALLEL")
	if v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 3 // 默认 3 并发，可由环境变量控制
}

// noveltyNode 返回新颖性初判的 Pregel 节点。
// 基于提取结果和关键词，使用 LLM 逐特征分析新颖性。
// 改造后优先使用 per-feature 评估模式，每个特征独立匹配最相关证据后调用 LLM；
// 不支持 JSON Schema ResponseFormat 时回退为 batch 模式。
func noveltyNode(provider agentcore.Provider) graph.PregelNode {
	batchCfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:        "disclosure-novelty",
			Model:       modelDefault,
			Provider:    provider,
			Temperature: 0.2,
		},
		SystemPrompt: prompt.ResolveSystemPromptOr("prompt://disclosure-novelty-analysis", noveltyAnalysisSystemPromptFallback),
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          1,
			ValidateArguments: true,
		},
	}
	perFeatureCfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:        "disclosure-novelty-per-feature",
			Model:       modelDefault,
			Provider:    provider,
			Temperature: 0.2,
		},
		SystemPrompt: prompt.ResolveSystemPromptOr("prompt://disclosure-novelty-per-feature", noveltyPerFeatureSystemPromptFallback),
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          1,
			ValidateArguments: true,
		},
	}
	usePerFeature := supportsJSONSchemaResponseFormat()
	if usePerFeature {
		batchCfg.ResponseFormat = agentcore.NewJSONSchemaResponseFormat("novelty_assessment", noveltySchema())
		perFeatureCfg.ResponseFormat = agentcore.NewJSONSchemaResponseFormat("per_feature_assessment", perFeatureSchema())
	}

	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		ext, ok := GetExtraction(state)
		if !ok || len(ext.Features) == 0 {
			state[StateKeyNovelty] = &NoveltyResult{
				Assessed:   false,
				Conclusion: "未提取到技术特征，无法进行新颖性初判",
				Notes:      "请确认交底书内容完整性后重新分析。",
			}
			return state, nil
		}

		evidence, _ := GetEvidence(state)

		if usePerFeature && len(evidence) > 0 {
			return runPerFeatureNovelty(ctx, state, perFeatureCfg, ext, evidence)
		}

		// 回退 batch 模式：无证据或模型不支持 JSON Schema 时一次性评估
		return runBatchNovelty(ctx, state, batchCfg)
	}
}

// runPerFeatureNovelty 逐特征进行新颖性评估。
// 每个特征独立匹配 Top-3 最相关证据，并发调用 LLM。
func runPerFeatureNovelty(ctx context.Context, state graph.PregelState,
	cfg agentcore.Config, ext *ExtractionResult, evidence []EvidenceChunk,
) (graph.PregelState, error) {
	maxParallel := getMaxParallelism()
	sem := make(chan struct{}, maxParallel)

	type featureResult struct {
		Assessment FeatureAssessment
		Err        error
	}

	results := make([]featureResult, len(ext.Features))

	for i := range ext.Features {
		sem <- struct{}{}
		go func(idx int, feature TechFeature) {
			defer func() { <-sem }()
			select {
			case <-ctx.Done():
				results[idx] = featureResult{Err: ctx.Err()}
				return
			default:
			}
			topK := selectTopEvidence(feature, evidence, 3)
			input := buildNoveltyInputPerFeature(state, feature, topK)

			agent := agentcore.New(cfg)
			defer agent.Close()

			output, err := agent.Run(ctx, input)
			if err != nil {
				results[idx] = featureResult{Err: err}
				return
			}

			assessment := parsePerFeatureOutput(output, feature.ID)
			assessment = validateCitedEvidenceIDs(assessment, evidence)
			results[idx] = featureResult{Assessment: assessment}
		}(i, ext.Features[i])
	}
	// Drain semaphore
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}

	var assessments []FeatureAssessment
	var errs []string
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, r.Err.Error())
			continue
		}
		assessments = append(assessments, r.Assessment)
	}

	if len(assessments) == 0 && len(errs) > 0 {
		fallback := assessNoveltyFromState(state)
		fallback.Notes += "\n\n【注意】逐特征评估全部失败，使用启发式回退：" + strings.Join(errs, "; ")
		state[StateKeyNovelty] = fallback
		return state, nil
	}

	result := aggregateNoveltyResult(assessments, state)
	if len(errs) > 0 {
		result.Notes += "\n\n【部分特征评估失败】" + strings.Join(errs, "; ")
	}
	state[StateKeyNovelty] = result
	return state, nil
}

// runBatchNovelty 一次性评估所有特征（原始方式，作为回退）。
func runBatchNovelty(ctx context.Context, state graph.PregelState,
	cfg agentcore.Config) (graph.PregelState, error) {
	input := buildNoveltyInput(state)
	if input == "" {
		state[StateKeyNovelty] = &NoveltyResult{
			Assessed:   false,
			Conclusion: "未提取到技术特征，无法进行新颖性初判",
			Notes:      "请确认交底书内容完整性后重新分析。",
		}
		return state, nil
	}

	agent := agentcore.New(cfg)
	defer agent.Close()

	output, err := agent.Run(ctx, input)
	if err != nil {
		fallback := assessNoveltyFromState(state)
		fallback.Notes += "\n\n【注意】LLM 评估失败，使用启发式回退：" + err.Error()
		state[StateKeyNovelty] = fallback
		return state, nil
	}

	result := parseNoveltyOutput(output, state)
	state[StateKeyNovelty] = result
	return state, nil
}
