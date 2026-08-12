package evidence

import (
	"fmt"
	"strings"
	"time"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

// Commonly repeated string constants.
const (
	judgmentRelevance       = "relevance"
	judgmentCritical        = "critical"
	judgmentAuthenticity    = "authenticity"
	burdenClaimant          = "claimant"
	evUnknown               = "unknown"
	cnPreponderance         = "优势证据"
	cnHighProbability       = "高度盖然性"
	standardClearConvincing = "clear_and_convincing"
	judgmentLegality        = "legality"
	judgmentLevelHigh       = "high"
	judgmentLevelMediumHigh = "medium_high"
	judgmentLevelLow        = "low"
)

var _ EvidenceJudgmentEngine = (*DefaultEngine)(nil)

// DefaultEngine 使用 RuleIndex 的默认证据判断引擎。
type DefaultEngine struct {
	index *RuleIndex
}

// NewEngine 创建证据判断引擎。如果 index 为 nil，自动创建新索引。
func NewEngine(index *RuleIndex) *DefaultEngine {
	if index == nil {
		index = NewRuleIndex()
	}
	return &DefaultEngine{index: index}
}

// Judge 对单条证据进行判断。
func (e *DefaultEngine) Judge(span agentcore_evidence.EvidenceSpan) (*EvidenceJudgment, error) {
	if span.ID == "" {
		return nil, fmt.Errorf("证据跨度缺少 ID")
	}
	evType := inferEvidenceType(span.SourceURI)
	judgment := &EvidenceJudgment{
		SpanID:      span.ID,
		EvaluatedAt: time.Now(),
		Confidence:  1.0,
	}

	e.evaluateTripleAttributes(span, judgment)
	e.evaluateTypeSpecific(span, evType, judgment)
	judgment.OverallScore = e.computeOverallScore(judgment)
	judgment.Reasoning = e.buildReasoning(judgment, evType)
	return judgment, nil
}

// evaluateTripleAttributes 对证据三性逐项评分并填入 judgment。
func (e *DefaultEngine) evaluateTripleAttributes(span agentcore_evidence.EvidenceSpan, judgment *EvidenceJudgment) {
	judgment.RelevanceJudgment = evaluateRelevance(span)
	judgment.LegalityJudgment = evaluateLegality(span)
	judgment.AuthenticityJudgment = evaluateAuthenticity(span)

	// 标记已发现的问题
	var issues []JudgmentIssue
	if judgment.RelevanceJudgment != nil && judgment.RelevanceJudgment.Score < 0.5 {
		issues = append(issues, JudgmentIssue{Type: judgmentRelevance, Description: "相关性不足", Severity: "major"})
	}
	if judgment.LegalityJudgment != nil && judgment.LegalityJudgment.Score < 0.5 {
		issues = append(issues, JudgmentIssue{Type: judgmentLegality, Description: "合法性存疑", Severity: judgmentCritical})
	}
	if judgment.AuthenticityJudgment != nil && judgment.AuthenticityJudgment.Score < 0.3 {
		issues = append(issues, JudgmentIssue{Type: judgmentAuthenticity, Description: "真实性无法确认", Severity: judgmentCritical})
	}
	judgment.FlaggedIssues = issues
}

// evaluateTypeSpecific 根据证据类型进行特定评估，结果填入 judgment。
func (e *DefaultEngine) evaluateTypeSpecific(span agentcore_evidence.EvidenceSpan, evType EvidenceType, judgment *EvidenceJudgment) {
	ts := &TypeSpecificJudgment{EvidenceType: evType}

	switch evType {
	case EvTypeElectronic:
		cred := PlatformCredibility(cleanEvidenceURI(span.SourceURI))
		ts.PlatformCredibility = &cred
		score := CredibilityToScore(cred)
		ts.CredibilityScore = &score
	case EvTypeForeignLang:
		ts.TranslationStatus = evUnknown
	case EvTypeOverseas:
		if span.ContentHash != "" {
			cred := CredHigh
			ts.PlatformCredibility = &cred
		}
	case EvTypeNotarial:
		ts.NotarizationStatus = "confirmed"
	case EvTypeWitness:
		ts.WitnessCredibility = "medium"
	case EvTypeCommonKnowledge:
		ts.ExemptionApplied = "无需举证"
	case EvTypePriorArtDate:
		ts.DateDetermination = DetermineInternetPublicationDate(span.SourceURI, span.DocVersion)
	case EvTypeInternetPublication:
		// 互联网公开日期推定
		ts.DateDetermination = DetermineInternetPublicationDate(span.SourceURI, span.DocVersion)
		// 平台可信度评估（先清理自定义 scheme 前缀）
		cleanedURI := cleanEvidenceURI(span.SourceURI)
		cred := PlatformCredibility(cleanedURI)
		ts.PlatformCredibility = &cred
		score := CredibilityToScore(cred)
		ts.CredibilityScore = &score
		// 平台分类
		ts.PlatformCategory = classifyInternetPlatform(span.SourceURI)
		// 内容完整性检查
		ts.ContentIntegrity = evaluateInternetContentIntegrity(span)
		// 公开意图判断
		ts.PublicIntent = evaluatePublicIntent(span)
	case EvTypePublicUse:
		// 使用公开日期推定
		ts.DateDetermination = DeterminePublicUseDate(span.Snippet, span.DocVersion, "")
		// 四要件检查
		ts.FourElementsCheck = evaluateFourElements(span)
		// 举证难度评估
		ts.BurdenDifficulty = assessPublicUseBurdenDifficulty(ts.FourElementsCheck)
		// 证据链完整性
		ts.ChainIntegrity = assessPublicUseChainIntegrity(span, ts.FourElementsCheck)
	}

	judgment.TypeSpecificJudgment = ts
}

// BatchJudge 批量判断多条证据。
func (e *DefaultEngine) BatchJudge(spans []agentcore_evidence.EvidenceSpan) ([]*EvidenceJudgment, error) {
	results := make([]*EvidenceJudgment, len(spans))
	for i, span := range spans {
		judgment, err := e.Judge(span)
		if err != nil {
			return nil, fmt.Errorf("评估 span %s 失败: %w", span.ID, err)
		}
		results[i] = judgment
	}
	return results, nil
}

// AssessBurdenOfProof 评估举证责任分配。
func (e *DefaultEngine) AssessBurdenOfProof(caseType string, context map[string]string) (*BurdenDetermination, error) {
	det := &BurdenDetermination{Standard: string(StandardPreponderance)}
	switch strings.ToLower(caseType) {
	case "invalidation", "invalidity", "无效":
		det.BurdenHolder = burdenClaimant
		det.Reasoning = "无效宣告程序中，请求人对其主张承担举证责任"
	case "infringement", "侵权":
		det.BurdenHolder = burdenClaimant
		det.Standard = standardClearConvincing
		det.Reasoning = "侵权诉讼中，权利人对其主张承担举证责任"
	case "new_product_method", "新产品制造方法":
		det.BurdenHolder = burdenClaimant
		det.HasShifted = true
		det.ShiftReason = "新产品制造方法举证责任倒置"
		det.Reasoning = "权利人须先证明：1) 产品为新产品；2) 被诉产品与依专利方法制造的产品为同样产品。证明后举证责任转移至被诉侵权人"
	default:
		det.BurdenHolder = burdenClaimant
		det.Reasoning = "适用谁主张谁举证原则"
	}
	if context != nil {
		if holder, ok := context["burden_holder"]; ok {
			det.BurdenHolder = holder
		}
	}
	return det, nil
}

// AssessProofStandard 评估是否达到指定证明标准。
func (e *DefaultEngine) AssessProofStandard(judgments []*EvidenceJudgment, standard string) (*ProofStandardResult, error) {
	result := &ProofStandardResult{Standard: standard}
	var totalScore float64
	var supporting, contradicting, validCount int

	for _, j := range judgments {
		if j == nil {
			continue
		}
		validCount++
		totalScore += j.OverallScore
		if j.OverallScore >= 0.6 {
			supporting++
		} else {
			contradicting++
		}
		if j.hasConflict() && j.OverallScore < 0.6 {
			contradicting++
		}
	}

	result.SupportingCount = supporting
	result.ContradictingCount = contradicting
	if validCount > 0 {
		result.Confidence = totalScore / float64(validCount)
	}

	switch standard {
	case "preponderance", cnPreponderance:
		result.Met = supporting > contradicting && result.Confidence >= 0.5
	case standardClearConvincing, cnHighProbability:
		result.Met = result.Confidence >= 0.7 && supporting > contradicting*2
	default:
		result.Met = result.Confidence >= 0.5
	}
	if contradicting > 0 {
		result.Gaps = append(result.Gaps, fmt.Sprintf("存在 %d 件矛盾或低分证据，需进一步审查", contradicting))
	}
	if validCount == 0 {
		result.Gaps = append(result.Gaps, "无证据支持")
		result.Met = false
	}
	return result, nil
}

// LoadRules 加载 YAML 规则。
func (e *DefaultEngine) LoadRules(yamlBytes []byte) error {
	return e.index.LoadBytes(yamlBytes)
}

// GetRules 返回所有规则。
func (e *DefaultEngine) GetRules() []EvidenceRule {
	return e.index.AllRules()
}

// GetRulesByType 返回指定类型的规则。
func (e *DefaultEngine) GetRulesByType(evType EvidenceType) []EvidenceRule {
	return e.index.GetRulesByType(evType)
}

// computeOverallScore 综合三个维度的评分，支持从 YAML 加载权重。
// 当证据涉及电子或互联网公开类型时，平台可信度分数作为修正系数纳入总分。
func (e *DefaultEngine) computeOverallScore(j *EvidenceJudgment) float64 {
	weights := map[string]float64{judgmentRelevance: 0.3, judgmentLegality: 0.3, judgmentAuthenticity: 0.4}
	rules := e.index.GetRulesByType(EvTypeGeneral)
	for _, rule := range rules {
		if rule.EvidenceAssessment != nil {
			for _, dim := range rule.EvidenceAssessment.Dimensions {
				if _, ok := weights[dim.Name]; ok && dim.Weight > 0 {
					weights[dim.Name] = dim.Weight
				}
			}
		}
	}
	var total, weightSum float64
	for _, dim := range []struct {
		judgment *DimensionJudgment
		weight   float64
	}{
		{j.RelevanceJudgment, weights[judgmentRelevance]},
		{j.LegalityJudgment, weights[judgmentLegality]},
		{j.AuthenticityJudgment, weights[judgmentAuthenticity]},
	} {
		if dim.judgment != nil {
			total += dim.judgment.Score * dim.weight
			weightSum += dim.weight
		}
	}
	if weightSum == 0 {
		return 0.5
	}
	base := total / weightSum

	// 可信度修正：对电子/互联网公开类证据，根据平台可信度分数微调总分。
	// 修正系数 = 0.9 + 0.2 * credScore，范围 [0.95, 1.09]。
	// 政府/学术平台 (0.95) → 微升，社交/未知平台 (0.25) → 微降。
	if ts := j.TypeSpecificJudgment; ts != nil && ts.CredibilityScore != nil {
		modifier := 0.9 + 0.2*(*ts.CredibilityScore)
		base *= modifier
	}
	return base
}

// buildReasoning 生成判断推理过程说明。
func (e *DefaultEngine) buildReasoning(j *EvidenceJudgment, evType EvidenceType) string {
	var parts []string
	if j.RelevanceJudgment != nil {
		parts = append(parts, fmt.Sprintf("关联性[%s]: %s", j.RelevanceJudgment.Level, j.RelevanceJudgment.Reasoning))
	}
	if j.LegalityJudgment != nil {
		parts = append(parts, fmt.Sprintf("合法性[%s]: %s", j.LegalityJudgment.Level, j.LegalityJudgment.Reasoning))
	}
	if j.AuthenticityJudgment != nil {
		parts = append(parts, fmt.Sprintf("真实性[%s]: %s", j.AuthenticityJudgment.Level, j.AuthenticityJudgment.Reasoning))
	}
	if j.TypeSpecificJudgment != nil {
		switch evType {
		case EvTypeInternetPublication:
			ts := j.TypeSpecificJudgment
			parts = append(parts, fmt.Sprintf("类型检查[互联网公开]: 日期=%s, 可信度=%s(%.2f), 完整性=%s, 意图=%s",
				ts.DateDeterminationString(),
				ts.PlatformCredibilityString(),
				credibilityScoreOrDefault(ts.CredibilityScore),
				ts.ContentIntegrity,
				ts.PublicIntent))
		case EvTypeElectronic:
			ts := j.TypeSpecificJudgment
			parts = append(parts, fmt.Sprintf("类型检查[电子证据]: 可信度=%s(%.2f)",
				ts.PlatformCredibilityString(),
				credibilityScoreOrDefault(ts.CredibilityScore)))
		case EvTypePublicUse:
			ts := j.TypeSpecificJudgment
			result := ts.FourElementsCheck
			var fourMet string
			if result != nil {
				fourMet = fmt.Sprintf("四要件=%t", result.AllMet())
			} else {
				fourMet = "四要件=未评估"
			}
			parts = append(parts, fmt.Sprintf("类型检查[使用公开]: %s, 举证难度=%s, 证据链=%s",
				fourMet,
				ts.BurdenDifficulty,
				ts.ChainIntegrity))
		default:
			parts = append(parts, fmt.Sprintf("类型检查[%s]: 已完成", evType))
		}
	}
	if len(parts) == 0 {
		return "未执行评估"
	}
	return strings.Join(parts, "; ")
}

// hasConflict 检查证据判断是否有冲突标记。
func (j *EvidenceJudgment) hasConflict() bool {
	for _, issue := range j.FlaggedIssues {
		if issue.Type == "conflict" {
			return true
		}
	}
	return false
}

// evaluatePublicIntent 判断互联网公开意图（是否对公众开放）。

// ---------- 工具函数 ----------

// DateDeterminationString 返回日期认定结果的摘要字符串。
func (ts *TypeSpecificJudgment) DateDeterminationString() string {
	if ts == nil || ts.DateDetermination == nil {
		return "未知"
	}
	dd := ts.DateDetermination
	return fmt.Sprintf("%s(%s/%s)", dd.Determined, dd.Reliability, dd.SourceType)
}

// PlatformCredibilityString 返回平台可信度的摘要字符串。
func (ts *TypeSpecificJudgment) PlatformCredibilityString() string {
	if ts == nil || ts.PlatformCredibility == nil {
		return "未知"
	}
	return string(*ts.PlatformCredibility)
}

// containsAny 检查字符串是否包含任一关键词。
func containsAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// credibilityScoreOrDefault 返回可信度分数，为 nil 时返回 0。
func credibilityScoreOrDefault(s *float64) float64 {
	if s == nil {
		return 0
	}
	return *s
}
