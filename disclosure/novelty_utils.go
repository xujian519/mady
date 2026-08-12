package disclosure

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/pkg/util"
)

// buildNoveltyInput 从 PregelState 构建新颖性分析的输入。
func buildNoveltyInput(state graph.PregelState) string {
	var sb strings.Builder

	if raw, ok := state[StateKeyExtraction]; ok {
		if ext, ok := raw.(*ExtractionResult); ok && ext != nil {
			fmt.Fprintf(&sb, "技术特征数量：%d\n\n", len(ext.Features))
			sb.WriteString("## 技术特征列表\n\n")
			for _, f := range ext.Features {
				fmt.Fprintf(&sb, "- ID: %s\n", f.ID)
				fmt.Fprintf(&sb, "  描述: %s\n", f.Description)
				fmt.Fprintf(&sb, "  分类: %s\n", f.Category)
				fmt.Fprintf(&sb, "  功能: %s\n", f.Function)
				fmt.Fprintf(&sb, "  现有技术状态: %s\n", f.PriorArtStatus)
				fmt.Fprintf(&sb, "  重要度: %s\n\n", f.Importance)
			}

			if len(ext.Problems) > 0 {
				sb.WriteString("## 要解决的技术问题\n\n")
				for _, p := range ext.Problems {
					fmt.Fprintf(&sb, "- %s\n", p)
				}
				sb.WriteString("\n")
			}

			if len(ext.Effects) > 0 {
				sb.WriteString("## 技术效果\n\n")
				for _, e := range ext.Effects {
					fmt.Fprintf(&sb, "- %s\n", e)
				}
				sb.WriteString("\n")
			}
		}
	}

	if kw, ok := state[StateKeySearchKeywords]; ok {
		if kwList, ok := kw.([]string); ok && len(kwList) > 0 {
			fmt.Fprintf(&sb, "## 检索关键词\n\n%s\n\n", strings.Join(kwList, "、"))
		}
	}

	// 注入 retrieve_prior_art 产出的现有技术证据，让 LLM 基于真实语料比对
	// 而非凭参数化知识猜测（对齐 design-prior-art-retrieval-stage.md 第3.3节）。
	if evidence := extractEvidenceForPrompt(state); evidence != "" {
		fmt.Fprintf(&sb, "## 现有技术证据（来自知识库检索）\n\n%s\n\n", evidence)
		sb.WriteString("**要求**：每个特征评估必须引用相关证据的 doc_id（填入 cited_evidence_ids），")
		sb.WriteString("无证据支撑的判断标注为「疑似」。引用不存在的 doc_id 视为不可信。\n\n")
	}

	return sb.String()
}

// =============================================================================
// 辅助函数
// =============================================================================

// selectTopEvidence 为单个特征选取最相关的 Top-K 条证据。
// 排序依据：EvidenceChunk.Score（检索相似度）。
func selectTopEvidence(_ TechFeature, evidence []EvidenceChunk, topK int) []EvidenceChunk {
	if len(evidence) == 0 || topK <= 0 {
		return nil
	}

	// 按 Score 降序排序
	sorted := make([]EvidenceChunk, len(evidence))
	copy(sorted, evidence)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })

	if len(sorted) > topK {
		sorted = sorted[:topK]
	}
	return sorted
}

// validateCitedEvidenceIDs 验证 LLM 引用的 doc_id 是否在真实证据中存在。
// 验证逻辑复用 evaluate/metrics.go EvidenceGroundedness 的离线验证思路。
func validateCitedEvidenceIDs(assessment FeatureAssessment, evidence []EvidenceChunk) FeatureAssessment {
	if len(assessment.CitedEvidenceIDs) == 0 {
		return assessment
	}

	validIDs := make(map[string]bool, len(evidence))
	for _, ev := range evidence {
		validIDs[ev.DocID] = true
	}

	var invalidCount int
	var verified []string
	for _, id := range assessment.CitedEvidenceIDs {
		if validIDs[id] {
			verified = append(verified, id)
		} else {
			invalidCount++
		}
	}
	assessment.CitedEvidenceIDs = verified

	if invalidCount > 0 {
		assessment.Reasoning += fmt.Sprintf(" [注：%d 个引用证据 ID 未在检索结果中找到，已剔除]", invalidCount)
		if assessment.Confidence == confHigh {
			assessment.Confidence = confMedium // 降置信度
		}
	}
	return assessment
}

