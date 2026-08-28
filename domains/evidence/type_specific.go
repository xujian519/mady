package evidence

import (
	"net/url"
	"strings"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

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

// classifyInternetPlatform 对互联网公开的来源平台进行分类。
func classifyInternetPlatform(uri string) string {
	if uri == "" {
		return "未知"
	}

	cleaned := cleanEvidenceURI(uri)
	parsed, err := url.Parse(cleaned)
	if err != nil {
		// URL 非法时降级为未知平台，不做可信度分类
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
