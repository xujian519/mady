package disclosure

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// extract schema field and value constants.
const (
	fieldText          = "text"
	jsNumber           = "number"
	enumKnown          = "known"
	promptTitleRequire = "要求："
)

// extractionType 区分三个提取 Agent。
type extractionType string

const (
	extractProblems extractionType = "problems"
	extractFeatures extractionType = "features"
	extractEffects  extractionType = "effects"
)

// newExtractionAgent 创建提取 Agent 的 PregelNode 工厂。
// extType 决定 SystemPrompt 和 JSON Schema ResponseFormat。
// stateKey 决定 Agent 的原始输出写入哪个 PregelState 键。
func newExtractionAgent(provider agentcore.Provider, extType extractionType, stateKey string) graph.PregelNode {
	cfg := buildExtractionConfig(provider, extType)
	return pregelAgentNode(cfg, stateKey)
}

// buildExtractionConfig 构建单个提取 Agent 的配置。
// 使用小非零 Temperature 以在一致性重试时产生变化，而非固定的温度 0 导致重试无效。
func buildExtractionConfig(provider agentcore.Provider, extType extractionType) agentcore.Config {
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:        "disclosure-extract-" + string(extType),
			Model:       modelDefault,
			Provider:    provider,
			Temperature: 0.3, // 非零温度允许重试时产生不同输出
		},
		SystemPrompt: buildExtractionPrompt(extType),
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 1,
		},
	}

	if schema := buildExtractionSchema(extType); schema != nil && supportsJSONSchemaResponseFormat() {
		cfg.ResponseFormat = agentcore.NewJSONSchemaResponseFormat(
			"disclosure_extract_"+string(extType), schema,
		)
	}

	return cfg
}

// buildExtractionPrompt 根据提取类型构造 SystemPrompt。
func buildExtractionPrompt(extType extractionType) string {
	base := strings.Join([]string{
		"你是一名资深专利代理师，负责分析技术交底书。",
		"请从交底书中精确提取指定要素，严格按 JSON Schema 输出。",
		"",
	}, "\n")

	switch extType {
	case extractProblems:
		return base + strings.Join([]string{
			"【任务】提取所有需要解决的技术问题。",
			promptTitleRequire,
			"  1. 每条技术问题用简洁的一句话描述",
			"  2. 技术问题应从「背景技术」或「要解决的技术问题」章节提取",
			"  3. 按问题的重要性排序",
		}, "\n")

	case extractFeatures:
		return base + strings.Join([]string{
			"【任务】提取所有技术特征，按「最小技术单元」粒度拆分。",
			promptTitleRequire,
			"  1. 每个特征是不可再分的原子技术手段",
			"  2. 分类为：structure（结构）/ method（方法/工艺）/ parameter（参数）/ material（材料）",
			"  3. 标注该特征在现有技术中是否为已知：known / unknown / partial",
			"  4. 标注重要性：high / medium / low",
			"  5. 如果特征解决了某个技术问题，关联对应的 problem_id",
		}, "\n")

	case extractEffects:
		return base + strings.Join([]string{
			"【任务】提取所有有益技术效果。",
			promptTitleRequire,
			"  1. 每条技术效果用一句话描述",
			"  2. 技术效果应从「有益效果」或「发明内容」章节提取",
			"  3. 按效果的直接性和重要性排序",
		}, "\n")
	}

	return base
}

// buildExtractionSchema 根据提取类型构造 JSON Schema。
// 返回 map[string]any 用于 agentcore.NewJSONSchemaResponseFormat。
// 所有 object 级别均包含 additionalProperties: false 以满足 strict 模式要求。
func buildExtractionSchema(extType extractionType) map[string]any {
	switch extType {
	case extractProblems:
		return map[string]any{
			jsType: jsObject,
			jsProperties: map[string]any{
				"problems": map[string]any{
					jsType: jsArray,
					jsItems: map[string]any{
						jsType: jsObject,
						jsProperties: map[string]any{
							"id":                   map[string]any{jsType: jsString},
							fieldText:              map[string]any{jsType: jsString},
							noveltyFieldConfidence: map[string]any{jsType: jsNumber},
						},
						jsRequired:             []string{"id", fieldText, noveltyFieldConfidence},
						jsAdditionalProperties: false,
					},
				},
			},
			jsRequired:             []string{"problems"},
			jsAdditionalProperties: false,
		}

	case extractFeatures:
		return map[string]any{
			jsType: jsObject,
			jsProperties: map[string]any{
				"features": map[string]any{
					jsType: jsArray,
					jsItems: map[string]any{
						jsType: jsObject,
						jsProperties: map[string]any{
							"id":          map[string]any{jsType: jsString},
							jsDescription: map[string]any{jsType: jsString},
							"category": map[string]any{
								jsType: jsString,
								jsEnum: []string{"structure", "method", "parameter", "material"},
							},
							"function": map[string]any{jsType: jsString},
							"prior_art_status": map[string]any{
								jsType: jsString,
								jsEnum: []string{enumKnown, "unknown", coveragePartial},
							},
							"importance": map[string]any{
								jsType: jsString,
								jsEnum: []string{confHigh, confMedium, confLow},
							},
							noveltyFieldConfidence: map[string]any{jsType: jsNumber},
							"solves": map[string]any{
								jsType:  jsArray,
								jsItems: map[string]any{jsType: jsString},
							},
						},
						jsRequired:             []string{"id", jsDescription, "category"},
						jsAdditionalProperties: false,
					},
				},
			},
			jsRequired:             []string{"features"},
			jsAdditionalProperties: false,
		}

	case extractEffects:
		return map[string]any{
			jsType: jsObject,
			jsProperties: map[string]any{
				"effects": map[string]any{
					jsType: jsArray,
					jsItems: map[string]any{
						jsType: jsObject,
						jsProperties: map[string]any{
							"id":      map[string]any{jsType: jsString},
							fieldText: map[string]any{jsType: jsString},
							"from": map[string]any{
								jsType:  jsArray,
								jsItems: map[string]any{jsType: jsString},
							},
							noveltyFieldConfidence: map[string]any{jsType: jsNumber},
						},
						jsRequired:             []string{"id", fieldText, noveltyFieldConfidence},
						jsAdditionalProperties: false,
					},
				},
			},
			jsRequired:             []string{"effects"},
			jsAdditionalProperties: false,
		}
	}

	return nil
}