// aggregateNoveltyResult 基于逐特征评估结果汇总结论。
func aggregateNoveltyResult(assessments []FeatureAssessment, state graph.PregelState) *NoveltyResult {
	var b strings.Builder
	b.WriteString("## 新颖性分析（逐特征 LLM 评估）\n\n")

	coverage, _ := state[StateKeyEvidenceCoverage].(string)
	switch coverage {
	case coverageNone:
		b.WriteString("⚠️ **未基于外部现有技术语料**：本次评估无可用证据，结论仅供参考，需人工核实。\n\n")
	case coveragePartial:
		b.WriteString("📎 **部分基于外部语料**：证据覆盖不完整，部分判断可能缺乏比对依据。\n\n")
	case coverageFull:
		b.WriteString("✅ **基于外部现有技术语料比对**\n\n")
	}

	novelCount, knownCount, unclearCount := 0, 0, 0
	for _, fa := range assessments {
		label := noveltyStatusLabels[fa.NoveltyStatus]
		if label == "" {
			label = fa.NoveltyStatus
		}
		fmt.Fprintf(&b, "- 特征 %s: **%s** (置信度: %s)\n", fa.FeatureID, label, fa.Confidence)
		b.WriteString("  " + fa.Reasoning + "\n")
		if fa.SimilarPriorArt != "" {
			b.WriteString("  相似现有技术: " + fa.SimilarPriorArt + "\n")
		}
		if len(fa.CitedEvidenceIDs) > 0 {
			fmt.Fprintf(&b, "  引用证据: %s\n", strings.Join(fa.CitedEvidenceIDs, ", "))
		}
		b.WriteString("\n")

		switch fa.NoveltyStatus {
		case noveltyStatusLikelyNovel:
			novelCount++
		case noveltyStatusPossiblyKnown, noveltyStatusLikelyKnown:
			knownCount++
		default:
			unclearCount++
		}
	}

	// 自动生成结论
	var conclusion string
	switch {
	case knownCount == 0 && novelCount > 0:
		conclusion = "全部特征可能具有新颖性，建议继续分析创造性。"
	case novelCount == 0 && knownCount > 0:
		conclusion = "全部特征可能属于现有技术，新颖性风险较高。"
	case knownCount > novelCount:
		conclusion = "大部分技术特征可能属于现有技术，新颖性存在风险。"
	case novelCount > knownCount:
		conclusion = "大部分技术特征可能具有新颖性，建议重点关注疑似已知的特征。"
	default:
		conclusion = "新颖性判断存在不确定性，建议人工复核。"
	}
	if unclearCount > 0 {
		conclusion += fmt.Sprintf(" 其中 %d 个特征判断不确定，需进一步分析。", unclearCount)
	}
	b.WriteString("\n**结论**: " + conclusion + "\n")

	// 整体置信度取最低
	overallConf := confHigh
	for _, fa := range assessments {
		switch fa.Confidence {
		case confLow:
			overallConf = confLow
		case confMedium:
			if overallConf != confLow {
				overallConf = confMedium
			}
		default:
			if overallConf == confHigh {
				overallConf = confMedium
			}
		}
	}
	fmt.Fprintf(&b, "\n**整体置信度**: %s\n", overallConf)
	b.WriteString("\n**注意：** 本评估为 AI 辅助预分析，不构成正式新颖性判断。")

	return &NoveltyResult{
		Assessed:           true,
		Conclusion:         conclusion,
		Notes:              b.String(),
		FeatureAssessments: assessments,
	}
}

