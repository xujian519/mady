package evidence

import (
	"net/url"
	"strings"
)

// PlatformCredibility 根据证据来源 URI 判定平台可信度等级。
// 使用 net/url 解析域名而非子串匹配，避免误匹配。
func PlatformCredibility(uri string) CredibilityLevel {
	if uri == "" {
		return CredLow
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		// URL 解析失败时回退到 low
		return CredLow
	}

	hostname := strings.ToLower(parsed.Hostname())

	// 政府/法院/专利局官方平台
	if isGovernmentDomain(hostname) {
		return CredHigh
	}

	// 学术数据库
	if isAcademicDomain(hostname) {
		return CredHigh
	}

	// 行业权威平台
	if isIndustryAuthority(hostname) {
		return CredMediumHigh
	}

	// 正规新闻媒体
	if isNewsMedia(hostname) {
		return CredMedium
	}

	// 内容平台（微信公众平台等）
	if isContentPlatform(hostname) {
		return CredMediumHigh
	}

	// 搜索引擎/聚合平台
	if isAggregator(hostname) {
		return CredMedium
	}

	// 社交/自媒体/未知平台
	return CredLow
}

// CredibilityToScore 将可信度等级映射为 0-1 分数。
func CredibilityToScore(level CredibilityLevel) float64 {
	switch level {
	case CredHigh:
		return 0.95
	case CredMediumHigh:
		return 0.75
	case CredMedium:
		return 0.55
	case CredLow:
		return 0.25
	default:
		return 0.25
	}
}

// AssessElectronicEvidence 对电子证据进行综合可信度评估。
// platformCredibility 由 PlatformCredibility 判定，contentHash 为内容摘要哈希值。
func AssessElectronicEvidence(platformCredibility CredibilityLevel, contentHash string, isVerifiedCopy bool) CredibilityLevel {
	score := CredibilityToScore(platformCredibility)

	if isVerifiedCopy {
		score = score*0.7 + 0.3
	}

	if contentHash != "" {
		score = score*0.85 + 0.15
	}

	switch {
	case score >= 0.85:
		return CredHigh
	case score >= 0.65:
		return CredMediumHigh
	case score >= 0.45:
		return CredMedium
	default:
		return CredLow
	}
}

// isGovernmentDomain 域名是否为政府/法院/专利局官方域名。
func isGovernmentDomain(hostname string) bool {
	govSuffixes := []string{
		".gov.cn", ".gov", ".court.gov.cn",
		".cnipa.gov.cn", ".sipo.gov.cn", ".epo.org",
		".wipo.int", ".uspto.gov", ".jpo.go.jp",
		".kpo.go.kr", ".ipo.gov.uk",
	}
	for _, suffix := range govSuffixes {
		if strings.HasSuffix(hostname, suffix) {
			return true
		}
	}
	return false
}

// isAcademicDomain 域名是否为学术数据库或教育机构。
func isAcademicDomain(hostname string) bool {
	academicSuffixes := []string{
		".edu.cn", ".edu", ".ac.cn",
		".cnki.net", ".wanfangdata.com.cn", ".cqvip.com",
		".ieee.org", ".acm.org", ".springer.com",
		".elsevier.com", ".nature.com", ".sciencemag.org",
	}
	for _, suffix := range academicSuffixes {
		if strings.HasSuffix(hostname, suffix) || hostname == strings.TrimPrefix(suffix, ".") {
			return true
		}
	}
	return false
}

// isIndustryAuthority 域名是否为行业权威平台。
func isIndustryAuthority(hostname string) bool {
	authorityDomains := []string{
		"patents.google.com", "patentscope.wipo.int",
		"globaldossier.net", "darts-ip.com",
	}
	for _, d := range authorityDomains {
		if hostname == d || strings.HasSuffix(hostname, "."+d) {
			return true
		}
	}
	return false
}

// isNewsMedia 域名是否为正规新闻媒体。
func isNewsMedia(hostname string) bool {
	newsSuffixes := []string{
		".xinhuanet.com", ".people.com.cn", ".chinanews.com.cn",
		".bbc.com", ".bbc.co.uk", ".reuters.com", ".ap.org",
		".nikkei.com", ".ft.com", ".wsj.com",
	}
	for _, suffix := range newsSuffixes {
		if strings.HasSuffix(hostname, suffix) || hostname == strings.TrimPrefix(suffix, ".") {
			return true
		}
	}
	return false
}

// isContentPlatform 域名是否为内容平台（如微信公众平台）。
// 内容平台介于正规新闻媒体和聚合平台之间，可信度中等偏高。
func isContentPlatform(hostname string) bool {
	contentDomains := []string{
		"mp.weixin.qq.com",
	}
	for _, d := range contentDomains {
		if hostname == d || strings.HasSuffix(hostname, "."+d) {
			return true
		}
	}
	return false
}

// isAggregator 域名是否为搜索引擎或聚合平台。
func isAggregator(hostname string) bool {
	aggregatorDomains := []string{
		"baidu.com", "google.com", "bing.com",
		"toutiao.com", "sohu.com", "sina.com.cn",
		"163.com", "qq.com",
	}
	for _, d := range aggregatorDomains {
		if hostname == d || strings.HasSuffix(hostname, "."+d) {
			return true
		}
	}
	return false
}
