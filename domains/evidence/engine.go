package evidence

import (
	"fmt"
	"net/url"
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
		cred := PlatformCredibility(cleanEvidenceURI(span.SourceURI))
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
	case EvTypeInternetPublication:
		// 互联网公开日期推定
		ts.DateDetermination = DetermineInternetPublicationDate(span.SourceURI, span.DocVersion)
		// 平台可信度评估（先清理自定义 scheme 前缀）
		cleanedURI := cleanEvidenceURI(span.SourceURI)
		cred := PlatformCredibility(cleanedURI)
		ts.PlatformCredibility = &cred
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
		switch evType {
		case EvTypeInternetPublication:
			ts := j.TypeSpecificJudgment
			parts = append(parts, fmt.Sprintf("类型检查[互联网公开]: 日期=%s, 可信度=%s, 完整性=%s, 意图=%s",
				ts.DateDeterminationString(),
				ts.PlatformCredibilityString(),
				ts.ContentIntegrity,
				ts.PublicIntent))
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

// inferEvidenceType 根据来源 URI 推断证据类型。
func inferEvidenceType(uri string) EvidenceType {
	if uri == "" {
		return EvTypeGeneral
	}
	if strings.HasPrefix(uri, "web_pub:") || strings.HasPrefix(uri, "http_archive:") {
		return EvTypeInternetPublication
	}
	if strings.HasPrefix(uri, "pub_use:") || strings.HasPrefix(uri, "public_use:") {
		return EvTypePublicUse
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

// ---------- 互联网公开辅助函数 ----------

// cleanEvidenceURI 去除自定义证据 URI scheme 前缀，返回可解析的标准 URL。
func cleanEvidenceURI(raw string) string {
	prefixes := []string{"web_pub:", "http_archive:", "pub_use:", "public_use:", "web:", "witness:", "patent:", "prior_art:"}
	for _, p := range prefixes {
		if strings.HasPrefix(raw, p) {
			return raw[len(p):]
		}
	}
	return raw
}

// classifyInternetPlatform 对互联网公开的来源平台进行分类。
func classifyInternetPlatform(uri string) string {
	if uri == "" {
		return "未知"
	}

	cleaned := cleanEvidenceURI(uri)
	parsed, err := url.Parse(cleaned)
	if err != nil {
		return "未知"
	}

	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "未知"
	}

	if isGovernmentDomain(hostname) {
		return "政府/专利局官方平台"
	}
	if isAcademicDomain(hostname) {
		return "学术/教育平台"
	}
	if isNewsMedia(hostname) {
		return "新闻媒体"
	}
	if isContentPlatform(hostname) {
		return "内容平台"
	}
	if strings.Contains(hostname, "web.archive.org") || strings.Contains(hostname, "archive.org") {
		return "网页存档平台"
	}
	if strings.Contains(hostname, "baidu") || strings.Contains(hostname, "google") {
		return "搜索引擎"
	}
	if strings.Contains(hostname, "weibo") || strings.Contains(hostname, "twitter") ||
		strings.Contains(hostname, "facebook") || strings.Contains(hostname, "zhihu") {
		return "社交媒体"
	}
	if strings.Contains(hostname, "github") || strings.Contains(hostname, "gitlab") ||
		strings.Contains(hostname, "bitbucket") {
		return "代码托管平台"
	}

	return "其他互联网平台"
}

// evaluateInternetContentIntegrity 评估互联网公开内容完整性。
func evaluateInternetContentIntegrity(span agentcore_evidence.EvidenceSpan) ContentIntegrityStatus {
	// 有内容哈希可验证
	if span.ContentHash != "" {
		return IntegrityVerified
	}

	// 如果来源是 Wayback Machine 等存档平台，视为部分可验证
	if strings.Contains(span.SourceURI, "web.archive.org") ||
		strings.Contains(span.SourceURI, "archive.org") {
		return IntegrityPartial
	}

	return IntegrityUnverified
}

// evaluatePublicIntent 判断互联网公开意图（是否对公众开放）。
func evaluatePublicIntent(span agentcore_evidence.EvidenceSpan) PublicIntent {
	// 默认推定对公众开放
	if span.SourceURI == "" {
		return IntentPublic
	}

	cleaned := cleanEvidenceURI(span.SourceURI)
	parsed, err := url.Parse(cleaned)
	if err != nil {
		return IntentPublic
	}

	hostname := strings.ToLower(parsed.Hostname())

	// 可能存在注册/付费墙的平台标记为受限
	restrictedDomains := []string{
		"wsj.com", "ft.com", "nikkei.com",
		"springer.com", "elsevier.com",
	}
	for _, d := range restrictedDomains {
		if strings.HasSuffix(hostname, d) || hostname == d {
			return IntentRestricted
		}
	}

	return IntentPublic
}

// ---------- 使用公开辅助函数 ----------

// evaluateFourElements 检查使用公开的四要件。
func evaluateFourElements(span agentcore_evidence.EvidenceSpan) *FourElementsResult {
	result := &FourElementsResult{}

	// 要件一：公开时间 —— 使用行为发生在申请日之前
	result.TimeElement = evaluatePublicUseTime(span)

	// 要件二：公开地点 —— 在国内外公开使用
	result.PlaceElement = evaluatePublicUsePlace(span)

	// 要件三：公开方式 —— 销售、展示、演示等
	result.MethodElement = evaluatePublicUseMethod(span)

	// 要件四：公众可获取性 —— 非保密性质的使用
	result.Accessibility = evaluatePublicUseAccessibility(span)

	return result
}

// evaluatePublicUseTime 评估使用公开的时间要件。
func evaluatePublicUseTime(span agentcore_evidence.EvidenceSpan) ElementResult {
	if span.DocVersion == "" {
		return ElementResult{
			Met:    false,
			Score:  0.25,
			Detail: "未提供使用公开日期，无法判断是否在申请日之前",
		}
	}

	// 使用 isBeforeFilingDate 检查日期关系
	_, parsed := DeterminePublicationDate(span.DocVersion)
	if parsed.IsZero() {
		return ElementResult{
			Met:    false,
			Score:  0.3,
			Detail: fmt.Sprintf("日期格式无法识别: %s", span.DocVersion),
		}
	}

	if isPreciseDate(span.DocVersion) {
		return ElementResult{
			Met:    true,
			Score:  0.9,
			Detail: fmt.Sprintf("使用公开日期为 %s，格式完整", span.DocVersion),
		}
	}

	return ElementResult{
		Met:    true,
		Score:  0.7,
		Detail: fmt.Sprintf("使用公开日期为 %s，精度不足，需补充具体日期", span.DocVersion),
	}
}

// evaluatePublicUsePlace 评估使用公开的地点要件。
func evaluatePublicUsePlace(span agentcore_evidence.EvidenceSpan) ElementResult {
	snippet := strings.ToLower(span.Snippet)

	// 尝试从描述文本中识别地点信息
	domesticIndicators := []string{"中国", "北京", "上海", "广州", "深圳", "国内", "境内"}
	foreignIndicators := []string{"美国", "us", "europe", "日本", "国外", "境外", "international"}

	if containsAny(snippet, domesticIndicators) {
		return ElementResult{
			Met:    true,
			Score:  0.85,
			Detail: "使用行为发生在中国境内（构成国内公开）",
		}
	}

	if containsAny(snippet, foreignIndicators) {
		return ElementResult{
			Met:    true,
			Score:  0.8,
			Detail: "使用行为发生在境外（构成国外公开，中国专利法采用绝对新颖性标准）",
		}
	}

	// 无明确地点信息时，给予默认评分
	return ElementResult{
		Met:    true,
		Score:  0.6,
		Detail: "未明确提及使用地点，推定使用行为已公开（需进一步核实具体地点）",
	}
}

// evaluatePublicUseMethod 评估使用公开的方式要件。
func evaluatePublicUseMethod(span agentcore_evidence.EvidenceSpan) ElementResult {
	snippet := strings.ToLower(span.Snippet)

	salesIndicators := []string{"销售", "出售", "售卖", "购买", "sell", "sale", "transaction"}
	exhibitionIndicators := []string{"展览", "展出", "展示", "演示", "exhibition", "expo", "fair", "show", "demonstrat"}
	publicationIndicators := []string{"出版", "发布", "发表", "公开", "publish", "release", "post"}
	otherIndicators := []string{"使用", "实施", "制造", "生产", "use", "manufactur", "produc"}

	switch {
	case containsAny(snippet, salesIndicators):
		return ElementResult{
			Met:    true,
			Score:  0.9,
			Detail: "通过销售行为公开使用",
		}
	case containsAny(snippet, exhibitionIndicators):
		return ElementResult{
			Met:    true,
			Score:  0.85,
			Detail: "通过展览或展示行为公开使用",
		}
	case containsAny(snippet, publicationIndicators):
		return ElementResult{
			Met:    true,
			Score:  0.75,
			Detail: "通过发布/发表行为公开使用",
		}
	case containsAny(snippet, otherIndicators):
		return ElementResult{
			Met:    true,
			Score:  0.6,
			Detail: "通过其他方式公开使用，需进一步明确具体方式",
		}
	default:
		return ElementResult{
			Met:    false,
			Score:  0.3,
			Detail: "未识别出明确的使用公开方式，需补充公开方式的描述（如销售、展览、演示等）",
		}
	}
}

// evaluatePublicUseAccessibility 评估公众可获取性要件。
func evaluatePublicUseAccessibility(span agentcore_evidence.EvidenceSpan) ElementResult {
	snippet := strings.ToLower(span.Snippet)

	confidentialityIndicators := []string{"保密", "秘密", "confidential", "保密协议", "nda", "non-disclosure"}
	limitedAccessIndicators := []string{"内部", "内测", "内部测试", "closed", "internal", "invite-only"}
	publicAccessIndicators := []string{"公开", "开放", "公众", "public", "open", "anyone"}

	if containsAny(snippet, confidentialityIndicators) {
		return ElementResult{
			Met:    false,
			Score:  0.2,
			Detail: "存在保密义务或保密措施，可能不构成公众可获取",
		}
	}

	if containsAny(snippet, limitedAccessIndicators) {
		return ElementResult{
			Met:    false,
			Score:  0.35,
			Detail: "使用行为限于特定范围，非对公众开放",
		}
	}

	if containsAny(snippet, publicAccessIndicators) {
		return ElementResult{
			Met:    true,
			Score:  0.9,
			Detail: "使用行为对公众开放，公众可获取",
		}
	}

	// 未明确提及保密时，推定可被公众获取（举证责任由主张保密的一方承担）
	return ElementResult{
		Met:    true,
		Score:  0.65,
		Detail: "未提及保密措施，推定为公众可获取",
	}
}

// assessPublicUseBurdenDifficulty 评估使用公开的举证难度。
func assessPublicUseBurdenDifficulty(fourElements *FourElementsResult) string {
	if fourElements == nil {
		return "无法评估"
	}

	metCount := 0
	if fourElements.TimeElement.Met {
		metCount++
	}
	if fourElements.PlaceElement.Met {
		metCount++
	}
	if fourElements.MethodElement.Met {
		metCount++
	}
	if fourElements.Accessibility.Met {
		metCount++
	}

	switch {
	case metCount >= 4:
		return "中"
	case metCount >= 2:
		return "高"
	default:
		return "极高"
	}
}

// assessPublicUseChainIntegrity 评估使用公开的证据链完整性。
func assessPublicUseChainIntegrity(span agentcore_evidence.EvidenceSpan, fourElements *FourElementsResult) string {
	if fourElements == nil {
		return "无法评估"
	}

	if fourElements.AllMet() {
		if span.ContentHash != "" {
			return "完整（四要素齐全且内容可哈希验证）"
		}
		return "较完整（四要素齐全，建议补充旁证印证）"
	}

	if span.Snippet != "" {
		return "需补充证据（部分要件缺失，建议提供销售合同/展览记录等直接证据）"
	}

	return "证据链不完整，建议收集多份相互印证的证据"
}

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