// =============================================================================
// Per-Feature 模式：Prompt、Schema、输入构建、解析
// =============================================================================

// perFeatureSchema 返回单个特征新颖性评估的 JSON Schema。
var perFeatureSchemaCache map[string]any
var perFeatureSchemaOnce sync.Once

func perFeatureSchema() map[string]any {
	perFeatureSchemaOnce.Do(func() {
		perFeatureSchemaCache = map[string]any{
			jsType: jsObject,
			jsProperties: map[string]any{
				noveltyFieldFeatureID:     map[string]any{jsType: jsString},
				noveltyFieldNoveltyStatus: map[string]any{jsType: jsString, jsEnum: []string{noveltyStatusLikelyNovel, noveltyStatusPossiblyKnown, noveltyStatusLikelyKnown, noveltyStatusUnclear}},
				noveltyFieldConfidence:    map[string]any{jsType: jsString, jsEnum: []string{confHigh, confMedium, confLow}},
				noveltyFieldReasoning:     map[string]any{jsType: jsString},
				noveltyFieldSimilarPriorArt: map[string]any{
					jsType:        jsString,
					jsDescription: "相似现有技术描述（如有）",
				},
				noveltyFieldCitedEvidenceIDs: map[string]any{
					jsType:        jsArray,
					jsItems:       map[string]any{jsType: jsString},
					jsDescription: "引用的证据 doc_id（来自提供的证据列表）",
				},
			},
			jsRequired:             []string{noveltyFieldFeatureID, noveltyFieldNoveltyStatus, noveltyFieldConfidence, noveltyFieldReasoning},
			jsAdditionalProperties: false,
		}
	})
	return perFeatureSchemaCache
}

// buildNoveltyInputPerFeature 为单个特征构建包含 Top-K 最相关证据的 prompt 输入。
func buildNoveltyInputPerFeature(_ graph.PregelState, feature TechFeature, topK []EvidenceChunk) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "## 技术特征\n\n")
	fmt.Fprintf(&sb, "ID: %s\n", feature.ID)
	fmt.Fprintf(&sb, "描述: %s\n", feature.Description)
	fmt.Fprintf(&sb, "分类: %s\n", feature.Category)
	if feature.Function != "" {
		fmt.Fprintf(&sb, "功能: %s\n", feature.Function)
	}
	fmt.Fprintf(&sb, "现有技术状态: %s\n\n", feature.PriorArtStatus)

	if len(topK) > 0 {
		sb.WriteString("## 相关现有技术证据\n\n")
		for i, c := range topK {
			fmt.Fprintf(&sb, "[%d] doc_id: %s\n", i+1, c.DocID)
			if c.Title != "" {
				fmt.Fprintf(&sb, "    标题: %s\n", c.Title)
			}
			fmt.Fprintf(&sb, "    原文: %s\n", c.Snippet)
			fmt.Fprintf(&sb, "    相似度: %.2f\n\n", c.Score)
		}
		sb.WriteString("**要求**：引用证据时填入对应的 doc_id。无证据支撑的判断标注 low 置信度。\n")
	} else {
		sb.WriteString("## 现有技术证据\n\n（无可用证据——无法基于外部语料判断，请在结论中注明此限制）\n")
	}

	return sb.String()
}

