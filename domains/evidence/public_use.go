package evidence

import (
	"fmt"
	"net/url"
	"strings"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

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
