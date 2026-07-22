package evidence

import (
	"fmt"
	"strings"
	"time"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

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
	rules := e.index.GetRulesByType(evType)
	judgment := &EvidenceJudgment{
		SpanID:      span.ID,
		EvaluatedAt: time.Now(),
		Confidence:  1.0,
	}

	e.evaluateTripleAttributes(span, rules, judgment)
	e.evaluateTypeSpecific(span, evType, rules, judgment)
	judgment.OverallScore = e.computeOverallScore(judgment)
	judgment.Reasoning = e.buildReasoning(judgment, evType)
	return judgment, nil
}

// evaluateTripleAttributes 对证据三性逐项评分并填入 judgment。
func (e *DefaultEngine) evaluateTripleAttributes(span agentcore_evidence.EvidenceSpan, rules []EvidenceRule, judgment *EvidenceJudgment) {
	judgment.RelevanceJudgment = evaluateRelevance(span)
	judgment.LegalityJudgment = evaluateLegality(span)
	judgment.AuthenticityJudgment = evaluateAuthenticity(span)

	// 标记已发现的问题
	var issues []JudgmentIssue
	if judgment.RelevanceJudgment != nil && judgment.RelevanceJudgment.Score < 0.5 {
		issues = append(issues, JudgmentIssue{Type: "relevance", Description: "相关性不足", Severity: "major"})
	}
	if judgment.LegalityJudgment != nil && judgment.LegalityJudgment.Score < 0.5 {
		issues = append(issues, JudgmentIssue{Type: "legality", Description: "合法性存疑", Severity: "critical"})
	}
	if judgment.AuthenticityJudgment != nil && judgment.AuthenticityJudgment.Score < 0.3 {
		issues = append(issues, JudgmentIssue{Type: "authenticity", Description: "真实性无法确认", Severity: "critical"})
	}
	judgment.FlaggedIssues = issues
}

// evaluateTypeSpecific 根据证据类型进行特定评估，结果填入 judgment。
func (e *DefaultEngine) evaluateTypeSpecific(span agentcore_evidence.EvidenceSpan, evType EvidenceType, rules []EvidenceRule, judgment *EvidenceJudgment) {
	ts := &TypeSpecificJudgment{EvidenceType: evType}

	switch evType {
	case EvTypeElectronic:
		cred := PlatformCredibility(span.SourceURI)
		ts.PlatformCredibility = &cred
	case EvTypeForeignLang:
		ts.TranslationStatus = "unknown"
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
	det := &BurdenDetermination{Standard: "preponderance"}
	switch strings.ToLower(caseType) {
	case "invalidation", "invalidity", "无效":
		det.BurdenHolder = "claimant"
		det.Reasoning = "无效宣告程序中，请求人对其主张承担举证责任"
	case "infringement", "侵权":
		det.BurdenHolder = "claimant"
		det.Standard = "clear_and_convincing"
		det.Reasoning = "侵权诉讼中，权利人对其主张承担举证责任"
	case "new_product_method", "新产品制造方法":
		det.BurdenHolder = "claimant"
		det.HasShifted = true
		det.ShiftReason = "新产品制造方法举证责任倒置"
		det.Reasoning = "权利人须先证明：1) 产品为新产品；2) 被诉产品与依专利方法制造的产品为同样产品。证明后举证责任转移至被诉侵权人"
	default:
		det.BurdenHolder = "claimant"
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
	case "preponderance", "优势证据":
		result.Met = supporting > contradicting && result.Confidence >= 0.5
	case "clear_and_convincing", "高度盖然性":
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
func (e *DefaultEngine) computeOverallScore(j *EvidenceJudgment) float64 {
	weights := map[string]float64{"relevance": 0.3, "legality": 0.3, "authenticity": 0.4}
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
		{j.RelevanceJudgment, weights["relevance"]},
		{j.LegalityJudgment, weights["legality"]},
		{j.AuthenticityJudgment, weights["authenticity"]},
	} {
		if dim.judgment != nil {
			total += dim.judgment.Score * dim.weight
			weightSum += dim.weight
		}
	}
	if weightSum == 0 {
		return 0.5
	}
	return total / weightSum
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
		parts = append(parts, fmt.Sprintf("类型检查[%s]: 已完成", evType))
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

// inferEvidenceType 根据来源 URI 推断证据类型。
func inferEvidenceType(uri string) EvidenceType {
	if uri == "" {
		return EvTypeGeneral
	}
	if strings.HasPrefix(uri, "web:") || strings.HasPrefix(uri, "http") {
		return EvTypeElectronic
	}
	if strings.HasPrefix(uri, "witness:") {
		return EvTypeWitness
	}
	if strings.HasPrefix(uri, "patent:") || strings.HasPrefix(uri, "prior_art:") {
		return EvTypePriorArtDate
	}
	return EvTypeGeneral
}

// evaluateRelevance 评估证据相关性（包级辅助函数）。
func evaluateRelevance(span agentcore_evidence.EvidenceSpan) *DimensionJudgment {
	j := &DimensionJudgment{Dimension: "relevance"}
	score := 0.5
	if span.SourceURI != "" {
		score += 0.1
	}
	if len(span.ClaimRefs) > 0 {
		score += 0.2
	}
	if span.Direction == agentcore_evidence.DirectionSupporting || span.Direction == agentcore_evidence.DirectionContradicting {
		score += 0.1
	}
	if span.Snippet != "" {
		score += 0.1
	}
	if score > 1.0 {
		score = 1.0
	}
	j.Score = score
	switch {
	case score >= 0.85:
		j.Level = "high"
	case score >= 0.65:
		j.Level = "medium_high"
	case score >= 0.45:
		j.Level = "medium"
	default:
		j.Level = "low"
	}
	j.Reasoning = "相关性评估完成"
	return j
}

// evaluateLegality 评估证据合法性（包级辅助函数）。
func evaluateLegality(span agentcore_evidence.EvidenceSpan) *DimensionJudgment {
	j := &DimensionJudgment{Dimension: "legality"}
	score := 0.7
	if span.SourceURI == "" {
		score -= 0.2
	}
	if span.ContentHash != "" {
		score += 0.2
	}
	if score > 1.0 {
		score = 1.0
	}
	if score < 0 {
		score = 0
	}
	j.Score = score
	switch {
	case score >= 0.85:
		j.Level = "high"
	case score >= 0.65:
		j.Level = "medium_high"
	default:
		j.Level = "low"
	}
	j.Reasoning = "合法性评估完成"
	return j
}

// evaluateAuthenticity 评估证据真实性（包级辅助函数）。
func evaluateAuthenticity(span agentcore_evidence.EvidenceSpan) *DimensionJudgment {
	j := &DimensionJudgment{Dimension: "authenticity"}
	score := 0.5
	if span.ContentHash != "" {
		score += 0.3
	}
	if span.DocVersion != "" {
		score += 0.1
	}
	if score > 1.0 {
		score = 1.0
	}
	j.Score = score
	switch {
	case score >= 0.85:
		j.Level = "high"
	case score >= 0.65:
		j.Level = "medium_high"
	default:
		j.Level = "low"
	}
	j.Reasoning = "真实性评估完成"
	return j
}