// parsePerFeatureOutput 解析单个特征新颖性评估的 LLM JSON 输出。
func parsePerFeatureOutput(output string, featureID string) FeatureAssessment {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return FeatureAssessment{
			FeatureID:     featureID,
			NoveltyStatus: noveltyStatusUnclear,
			Confidence:    confLow,
			Reasoning:     "LLM 输出解析失败，无法判断",
		}
	}

	var parsed struct {
		FeatureID        string   `json:"feature_id"`
		NoveltyStatus    string   `json:"novelty_status"`
		Confidence       string   `json:"confidence"`
		Reasoning        string   `json:"reasoning"`
		SimilarPriorArt  string   `json:"similar_prior_art,omitempty"`
		CitedEvidenceIDs []string `json:"cited_evidence_ids,omitempty"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return FeatureAssessment{
			FeatureID:     featureID,
			NoveltyStatus: noveltyStatusUnclear,
			Confidence:    confLow,
			Reasoning:     fmt.Sprintf("JSON 解析错误: %v", err),
		}
	}

	// 保证 feature_id 匹配
	if parsed.FeatureID == "" {
		parsed.FeatureID = featureID
	}
	if parsed.NoveltyStatus == "" {
		parsed.NoveltyStatus = noveltyStatusUnclear
	}
	if parsed.Confidence == "" {
		parsed.Confidence = confLow
	}

	return FeatureAssessment{
		FeatureID:        parsed.FeatureID,
		NoveltyStatus:    parsed.NoveltyStatus,
		Confidence:       parsed.Confidence,
		Reasoning:        parsed.Reasoning,
		SimilarPriorArt:  parsed.SimilarPriorArt,
		CitedEvidenceIDs: parsed.CitedEvidenceIDs,
	}
}

// extractEvidenceForPrompt 格式化证据片段供 LLM prompt 使用，并标注覆盖率
// 以便 LLM 在无证据时明确说明"无法基于外部语料判断"。
func extractEvidenceForPrompt(state graph.PregelState) string {
	coverage, _ := state[StateKeyEvidenceCoverage].(string)
	if coverage == coverageNone {
		return "（无可用现有技术证据——无法基于外部语料判断，请在结论中说明此限制）"
	}
	raw, ok := state[StateKeyEvidence]
	if !ok {
		return ""
	}
	chunks, ok := raw.([]EvidenceChunk)
	if !ok || len(chunks) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, c := range chunks {
		fmt.Fprintf(&sb, "[%d] doc_id: %s\n", i+1, c.DocID)
		if c.Title != "" {
			fmt.Fprintf(&sb, "    标题: %s\n", c.Title)
		}
		fmt.Fprintf(&sb, "    原文: %s\n", c.Snippet)
		fmt.Fprintf(&sb, "    相似度: %.2f\n\n", c.Score)
	}
	return sb.String()
}

// noveltySchema 返回新颖性分析的 JSON Schema。
var noveltySchemaCache map[string]any
var noveltySchemaOnce sync.Once

func noveltySchema() map[string]any {
	noveltySchemaOnce.Do(func() {
		noveltySchemaCache = map[string]any{
			jsType: jsObject,
			jsProperties: map[string]any{
				noveltyFieldConclusion: map[string]any{
					jsType:        jsString,
					jsDescription: "整体新颖性判断结论",
				},
				noveltyFieldFeatureAssessments: map[string]any{
					jsType: jsArray,
					jsItems: map[string]any{
						jsType: jsObject,
						jsProperties: map[string]any{
							noveltyFieldFeatureID:       map[string]any{jsType: jsString},
							noveltyFieldNoveltyStatus:   map[string]any{jsType: jsString, jsEnum: []string{noveltyStatusLikelyNovel, noveltyStatusPossiblyKnown, noveltyStatusLikelyKnown, noveltyStatusUnclear}},
							noveltyFieldConfidence:      map[string]any{jsType: jsString, jsEnum: []string{confHigh, confMedium, confLow}},
							noveltyFieldReasoning:       map[string]any{jsType: jsString},
							noveltyFieldSimilarPriorArt: map[string]any{jsType: jsString},
							noveltyFieldCitedEvidenceIDs: map[string]any{
								jsType:        jsArray,
								jsItems:       map[string]any{jsType: jsString},
								jsDescription: "引用的现有技术证据 doc_id 列表（来自 prompt 中的证据列表）。无证据时为空数组。",
							},
						},
						jsRequired: []string{noveltyFieldFeatureID, noveltyFieldNoveltyStatus, noveltyFieldConfidence, noveltyFieldReasoning},
					},
				},
				noveltyFieldOverallConfidence: map[string]any{
					jsType: jsString,
					jsEnum: []string{confHigh, confMedium, confLow},
				},
			},
			jsRequired: []string{noveltyFieldConclusion, noveltyFieldFeatureAssessments, noveltyFieldOverallConfidence},
		}
	})
	return noveltySchemaCache
}

var noveltyStatusLabels = map[string]string{
	"likely_novel":   "可能具有新颖性",
	"possibly_known": "可能属于现有技术",
	"likely_known":   "很可能属于现有技术",
	"unclear":        "不确定",
}

type noveltyOutput struct {
	Conclusion         string              `json:"conclusion"`
	FeatureAssessments []FeatureAssessment `json:"feature_assessments"`
	OverallConfidence  string              `json:"overall_confidence"`
}

// parseNoveltyOutput 解析 LLM Batch 模式的 JSON 输出为 NoveltyResult。
// 除渲染为 Notes 文本外，同时填充 FeatureAssessments 结构化字段。
func parseNoveltyOutput(output string, state graph.PregelState) *NoveltyResult {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return &NoveltyResult{
			Assessed:   true,
			Conclusion: "LLM 输出解析失败",
			Notes:      "原始输出：\n" + output,
		}
	}

	var parsed noveltyOutput
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return &NoveltyResult{
			Assessed:   true,
			Conclusion: "LLM 输出解析失败",
			Notes:      fmt.Sprintf("JSON 解析错误: %v\n原始输出：\n%s", err, output),
		}
	}

	var b strings.Builder
	b.WriteString("## 新颖性分析（LLM 评估）\n\n")

	// 反映证据覆盖状态，让人工审阅者知道判断是否基于真实语料。
	coverage, _ := state[StateKeyEvidenceCoverage].(string)
	switch coverage {
	case coverageNone:
		b.WriteString("⚠️ **未基于外部现有技术语料**：本次评估无可用证据，结论仅供参考，需人工核实。\n\n")
	case coveragePartial:
		b.WriteString("📎 **部分基于外部语料**：证据覆盖不完整，部分判断可能缺乏比对依据。\n\n")
	case coverageFull:
		b.WriteString("✅ **基于外部现有技术语料比对**\n\n")
	}

	for _, fa := range parsed.FeatureAssessments {
		label := noveltyStatusLabels[fa.NoveltyStatus]
		if label == "" {
			label = fa.NoveltyStatus
		}
		fmt.Fprintf(&b, "- 特征 %s: **%s** (置信度: %s)\n", fa.FeatureID, label, fa.Confidence)
		b.WriteString("  " + fa.Reasoning + "\n")
		if fa.SimilarPriorArt != "" {
			b.WriteString("  相似现有技术: " + fa.SimilarPriorArt + "\n")
		}
		if len(fa.CitedEvidenceIDs) > 0 {
			fmt.Fprintf(&b, "  引用证据: %s\n", strings.Join(fa.CitedEvidenceIDs, ", "))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\n**整体置信度**: %s\n", parsed.OverallConfidence)
	b.WriteString("\n**注意：** 本评估为 AI 辅助预分析，不构成正式新颖性判断。")

	return &NoveltyResult{
		Assessed:           true,
		Conclusion:         parsed.Conclusion,
		Notes:              b.String(),
		FeatureAssessments: parsed.FeatureAssessments,
	}
}

// extractJSON 从文本中提取第一个 JSON 对象。
// 优先尝试从 markdown 代码块 ```json ... ``` 内提取，失败后再回退到
// 首 { 到末 } 的简单匹配（避免 LLM 输出中的花括号导致捕获非 JSON 内容）。
func extractJSON(text string) string {
	return util.ExtractJSON(text)
}
